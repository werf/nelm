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

func NewUninstallPlanBuilder(
	releaseName string,
	releaseNamespace string,
	taskStore *statestore.TaskStore,
	logStore *kdutil.Concurrent[*logstore.LogStore],
	prevReleaseHookResourceInfos []*info.DeployablePrevReleaseHookResourceInfo,
	prevReleaseGeneralResourceInfos []*info.DeployablePrevReleaseGeneralResourceInfo,
	prevRelease *release.Release,
	history release.Historier,
	kubeClient kube.KubeClienter,
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper meta.ResettableRESTMapper,
	opts UninstallPlanBuilderOptions,
) *UninstallPlanBuilder {
	plan := NewPlan()

	preHookResourcesInfos := lo.Filter(prevReleaseHookResourceInfos, func(info *info.DeployablePrevReleaseHookResourceInfo, _ int) bool {
		return info.Resource().OnPreDelete()
	})

	postHookResourcesInfos := lo.Filter(prevReleaseHookResourceInfos, func(info *info.DeployablePrevReleaseHookResourceInfo, _ int) bool {
		return info.Resource().OnPostDelete()
	})

	prePostHookResourcesIDs := lo.FilterMap(prevReleaseHookResourceInfos, func(info *info.DeployablePrevReleaseHookResourceInfo, _ int) (*resid.ResourceID, bool) {
		res := info.Resource()
		return res.ResourceID, res.OnPreDelete() && res.OnPostDelete()
	})

	return &UninstallPlanBuilder{
		taskStore:                       taskStore,
		logStore:                        logStore,
		plan:                            plan,
		releaseName:                     releaseName,
		releaseNamespace:                releaseNamespace,
		preHookResourcesInfos:           preHookResourcesInfos,
		postHookResourcesInfos:          postHookResourcesInfos,
		prePostHookResourcesIDs:         prePostHookResourcesIDs,
		prevReleaseGeneralResourceInfos: prevReleaseGeneralResourceInfos,
		prevRelease:                     prevRelease,
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

type UninstallPlanBuilderOptions struct {
	CreationTimeout  time.Duration
	DeletionTimeout  time.Duration
	ReadinessTimeout time.Duration
}

type UninstallPlanBuilder struct {
	taskStore                       *statestore.TaskStore
	logStore                        *kdutil.Concurrent[*logstore.LogStore]
	releaseName                     string
	releaseNamespace                string
	preHookResourcesInfos           []*info.DeployablePrevReleaseHookResourceInfo
	postHookResourcesInfos          []*info.DeployablePrevReleaseHookResourceInfo
	prePostHookResourcesIDs         []*resid.ResourceID
	prevReleaseGeneralResourceInfos []*info.DeployablePrevReleaseGeneralResourceInfo
	prevRelease                     *release.Release
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

func (b *UninstallPlanBuilder) Build(ctx context.Context) (*Plan, error) {
	log.Default.Debug(ctx, "Setting up init operations")
	if err := b.setupInitOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up init operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up pre hook resources operations")
	if err := b.setupPreHookResourcesOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up pre hooks operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up prev release general resources operations")
	if err := b.setupPrevReleaseGeneralResourcesOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up prev release general resources operations: %w", err)
	}

	log.Default.Debug(ctx, "Setting up post hook resources operations")
	if err := b.setupPostHookResourcesOperations(); err != nil {
		return b.plan, fmt.Errorf("error setting up post hooks operations: %w", err)
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

func (b *UninstallPlanBuilder) setupInitOperations() error {
	opPendingUninstallRel := operation.NewPendingUninstallReleaseOperation(b.prevRelease, b.history)
	b.plan.AddStagedOperation(
		opPendingUninstallRel,
		StageOpNamePrefixInit+"/"+StageOpNameSuffixStart,
		StageOpNamePrefixInit+"/"+StageOpNameSuffixEnd,
	)

	return nil
}

func (b *UninstallPlanBuilder) setupPreHookResourcesOperations() error {
	weighedInfos := lo.GroupBy(b.preHookResourcesInfos, func(info *info.DeployablePrevReleaseHookResourceInfo) int {
		return info.Resource().Weight()
	})

	weights := lo.Keys(weighedInfos)
	sort.Ints(weights)

	for _, weight := range weights {
		crdInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployablePrevReleaseHookResourceInfo, _ int) bool {
			return util.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		crdsStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookCRDs, weight, StageOpNameSuffixStart)
		crdsStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixHookCRDs, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(crdInfos, crdsStageStartOpID, crdsStageEndOpID, true); err != nil {
			return fmt.Errorf("error setting up hook crds operations: %w", err)
		}

		resourceInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployablePrevReleaseHookResourceInfo, _ int) bool {
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

func (b *UninstallPlanBuilder) setupPostHookResourcesOperations() error {
	weighedInfos := lo.GroupBy(b.postHookResourcesInfos, func(info *info.DeployablePrevReleaseHookResourceInfo) int {
		return info.Resource().Weight()
	})

	weights := lo.Keys(weighedInfos)
	sort.Ints(weights)

	for _, weight := range weights {
		crdInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployablePrevReleaseHookResourceInfo, _ int) bool {
			return util.IsCRDFromGK(info.GroupVersionKind().GroupKind())
		})
		crdsStageStartOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookCRDs, weight, StageOpNameSuffixStart)
		crdsStageEndOpID := fmt.Sprintf("%s/weight:%d/%s", StageOpNamePrefixPostHookCRDs, weight, StageOpNameSuffixEnd)

		if err := b.setupHookOperations(crdInfos, crdsStageStartOpID, crdsStageEndOpID, false); err != nil {
			return fmt.Errorf("error setting up hook crds operations: %w", err)
		}

		resourceInfos := lo.Filter(weighedInfos[weight], func(info *info.DeployablePrevReleaseHookResourceInfo, _ int) bool {
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

func (b *UninstallPlanBuilder) setupHookOperations(infos []*info.DeployablePrevReleaseHookResourceInfo, stageStartOpID, stageEndOpID string, pre bool) error {
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
		cleanup := info.ShouldCleanup(b.releaseName, b.releaseNamespace)
		var trackReadiness bool
		if track := info.ShouldTrackReadiness(); track && !extraPost {
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
					IgnoreLogs:                               info.Resource().SkipLogs(),
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

func (b *UninstallPlanBuilder) setupPrevReleaseGeneralResourcesOperations() error {
	for _, info := range b.prevReleaseGeneralResourceInfos {
		delete := info.ShouldDelete(nil, b.releaseName, b.releaseNamespace, common.DeployTypeUninstall)

		if delete {
			stageStartOpID := fmt.Sprintf("%s/weight:0/%s", StageOpNamePrefixGeneralResources, StageOpNameSuffixStart)
			stageEndOpID := fmt.Sprintf("%s/weight:0/%s", StageOpNamePrefixGeneralResources, StageOpNameSuffixEnd)

			opDelete := operation.NewDeleteResourceOperation(
				info.ResourceID,
				b.kubeClient,
				operation.DeleteResourceOperationOptions{},
			)
			b.plan.AddStagedOperation(
				opDelete,
				stageStartOpID,
				stageEndOpID,
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
			b.plan.AddStagedOperation(
				opTrackDeletion,
				stageStartOpID,
				stageEndOpID,
			)
			if err := b.plan.AddDependency(opDelete.ID(), opTrackDeletion.ID()); err != nil {
				return fmt.Errorf("error adding dependency: %w", err)
			}
		}
	}

	return nil
}

func (b *UninstallPlanBuilder) setupFinalizationOperations() error {
	b.prevRelease.Uninstall()

	releases, err := b.history.Releases()
	if err != nil {
		return fmt.Errorf("error getting releases from history: %w", err)
	}

	for _, rel := range releases {
		opDeleteRel := operation.NewDeleteReleaseOperation(rel, b.history)
		b.plan.AddStagedOperation(
			opDeleteRel,
			StageOpNamePrefixFinal+"/"+StageOpNameSuffixStart,
			StageOpNamePrefixFinal+"/"+StageOpNameSuffixEnd,
		)
	}

	return nil
}

func (b *UninstallPlanBuilder) connectStages() error {
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

func (b *UninstallPlanBuilder) connectInternalDependencies() error {
	hookInfos := lo.Union(
		b.preHookResourcesInfos,
		lo.Filter(
			b.postHookResourcesInfos,
			func(info *info.DeployablePrevReleaseHookResourceInfo, _ int) bool {
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

	return nil
}
