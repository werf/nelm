package plan

import (
	"fmt"
	"sort"

	"github.com/mitchellh/copystructure"
	"github.com/samber/lo"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
)

type BuildPlanOptions struct {
	NoFinalTracking bool
}

func BuildPlan(installableInfos []*InstallableResourceInfo, deletableInfos []*DeletableResourceInfo, releaseInfos []*ReleaseInfo, opts BuildPlanOptions) (*Plan, error) {
	plan := NewPlan()

	if err := addMainStages(plan); err != nil {
		return plan, fmt.Errorf("add main stages: %w", err)
	}

	if err := addWeightedSubStages(plan, installableInfos); err != nil {
		return plan, fmt.Errorf("add weighted substages: %w", err)
	}

	if err := addReleaseOperations(plan, releaseInfos); err != nil {
		return plan, fmt.Errorf("add release operations: %w", err)
	}

	if err := addDeleteResourcesOps(plan, deletableInfos); err != nil {
		return plan, fmt.Errorf("add delete resources operations: %w", err)
	}

	if err := addInstallResourceOps(plan, installableInfos); err != nil {
		return plan, fmt.Errorf("add install resource operations: %w", err)
	}

	if err := connectInternalDependencies(plan, installableInfos); err != nil {
		return plan, fmt.Errorf("connect internal dependencies: %w", err)
	}

	if err := plan.Optimize(opts.NoFinalTracking); err != nil {
		return plan, fmt.Errorf("optimize plan: %w", err)
	}

	return plan, nil
}

type BuildFailurePlanOptions struct {
	NoFinalTracking bool
}

func BuildFailurePlan(failedPlan *Plan, installableInfos []*InstallableResourceInfo, releaseInfos []*ReleaseInfo, opts BuildFailurePlanOptions) (*Plan, error) {
	plan := NewPlan()

	if err := addMainStages(plan); err != nil {
		return plan, fmt.Errorf("add main stages: %w", err)
	}

	if err := addFailureReleaseOperations(failedPlan, plan, releaseInfos); err != nil {
		return plan, fmt.Errorf("add failure release operations: %w", err)
	}

	if err := addFailureResourceOperations(failedPlan, plan, installableInfos); err != nil {
		return plan, fmt.Errorf("add failure resource operations: %w", err)
	}

	if err := plan.Optimize(opts.NoFinalTracking); err != nil {
		return plan, fmt.Errorf("optimize plan: %w", err)
	}

	return plan, nil
}

func addMainStages(plan *Plan) error {
	chain := plan.AddOperationChain()
	for _, stage := range common.StagesOrdered {
		startOp := &Operation{
			Type:     OperationTypeNoop,
			Version:  OperationVersionNoop,
			Category: OperationCategoryMeta,
			Config: &OperationConfigNoop{
				OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, stage, common.StageStartSuffix),
			},
		}
		chain.AddOperation(startOp)

		endOp := &Operation{
			Type:     OperationTypeNoop,
			Version:  OperationVersionNoop,
			Category: OperationCategoryMeta,
			Config: &OperationConfigNoop{
				OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, stage, common.StageEndSuffix),
			},
		}
		chain.AddOperation(endOp)
	}

	if err := chain.Do(); err != nil {
		return fmt.Errorf("do add chain operations: %w", err)
	}

	return nil
}

func addReleaseOperations(plan *Plan, releaseInfos []*ReleaseInfo) error {
	for _, info := range releaseInfos {
		switch info.Must {
		case ReleaseTypeInstall:
			if err := addPendingAndDeployedReleaseOps(plan, info, helmrelease.StatusPendingInstall); err != nil {
				return fmt.Errorf("add pending/deployed ops for release install: %w", err)
			}
		case ReleaseTypeUpgrade:
			if err := addPendingAndDeployedReleaseOps(plan, info, helmrelease.StatusPendingUpgrade); err != nil {
				return fmt.Errorf("add pending/deployed ops for release upgrade: %w", err)
			}
		case ReleaseTypeRollback:
			if err := addPendingAndDeployedReleaseOps(plan, info, helmrelease.StatusPendingRollback); err != nil {
				return fmt.Errorf("add pending/deployed ops for release rollback: %w", err)
			}
		case ReleaseTypeSupersede:
			if err := addSupersedeReleaseOps(plan, info); err != nil {
				return fmt.Errorf("add supersede ops for release: %w", err)
			}
		case ReleaseTypeUninstall:
			if err := addUninstallReleaseOps(plan, info); err != nil {
				return fmt.Errorf("add uninstall ops for release: %w", err)
			}
		case ReleaseTypeDelete:
			addDeleteReleaseOps(plan, info)
		case ReleaseTypeNone:
		default:
			panic("unexpected release must condition")
		}
	}

	return nil
}

func addFailureReleaseOperations(failedPlan, plan *Plan, releaseInfos []*ReleaseInfo) error {
	for _, info := range releaseInfos {
		if !info.MustFailOnFailedDeploy {
			continue
		}

		if _, releaseCreated := lo.Find(failedPlan.Operations(), func(op *Operation) bool {
			return (op.Type == OperationTypeCreateRelease ||
				op.Type == OperationTypeUpdateRelease) &&
				op.Status == OperationStatusCompleted &&
				op.Config.(*OperationConfigCreateRelease).Release.ID() == info.Release.ID()
		}); !releaseCreated {
			continue
		}

		if err := addFailedReleaseOps(plan, info); err != nil {
			return fmt.Errorf("add failed release ops for release: %w", err)
		}
	}

	return nil
}

func addPendingAndDeployedReleaseOps(plan *Plan, info *ReleaseInfo, pendingStatus helmrelease.Status) error {
	var pendingRel *helmrelease.Release
	if rel, err := copystructure.Copy(info.Release); err != nil {
		return fmt.Errorf("deep copy release: %w", err)
	} else {
		pendingRel = rel.(*helmrelease.Release)
	}

	pendingRel.Info.Status = pendingStatus

	pendingOp := &Operation{
		Type:     OperationTypeCreateRelease,
		Version:  OperationVersionCreateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigCreateRelease{
			Release: pendingRel,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(pendingOp).Stage(common.StageInit).Do())

	var succeededRel *helmrelease.Release
	if rel, err := copystructure.Copy(pendingRel); err != nil {
		return fmt.Errorf("deep copy release: %w", err)
	} else {
		succeededRel = rel.(*helmrelease.Release)
	}

	succeededRel.Info.Status = helmrelease.StatusDeployed

	succeededOp := &Operation{
		Type:     OperationTypeUpdateRelease,
		Version:  OperationVersionUpdateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigUpdateRelease{
			Release: succeededRel,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(succeededOp).Stage(common.StageFinal).Do())

	return nil
}

func addFailedReleaseOps(plan *Plan, info *ReleaseInfo) error {
	var failedRel *helmrelease.Release
	if rel, err := copystructure.Copy(info.Release); err != nil {
		return fmt.Errorf("deep copy release: %w", err)
	} else {
		failedRel = rel.(*helmrelease.Release)
	}

	failedRel.Info.Status = helmrelease.StatusFailed

	failedOp := &Operation{
		Type:     OperationTypeUpdateRelease,
		Version:  OperationVersionUpdateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigUpdateRelease{
			Release: failedRel,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(failedOp).Stage(common.StageInit).Do())

	return nil
}

func addSupersedeReleaseOps(plan *Plan, info *ReleaseInfo) error {
	var supersededRel *helmrelease.Release
	if rel, err := copystructure.Copy(info.Release); err != nil {
		return fmt.Errorf("deep copy release: %w", err)
	} else {
		supersededRel = rel.(*helmrelease.Release)
	}

	supersededRel.Info.Status = helmrelease.StatusSuperseded

	supersedeOp := &Operation{
		Type:     OperationTypeUpdateRelease,
		Version:  OperationVersionUpdateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigUpdateRelease{
			Release: supersededRel,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(supersedeOp).Stage(common.StageFinal).Do())

	return nil
}

func addUninstallReleaseOps(plan *Plan, info *ReleaseInfo) error {
	var uninstallingRel *helmrelease.Release
	if rel, err := copystructure.Copy(info.Release); err != nil {
		return fmt.Errorf("deep copy release: %w", err)
	} else {
		uninstallingRel = rel.(*helmrelease.Release)
	}

	uninstallingRel.Info.Status = helmrelease.StatusUninstalling

	uninstallingOp := &Operation{
		Type:     OperationTypeUpdateRelease,
		Version:  OperationVersionUpdateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigUpdateRelease{
			Release: uninstallingRel,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(uninstallingOp).Stage(common.StageInit).Do())

	uninstalledOp := &Operation{
		Type:     OperationTypeDeleteRelease,
		Version:  OperationVersionDeleteRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigDeleteRelease{
			ReleaseName:      uninstallingRel.Name,
			ReleaseNamespace: uninstallingRel.Namespace,
			ReleaseRevision:  uninstallingRel.Version,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(uninstalledOp).Stage(common.StageFinal).Do())

	return nil
}

func addDeleteReleaseOps(plan *Plan, info *ReleaseInfo) {
	deletedOp := &Operation{
		Type:     OperationTypeDeleteRelease,
		Version:  OperationVersionDeleteRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigDeleteRelease{
			ReleaseName:      info.Release.Name,
			ReleaseNamespace: info.Release.Namespace,
			ReleaseRevision:  info.Release.Version,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(deletedOp).Stage(common.StageFinal).Do())
}

func addDeleteResourcesOps(plan *Plan, infos []*DeletableResourceInfo) error {
	for _, info := range infos {
		if info.MustDelete {
			chain := plan.AddOperationChain()

			deleteOp := &Operation{
				Type:     OperationTypeDelete,
				Version:  OperationVersionDelete,
				Category: OperationCategoryResource,
				Config: &OperationConfigDelete{
					ResourceMeta: info.ResourceMeta,
				},
			}

			chain.AddOperation(deleteOp).Stage(info.Stage)

			if info.MustTrackAbsence {
				trackOp := &Operation{
					Type:     OperationTypeTrackAbsence,
					Version:  OperationVersionTrackAbsence,
					Category: OperationCategoryTrack,
					Config: &OperationConfigTrackAbsence{
						ResourceMeta: info.ResourceMeta,
					},
				}
				chain.AddOperation(trackOp).Stage(info.Stage)
			}

			if err := chain.Do(); err != nil {
				return fmt.Errorf("do add chain operations: %w", err)
			}
		}
	}

	return nil
}

func addWeightedSubStages(plan *Plan, infos []*InstallableResourceInfo) error {
	stageWeights := map[common.Stage][]int{}
	for _, info := range infos {
		if info.LocalResource.Weight == nil {
			continue
		}

		if _, found := stageWeights[info.Stage]; !found {
			stageWeights[info.Stage] = []int{}
		}

		stageWeights[info.Stage] = append(stageWeights[info.Stage], *info.LocalResource.Weight)
	}

	for stage := range stageWeights {
		sort.Ints(stageWeights[stage])
		stageWeights[stage] = lo.Uniq(stageWeights[stage])
	}

	for stage, weights := range stageWeights {
		chain := plan.AddOperationChain()

		for _, weight := range weights {
			weightedSubStage := common.SubStageWeighted(stage, weight)

			startOp := &Operation{
				Type:     OperationTypeNoop,
				Version:  OperationVersionNoop,
				Category: OperationCategoryMeta,
				Config: &OperationConfigNoop{
					OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, weightedSubStage, common.StageStartSuffix),
				},
			}
			chain.AddOperation(startOp).Stage(stage)

			endOp := &Operation{
				Type:     OperationTypeNoop,
				Version:  OperationVersionNoop,
				Category: OperationCategoryMeta,
				Config: &OperationConfigNoop{
					OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, weightedSubStage, common.StageEndSuffix),
				},
			}
			chain.AddOperation(endOp).Stage(stage)
		}

		if err := chain.Do(); err != nil {
			return fmt.Errorf("do add chain operations: %w", err)
		}
	}

	return nil
}

func addInstallResourceOps(plan *Plan, infos []*InstallableResourceInfo) error {
	for _, info := range infos {
		chain := plan.AddOperationChain()

		var stg common.Stage
		if info.LocalResource.Weight != nil {
			stg = common.SubStageWeighted(info.Stage, *info.LocalResource.Weight)
		} else {
			stg = info.Stage
		}

		if info.MustInstall != ResourceInstallTypeNone {
			for _, extDep := range info.LocalResource.ExternalDependencies {
				trackOp := &Operation{
					Type:     OperationTypeTrackPresence,
					Version:  OperationVersionTrackPresence,
					Category: OperationCategoryTrack,
					Config: &OperationConfigTrackPresence{
						ResourceMeta: extDep.ResourceMeta,
					},
				}
				chain.AddOperation(trackOp).Stage(stg).SkipOnDuplicate()
			}
		}

		switch info.MustInstall {
		case ResourceInstallTypeCreate:
			createOp := &Operation{
				Type:      OperationTypeCreate,
				Version:   OperationVersionCreate,
				Category:  OperationCategoryResource,
				Iteration: OperationIteration(info.Iteration),
				Config: &OperationConfigCreate{
					ResourceSpec:  info.LocalResource.ResourceSpec,
					ForceReplicas: info.LocalResource.DefaultReplicasOnCreation,
				},
			}
			chain.AddOperation(createOp).Stage(stg)
		case ResourceInstallTypeRecreate:
			recreateOp := &Operation{
				Type:      OperationTypeRecreate,
				Version:   OperationVersionRecreate,
				Category:  OperationCategoryResource,
				Iteration: OperationIteration(info.Iteration),
				Config: &OperationConfigRecreate{
					ResourceSpec:  info.LocalResource.ResourceSpec,
					ForceReplicas: info.LocalResource.DefaultReplicasOnCreation,
				},
			}
			chain.AddOperation(recreateOp).Stage(stg)
		case ResourceInstallTypeUpdate:
			updateOp := &Operation{
				Type:      OperationTypeUpdate,
				Version:   OperationVersionUpdate,
				Category:  OperationCategoryResource,
				Iteration: OperationIteration(info.Iteration),
				Config: &OperationConfigUpdate{
					ResourceSpec: info.LocalResource.ResourceSpec,
				},
			}
			chain.AddOperation(updateOp).Stage(stg)
		case ResourceInstallTypeApply:
			applyOp := &Operation{
				Type:      OperationTypeApply,
				Version:   OperationVersionApply,
				Category:  OperationCategoryResource,
				Iteration: OperationIteration(info.Iteration),
				Config: &OperationConfigApply{
					ResourceSpec: info.LocalResource.ResourceSpec,
				},
			}
			chain.AddOperation(applyOp).Stage(stg)
		case ResourceInstallTypeNone:
		default:
			panic("unexpected resource must condition")
		}

		if info.MustTrackReadiness {
			trackOp := &Operation{
				Type:      OperationTypeTrackReadiness,
				Version:   OperationVersionTrackReadiness,
				Category:  OperationCategoryTrack,
				Iteration: OperationIteration(info.Iteration),
				Config: &OperationConfigTrackReadiness{
					ResourceMeta:                             info.ResourceMeta,
					FailMode:                                 info.LocalResource.FailMode,
					FailuresAllowed:                          info.LocalResource.FailuresAllowed,
					IgnoreLogs:                               info.LocalResource.SkipLogs,
					IgnoreLogsForContainers:                  info.LocalResource.SkipLogsForContainers,
					IgnoreReadinessProbeFailsByContainerName: info.LocalResource.IgnoreReadinessProbeFailsForContainers,
					NoActivityTimeout:                        info.LocalResource.NoActivityTimeout,
					SaveEvents:                               info.LocalResource.ShowServiceMessages,
					SaveLogsByRegex:                          info.LocalResource.LogRegex,
					SaveLogsByRegexForContainers:             info.LocalResource.LogRegexesForContainers,
					SaveLogsOnlyForContainers:                info.LocalResource.ShowLogsOnlyForContainers,
					SaveLogsOnlyForNumberOfReplicas:          info.LocalResource.ShowLogsOnlyForNumberOfReplicas,
				},
			}
			chain.AddOperation(trackOp).Stage(stg)
		}

		if info.MustDeleteOnSuccessfulInstall {
			deleteOp := &Operation{
				Type:      OperationTypeDelete,
				Version:   OperationVersionDelete,
				Category:  OperationCategoryResource,
				Iteration: OperationIteration(info.Iteration),
				Config: &OperationConfigDelete{
					ResourceMeta: info.ResourceMeta,
				},
			}
			chain.AddOperation(deleteOp).Stage(stg)

			opTrack := &Operation{
				Type:      OperationTypeTrackAbsence,
				Version:   OperationVersionTrackAbsence,
				Category:  OperationCategoryTrack,
				Iteration: OperationIteration(info.Iteration),
				Config: &OperationConfigTrackAbsence{
					ResourceMeta: info.ResourceMeta,
				},
			}
			chain.AddOperation(opTrack).Stage(stg)
		}

		if err := chain.Do(); err != nil {
			return fmt.Errorf("do add chain operations: %w", err)
		}
	}

	return nil
}

func addFailureResourceOperations(failedPlan, plan *Plan, infos []*InstallableResourceInfo) error {
	for _, info := range infos {
		if !info.MustDeleteOnFailedInstall {
			continue
		}

		trackReadinessOp := lo.Must(failedPlan.Operation(OperationID(OperationTypeTrackReadiness, OperationVersionTrackReadiness, OperationIteration(info.Iteration), info.ID())))

		if trackReadinessOp.Status != OperationStatusFailed {
			continue
		}

		if info.MustDeleteOnSuccessfulInstall {
			deleteOnSuccessfulInstallOp := lo.Must(lo.Find(failedPlan.Operations(), func(op *Operation) bool {
				return op.Type == OperationTypeDelete &&
					op.Iteration == OperationIteration(info.Iteration) &&
					op.Config.(*OperationConfigDelete).ResourceMeta.ID() == info.ID()
			}))

			if deleteOnSuccessfulInstallOp.Status == OperationStatusCompleted {
				continue
			}
		}

		chain := plan.AddOperationChain()

		deleteOp := &Operation{
			Type:      OperationTypeDelete,
			Version:   OperationVersionDelete,
			Category:  OperationCategoryResource,
			Iteration: OperationIteration(info.Iteration),
			Config: &OperationConfigDelete{
				ResourceMeta: info.ResourceMeta,
			},
		}
		chain.AddOperation(deleteOp).Stage(common.StageUninstall)

		trackAbsenceOp := &Operation{
			Type:      OperationTypeTrackAbsence,
			Version:   OperationVersionTrackAbsence,
			Category:  OperationCategoryTrack,
			Iteration: OperationIteration(info.Iteration),
			Config: &OperationConfigTrackAbsence{
				ResourceMeta: info.ResourceMeta,
			},
		}
		chain.AddOperation(trackAbsenceOp).Stage(common.StageUninstall)

		if err := chain.Do(); err != nil {
			return fmt.Errorf("do add chain operations: %w", err)
		}
	}

	return nil
}

func connectInternalDependencies(plan *Plan, infos []*InstallableResourceInfo) error {
	for _, info := range infos {
		internalDeps := lo.Union(info.LocalResource.AutoInternalDependencies, info.LocalResource.ManualInternalDependencies)
		if len(internalDeps) == 0 {
			continue
		}

		deployOp, found := getDeployOp(plan, info, info.Iteration)
		if !found {
			continue
		}

		for _, dep := range internalDeps {
			var (
				dependUponOp      *Operation
				dependUponOpFound bool
			)

			switch dep.ResourceState {
			case common.ResourceStatePresent:
				deployOps := getAllFirstIterationDeployOps(plan)

				dependUponOp, dependUponOpFound = lo.Find(deployOps, func(op *Operation) bool {
					return dep.Match(getOpMeta(op))
				})
			case common.ResourceStateReady:
				trackOps := getAllFirstIterationTrackReadinessOps(plan)

				dependUponOp, dependUponOpFound = lo.Find(trackOps, func(op *Operation) bool {
					return dep.Match(getOpMeta(op))
				})
			default:
				panic("unexpected internal dependency resource state")
			}

			if !dependUponOpFound {
				continue
			}

			if err := plan.Connect(dependUponOp.ID(), deployOp.ID()); err != nil {
				return fmt.Errorf("depend %q from %q: %w", deployOp.ID(), dependUponOp.ID(), err)
			}
		}
	}

	return nil
}

func getDeployOp(plan *Plan, info *InstallableResourceInfo, iteration int) (op *Operation, found bool) {
	var deployOpID string
	switch info.MustInstall {
	case ResourceInstallTypeCreate:
		deployOpID = OperationID(OperationTypeCreate, OperationVersionCreate, OperationIteration(iteration), info.ID())
	case ResourceInstallTypeRecreate:
		deployOpID = OperationID(OperationTypeRecreate, OperationVersionRecreate, OperationIteration(iteration), info.ID())
	case ResourceInstallTypeUpdate:
		deployOpID = OperationID(OperationTypeUpdate, OperationVersionUpdate, OperationIteration(iteration), info.ID())
	case ResourceInstallTypeApply:
		deployOpID = OperationID(OperationTypeApply, OperationVersionApply, OperationIteration(iteration), info.ID())
	case ResourceInstallTypeNone:
		return nil, false
	default:
		panic("unexpected resource must condition")
	}

	return lo.Must(plan.Operation(deployOpID)), true
}

func getAllFirstIterationDeployOps(plan *Plan) []*Operation {
	var deployOps []*Operation
	for _, op := range plan.Operations() {
		if op.Iteration != 0 {
			continue
		}

		switch op.Type {
		case OperationTypeCreate,
			OperationTypeRecreate,
			OperationTypeUpdate,
			OperationTypeApply:
			deployOps = append(deployOps, op)
		default:
			continue
		}
	}

	return deployOps
}

func getAllFirstIterationTrackReadinessOps(plan *Plan) []*Operation {
	var trackOps []*Operation
	for _, op := range plan.Operations() {
		if op.Iteration != 0 {
			continue
		}

		switch op.Type {
		case OperationTypeTrackReadiness:
			trackOps = append(trackOps, op)
		default:
			continue
		}
	}

	return trackOps
}

func getOpMeta(op *Operation) *spec.ResourceMeta {
	switch cfg := op.Config.(type) {
	case *OperationConfigCreate:
		return cfg.ResourceSpec.ResourceMeta
	case *OperationConfigRecreate:
		return cfg.ResourceSpec.ResourceMeta
	case *OperationConfigUpdate:
		return cfg.ResourceSpec.ResourceMeta
	case *OperationConfigApply:
		return cfg.ResourceSpec.ResourceMeta
	case *OperationConfigDelete:
		return cfg.ResourceMeta
	case *OperationConfigTrackReadiness:
		return cfg.ResourceMeta
	case *OperationConfigTrackPresence:
		return cfg.ResourceMeta
	case *OperationConfigTrackAbsence:
		return cfg.ResourceMeta
	default:
		panic("unexpected op config")
	}
}
