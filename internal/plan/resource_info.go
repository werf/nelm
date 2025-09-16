package plan

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/wI2L/jsondiff"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/meta"
	"github.com/werf/nelm/internal/util"
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

func BuildResourceInfos(
	ctx context.Context,
	releaseName string,
	releaseNamespace string,
	instResources []*resource.InstallableResource,
	delResources []*resource.DeletableResource,
	prevReleaseFailed bool,
	kubeClient kube.KubeClienter,
	mapper apimeta.ResettableRESTMapper,
	parallelism int,
) (
	instResourceInfos []*InstallableResourceInfo,
	delResourceInfos []*DeletableResourceInfo,
	err error,
) {
	totalResourcesCount := len(instResources) + len(delResources)

	routines := lo.Max([]int{len(instResources) / lo.Max([]int{totalResourcesCount, 1}) * parallelism, 1})
	instResourcesPool := pool.NewWithResults[*InstallableResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range instResources {
		res := res
		instResourcesPool.Go(func(ctx context.Context) (*InstallableResourceInfo, error) {
			info, err := BuildInstallableResourceInfo(ctx, res, releaseNamespace, prevReleaseFailed, kubeClient, mapper)
			if err != nil {
				return nil, fmt.Errorf("build installable resource info: %w", err)
			}

			return info, nil
		})
	}

	routines = lo.Max([]int{len(delResources) / lo.Max([]int{totalResourcesCount, 1}) * parallelism, 1})
	delResourcesPool := pool.NewWithResults[*DeletableResourceInfo]().WithContext(ctx).WithMaxGoroutines(routines).WithCancelOnError().WithFirstError()
	for _, res := range delResources {
		res := res
		delResourcesPool.Go(func(ctx context.Context) (*DeletableResourceInfo, error) {
			info, err := BuildDeletableResourceInfo(ctx, res, releaseName, releaseNamespace, kubeClient, mapper)
			if err != nil {
				return nil, fmt.Errorf("build deletable resource info: %w", err)
			}

			return info, nil
		})
	}

	instResourceInfos, err = instResourcesPool.Wait()
	if err != nil {
		return nil, nil, fmt.Errorf("wait for resource pool: %w", err)
	}

	delResourceInfos, err = delResourcesPool.Wait()
	if err != nil {
		return nil, nil, fmt.Errorf("wait for prev release resource pool: %w", err)
	}

	sort.SliceStable(instResourceInfos, func(i, j int) bool {
		return resource.InstallableResourceSortByStageAndWeightHandler(instResourceInfos[i].LocalResource, instResourceInfos[j].LocalResource)
	})

	sort.SliceStable(delResourceInfos, func(i, j int) bool {
		return meta.ResourceMetaSortHandler(delResourceInfos[i].LocalResource.ResourceMeta, delResourceInfos[j].LocalResource.ResourceMeta)
	})

	iterateInstallableResourceInfos(instResourceInfos)

	delResourceInfos = filterDelResourcesPresentInInstResources(instResourceInfos, delResourceInfos)
	delResourceInfos = deduplicateDeletableResourceInfos(delResourceInfos)

	return instResourceInfos, delResourceInfos, nil
}

// TODO(v2): keep annotation should probably forbid resource recreations
func BuildInstallableResourceInfo(ctx context.Context, localRes *resource.InstallableResource, releaseNamespace string, prevRelFailed bool, kubeClient kube.KubeClienter, mapper apimeta.ResettableRESTMapper) (*InstallableResourceInfo, error) {
	getObj, getErr := kubeClient.Get(ctx, localRes.ResourceMeta, kube.KubeClientGetOptions{
		TryCache: true,
	})
	if getErr != nil {
		if kube.IsNotFoundErr(getErr) || kube.IsNoSuchKindErr(getErr) {
			return &InstallableResourceInfo{
				ResourceMeta:                  localRes.ResourceMeta,
				LocalResource:                 localRes,
				MustInstall:                   ResourceInstallTypeCreate,
				MustDeleteOnSuccessfulInstall: mustDeleteOnSuccessfulDeploy(localRes, nil, ResourceInstallTypeCreate, releaseNamespace),
				MustDeleteOnFailedInstall:     mustDeleteOnFailedDeploy(localRes, nil, ResourceInstallTypeCreate, releaseNamespace),
				MustTrackReadiness:            mustTrackReadiness(localRes, ResourceInstallTypeCreate, false, prevRelFailed),
			}, nil
		} else {
			return nil, fmt.Errorf("get resource %q: %w", localRes.IDHuman(), getErr)
		}
	}

	var err error
	getObj, err = fixManagedFieldsInCluster(ctx, releaseNamespace, getObj, localRes.ResourceMeta, kubeClient)
	if err != nil {
		return nil, fmt.Errorf("fix managed fields for resource %q: %w", localRes.IDHuman(), err)
	}

	dryApplyObj, dryApplyErr := kubeClient.Apply(ctx, localRes.ResourceSpec, kube.KubeClientApplyOptions{
		DryRun: true,
	})

	installType, err := resourceInstallType(localRes, getObj, dryApplyObj, dryApplyErr)
	if err != nil {
		return nil, fmt.Errorf("determine install type for resource %q: %w", localRes.IDHuman(), err)
	}

	getMeta := meta.NewResourceMetaFromUnstructured(getObj, releaseNamespace, localRes.FilePath)

	return &InstallableResourceInfo{
		ResourceMeta:                  localRes.ResourceMeta,
		LocalResource:                 localRes,
		GetResult:                     getObj,
		DryApplyResult:                dryApplyObj,
		DryApplyErr:                   dryApplyErr,
		MustInstall:                   installType,
		MustDeleteOnSuccessfulInstall: mustDeleteOnSuccessfulDeploy(localRes, getMeta, installType, releaseNamespace),
		MustDeleteOnFailedInstall:     mustDeleteOnFailedDeploy(localRes, getMeta, installType, releaseNamespace),
		MustTrackReadiness:            mustTrackReadiness(localRes, installType, true, prevRelFailed),
	}, nil
}

func BuildDeletableResourceInfo(ctx context.Context, localRes *resource.DeletableResource, releaseName, releaseNamespace string, kubeClient kube.KubeClienter, mapper apimeta.ResettableRESTMapper) (*DeletableResourceInfo, error) {
	noDeleteInfo := &DeletableResourceInfo{
		ResourceMeta:  localRes.ResourceMeta,
		LocalResource: localRes,
	}

	if localRes.KeepOnDelete || localRes.Ownership == common.OwnershipEveryone {
		return noDeleteInfo, nil
	}

	getObj, getErr := kubeClient.Get(ctx, localRes.ResourceMeta, kube.KubeClientGetOptions{
		TryCache: true,
	})
	if getErr != nil {
		if kube.IsNotFoundErr(getErr) || kube.IsNoSuchKindErr(getErr) {
			return noDeleteInfo, nil
		} else {
			return nil, fmt.Errorf("get resource %q: %w", localRes.IDHuman(), getErr)
		}
	}

	getMeta := meta.NewResourceMetaFromUnstructured(getObj, releaseNamespace, localRes.FilePath)

	if err := resource.ValidateResourcePolicy(getMeta); err != nil {
		return noDeleteInfo, nil
	} else {
		if keep := resource.KeepOnDelete(getMeta, releaseNamespace); keep {
			return noDeleteInfo, nil
		}
	}

	if resource.Orphaned(getMeta, releaseName, releaseNamespace) {
		return noDeleteInfo, nil
	}

	return &DeletableResourceInfo{
		ResourceMeta:  localRes.ResourceMeta,
		LocalResource: localRes,
		GetResult:     getObj,
		MustDelete:    true,
		// TODO: make switchable
		MustTrackAbsence: true,
	}, nil
}

type InstallableResourceInfo struct {
	*meta.ResourceMeta

	LocalResource  *resource.InstallableResource
	GetResult      *unstructured.Unstructured
	DryApplyResult *unstructured.Unstructured
	DryApplyErr    error

	MustInstall                   ResourceInstallType
	MustDeleteOnSuccessfulInstall bool
	MustDeleteOnFailedInstall     bool
	MustTrackReadiness            bool

	Iteration int
}

type DeletableResourceInfo struct {
	*meta.ResourceMeta

	LocalResource *resource.DeletableResource
	GetResult     *unstructured.Unstructured

	MustDelete       bool
	MustTrackAbsence bool
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

func resourceInstallType(localRes *resource.InstallableResource, getObj, dryApplyObj *unstructured.Unstructured, dryApplyErr error) (ResourceInstallType, error) {
	if dryApplyErr != nil && kube.IsImmutableErr(dryApplyErr) && !localRes.Recreate {
		return "", fmt.Errorf("immutable fields change in resource %q, but recreation is not requested: %w", localRes.IDHuman(), dryApplyErr)
	}

	if localRes.Recreate {
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
		return ResourceInstallTypeUpdate, nil
	}

	return ResourceInstallTypeNone, nil
}

func mustDeleteOnSuccessfulDeploy(localRes *resource.InstallableResource, getMeta *meta.ResourceMeta, installType ResourceInstallType, releaseNamespace string) bool {
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
		if getMeta != nil {
			return true
		}

		return false
	}

	return true
}

func mustDeleteOnFailedDeploy(res *resource.InstallableResource, getMeta *meta.ResourceMeta, installType ResourceInstallType, releaseNamespace string) bool {
	if !res.DeleteOnFailed ||
		res.KeepOnDelete ||
		installType == ResourceInstallTypeNone {
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

func mustTrackReadiness(res *resource.InstallableResource, resInstallType ResourceInstallType, exists, prevRelFailed bool) bool {
	if resource.IsCRD(res.Unstruct.GroupVersionKind().GroupKind()) ||
		res.TrackTerminationMode == multitrack.NonBlocking {
		return false
	}

	if resInstallType == ResourceInstallTypeNone {
		if exists && prevRelFailed {
			return true
		}

		return false
	}

	return true
}

func fixManagedFieldsInCluster(ctx context.Context, releaseNamespace string, getObj *unstructured.Unstructured, meta *meta.ResourceMeta, kubeClient kube.KubeClienter) (*unstructured.Unstructured, error) {
	if changed, err := fixManagedFields(getObj); err != nil {
		return nil, fmt.Errorf("fix managed fields for resource %q: %w", meta.IDHuman(), err)
	} else if !changed {
		return getObj, nil
	}

	unstruct := unstructured.Unstructured{Object: map[string]interface{}{}}
	unstruct.SetManagedFields(unstruct.GetManagedFields())

	patch, err := json.Marshal(unstruct.UnstructuredContent())
	if err != nil {
		return nil, fmt.Errorf("marshal fixed managed fields for resource %q: %w", meta.IDHuman(), err)
	}

	log.Default.Debug(ctx, "Fixing managed fields for resource %q", meta.IDHuman())
	getObj, err = kubeClient.MergePatch(ctx, meta, patch)
	if err != nil {
		return nil, fmt.Errorf("patch managed fields: %w", err)
	}

	return getObj, nil
}

func fixManagedFields(unstruct *unstructured.Unstructured) (changed bool, err error) {
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

	if newManagedFields, newOursEntry, chngd := removeUndesirableManagers(managedFields, oursEntry); chngd {
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

func removeUndesirableManagers(managedFields []v1.ManagedFieldsEntry, oursEntry v1.ManagedFieldsEntry) (newManagedFields []v1.ManagedFieldsEntry, newOursEntry v1.ManagedFieldsEntry, changed bool) {
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

			merged, mergeChanged := lo.Must2(util.MergeJson(fieldsByte, oursFieldsByte))
			if mergeChanged {
				oursFieldsByte = merged
				lo.Must0(newOursEntry.FieldsV1.UnmarshalJSON(merged))
			}

			changed = true
		} else if managedField.Manager == common.KubectlEditFieldManager ||
			strings.HasPrefix(managedField.Manager, common.OldFieldManagerPrefix) {
			merged, mergeChanged := lo.Must2(util.MergeJson(fieldsByte, oursFieldsByte))
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

		subtracted, subtractChanged := lo.Must2(util.SubtractJson(fieldsByte, oursFieldsByte))
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
