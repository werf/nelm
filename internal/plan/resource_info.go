package plan

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/wI2L/jsondiff"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

type ResourceInstallType string

const (
	ResourceInstallTypeNone     ResourceInstallType = "none"
	ResourceInstallTypeCreate   ResourceInstallType = "create"
	ResourceInstallTypeRecreate ResourceInstallType = "recreate"
	ResourceInstallTypeUpdate   ResourceInstallType = "update"
	ResourceInstallTypeApply    ResourceInstallType = "apply"
)

var OrderedResourceInstallTypes = []ResourceInstallType{
	ResourceInstallTypeNone,
	ResourceInstallTypeCreate,
	ResourceInstallTypeRecreate,
	ResourceInstallTypeUpdate,
	ResourceInstallTypeApply,
}

func ResourceInstallTypeSortHandler(type1, type2 ResourceInstallType) bool {
	type1I := lo.IndexOf(OrderedResourceInstallTypes, type1)
	type2I := lo.IndexOf(OrderedResourceInstallTypes, type2)

	return type1I < type2I
}

type InstallableResourceInfo struct {
	*spec.ResourceMeta

	LocalResource  *resource.InstallableResource
	GetResult      *unstructured.Unstructured
	DryApplyResult *unstructured.Unstructured
	DryApplyErr    error

	MustInstall                   ResourceInstallType
	MustDeleteOnSuccessfulInstall bool
	MustDeleteOnFailedInstall     bool
	MustTrackReadiness            bool

	Stage     common.Stage
	Iteration int
}

type DeletableResourceInfo struct {
	*spec.ResourceMeta

	LocalResource *resource.DeletableResource
	GetResult     *unstructured.Unstructured

	MustDelete       bool
	MustTrackAbsence bool

	Stage common.Stage
}

type BuildResourceInfosOptions struct {
	NetworkParallelism    int
	NoRemoveManualChanges bool
}

func BuildResourceInfos(ctx context.Context, deployType common.DeployType, releaseName, releaseNamespace string, instResources []*resource.InstallableResource, delResources []*resource.DeletableResource, prevReleaseFailed bool, clientFactory kube.ClientFactorier, opts BuildResourceInfosOptions) (instResourceInfos []*InstallableResourceInfo, delResourceInfos []*DeletableResourceInfo, err error) {
	totalResourcesCount := len(instResources) + len(delResources)

	routines := lo.Max([]int{len(instResources) / lo.Max([]int{totalResourcesCount, 1}) * opts.NetworkParallelism, 1})

	instResourcesPool := pool.NewWithResults[[]*InstallableResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range instResources {
		instResourcesPool.Go(func(ctx context.Context) ([]*InstallableResourceInfo, error) {
			infos, err := BuildInstallableResourceInfo(ctx, res, deployType, releaseNamespace, prevReleaseFailed, opts.NoRemoveManualChanges, clientFactory)
			if err != nil {
				return nil, fmt.Errorf("build installable resource info: %w", err)
			}

			return infos, nil
		})
	}

	routines = lo.Max([]int{len(delResources) / lo.Max([]int{totalResourcesCount, 1}) * opts.NetworkParallelism, 1})

	delResourcesPool := pool.NewWithResults[*DeletableResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range delResources {
		delResourcesPool.Go(func(ctx context.Context) (*DeletableResourceInfo, error) {
			info, err := BuildDeletableResourceInfo(ctx, res, deployType, releaseName, releaseNamespace, clientFactory)
			if err != nil {
				return nil, fmt.Errorf("build deletable resource info: %w", err)
			}

			return info, nil
		})
	}

	if infos, err := instResourcesPool.Wait(); err != nil {
		return nil, nil, fmt.Errorf("wait for resource pool: %w", err)
	} else {
		instResourceInfos = lo.Flatten(infos)
	}

	delResourceInfos, err = delResourcesPool.Wait()
	if err != nil {
		return nil, nil, fmt.Errorf("wait for prev release resource pool: %w", err)
	}

	sort.SliceStable(instResourceInfos, func(i, j int) bool {
		return InstallableResourceInfoSortByStageHandler(instResourceInfos[i], instResourceInfos[j])
	})

	sort.SliceStable(delResourceInfos, func(i, j int) bool {
		return spec.ResourceMetaSortHandler(delResourceInfos[i].LocalResource.ResourceMeta, delResourceInfos[j].LocalResource.ResourceMeta)
	})

	iterateInstallableResourceInfos(instResourceInfos)

	delResourceInfos = filterDelResourcesPresentInInstResources(instResourceInfos, delResourceInfos)
	delResourceInfos = deduplicateDeletableResourceInfos(delResourceInfos)

	log.Default.TraceStruct(ctx, instResourceInfos, "Built InstallableResourceInfos:")
	log.Default.TraceStruct(ctx, delResourceInfos, "Built DeletableResourceInfos:")

	return instResourceInfos, delResourceInfos, nil
}

// TODO(v2): keep annotation should probably forbid resource recreations
func BuildInstallableResourceInfo(ctx context.Context, localRes *resource.InstallableResource, deployType common.DeployType, releaseNamespace string, prevRelFailed, noRemoveManualChanges bool, clientFactory kube.ClientFactorier) ([]*InstallableResourceInfo, error) {
	var stages []common.Stage
	switch deployType {
	case common.DeployTypeInitial, common.DeployTypeInstall:
		stages = localRes.DeployConditions[common.InstallOnInstall]
	case common.DeployTypeUpgrade:
		stages = localRes.DeployConditions[common.InstallOnUpgrade]
	case common.DeployTypeRollback:
		stages = localRes.DeployConditions[common.InstallOnRollback]
	case common.DeployTypeUninstall:
		stages = localRes.DeployConditions[common.InstallOnDelete]
	default:
		panic("unexpected deploy type")
	}

	if len(stages) == 0 {
		return nil, nil
	}

	getObj, getErr := clientFactory.KubeClient().Get(ctx, localRes.ResourceMeta, kube.KubeClientGetOptions{
		DefaultNamespace: releaseNamespace,
		TryCache:         true,
	})
	if getErr != nil {
		if kube.IsNotFoundErr(getErr) || kube.IsNoSuchKindErr(getErr) {
			mustDeleteOnSuccess := mustDeleteOnSuccessfulDeploy(localRes, nil, ResourceInstallTypeCreate, releaseNamespace)
			trackReadiness := mustTrackReadiness(localRes, ResourceInstallTypeCreate, false, prevRelFailed, mustDeleteOnSuccess)

			return lo.Map(stages, func(stg common.Stage, _ int) *InstallableResourceInfo {
				return &InstallableResourceInfo{
					ResourceMeta:                  localRes.ResourceMeta,
					LocalResource:                 localRes,
					MustInstall:                   ResourceInstallTypeCreate,
					MustDeleteOnSuccessfulInstall: mustDeleteOnSuccess,
					MustDeleteOnFailedInstall:     mustDeleteOnFailedDeploy(localRes, nil, ResourceInstallTypeCreate, releaseNamespace, trackReadiness),
					MustTrackReadiness:            trackReadiness,
					Stage:                         stg,
				}
			}), nil
		} else {
			return nil, fmt.Errorf("get resource %q: %w", localRes.IDHuman(), getErr)
		}
	}

	var err error

	getObj, err = fixManagedFieldsInCluster(ctx, releaseNamespace, getObj, localRes.ResourceMeta, noRemoveManualChanges, clientFactory)
	if err != nil {
		return nil, fmt.Errorf("fix managed fields for resource %q: %w", localRes.IDHuman(), err)
	}

	dryApplyObj, dryApplyErr := clientFactory.KubeClient().Apply(ctx, localRes.ResourceSpec, kube.KubeClientApplyOptions{
		DefaultNamespace: releaseNamespace,
		DryRun:           true,
	})

	installType, err := resourceInstallType(ctx, localRes, getObj, dryApplyObj, dryApplyErr)
	if err != nil {
		return nil, fmt.Errorf("determine install type for resource %q: %w", localRes.IDHuman(), err)
	}

	getMeta := spec.NewResourceMetaFromUnstructured(getObj, releaseNamespace, localRes.FilePath)
	mustDeleteOnSuccess := mustDeleteOnSuccessfulDeploy(localRes, getMeta, installType, releaseNamespace)
	trackReadiness := mustTrackReadiness(localRes, installType, true, prevRelFailed, mustDeleteOnSuccess)

	return lo.Map(stages, func(stg common.Stage, _ int) *InstallableResourceInfo {
		return &InstallableResourceInfo{
			ResourceMeta:                  localRes.ResourceMeta,
			LocalResource:                 localRes,
			GetResult:                     getObj,
			DryApplyResult:                dryApplyObj,
			DryApplyErr:                   dryApplyErr,
			MustInstall:                   installType,
			MustDeleteOnSuccessfulInstall: mustDeleteOnSuccess,
			MustDeleteOnFailedInstall:     mustDeleteOnFailedDeploy(localRes, getMeta, installType, releaseNamespace, trackReadiness),
			MustTrackReadiness:            trackReadiness,
			Stage:                         stg,
		}
	}), nil
}

func BuildDeletableResourceInfo(ctx context.Context, localRes *resource.DeletableResource, deployType common.DeployType, releaseName, releaseNamespace string, clientFactory kube.ClientFactorier) (*DeletableResourceInfo, error) {
	var stage common.Stage
	if deployType == common.DeployTypeUninstall {
		stage = common.StageUninstall
	} else {
		stage = common.StagePrePreUninstall
	}

	noDeleteInfo := &DeletableResourceInfo{
		ResourceMeta:  localRes.ResourceMeta,
		LocalResource: localRes,
		Stage:         stage,
	}

	if localRes.KeepOnDelete || localRes.Ownership == common.OwnershipAnyone {
		return noDeleteInfo, nil
	}

	getObj, getErr := clientFactory.KubeClient().Get(ctx, localRes.ResourceMeta, kube.KubeClientGetOptions{
		DefaultNamespace: releaseNamespace,
		TryCache:         true,
	})

	noDeleteInfo.GetResult = getObj
	if getErr != nil {
		if kube.IsNotFoundErr(getErr) || kube.IsNoSuchKindErr(getErr) {
			return noDeleteInfo, nil
		} else {
			return nil, fmt.Errorf("get resource %q: %w", localRes.IDHuman(), getErr)
		}
	}

	getMeta := spec.NewResourceMetaFromUnstructured(getObj, releaseNamespace, localRes.FilePath)

	if err := resource.ValidateResourcePolicy(getMeta); err != nil {
		return noDeleteInfo, nil
	} else {
		if keep := resource.KeepOnDelete(getMeta, releaseNamespace); keep {
			return noDeleteInfo, nil
		}
	}

	if orphaned(getMeta, releaseName, releaseNamespace) {
		return noDeleteInfo, nil
	}

	return &DeletableResourceInfo{
		ResourceMeta:  localRes.ResourceMeta,
		LocalResource: localRes,
		GetResult:     getObj,
		MustDelete:    true,
		// TODO: make switchable
		MustTrackAbsence: true,
		Stage:            stage,
	}, nil
}

func filterDelResourcesPresentInInstResources(instResourceInfos []*InstallableResourceInfo, delResourceInfos []*DeletableResourceInfo) []*DeletableResourceInfo {
	var instResourcesUIDs []types.UID
	for _, instInfo := range instResourceInfos {
		if instInfo.GetResult == nil {
			continue
		}

		instResourcesUIDs = append(instResourcesUIDs, instInfo.GetResult.GetUID())
	}

	var filteredDelResourceInfos []*DeletableResourceInfo
	for _, delInfo := range delResourceInfos {
		if delInfo.GetResult != nil &&
			lo.Contains(instResourcesUIDs, delInfo.GetResult.GetUID()) {
			continue
		}

		filteredDelResourceInfos = append(filteredDelResourceInfos, delInfo)
	}

	return filteredDelResourceInfos
}

func iterateInstallableResourceInfos(infos []*InstallableResourceInfo) {
	var seenInfos []*InstallableResourceInfo
	for _, info := range infos {
		seenInfo, seen := lo.Find(seenInfos, func(inf *InstallableResourceInfo) bool {
			return info.ID() == inf.ID()
		})
		if seen {
			info.Iteration = seenInfo.Iteration + 1
		}

		seenInfos = append(seenInfos, info)
	}

	var highestIteration int

	nonZeroIterInfos := lo.Filter(infos, func(info *InstallableResourceInfo, _ int) bool {
		highestIteration = lo.Max([]int{highestIteration, info.Iteration})
		return info.Iteration > 0
	})

	if highestIteration == 0 {
		return
	}

	for iter := 1; iter <= highestIteration; iter++ {
		iterInfos := lo.Filter(nonZeroIterInfos, func(info *InstallableResourceInfo, _ int) bool {
			return info.Iteration == iter
		})

		for _, iterInfo := range iterInfos {
			if iterInfo.MustInstall == ResourceInstallTypeNone {
				continue
			}

			prevIterInfo := lo.Must(lo.Find(infos, func(inf *InstallableResourceInfo) bool {
				return iterInfo.ID() == inf.ID() && inf.Iteration == iterInfo.Iteration-1
			}))

			if prevIterInfo.MustDeleteOnSuccessfulInstall {
				iterInfo.MustInstall = ResourceInstallTypeCreate
			}
		}
	}
}

func deduplicateDeletableResourceInfos(infos []*DeletableResourceInfo) []*DeletableResourceInfo {
	return lo.UniqBy(infos, func(info *DeletableResourceInfo) string {
		return info.ID()
	})
}

func resourceInstallType(ctx context.Context, localRes *resource.InstallableResource, getObj, dryApplyObj *unstructured.Unstructured, dryApplyErr error) (ResourceInstallType, error) {
	isImmutable := dryApplyErr != nil && kube.IsImmutableErr(dryApplyErr)
	if isImmutable && !localRes.Recreate && !localRes.RecreateOnImmutable {
		return "", fmt.Errorf("immutable fields change in resource %q, but recreation is not requested: %w", localRes.IDHuman(), dryApplyErr)
	}

	if localRes.Recreate || (isImmutable && localRes.RecreateOnImmutable) {
		return ResourceInstallTypeRecreate, nil
	}

	if dryApplyErr != nil {
		return ResourceInstallTypeApply, nil
	}

	diffableGetObj := resource.CleanUnstruct(getObj, resource.CleanUnstructOptions{
		CleanHelmShAnnos: true,
		CleanWerfIoAnnos: true,
		CleanRuntimeData: true,
	})

	diffableDryApplyObj := resource.CleanUnstruct(dryApplyObj, resource.CleanUnstructOptions{
		CleanHelmShAnnos: true,
		CleanWerfIoAnnos: true,
		CleanRuntimeData: true,
	})

	if patch, err := jsondiff.Compare(diffableGetObj, diffableDryApplyObj); err != nil {
		return "", fmt.Errorf("compare live and dry-apply versions of resource %q: %w", localRes.IDHuman(), err)
	} else if len(patch) > 0 {
		log.Default.Trace(ctx, "Get/DryApply patch for %q: %s", localRes.IDHuman(), patch.String())
		return ResourceInstallTypeUpdate, nil
	}

	return ResourceInstallTypeNone, nil
}

func mustDeleteOnSuccessfulDeploy(localRes *resource.InstallableResource, getMeta *spec.ResourceMeta, installType ResourceInstallType, releaseNamespace string) bool {
	if !localRes.DeleteOnSucceeded || localRes.KeepOnDelete {
		return false
	}

	if getMeta != nil {
		if err := resource.ValidateResourcePolicy(getMeta); err != nil {
			return false
		} else {
			if keep := resource.KeepOnDelete(getMeta, releaseNamespace); keep {
				return false
			}
		}
	}

	if installType == ResourceInstallTypeNone {
		return getMeta != nil
	}

	return true
}

func mustDeleteOnFailedDeploy(res *resource.InstallableResource, getMeta *spec.ResourceMeta, installType ResourceInstallType, releaseNamespace string, mustTrackReadiness bool) bool {
	if !res.DeleteOnFailed ||
		res.KeepOnDelete ||
		installType == ResourceInstallTypeNone ||
		!mustTrackReadiness {
		return false
	}

	if getMeta != nil {
		if err := resource.ValidateResourcePolicy(getMeta); err != nil {
			return false
		} else {
			if keep := resource.KeepOnDelete(getMeta, releaseNamespace); keep {
				return false
			}
		}
	}

	return true
}

func mustTrackReadiness(res *resource.InstallableResource, resInstallType ResourceInstallType, exists, prevRelFailed, mustDeleteOnSuccessfulInstall bool) bool {
	if spec.IsCRD(res.Unstruct.GroupVersionKind().GroupKind()) ||
		res.TrackTerminationMode == multitrack.NonBlocking {
		return false
	}

	if resInstallType == ResourceInstallTypeNone {
		if exists && (prevRelFailed || mustDeleteOnSuccessfulInstall) {
			return true
		}

		return false
	}

	return true
}

func fixManagedFieldsInCluster(ctx context.Context, releaseNamespace string, getObj *unstructured.Unstructured, meta *spec.ResourceMeta, noRemoveManualChanges bool, clientFactory kube.ClientFactorier) (*unstructured.Unstructured, error) {
	if changed, err := fixManagedFields(getObj, noRemoveManualChanges); err != nil {
		return nil, fmt.Errorf("fix managed fields for resource %q: %w", meta.IDHuman(), err)
	} else if !changed {
		return getObj, nil
	}

	unstruct := unstructured.Unstructured{Object: map[string]interface{}{}}
	unstruct.SetManagedFields(getObj.GetManagedFields())

	patch, err := json.Marshal(unstruct.UnstructuredContent())
	if err != nil {
		return nil, fmt.Errorf("marshal fixed managed fields for resource %q: %w", meta.IDHuman(), err)
	}

	log.Default.Debug(ctx, "Fixing managed fields for resource %q", meta.IDHuman())

	patchedObj, err := clientFactory.KubeClient().MergePatch(ctx, meta, patch, kube.KubeClientMergePatchOptions{
		DefaultNamespace: releaseNamespace,
	})
	if err != nil {
		if kube.IsNotFoundErr(err) {
			return getObj, nil
		}

		return nil, fmt.Errorf("patch managed fields: %w", err)
	}

	return patchedObj, nil
}

func fixManagedFields(unstruct *unstructured.Unstructured, noRemoveManualChanges bool) (changed bool, err error) {
	managedFields := unstruct.GetManagedFields()
	if len(managedFields) == 0 {
		return false, nil
	}

	var oursEntry v1.ManagedFieldsEntry
	if e, found := lo.Find(managedFields, func(e v1.ManagedFieldsEntry) bool {
		return e.Manager == common.DefaultFieldManager && e.Operation == v1.ManagedFieldsOperationApply
	}); found {
		oursEntry = e
	} else {
		oursEntry = v1.ManagedFieldsEntry{
			Manager:    common.DefaultFieldManager,
			Operation:  v1.ManagedFieldsOperationApply,
			APIVersion: unstruct.GetAPIVersion(),
			Time:       lo.ToPtr(v1.Now()),
			FieldsType: "FieldsV1",
			FieldsV1:   &v1.FieldsV1{Raw: []byte("{}")},
		}
	}

	var fixedManagedFields []v1.ManagedFieldsEntry

	fixedManagedFields = append(fixedManagedFields, differentSubresourceManagers(managedFields, oursEntry)...)

	if newManagedFields, newOursEntry, chngd := removeUndesirableManagers(managedFields, oursEntry, noRemoveManualChanges); chngd {
		fixedManagedFields = append(fixedManagedFields, newManagedFields...)
		oursEntry = newOursEntry
		changed = true
	}

	if newManagedFields, chngd := exclusiveOwnershipForOurManager(managedFields, oursEntry); chngd {
		fixedManagedFields = append(fixedManagedFields, newManagedFields...)
		changed = true
	}

	if string(oursEntry.FieldsV1.Raw) != "{}" {
		fixedManagedFields = append(fixedManagedFields, oursEntry)
	}

	if changed {
		unstruct.SetManagedFields(fixedManagedFields)
	}

	return changed, nil
}

func differentSubresourceManagers(managedFields []v1.ManagedFieldsEntry, oursEntry v1.ManagedFieldsEntry) (newManagedFields []v1.ManagedFieldsEntry) {
	for _, managedField := range managedFields {
		if managedField.Subresource != oursEntry.Subresource {
			newManagedFields = append(newManagedFields, managedField)
			continue
		}
	}

	return newManagedFields
}

func removeUndesirableManagers(managedFields []v1.ManagedFieldsEntry, oursEntry v1.ManagedFieldsEntry, noRemoveManualChanges bool) (newManagedFields []v1.ManagedFieldsEntry, newOursEntry v1.ManagedFieldsEntry, changed bool) {
	oursFieldsByte := lo.Must(json.Marshal(oursEntry.FieldsV1))

	newOursEntry = oursEntry
	for _, managedField := range managedFields {
		if managedField.Subresource != oursEntry.Subresource {
			continue
		}

		fieldsByte := lo.Must(json.Marshal(managedField.FieldsV1))

		if managedField.Manager == common.DefaultFieldManager {
			if managedField.Operation == v1.ManagedFieldsOperationApply {
				continue
			}

			merged, mergeChanged := lo.Must2(util.MergeJSON(fieldsByte, oursFieldsByte))
			if mergeChanged {
				oursFieldsByte = merged
				lo.Must0(newOursEntry.FieldsV1.UnmarshalJSON(merged))
			}

			changed = true
		} else if (!noRemoveManualChanges && managedField.Manager == common.KubectlEditFieldManager) ||
			strings.HasPrefix(managedField.Manager, common.OldFieldManagerPrefix) {
			merged, mergeChanged := lo.Must2(util.MergeJSON(fieldsByte, oursFieldsByte))
			if mergeChanged {
				oursFieldsByte = merged
				lo.Must0(newOursEntry.FieldsV1.UnmarshalJSON(merged))
			}

			changed = true
		}
	}

	return newManagedFields, newOursEntry, changed
}

func exclusiveOwnershipForOurManager(managedFields []v1.ManagedFieldsEntry, oursEntry v1.ManagedFieldsEntry) (newManagedFields []v1.ManagedFieldsEntry, changed bool) {
	oursFieldsByte := lo.Must(json.Marshal(oursEntry.FieldsV1))

	for _, managedField := range managedFields {
		if managedField.Subresource != oursEntry.Subresource {
			continue
		}

		fieldsByte := lo.Must(json.Marshal(managedField.FieldsV1))

		if managedField.Manager == common.DefaultFieldManager ||
			managedField.Manager == common.KubectlEditFieldManager ||
			strings.HasPrefix(managedField.Manager, common.OldFieldManagerPrefix) {
			continue
		}

		subtracted, subtractChanged := lo.Must2(util.SubtractJSON(fieldsByte, oursFieldsByte))
		if !subtractChanged {
			newManagedFields = append(newManagedFields, managedField)
			continue
		}

		if string(subtracted) != "{}" {
			lo.Must0(managedField.FieldsV1.UnmarshalJSON(subtracted))
			newManagedFields = append(newManagedFields, managedField)
		}

		changed = true
	}

	return newManagedFields, changed
}

func orphaned(meta *spec.ResourceMeta, releaseName, releaseNamespace string) bool {
	if _, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternReleaseName); !found || value != releaseName {
		return true
	}

	if _, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternReleaseNamespace); !found || value != releaseNamespace {
		return true
	}

	if _, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Labels, common.LabelKeyPatternManagedBy); !found || value != "Helm" {
		return true
	}

	return false
}
