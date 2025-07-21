package plan

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
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan/dependency"
	"github.com/werf/nelm/internal/plan/operation"
	info "github.com/werf/nelm/internal/plan/resourceinfo"
	"github.com/werf/nelm/internal/release"
	resid "github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
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
	StageOpNamePrefixInit              = operation.TypeStageOperation + "/initialization"
	StageOpNamePrefixStandaloneCRDs    = operation.TypeStageOperation + "/standalone-crds"
	StageOpNamePrefixHookCRDs          = operation.TypeStageOperation + "/pre-hook-crds"
	StageOpNamePrefixHookResources     = operation.TypeStageOperation + "/pre-hook-resources"
	StageOpNamePrefixGeneralCRDs       = operation.TypeStageOperation + "/general-crds"
	StageOpNamePrefixGeneralResources  = operation.TypeStageOperation + "/general-resources"
	StageOpNamePrefixPostHookCRDs      = operation.TypeStageOperation + "/post-hook-crds"
	StageOpNamePrefixPostHookResources = operation.TypeStageOperation + "/post-hooks-resources"
	StageOpNamePrefixFinal             = operation.TypeStageOperation + "/finalization"
)

func NewDeployPlanBuilder(
	releaseNamespace string,
	deployType common.DeployType,
	taskStore *statestore.TaskStore,
	logStore *kdutil.Concurrent[*logstore.LogStore],
	standaloneCRDsInfos []*info.DeployableStandaloneCRDInfo,
	hookResourcesInfos []*info.DeployableHookResourceInfo,
	generalResourcesInfos []*info.DeployableGeneralResourceInfo,
	prevReleaseGeneralResourceInfos []*info.DeployablePrevReleaseGeneralResourceInfo,
	newRelease *release.Release,
	history release.Historier,
	kubeClient kube.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper meta.ResettableRESTMapper,
	opts DeployPlanBuilderOptions,
) *DeployPlanBuilder {
	plan := NewPlan()

	preHookResourcesInfos := lo.Filter(hookResourcesInfos, func(info *info.DeployableHookResourceInfo, _ int) bool {
		switch deployType {
		case common.DeployTypeInitial, common.DeployTypeInstall:
			return info.Resource().OnPreInstall()
		case common.DeployTypeUpgrade:
			return info.Resource().OnPreUpgrade()
		case common.DeployTypeRollback:
			return info.Resource().OnPreRollback()
		}

		return false
	})

	postHookResourcesInfos := lo.Filter(hookResourcesInfos, func(info *info.DeployableHookResourceInfo, _ int) bool {
		switch deployType {
		case common.DeployTypeInitial, common.DeployTypeInstall:
			return info.Resource().OnPostInstall()
		case common.DeployTypeUpgrade:
			return info.Resource().OnPostUpgrade()
		case common.DeployTypeRollback:
			return info.Resource().OnPostRollback()
		}

		return false
	})

	prePostHookResourcesIDs := lo.FilterMap(hookResourcesInfos, func(info *info.DeployableHookResourceInfo, _ int) (*resid.ResourceID, bool) {
		res := info.Resource()

		switch deployType {
		case common.DeployTypeInitial, common.DeployTypeInstall:
			return res.ResourceID, res.OnPreInstall() && res.OnPostInstall()
		case common.DeployTypeUpgrade:
			return res.ResourceID, res.OnPreUpgrade() && res.OnPostUpgrade()
		case common.DeployTypeRollback:
			return res.ResourceID, res.OnPreRollback() && res.OnPostRollback()
		}

		return res.ResourceID, false
	})

	curReleaseExistResourcesUIDs, _ := CurrentReleaseExistingResourcesUIDs(standaloneCRDsInfos, hookResourcesInfos, generalResourcesInfos)

	return &DeployPlanBuilder{
		ignoreLogs:                      opts.IgnoreLogs,
		taskStore:                       taskStore,
		logStore:                        logStore,
		deployType:                      deployType,
		plan:                            plan,
		releaseNamespace:                releaseNamespace,
		standaloneCRDsInfos:             standaloneCRDsInfos,
		preHookResourcesInfos:           preHookResourcesInfos,
		postHookResourcesInfos:          postHookResourcesInfos,
		prePostHookResourcesIDs:         prePostHookResourcesIDs,
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
	IgnoreLogs          bool
	PrevRelease         *release.Release
	PrevDeployedRelease *release.Release
	CreationTimeout     time.Duration
	ReadinessTimeout    time.Duration
	DeletionTimeout     time.Duration
}

type DeployPlanBuilder struct {
	ignoreLogs                      bool
	taskStore                       *statestore.TaskStore
	logStore                        *kdutil.Concurrent[*logstore.LogStore]
	releaseNamespace                string
	deployType                      common.DeployType
	standaloneCRDsInfos             []*info.DeployableStandaloneCRDInfo
	preHookResourcesInfos           []*info.DeployableHookResourceInfo
	postHookResourcesInfos          []*info.DeployableHookResourceInfo
	prePostHookResourcesIDs         []*resid.ResourceID
	generalResourcesInfos           []*info.DeployableGeneralResourceInfo
	prevReleaseGeneralResourceInfos []*info.DeployablePrevReleaseGeneralResourceInfo
	curReleaseExistingResourcesUIDs []types.UID
	newRelease                      *release.Release
	prevRelease                     *release.Release
	prevDeployedRelease             *release.Release
	history                         release.Historier
	kubeClient                      kube.KubeClienter
	staticClient                    kubernetes.Interface
	dynamicClient                   dynamic.Interface
	discoveryClient                 discovery.CachedDiscoveryInterface
	mapper                          meta.ResettableRESTMapper
	creationTimeout                 time.Duration
	readinessTimeout                time.Duration
	deletionTimeout                 time.Duration

	plan *Plan
}

func (b *DeployPlanBuilder) Build(ctx context.Context) (*Plan, error) {
	log.Default.Debug(ctx, "Setting up init operations")
	if err := b.setupInitOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up init operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up standalone CRDs operations")
	if err := b.setupStandaloneCRDsOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up standalone CRDs operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up pre hook resources operations")
	if err := b.setupPreHookResourcesOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up pre hooks operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up general resources operations")
	if err := b.setupGeneralResourcesOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up general resources operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up post hook resources operations")
	if err := b.setupPostHookResourcesOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up post hooks operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up prev release general resources operations")
	if err := b.setupPrevReleaseGeneralResourcesOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up prev release general resources operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up finalization operations")
	if err := b.setupFinalizationOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up finalization operations: %w", err)
	}

	log.Default.Debug(ctx, "Connecting stages")
	if err := b.connectStages(); err != nil {
		return b.plan, fmt.Errorf("error connecting stages: %w", err)
	}

	log.Default.Debug(ctx, "Connecting internal dependencies")
	if err := b.connectInternalDependencies(); err != nil {
		return b.plan, fmt.Errorf("error connecting internal dependencies: %w", err)
	}

	log.Default.Debug(ctx, "Optimizing plan")
	if err := b.plan.Optimize(); err != nil {
		return b.plan, fmt.Errorf("error optimizing plan: %w", err)
	}

	return b.plan, nil
}

func (b *DeployPlanBuilder) setupInitOperations() error {
	opCreatePendingRel := operation.NewCreatePendingReleaseOperation(b.newRelease, b.deployType, b.history)
	b.plan.AddStagedOperation(
		opCreatePendingRel,
		StageOpNamePrefixInit+"/"+StageOpNameSuffixStart,
		StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
	)

	return nil
}

func (b *DeployPlanBuilder) setupStandaloneCRDsOperations() error {
	for _, info := range b.standaloneCRDsInfos {
		create := info.ShouldCreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()

		var opDeploy operation.Operation
		if create {
			opDeploy = operation.NewCreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				operation.CreateResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
		} else if update {
			var err error
			opDeploy, err = operation.NewUpdateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				operation.UpdateResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
			if err != nil {
				return fmt.Errorf("error creating update resource operation: %w", err)
			}
		} else if apply {
			var err error
			opDeploy, err = operation.NewApplyResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				operation.ApplyResourceOperationOptions{
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
	weighedInfos := lo.GroupBy(b.preHookResourcesInfos, func(info *info.DeployableHookResourceInfo) int {
		return info.Resource().Weight()
	})

	weights := lo.Keys(weighedInfos)
	sort.Ints(weights)

	for _, weight := range weights {
		crdInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployableHookResourceInfo, _ int) bool {
			return util.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		crdsStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookCRDs, weight, StageOpNameSuffixStart)
		crdsStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookCRDs, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(crdInfos, crdsStageStartOpID, crdsStageEndOpID, true); err != nil {
			return fmt.Errorf("error setting up hook crds operations: %w", err)
		}

		resourceInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployableHookResourceInfo, _ int) bool {
			return !util.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		resourcesStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookResources, weight, StageOpNameSuffixStart)
		resourcesStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookResources, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(resourceInfos, resourcesStageStartOpID, resourcesStageEndOpID, true); err != nil {
			return fmt.Errorf("error setting up hook resources operations: %w", err)
		}
	}

	return nil
}

func (b *DeployPlanBuilder) setupPostHookResourcesOperations() error {
	weighedInfos := lo.GroupBy(b.postHookResourcesInfos, func(info *info.DeployableHookResourceInfo) int {
		return info.Resource().Weight()
	})

	weights := lo.Keys(weighedInfos)
	sort.Ints(weights)

	for _, weight := range weights {
		crdInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployableHookResourceInfo, _ int) bool {
			return util.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		crdsStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookCRDs, weight, StageOpNameSuffixStart)
		crdsStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookCRDs, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(crdInfos, crdsStageStartOpID, crdsStageEndOpID, false); err != nil {
			return fmt.Errorf("error setting up hook crds operations: %w", err)
		}

		resourceInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployableHookResourceInfo, _ int) bool {
			return !util.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		resourcesStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookResources, weight, StageOpNameSuffixStart)
		resourcesStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookResources, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(resourceInfos, resourcesStageStartOpID, resourcesStageEndOpID, false); err != nil {
			return fmt.Errorf("error setting up hook resources operations: %w", err)
		}
	}

	return nil
}

func (b *DeployPlanBuilder) setupGeneralResourcesOperations() error {
	weighedInfos := lo.GroupBy(b.generalResourcesInfos, func(info *info.DeployableGeneralResourceInfo) int {
		return info.Resource().Weight()
	})

	weights := lo.Keys(weighedInfos)
	sort.Ints(weights)

	for _, weight := range weights {
		crdInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployableGeneralResourceInfo, _ int) bool {
			return util.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		crdsStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixGeneralCRDs, weight, StageOpNameSuffixStart)
		crdsStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixGeneralCRDs, weight, StageOpNameSuffixEnd)

		if err := b.setupGeneralOperations(crdInfos, crdsStageStartOpID, crdsStageEndOpID); err != nil {
			return fmt.Errorf("error setting up general resources operations: %w", err)
		}

		resourceInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployableGeneralResourceInfo, _ int) bool {
			return !util.IsCRDFromGK(info.GroupVersionKind().GroupKind())
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
		delete := info.ShouldDelete(b.curReleaseExistingResourcesUIDs, b.newRelease.Name(), b.releaseNamespace, b.deployType)

		if delete {
			opDelete := operation.NewDeleteResourceOperation(
				info.ResourceID,
				b.kubeClient,
				operation.DeleteResourceOperationOptions{},
			)
			b.plan.AddInStagedOperation(
				opDelete,
				StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
			)

			taskState := kdutil.NewConcurrent(
				statestore.NewAbsenceTaskState(
					info.Name(),
					info.Namespace(),
					info.GroupVersionKind(),
					statestore.AbsenceTaskStateOptions{},
				),
			)
			b.taskStore.AddAbsenceTaskState(taskState)

			opTrackDeletion := operation.NewTrackResourceAbsenceOperation(
				info.ResourceID,
				taskState,
				b.dynamicClient,
				b.mapper,
				operation.TrackResourceAbsenceOperationOptions{
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
	opUpdateSucceededRel := operation.NewSucceedReleaseOperation(b.newRelease, b.history)
	b.plan.AddStagedOperation(
		opUpdateSucceededRel,
		StageOpNamePrefixFinal+"/"+StageOpNameSuffixStart,
		StageOpNamePrefixFinal+"/"+StageOpNameSuffixEnd,
	)

	if b.prevDeployedRelease != nil {
		opUpdateSupersededRel := operation.NewSupersedeReleaseOperation(b.prevDeployedRelease, b.history)
		b.plan.AddStagedOperation(
			opUpdateSupersededRel,
			StageOpNamePrefixFinal+"/"+StageOpNameSuffixStart,
			StageOpNamePrefixFinal+"/"+StageOpNameSuffixEnd,
		)
	}

	return nil
}

func (b *DeployPlanBuilder) connectInternalDependencies() error {
	hookInfos := lo.Union(
		b.preHookResourcesInfos,
		lo.Filter(
			b.postHookResourcesInfos,
			func(info *info.DeployableHookResourceInfo, _ int) bool {
				_, found := lo.Find(b.prePostHookResourcesIDs, func(rid *resid.ResourceID) bool {
					return rid.ID() == info.ResourceID.ID()
				})

				return !found
			},
		),
	)

	for _, info := range hookInfos {
		var opDeploy operation.Operation
		if info.ShouldCreate() {
			opDeploy = lo.Must(b.plan.Operation(operation.TypeCreateResourceOperation + "/" + info.ID()))
		} else if info.ShouldRecreate() {
			opDeploy = lo.Must(b.plan.Operation(operation.TypeRecreateResourceOperation + "/" + info.ID()))
		} else if info.ShouldUpdate() {
			opDeploy = lo.Must(b.plan.Operation(operation.TypeUpdateResourceOperation + "/" + info.ID()))
		} else if info.ShouldApply() {
			opDeploy = lo.Must(b.plan.Operation(operation.TypeApplyResourceOperation + "/" + info.ID()))
		} else {
			continue
		}

		autoInternalDeps, _ := info.Resource().AutoInternalDependencies()
		manualInternalDeps, _ := info.Resource().ManualInternalDependencies()

		for _, dep := range lo.Union(autoInternalDeps, manualInternalDeps) {
			var dependOnOpCandidateRegex *regexp.Regexp
			switch dep.ResourceState {
			case dependency.ResourceStatePresent:
				dependOnOpCandidateRegex = regexp.MustCompile(fmt.Sprintf(`^(%s|%s|%s|%s)/`, operation.TypeCreateResourceOperation, operation.TypeRecreateResourceOperation, operation.TypeUpdateResourceOperation, operation.TypeApplyResourceOperation))
			case dependency.ResourceStateReady:
				dependOnOpCandidateRegex = regexp.MustCompile(fmt.Sprintf(`^%s/`, operation.TypeTrackResourceReadinessOperation))
			default:
				panic(fmt.Sprintf("unexpected resource state %q", dep.ResourceState))
			}

			dependOnOpCandidates, found, err := b.plan.OperationsMatch(dependOnOpCandidateRegex)
			if err != nil {
				return fmt.Errorf("error looking for operations by regex: %w", err)
			} else if !found {
				continue
			}

			dependOnOp, found := lo.Find(dependOnOpCandidates, func(op operation.Operation) bool {
				_, id := lo.Must2(strings.Cut(op.ID(), "/"))

				resID := resid.NewResourceIDFromID(id, resid.ResourceIDOptions{
					DefaultNamespace: b.releaseNamespace,
					Mapper:           b.mapper,
				})

				return dep.Match(resID)
			})
			if !found {
				continue
			}

			if err := b.plan.AddDependency(dependOnOp.ID(), opDeploy.ID()); err != nil {
				return fmt.Errorf("error adding dependency: %w", err)
			}
		}
	}

	// TODO(ilya-lesikov): almost identical with hooks, refactor
	for _, info := range b.generalResourcesInfos {
		var opDeploy operation.Operation
		if info.ShouldCreate() {
			opDeploy = lo.Must(b.plan.Operation(operation.TypeCreateResourceOperation + "/" + info.ID()))
		} else if info.ShouldRecreate() {
			opDeploy = lo.Must(b.plan.Operation(operation.TypeRecreateResourceOperation + "/" + info.ID()))
		} else if info.ShouldUpdate() {
			opDeploy = lo.Must(b.plan.Operation(operation.TypeUpdateResourceOperation + "/" + info.ID()))
		} else if info.ShouldApply() {
			opDeploy = lo.Must(b.plan.Operation(operation.TypeApplyResourceOperation + "/" + info.ID()))
		} else {
			continue
		}

		autoInternalDeps, _ := info.Resource().AutoInternalDependencies()
		manualInternalDeps, _ := info.Resource().ManualInternalDependencies()

		for _, dep := range lo.Union(autoInternalDeps, manualInternalDeps) {
			var dependOnOpCandidateRegex *regexp.Regexp
			switch dep.ResourceState {
			case dependency.ResourceStatePresent:
				dependOnOpCandidateRegex = regexp.MustCompile(fmt.Sprintf(`^(%s|%s|%s|%s)/`, operation.TypeCreateResourceOperation, operation.TypeRecreateResourceOperation, operation.TypeUpdateResourceOperation, operation.TypeApplyResourceOperation))
			case dependency.ResourceStateReady:
				dependOnOpCandidateRegex = regexp.MustCompile(fmt.Sprintf(`^%s/`, operation.TypeTrackResourceReadinessOperation))
			default:
				panic(fmt.Sprintf("unexpected resource state %q", dep.ResourceState))
			}

			dependOnOpCandidates, found, err := b.plan.OperationsMatch(dependOnOpCandidateRegex)
			if err != nil {
				return fmt.Errorf("error looking for operations by regex: %w", err)
			} else if !found {
				continue
			}

			dependOnOp, found := lo.Find(dependOnOpCandidates, func(op operation.Operation) bool {
				_, id := lo.Must2(strings.Cut(op.ID(), "/"))

				resID := resid.NewResourceIDFromID(id, resid.ResourceIDOptions{
					DefaultNamespace: b.releaseNamespace,
					Mapper:           b.mapper,
				})

				return dep.Match(resID)
			})
			if !found {
				continue
			}

			if err := b.plan.AddDependency(dependOnOp.ID(), opDeploy.ID()); err != nil {
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

func (b *DeployPlanBuilder) setupHookOperations(infos []*info.DeployableHookResourceInfo, stageStartOpID, stageEndOpID string, pre bool) error {
	var prevReleaseFailed bool
	if b.prevRelease != nil {
		prevReleaseFailed = b.prevRelease.Failed()
	}

	for _, info := range infos {
		var extraPost bool
		if !pre {
			_, extraPost = lo.Find(b.prePostHookResourcesIDs, func(rid *resid.ResourceID) bool {
				return rid.ID() == info.ResourceID.ID()
			})
		}

		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup(b.newRelease.Name(), b.releaseNamespace)
		var trackReadiness bool
		if track := info.ShouldTrackReadiness(prevReleaseFailed); track && !extraPost {
			trackReadiness = true
		}
		_, manIntDepsSet := info.Resource().ManualInternalDependencies()
		var externalDeps []*dependency.ExternalDependency
		var extDepsSet bool
		if !extraPost {
			var err error
			externalDeps, extDepsSet, err = info.Resource().ExternalDependencies()
			if err != nil {
				return fmt.Errorf("error getting external dependencies: %w", err)
			}
		}
		var forceReplicas *int
		if r, set := info.Resource().DefaultReplicasOnCreation(); set {
			forceReplicas = &r
		}

		var opDeploy operation.Operation
		if create {
			opDeploy = operation.NewCreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				operation.CreateResourceOperationOptions{
					ManageableBy:  info.Resource().ManageableBy(),
					ForceReplicas: forceReplicas,
					ExtraPost:     extraPost,
				},
			)
		} else if recreate {
			absenceTaskState := kdutil.NewConcurrent(
				statestore.NewAbsenceTaskState(info.Name(), info.Namespace(), info.GroupVersionKind(), statestore.AbsenceTaskStateOptions{}),
			)
			b.taskStore.AddAbsenceTaskState(absenceTaskState)

			opDeploy = operation.NewRecreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				absenceTaskState,
				b.kubeClient,
				b.dynamicClient,
				b.mapper,
				operation.RecreateResourceOperationOptions{
					ManageableBy:         info.Resource().ManageableBy(),
					ForceReplicas:        forceReplicas,
					DeletionTrackTimeout: b.deletionTimeout,
					ExtraPost:            extraPost,
				},
			)
		} else if update {
			var err error
			opDeploy, err = operation.NewUpdateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				operation.UpdateResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
					ExtraPost:    extraPost,
				},
			)
			if err != nil {
				return fmt.Errorf("error creating update resource operation: %w", err)
			}
		} else if apply {
			var err error
			opDeploy, err = operation.NewApplyResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				operation.ApplyResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
					ExtraPost:    extraPost,
				},
			)
			if err != nil {
				return fmt.Errorf("error creating apply resource operation: %w", err)
			}
		}

		if opDeploy != nil {
			if manIntDepsSet {
				b.plan.AddStagedOperation(
					opDeploy,
					StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
					StageOpNamePrefixFinal+"/"+StageOpNameSuffixStart,
				)
			} else {
				b.plan.AddStagedOperation(
					opDeploy,
					stageStartOpID,
					stageEndOpID,
				)
			}
		}

		if extDepsSet && opDeploy != nil {
			for _, dep := range externalDeps {
				taskState, taskStateFound := lo.Find(b.taskStore.PresenceTasksStates(), func(ts *kdutil.Concurrent[*statestore.PresenceTaskState]) bool {
					var found bool

					ts.RTransaction(func(pts *statestore.PresenceTaskState) {
						if pts.Name() == dep.Name() &&
							pts.Namespace() == dep.Namespace() &&
							pts.GroupVersionKind() == dep.GroupVersionKind() {
							found = true
						}
					})

					return found
				})

				if !taskStateFound {
					taskState = kdutil.NewConcurrent(
						statestore.NewPresenceTaskState(
							dep.Name(),
							dep.Namespace(),
							dep.GroupVersionKind(),
							statestore.PresenceTaskStateOptions{},
						),
					)
					b.taskStore.AddPresenceTaskState(taskState)
				}

				opTrackReadiness := operation.NewTrackResourcePresenceOperation(
					dep.ResourceID,
					taskState,
					b.dynamicClient,
					b.mapper,
					operation.TrackResourcePresenceOperationOptions{
						Timeout: b.readinessTimeout,
					},
				)

				b.plan.AddInStagedOperation(
					opTrackReadiness,
					StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
				)

				lo.Must0(b.plan.AddDependency(opTrackReadiness.ID(), opDeploy.ID()))
			}
		}

		var opTrackReadiness *operation.TrackResourceReadinessOperation
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

			taskState := kdutil.NewConcurrent(
				statestore.NewReadinessTaskState(info.Name(), info.Namespace(), info.GroupVersionKind(), statestore.ReadinessTaskStateOptions{
					FailMode:                info.Resource().FailMode(),
					TotalAllowFailuresCount: info.Resource().FailuresAllowed(),
				}),
			)
			b.taskStore.AddReadinessTaskState(taskState)

			opTrackReadiness = operation.NewTrackResourceReadinessOperation(
				info.ResourceID,
				taskState,
				b.logStore,
				b.staticClient,
				b.dynamicClient,
				b.discoveryClient,
				b.mapper,
				operation.TrackResourceReadinessOperationOptions{
					Timeout:                                  b.readinessTimeout,
					NoActivityTimeout:                        noActivityTimeout,
					IgnoreReadinessProbeFailsByContainerName: ignoreReadinessProbes,
					SaveLogsOnlyForContainers:                showLogsOnlyFor,
					SaveLogsByRegex:                          logRegex,
					SaveLogsByRegexForContainers:             logRegexesFor,
					IgnoreLogs:                               lo.TernaryF(b.ignoreLogs, alwaysTrue, info.Resource().SkipLogs),
					IgnoreLogsForContainers:                  skipLogsFor,
					SaveEvents:                               info.Resource().ShowServiceMessages(),
				},
			)
			if manIntDepsSet {
				b.plan.AddStagedOperation(
					opTrackReadiness,
					StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
					StageOpNamePrefixFinal+"/"+StageOpNameSuffixStart,
				)
			} else {
				b.plan.AddStagedOperation(
					opTrackReadiness,
					stageStartOpID,
					stageEndOpID,
				)
			}
			if opDeploy != nil {
				lo.Must0(b.plan.AddDependency(opDeploy.ID(), opTrackReadiness.ID()))
			}
		}

		if cleanup {
			cleanupOp := operation.NewDeleteResourceOperation(
				info.ResourceID,
				b.kubeClient,
				operation.DeleteResourceOperationOptions{
					ExtraPost: extraPost,
				},
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

			taskState := kdutil.NewConcurrent(
				statestore.NewAbsenceTaskState(
					info.Name(),
					info.Namespace(),
					info.GroupVersionKind(),
					statestore.AbsenceTaskStateOptions{},
				),
			)
			b.taskStore.AddAbsenceTaskState(taskState)

			opTrackDeletion := operation.NewTrackResourceAbsenceOperation(
				info.ResourceID,
				taskState,
				b.dynamicClient,
				b.mapper,
				operation.TrackResourceAbsenceOperationOptions{
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
func (b *DeployPlanBuilder) setupGeneralOperations(infos []*info.DeployableGeneralResourceInfo, stageStartOpID, stageEndOpID string) error {
	var prevReleaseFailed bool
	if b.prevRelease != nil {
		prevReleaseFailed = b.prevRelease.Failed()
	}

	for _, info := range infos {
		create := info.ShouldCreate()
		recreate := info.ShouldRecreate()
		update := info.ShouldUpdate()
		apply := info.ShouldApply()
		cleanup := info.ShouldCleanup(b.newRelease.Name(), b.releaseNamespace)
		trackReadiness := info.ShouldTrackReadiness(prevReleaseFailed)
		_, manIntDepsSet := info.Resource().ManualInternalDependencies()
		externalDeps, extDepsSet, err := info.Resource().ExternalDependencies()
		if err != nil {
			return fmt.Errorf("error getting external dependencies: %w", err)
		}
		var forceReplicas *int
		if r, set := info.Resource().DefaultReplicasOnCreation(); set {
			forceReplicas = &r
		}

		var opDeploy operation.Operation
		if create {
			opDeploy = operation.NewCreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				operation.CreateResourceOperationOptions{
					ManageableBy:  info.Resource().ManageableBy(),
					ForceReplicas: forceReplicas,
				},
			)
		} else if recreate {
			absenceTaskState := kdutil.NewConcurrent(
				statestore.NewAbsenceTaskState(info.Name(), info.Namespace(), info.GroupVersionKind(), statestore.AbsenceTaskStateOptions{}),
			)
			b.taskStore.AddAbsenceTaskState(absenceTaskState)

			opDeploy = operation.NewRecreateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				absenceTaskState,
				b.kubeClient,
				b.dynamicClient,
				b.mapper,
				operation.RecreateResourceOperationOptions{
					ManageableBy:         info.Resource().ManageableBy(),
					ForceReplicas:        forceReplicas,
					DeletionTrackTimeout: b.deletionTimeout,
				},
			)
		} else if update {
			var err error
			opDeploy, err = operation.NewUpdateResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				operation.UpdateResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
			if err != nil {
				return fmt.Errorf("error creating update resource operation: %w", err)
			}
		} else if apply {
			var err error
			opDeploy, err = operation.NewApplyResourceOperation(
				info.ResourceID,
				info.Resource().Unstructured(),
				b.kubeClient,
				operation.ApplyResourceOperationOptions{
					ManageableBy: info.Resource().ManageableBy(),
				},
			)
			if err != nil {
				return fmt.Errorf("error creating apply resource operation: %w", err)
			}
		}

		if opDeploy != nil {
			if manIntDepsSet {
				b.plan.AddStagedOperation(
					opDeploy,
					StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
					StageOpNamePrefixFinal+"/"+StageOpNameSuffixStart,
				)
			} else {
				b.plan.AddStagedOperation(
					opDeploy,
					stageStartOpID,
					stageEndOpID,
				)
			}
		}

		if extDepsSet && opDeploy != nil {
			for _, dep := range externalDeps {
				taskState, taskStateFound := lo.Find(b.taskStore.PresenceTasksStates(), func(ts *kdutil.Concurrent[*statestore.PresenceTaskState]) bool {
					var found bool

					ts.RTransaction(func(pts *statestore.PresenceTaskState) {
						if pts.Name() == dep.Name() &&
							pts.Namespace() == dep.Namespace() &&
							pts.GroupVersionKind() == dep.GroupVersionKind() {
							found = true
						}
					})

					return found
				})

				if !taskStateFound {
					taskState = kdutil.NewConcurrent(
						statestore.NewPresenceTaskState(
							dep.Name(),
							dep.Namespace(),
							dep.GroupVersionKind(),
							statestore.PresenceTaskStateOptions{},
						),
					)
					b.taskStore.AddPresenceTaskState(taskState)
				}

				opTrackReadiness := operation.NewTrackResourcePresenceOperation(
					dep.ResourceID,
					taskState,
					b.dynamicClient,
					b.mapper,
					operation.TrackResourcePresenceOperationOptions{
						Timeout: b.readinessTimeout,
					},
				)

				b.plan.AddInStagedOperation(
					opTrackReadiness,
					StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
				)

				lo.Must0(b.plan.AddDependency(opTrackReadiness.ID(), opDeploy.ID()))
			}
		}

		var opTrackReadiness *operation.TrackResourceReadinessOperation
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

			taskState := kdutil.NewConcurrent(
				statestore.NewReadinessTaskState(info.Name(), info.Namespace(), info.GroupVersionKind(), statestore.ReadinessTaskStateOptions{
					FailMode:                info.Resource().FailMode(),
					TotalAllowFailuresCount: info.Resource().FailuresAllowed(),
				}),
			)
			b.taskStore.AddReadinessTaskState(taskState)

			opTrackReadiness = operation.NewTrackResourceReadinessOperation(
				info.ResourceID,
				taskState,
				b.logStore,
				b.staticClient,
				b.dynamicClient,
				b.discoveryClient,
				b.mapper,
				operation.TrackResourceReadinessOperationOptions{
					Timeout:                                  b.readinessTimeout,
					NoActivityTimeout:                        noActivityTimeout,
					IgnoreReadinessProbeFailsByContainerName: ignoreReadinessProbes,
					SaveLogsOnlyForContainers:                showLogsOnlyFor,
					SaveLogsByRegex:                          logRegex,
					SaveLogsByRegexForContainers:             logRegexesFor,
					IgnoreLogs:                               lo.TernaryF(b.ignoreLogs, alwaysTrue, info.Resource().SkipLogs),
					IgnoreLogsForContainers:                  skipLogsFor,
					SaveEvents:                               info.Resource().ShowServiceMessages(),
				},
			)
			if manIntDepsSet {
				b.plan.AddStagedOperation(
					opTrackReadiness,
					StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
					StageOpNamePrefixFinal+"/"+StageOpNameSuffixStart,
				)
			} else {
				b.plan.AddStagedOperation(
					opTrackReadiness,
					stageStartOpID,
					stageEndOpID,
				)
			}
			if opDeploy != nil {
				lo.Must0(b.plan.AddDependency(opDeploy.ID(), opTrackReadiness.ID()))
			}
		}

		if cleanup {
			cleanupOp := operation.NewDeleteResourceOperation(
				info.ResourceID,
				b.kubeClient,
				operation.DeleteResourceOperationOptions{},
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

			taskState := kdutil.NewConcurrent(
				statestore.NewAbsenceTaskState(
					info.Name(),
					info.Namespace(),
					info.GroupVersionKind(),
					statestore.AbsenceTaskStateOptions{},
				),
			)
			b.taskStore.AddAbsenceTaskState(taskState)

			opTrackDeletion := operation.NewTrackResourceAbsenceOperation(
				info.ResourceID,
				taskState,
				b.dynamicClient,
				b.mapper,
				operation.TrackResourceAbsenceOperationOptions{
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
