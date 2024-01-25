package plnbuilder

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/opertn"
	"github.com/werf/nelm/pkg/pln"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcid"
	"github.com/werf/nelm/pkg/resrcinfo"
	"github.com/werf/nelm/pkg/rls"
	"github.com/werf/nelm/pkg/rlshistor"
)

var StageOpNamesOrdered = []string{
	StageOpNamePrefixInit,
	StageOpNamePrefixStandaloneCRDs,
	StageOpNamePrefixHookCRDs,
	StageOpNamePrefixHookResources,
	StageOpNamePrefixGeneralCRDs,
	StageOpNamePrefixGeneralResources,
	StageOpNamePrefixPostHookCRDs,
	StageOpNamePrefixPostHookResources,
	StageOpNamePrefixFinal,
}

const (
	StageOpNamePrefixInit              = opertn.TypeStageOperation + "/initialization"
	StageOpNamePrefixStandaloneCRDs    = opertn.TypeStageOperation + "/standalone-crds"
	StageOpNamePrefixHookCRDs          = opertn.TypeStageOperation + "/pre-hook-crds"
	StageOpNamePrefixHookResources     = opertn.TypeStageOperation + "/pre-hook-resources"
	StageOpNamePrefixGeneralCRDs       = opertn.TypeStageOperation + "/general-crds"
	StageOpNamePrefixGeneralResources  = opertn.TypeStageOperation + "/general-resources"
	StageOpNamePrefixPostHookCRDs      = opertn.TypeStageOperation + "/post-hook-crds"
	StageOpNamePrefixPostHookResources = opertn.TypeStageOperation + "/post-hooks-resources"
	StageOpNamePrefixFinal             = opertn.TypeStageOperation + "/finalization"
)

func NewDeployPlanBuilder(
	deployType common.DeployType,
	taskStore *statestore.TaskStore,
	logStore *util.Concurrent[*logstore.LogStore],
	releaseNamespaceInfo *resrcinfo.DeployableReleaseNamespaceInfo,
	standaloneCRDsInfos []*resrcinfo.DeployableStandaloneCRDInfo,
	hookResourcesInfos []*resrcinfo.DeployableHookResourceInfo,
	generalResourcesInfos []*resrcinfo.DeployableGeneralResourceInfo,
	prevReleaseGeneralResourceInfos []*resrcinfo.DeployablePrevReleaseGeneralResourceInfo,
	newRelease *rls.Release,
	history rlshistor.Historier,
	kubeClient kubeclnt.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper meta.ResettableRESTMapper,
	opts DeployPlanBuilderOptions,
) *DeployPlanBuilder {
	plan := pln.NewPlan()

	preHookResourcesInfos := lo.Filter(hookResourcesInfos, func(info *resrcinfo.DeployableHookResourceInfo, _ int) bool {
		return info.Resource().OnPreAnything()
	})
	postHookResourcesInfos := lo.Filter(hookResourcesInfos, func(info *resrcinfo.DeployableHookResourceInfo, _ int) bool {
		return info.Resource().OnPostAnything()
	})

	curReleaseExistResourcesUIDs, _ := CurrentReleaseExistingResourcesUIDs(standaloneCRDsInfos, hookResourcesInfos, generalResourcesInfos)

	return &DeployPlanBuilder{
		taskStore:                       taskStore,
		logStore:                        logStore,
		deployType:                      deployType,
		plan:                            plan,
		releaseNamespaceInfo:            releaseNamespaceInfo,
		standaloneCRDsInfos:             standaloneCRDsInfos,
		preHookResourcesInfos:           preHookResourcesInfos,
		postHookResourcesInfos:          postHookResourcesInfos,
		generalResourcesInfos:           generalResourcesInfos,
		prevReleaseGeneralResourceInfos: prevReleaseGeneralResourceInfos,
		curReleaseExistingResourcesUIDs: curReleaseExistResourcesUIDs,
		newRelease:                      newRelease,
		prevRelease:                     opts.PrevRelease,
		prevDeployedRelease:             opts.PrevDeployedRelease,
		history:                         history,
		kubeClient:                      kubeClient,
		staticClient:                    staticClient,
		dynamicClient:                   dynamicClient,
		discoveryClient:                 discoveryClient,
		mapper:                          mapper,
		creationTimeout:                 opts.CreationTimeout,
		readinessTimeout:                opts.ReadinessTimeout,
		deletionTimeout:                 opts.DeletionTimeout,
	}
}

type DeployPlanBuilderOptions struct {
	PrevRelease         *rls.Release
	PrevDeployedRelease *rls.Release
	CreationTimeout     time.Duration
	ReadinessTimeout    time.Duration
	DeletionTimeout     time.Duration
}

type DeployPlanBuilder struct {
	taskStore                       *statestore.TaskStore
	logStore                        *util.Concurrent[*logstore.LogStore]
	deployType                      common.DeployType
	releaseNamespaceInfo            *resrcinfo.DeployableReleaseNamespaceInfo
	standaloneCRDsInfos             []*resrcinfo.DeployableStandaloneCRDInfo
	preHookResourcesInfos           []*resrcinfo.DeployableHookResourceInfo
	postHookResourcesInfos          []*resrcinfo.DeployableHookResourceInfo
	generalResourcesInfos           []*resrcinfo.DeployableGeneralResourceInfo
	prevReleaseGeneralResourceInfos []*resrcinfo.DeployablePrevReleaseGeneralResourceInfo
	curReleaseExistingResourcesUIDs []types.UID
	newRelease                      *rls.Release
	prevRelease                     *rls.Release
	prevDeployedRelease             *rls.Release
	history                         rlshistor.Historier
	kubeClient                      kubeclnt.KubeClienter
	staticClient                    kubernetes.Interface
	dynamicClient                   dynamic.Interface
	discoveryClient                 discovery.CachedDiscoveryInterface
	mapper                          meta.ResettableRESTMapper
	creationTimeout                 time.Duration
	readinessTimeout                time.Duration
	deletionTimeout                 time.Duration

	plan *pln.Plan
}

func (b *DeployPlanBuilder) Build(ctx context.Context) (*pln.Plan, error) {
	log.Default.Debug(ctx, "Setting up init operations")
	if err := b.setupInitOperations(); err != nil {
		return nil, fmt.Errorf("error setting up init operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up standalone CRDs operations")
	if err := b.setupStandaloneCRDsOperations(); err != nil {
		return nil, fmt.Errorf("error setting up standalone CRDs operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up pre hook resources operations")
	if err := b.setupPreHookResourcesOperations(); err != nil {
		return nil, fmt.Errorf("error setting up pre hooks operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up general resources operations")
	if err := b.setupGeneralResourcesOperations(); err != nil {
		return nil, fmt.Errorf("error setting up general resources operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up post hook resources operations")
	if err := b.setupPostHookResourcesOperations(); err != nil {
		return nil, fmt.Errorf("error setting up post hooks operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up prev release general resources operations")
	if err := b.setupPrevReleaseGeneralResourcesOperations(); err != nil {
		return nil, fmt.Errorf("error setting up prev release general resources operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up finalization operations")
	if err := b.setupFinalizationOperations(); err != nil {
		return nil, fmt.Errorf("error setting up finalization operations: %w", err)
	}

	log.Default.Debug(ctx, "Connecting stages")
	if err := b.connectStages(); err != nil {
		return nil, fmt.Errorf("error connecting stages: %w", err)
	}

	log.Default.Debug(ctx, "Connecting internal dependencies")
	if err := b.connectInternalDependencies(); err != nil {
		return nil, fmt.Errorf("error connecting internal dependencies: %w", err)
	}

	log.Default.Debug(ctx, "Optimizing plan")
	if err := b.plan.Optimize(); err != nil {
		return nil, fmt.Errorf("error optimizing plan: %w", err)
	}

	return b.plan, nil
}

func (b *DeployPlanBuilder) setupInitOperations() error {
	relNsInfo := b.releaseNamespaceInfo
	createRelNs := relNsInfo.ShouldCreate()
	updateRelNs := relNsInfo.ShouldUpdate()
	applyRelNs := relNsInfo.ShouldApply()

	var opDeployRelNs opertn.Operation
	if createRelNs {
		opDeployRelNs = opertn.NewCreateResourceOperation(
			relNsInfo.ResourceID,
			relNsInfo.Resource().Unstructured(),
			b.kubeClient,
			opertn.CreateResourceOperationOptions{
				ManageableBy: relNsInfo.Resource().ManageableBy(),
			},
		)
	} else if updateRelNs {
		var err error
		opDeployRelNs, err = opertn.NewUpdateResourceOperation(
			relNsInfo.ResourceID,
			relNsInfo.Resource().Unstructured(),
			b.kubeClient,
			opertn.UpdateResourceOperationOptions{
				ManageableBy: relNsInfo.Resource().ManageableBy(),
			},
		)
		if err != nil {
			return fmt.Errorf("error creating update resource operation: %w", err)
		}
	} else if applyRelNs {
		var err error
		opDeployRelNs, err = opertn.NewApplyResourceOperation(
			relNsInfo.ResourceID,
			relNsInfo.Resource().Unstructured(),
			b.kubeClient,
			opertn.ApplyResourceOperationOptions{
				ManageableBy: relNsInfo.Resource().ManageableBy(),
			},
		)
		if err != nil {
			return fmt.Errorf("error creating apply resource operation: %w", err)
		}
	}

	if opDeployRelNs != nil {
		b.plan.AddStagedOperation(
			opDeployRelNs,
			StageOpNamePrefixInit+"/"+StageOpNameSuffixStart,
			StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
		)
	}

	opCreatePendingRel := opertn.NewCreatePendingReleaseOperation(b.newRelease, b.deployType, b.history)
	b.plan.AddStagedOperation(
		opCreatePendingRel,
		StageOpNamePrefixInit+"/"+StageOpNameSuffixStart,
		StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
	)

	if opDeployRelNs != nil {
		lo.Must0(b.plan.AddDependency(opDeployRelNs.ID(), opCreatePendingRel.ID()))
	}

	return nil
}

func (b *DeployPlanBuilder) setupStandaloneCRDsOperations() error {
	for _, info := range b.standaloneCRDsInfos {
		create := info.ShouldCreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()

		var opDeploy opertn.Operation
		if create {
			opDeploy = opertn.NewCreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				opertn.CreateResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
		} else if update {
			var err error
			opDeploy, err = opertn.NewUpdateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				opertn.UpdateResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
			if err != nil {
				return fmt.Errorf("error creating update resource operation: %w", err)
			}
		} else if apply {
			var err error
			opDeploy, err = opertn.NewApplyResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				opertn.ApplyResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
			if err != nil {
				return fmt.Errorf("error creating apply resource operation: %w", err)
			}
		}

		if opDeploy != nil {
			b.plan.AddStagedOperation(
				opDeploy,
				StageOpNamePrefixStandaloneCRDs+"/"+StageOpNameSuffixStart,
				StageOpNamePrefixStandaloneCRDs+"/"+StageOpNameSuffixEnd,
			)
		}
	}

	return nil
}

func (b *DeployPlanBuilder) setupPreHookResourcesOperations() error {
	weighedInfos := lo.GroupBy(b.preHookResourcesInfos, func(info *resrcinfo.DeployableHookResourceInfo) int {
		return info.Resource().Weight()
	})

	weights := lo.Keys(weighedInfos)
	sort.Ints(weights)

	for _, weight := range weights {
		crdInfos := lo.Filter(weighedInfos[weight], func(info *resrcinfo.DeployableHookResourceInfo, _ int) bool {
			return resrc.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		crdsStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookCRDs, weight, StageOpNameSuffixStart)
		crdsStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookCRDs, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(crdInfos, crdsStageStartOpID, crdsStageEndOpID); err != nil {
			return fmt.Errorf("error setting up hook crds operations: %w", err)
		}

		resourceInfos := lo.Filter(weighedInfos[weight], func(info *resrcinfo.DeployableHookResourceInfo, _ int) bool {
			return !resrc.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		resourcesStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookResources, weight, StageOpNameSuffixStart)
		resourcesStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookResources, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(resourceInfos, resourcesStageStartOpID, resourcesStageEndOpID); err != nil {
			return fmt.Errorf("error setting up hook resources operations: %w", err)
		}
	}

	return nil
}

func (b *DeployPlanBuilder) setupPostHookResourcesOperations() error {
	weighedInfos := lo.GroupBy(b.postHookResourcesInfos, func(info *resrcinfo.DeployableHookResourceInfo) int {
		return info.Resource().Weight()
	})

	weights := lo.Keys(weighedInfos)
	sort.Ints(weights)

	for _, weight := range weights {
		crdInfos := lo.Filter(weighedInfos[weight], func(info *resrcinfo.DeployableHookResourceInfo, _ int) bool {
			return resrc.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		crdsStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookCRDs, weight, StageOpNameSuffixStart)
		crdsStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookCRDs, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(crdInfos, crdsStageStartOpID, crdsStageEndOpID); err != nil {
			return fmt.Errorf("error setting up hook crds operations: %w", err)
		}

		resourceInfos := lo.Filter(weighedInfos[weight], func(info *resrcinfo.DeployableHookResourceInfo, _ int) bool {
			return !resrc.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		resourcesStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookResources, weight, StageOpNameSuffixStart)
		resourcesStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookResources, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(resourceInfos, resourcesStageStartOpID, resourcesStageEndOpID); err != nil {
			return fmt.Errorf("error setting up hook resources operations: %w", err)
		}
	}

	return nil
}

func (b *DeployPlanBuilder) setupGeneralResourcesOperations() error {
	weighedInfos := lo.GroupBy(b.generalResourcesInfos, func(info *resrcinfo.DeployableGeneralResourceInfo) int {
		return info.Resource().Weight()
	})

	weights := lo.Keys(weighedInfos)
	sort.Ints(weights)

	for _, weight := range weights {
		crdInfos := lo.Filter(weighedInfos[weight], func(info *resrcinfo.DeployableGeneralResourceInfo, _ int) bool {
			return resrc.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		crdsStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixGeneralCRDs, weight, StageOpNameSuffixStart)
		crdsStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixGeneralCRDs, weight, StageOpNameSuffixEnd)

		if err := b.setupGeneralOperations(crdInfos, crdsStageStartOpID, crdsStageEndOpID); err != nil {
			return fmt.Errorf("error setting up general resources operations: %w", err)
		}

		resourceInfos := lo.Filter(weighedInfos[weight], func(info *resrcinfo.DeployableGeneralResourceInfo, _ int) bool {
			return !resrc.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		resourcesStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixGeneralResources, weight, StageOpNameSuffixStart)
		resourcesStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixGeneralResources, weight, StageOpNameSuffixEnd)

		if err := b.setupGeneralOperations(resourceInfos, resourcesStageStartOpID, resourcesStageEndOpID); err != nil {
			return fmt.Errorf("error setting up general resources operations: %w", err)
		}
	}

	return nil
}

func (b *DeployPlanBuilder) setupPrevReleaseGeneralResourcesOperations() error {
	for _, info := range b.prevReleaseGeneralResourceInfos {
		delete := info.ShouldDelete(b.curReleaseExistingResourcesUIDs)

		if delete {
			opDelete := opertn.NewDeleteResourceOperation(
				info.ResourceID,
				b.kubeClient,
			)
			b.plan.AddInStagedOperation(
				opDelete,
				StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
			)

			taskState := util.NewConcurrent(
				statestore.NewAbsenceTaskState(
					info.Name(),
					info.Namespace(),
					info.GroupVersionKind(),
					statestore.AbsenceTaskStateOptions{},
				),
			)
			b.taskStore.AddAbsenceTaskState(taskState)

			opTrackDeletion := opertn.NewTrackResourceAbsenceOperation(
				info.ResourceID,
				taskState,
				b.dynamicClient,
				b.mapper,
				opertn.TrackResourceAbsenceOperationOptions{
					Timeout: b.deletionTimeout,
				},
			)
			b.plan.AddOperation(opTrackDeletion)
			if err := b.plan.AddDependency(opDelete.ID(), opTrackDeletion.ID()); err != nil {
				return fmt.Errorf("error adding dependency: %w", err)
			}
		}
	}

	return nil
}

func (b *DeployPlanBuilder) setupFinalizationOperations() error {
	opUpdateSucceededRel := opertn.NewSucceedReleaseOperation(b.newRelease, b.history)
	b.plan.AddStagedOperation(
		opUpdateSucceededRel,
		StageOpNamePrefixFinal+"/"+StageOpNameSuffixStart,
		StageOpNamePrefixFinal+"/"+StageOpNameSuffixEnd,
	)

	if b.prevDeployedRelease != nil {
		opUpdateSupersededRel := opertn.NewSupersedeReleaseOperation(b.prevDeployedRelease, b.history)
		b.plan.AddStagedOperation(
			opUpdateSupersededRel,
			StageOpNamePrefixFinal+"/"+StageOpNameSuffixStart,
			StageOpNamePrefixFinal+"/"+StageOpNameSuffixEnd,
		)
	}

	return nil
}

func (b *DeployPlanBuilder) connectInternalDependencies() error {
	for _, info := range lo.Union(b.preHookResourcesInfos, b.postHookResourcesInfos) {
		var opDeploy opertn.Operation
		if info.ShouldCreate() {
			opDeploy = lo.Must(b.plan.Operation(opertn.TypeCreateResourceOperation + "/" + info.ID()))
		} else if info.ShouldRecreate() {
			opDeploy = lo.Must(b.plan.Operation(opertn.TypeRecreateResourceOperation + "/" + info.ID()))
		} else if info.ShouldUpdate() {
			opDeploy = lo.Must(b.plan.Operation(opertn.TypeUpdateResourceOperation + "/" + info.ID()))
		} else if info.ShouldApply() {
			opDeploy = lo.Must(b.plan.Operation(opertn.TypeApplyResourceOperation + "/" + info.ID()))
		} else {
			continue
		}

		autoInternalDeps, _ := info.Resource().AutoInternalDependencies()
		manualInternalDeps, _ := info.Resource().ManualInternalDependencies()

		for _, dep := range lo.Union(autoInternalDeps, manualInternalDeps) {
			opDeployRegex := regexp.MustCompile(fmt.Sprintf(`^(%s|%s|%s|%s)/`, opertn.TypeCreateResourceOperation, opertn.TypeRecreateResourceOperation, opertn.TypeUpdateResourceOperation, opertn.TypeApplyResourceOperation))
			opsDeploy, found, err := b.plan.OperationsMatch(opDeployRegex)
			if err != nil {
				return fmt.Errorf("error looking for operations by regex: %w", err)
			} else if !found {
				continue
			}

			opDepDeploy, found := lo.Find(opsDeploy, func(op opertn.Operation) bool {
				_, id := lo.Must2(strings.Cut(op.ID(), "/"))

				resID := resrcid.NewResourceIDFromID(id, resrcid.ResourceIDOptions{
					DefaultNamespace: b.releaseNamespaceInfo.Name(),
					Mapper:           b.mapper,
				})

				return dep.Match(resID)
			})
			if !found {
				continue
			}

			if err := b.plan.AddDependency(opDepDeploy.ID(), opDeploy.ID()); err != nil {
				return fmt.Errorf("error adding dependency: %w", err)
			}
		}
	}

	// TODO(ilya-lesikov): almost identical with hooks, refactor
	for _, info := range b.generalResourcesInfos {
		var opDeploy opertn.Operation
		if info.ShouldCreate() {
			opDeploy = lo.Must(b.plan.Operation(opertn.TypeCreateResourceOperation + "/" + info.ID()))
		} else if info.ShouldRecreate() {
			opDeploy = lo.Must(b.plan.Operation(opertn.TypeRecreateResourceOperation + "/" + info.ID()))
		} else if info.ShouldUpdate() {
			opDeploy = lo.Must(b.plan.Operation(opertn.TypeUpdateResourceOperation + "/" + info.ID()))
		} else if info.ShouldApply() {
			opDeploy = lo.Must(b.plan.Operation(opertn.TypeApplyResourceOperation + "/" + info.ID()))
		} else {
			continue
		}

		autoInternalDeps, _ := info.Resource().AutoInternalDependencies()
		manualInternalDeps, _ := info.Resource().ManualInternalDependencies()

		for _, dep := range lo.Union(autoInternalDeps, manualInternalDeps) {
			opDeployRegex := regexp.MustCompile(fmt.Sprintf(`^(%s|%s|%s|%s)/`, opertn.TypeCreateResourceOperation, opertn.TypeRecreateResourceOperation, opertn.TypeUpdateResourceOperation, opertn.TypeApplyResourceOperation))
			opsDeploy, found, err := b.plan.OperationsMatch(opDeployRegex)
			if err != nil {
				return fmt.Errorf("error looking for operations by regex: %w", err)
			} else if !found {
				continue
			}

			opDepDeploy, found := lo.Find(opsDeploy, func(op opertn.Operation) bool {
				_, id := lo.Must2(strings.Cut(op.ID(), "/"))

				resID := resrcid.NewResourceIDFromID(id, resrcid.ResourceIDOptions{
					DefaultNamespace: b.releaseNamespaceInfo.Name(),
					Mapper:           b.mapper,
				})

				return dep.Match(resID)
			})
			if !found {
				continue
			}

			if err := b.plan.AddDependency(opDepDeploy.ID(), opDeploy.ID()); err != nil {
				return fmt.Errorf("error adding dependency: %w", err)
			}
		}
	}

	return nil
}

func (b *DeployPlanBuilder) connectStages() error {
	opsStagesRegex := regexp.MustCompile(fmt.Sprintf(`^(%s)/`, strings.Join(StageOpNamesOrdered, "|")))

	opsStages, found, err := b.plan.OperationsMatch(opsStagesRegex)
	if err != nil {
		return fmt.Errorf("error looking for operations by regex: %w", err)
	} else if !found {
		return nil
	}

	sort.Slice(opsStages, func(i, j int) bool {
		iID := opsStages[i].ID()
		_, iIndex := lo.Must2(lo.FindIndexOf(StageOpNamesOrdered, func(name string) bool {
			return strings.HasPrefix(iID, name+"/")
		}))

		jID := opsStages[j].ID()
		_, jIndex := lo.Must2(lo.FindIndexOf(StageOpNamesOrdered, func(name string) bool {
			return strings.HasPrefix(jID, name+"/")
		}))

		if iIndex == jIndex {
			var iWeight *int
			for _, iIDSplit := range strings.Split(iID, "/") {
				parts := strings.SplitN(iIDSplit, ":", 2)

				if parts[0] != "weight" {
					continue
				}

				iWeight = lo.ToPtr(lo.Must(strconv.Atoi(parts[1])))
				break
			}

			var jWeight *int
			for _, jIDSplit := range strings.Split(jID, "/") {
				parts := strings.SplitN(jIDSplit, ":", 2)

				if parts[0] != "weight" {
					continue
				}

				jWeight = lo.ToPtr(lo.Must(strconv.Atoi(parts[1])))
				break
			}

			if iWeight != nil && jWeight != nil {
				if *iWeight == *jWeight {
					return strings.HasSuffix(iID, "/"+StageOpNameSuffixStart)
				}

				return *iWeight < *jWeight
			}

			return strings.HasSuffix(iID, "/"+StageOpNameSuffixStart)
		}

		return iIndex < jIndex
	})

	for i := 0; i < len(opsStages); i++ {
		if i == 0 {
			continue
		}

		if err := b.plan.AddDependency(opsStages[i-1].ID(), opsStages[i].ID()); err != nil {
			return fmt.Errorf("error adding dependency: %w", err)
		}
	}

	return nil
}

func (b *DeployPlanBuilder) setupHookOperations(infos []*resrcinfo.DeployableHookResourceInfo, stageStartOpID, stageEndOpID string) error {
	var prevReleaseFailed bool
	if b.prevRelease != nil {
		prevReleaseFailed = b.prevRelease.Failed()
	}

	for _, info := range infos {
		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup()
		var trackReadiness bool
		if track := info.ShouldTrackReadiness(prevReleaseFailed); track {
			if _, manIntDepsSet := info.Resource().ManualInternalDependencies(); !manIntDepsSet {
				trackReadiness = true
			}
		}
		externalDeps, extDepsSet, err := info.Resource().ExternalDependencies()
		if err != nil {
			return fmt.Errorf("error getting external dependencies: %w", err)
		}
		var forceReplicas *int
		if r, set := info.Resource().DefaultReplicasOnCreation(); set {
			forceReplicas = &r
		}

		var opDeploy opertn.Operation
		if create {
			opDeploy = opertn.NewCreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				opertn.CreateResourceOperationOptions{
					ManageableBy:  info.Resource().ManageableBy(),
					ForceReplicas: forceReplicas,
				},
			)
		} else if recreate {
			absenceTaskState := util.NewConcurrent(
				statestore.NewAbsenceTaskState(info.Name(), info.Namespace(), info.GroupVersionKind(), statestore.AbsenceTaskStateOptions{}),
			)
			b.taskStore.AddAbsenceTaskState(absenceTaskState)

			opDeploy = opertn.NewRecreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				absenceTaskState,
				b.kubeClient,
				b.dynamicClient,
				b.mapper,
				opertn.RecreateResourceOperationOptions{
					ManageableBy:         info.Resource().ManageableBy(),
					ForceReplicas:        forceReplicas,
					DeletionTrackTimeout: b.deletionTimeout,
				},
			)
		} else if update {
			var err error
			opDeploy, err = opertn.NewUpdateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				opertn.UpdateResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
			if err != nil {
				return fmt.Errorf("error creating update resource operation: %w", err)
			}
		} else if apply {
			var err error
			opDeploy, err = opertn.NewApplyResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				opertn.ApplyResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
			if err != nil {
				return fmt.Errorf("error creating apply resource operation: %w", err)
			}
		}

		if opDeploy != nil {
			b.plan.AddStagedOperation(
				opDeploy,
				stageStartOpID,
				stageEndOpID,
			)
		}

		if extDepsSet && opDeploy != nil {
			for _, dep := range externalDeps {
				taskState := util.NewConcurrent(
					statestore.NewPresenceTaskState(
						dep.Name(),
						dep.Namespace(),
						dep.GroupVersionKind(),
						statestore.PresenceTaskStateOptions{},
					),
				)
				b.taskStore.AddPresenceTaskState(taskState)

				opTrackReadiness := opertn.NewTrackResourcePresenceOperation(
					dep.ResourceID,
					taskState,
					b.dynamicClient,
					b.mapper,
					opertn.TrackResourcePresenceOperationOptions{
						Timeout: b.readinessTimeout,
					},
				)
				b.plan.AddInStagedOperation(
					opTrackReadiness,
					stageStartOpID,
				)
				lo.Must0(b.plan.AddDependency(opTrackReadiness.ID(), opDeploy.ID()))
			}
		}

		var opTrackReadiness *opertn.TrackResourceReadinessOperation
		if trackReadiness {
			logRegex, _ := info.Resource().LogRegex()
			logRegexesFor, _ := info.Resource().LogRegexesForContainers()
			skipLogsFor, _ := info.Resource().SkipLogsForContainers()
			showLogsOnlyFor, _ := info.Resource().ShowLogsOnlyForContainers()
			ignoreReadinessProbes, _ := info.Resource().IgnoreReadinessProbeFailsForContainers()
			var noActivityTimeout time.Duration
			if timeout, set := info.Resource().NoActivityTimeout(); set {
				noActivityTimeout = *timeout
			}

			taskState := util.NewConcurrent(
				statestore.NewReadinessTaskState(info.Name(), info.Namespace(), info.GroupVersionKind(), statestore.ReadinessTaskStateOptions{
					FailMode:                info.Resource().FailMode(),
					TotalAllowFailuresCount: info.Resource().FailuresAllowed(),
				}),
			)
			b.taskStore.AddReadinessTaskState(taskState)

			opTrackReadiness = opertn.NewTrackResourceReadinessOperation(
				info.ResourceID,
				taskState,
				b.logStore,
				b.staticClient,
				b.dynamicClient,
				b.discoveryClient,
				b.mapper,
				opertn.TrackResourceReadinessOperationOptions{
					Timeout:                                  b.readinessTimeout,
					NoActivityTimeout:                        noActivityTimeout,
					IgnoreReadinessProbeFailsByContainerName: ignoreReadinessProbes,
					SaveLogsOnlyForContainers:                showLogsOnlyFor,
					SaveLogsByRegex:                          logRegex,
					SaveLogsByRegexForContainers:             logRegexesFor,
					IgnoreLogs:                               info.Resource().SkipLogs(),
					IgnoreLogsForContainers:                  skipLogsFor,
					SaveEvents:                               info.Resource().ShowServiceMessages(),
				},
			)
			b.plan.AddStagedOperation(
				opTrackReadiness,
				stageStartOpID,
				stageEndOpID,
			)
			if opDeploy != nil {
				lo.Must0(b.plan.AddDependency(opDeploy.ID(), opTrackReadiness.ID()))
			}
		}

		if cleanup {
			cleanupOp := opertn.NewDeleteResourceOperation(
				info.ResourceID,
				b.kubeClient,
			)

			if trackReadiness {
				b.plan.AddOperation(cleanupOp)
				lo.Must0(b.plan.AddDependency(opTrackReadiness.ID(), cleanupOp.ID()))
			} else if opDeploy != nil {
				b.plan.AddOperation(cleanupOp)
				lo.Must0(b.plan.AddDependency(opDeploy.ID(), cleanupOp.ID()))
			} else {
				b.plan.AddInStagedOperation(
					cleanupOp,
					StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
				)
			}

			taskState := util.NewConcurrent(
				statestore.NewAbsenceTaskState(
					info.Name(),
					info.Namespace(),
					info.GroupVersionKind(),
					statestore.AbsenceTaskStateOptions{},
				),
			)
			b.taskStore.AddAbsenceTaskState(taskState)

			opTrackDeletion := opertn.NewTrackResourceAbsenceOperation(
				info.ResourceID,
				taskState,
				b.dynamicClient,
				b.mapper,
				opertn.TrackResourceAbsenceOperationOptions{
					Timeout: b.deletionTimeout,
				},
			)
			b.plan.AddOperation(opTrackDeletion)
			if err := b.plan.AddDependency(cleanupOp.ID(), opTrackDeletion.ID()); err != nil {
				return fmt.Errorf("error adding dependency: %w", err)
			}
		}
	}

	return nil
}

// TODO(ilya-lesikov): almost identical with setupHookOperations, refactor
func (b *DeployPlanBuilder) setupGeneralOperations(infos []*resrcinfo.DeployableGeneralResourceInfo, stageStartOpID, stageEndOpID string) error {
	var prevReleaseFailed bool
	if b.prevRelease != nil {
		prevReleaseFailed = b.prevRelease.Failed()
	}

	for _, info := range infos {
		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup()
		var trackReadiness bool
		if track := info.ShouldTrackReadiness(prevReleaseFailed); track {
			if _, manIntDepsSet := info.Resource().ManualInternalDependencies(); !manIntDepsSet {
				trackReadiness = true
			}
		}
		externalDeps, extDepsSet, err := info.Resource().ExternalDependencies()
		if err != nil {
			return fmt.Errorf("error getting external dependencies: %w", err)
		}
		var forceReplicas *int
		if r, set := info.Resource().DefaultReplicasOnCreation(); set {
			forceReplicas = &r
		}

		var opDeploy opertn.Operation
		if create {
			opDeploy = opertn.NewCreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				opertn.CreateResourceOperationOptions{
					ManageableBy:  info.Resource().ManageableBy(),
					ForceReplicas: forceReplicas,
				},
			)
		} else if recreate {
			absenceTaskState := util.NewConcurrent(
				statestore.NewAbsenceTaskState(info.Name(), info.Namespace(), info.GroupVersionKind(), statestore.AbsenceTaskStateOptions{}),
			)
			b.taskStore.AddAbsenceTaskState(absenceTaskState)

			opDeploy = opertn.NewRecreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				absenceTaskState,
				b.kubeClient,
				b.dynamicClient,
				b.mapper,
				opertn.RecreateResourceOperationOptions{
					ManageableBy:         info.Resource().ManageableBy(),
					ForceReplicas:        forceReplicas,
					DeletionTrackTimeout: b.deletionTimeout,
				},
			)
		} else if update {
			var err error
			opDeploy, err = opertn.NewUpdateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				opertn.UpdateResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
			if err != nil {
				return fmt.Errorf("error creating update resource operation: %w", err)
			}
		} else if apply {
			var err error
			opDeploy, err = opertn.NewApplyResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				opertn.ApplyResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
			if err != nil {
				return fmt.Errorf("error creating apply resource operation: %w", err)
			}
		}

		if opDeploy != nil {
			b.plan.AddStagedOperation(
				opDeploy,
				stageStartOpID,
				stageEndOpID,
			)
		}

		if extDepsSet && opDeploy != nil {
			for _, dep := range externalDeps {
				taskState := util.NewConcurrent(
					statestore.NewPresenceTaskState(
						dep.Name(),
						dep.Namespace(),
						dep.GroupVersionKind(),
						statestore.PresenceTaskStateOptions{},
					),
				)
				b.taskStore.AddPresenceTaskState(taskState)

				opTrackReadiness := opertn.NewTrackResourcePresenceOperation(
					dep.ResourceID,
					taskState,
					b.dynamicClient,
					b.mapper,
					opertn.TrackResourcePresenceOperationOptions{
						Timeout: b.readinessTimeout,
					},
				)
				b.plan.AddInStagedOperation(
					opTrackReadiness,
					stageStartOpID,
				)
				lo.Must0(b.plan.AddDependency(opTrackReadiness.ID(), opDeploy.ID()))
			}
		}

		var opTrackReadiness *opertn.TrackResourceReadinessOperation
		if trackReadiness {
			logRegex, _ := info.Resource().LogRegex()
			logRegexesFor, _ := info.Resource().LogRegexesForContainers()
			skipLogsFor, _ := info.Resource().SkipLogsForContainers()
			showLogsOnlyFor, _ := info.Resource().ShowLogsOnlyForContainers()
			ignoreReadinessProbes, _ := info.Resource().IgnoreReadinessProbeFailsForContainers()
			var noActivityTimeout time.Duration
			if timeout, set := info.Resource().NoActivityTimeout(); set {
				noActivityTimeout = *timeout
			}

			taskState := util.NewConcurrent(
				statestore.NewReadinessTaskState(info.Name(), info.Namespace(), info.GroupVersionKind(), statestore.ReadinessTaskStateOptions{
					FailMode:                info.Resource().FailMode(),
					TotalAllowFailuresCount: info.Resource().FailuresAllowed(),
				}),
			)
			b.taskStore.AddReadinessTaskState(taskState)

			opTrackReadiness = opertn.NewTrackResourceReadinessOperation(
				info.ResourceID,
				taskState,
				b.logStore,
				b.staticClient,
				b.dynamicClient,
				b.discoveryClient,
				b.mapper,
				opertn.TrackResourceReadinessOperationOptions{
					Timeout:                                  b.readinessTimeout,
					NoActivityTimeout:                        noActivityTimeout,
					IgnoreReadinessProbeFailsByContainerName: ignoreReadinessProbes,
					SaveLogsOnlyForContainers:                showLogsOnlyFor,
					SaveLogsByRegex:                          logRegex,
					SaveLogsByRegexForContainers:             logRegexesFor,
					IgnoreLogs:                               info.Resource().SkipLogs(),
					IgnoreLogsForContainers:                  skipLogsFor,
					SaveEvents:                               info.Resource().ShowServiceMessages(),
				},
			)
			b.plan.AddStagedOperation(
				opTrackReadiness,
				stageStartOpID,
				stageEndOpID,
			)
			if opDeploy != nil {
				lo.Must0(b.plan.AddDependency(opDeploy.ID(), opTrackReadiness.ID()))
			}
		}

		if cleanup {
			cleanupOp := opertn.NewDeleteResourceOperation(
				info.ResourceID,
				b.kubeClient,
			)

			if trackReadiness {
				b.plan.AddOperation(cleanupOp)
				lo.Must0(b.plan.AddDependency(opTrackReadiness.ID(), cleanupOp.ID()))
			} else if opDeploy != nil {
				b.plan.AddOperation(cleanupOp)
				lo.Must0(b.plan.AddDependency(opDeploy.ID(), cleanupOp.ID()))
			} else {
				b.plan.AddInStagedOperation(
					cleanupOp,
					StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
				)
			}

			taskState := util.NewConcurrent(
				statestore.NewAbsenceTaskState(
					info.Name(),
					info.Namespace(),
					info.GroupVersionKind(),
					statestore.AbsenceTaskStateOptions{},
				),
			)
			b.taskStore.AddAbsenceTaskState(taskState)

			opTrackDeletion := opertn.NewTrackResourceAbsenceOperation(
				info.ResourceID,
				taskState,
				b.dynamicClient,
				b.mapper,
				opertn.TrackResourceAbsenceOperationOptions{
					Timeout: b.deletionTimeout,
				},
			)
			b.plan.AddOperation(opTrackDeletion)
			if err := b.plan.AddDependency(cleanupOp.ID(), opTrackDeletion.ID()); err != nil {
				return fmt.Errorf("error adding dependency: %w", err)
			}
		}
	}

	return nil
}
