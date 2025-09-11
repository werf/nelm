package plan

import (
	"fmt"
	"sort"

	"github.com/mitchellh/copystructure"
	"github.com/samber/lo"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/plan/resinfo"
	"github.com/werf/nelm/internal/resource/meta"
)

func BuildPlan(installableInfos []*resinfo.InstallableResourceInfo, deletableInfos []*resinfo.DeletableResourceInfo, releaseInfos []*resinfo.ReleaseInfo) (*Plan, error) {
	plan := NewPlan()

	if err := addMainStages(plan); err != nil {
		return nil, fmt.Errorf("add main stages: %w", err)
	}

	if err := addWeightedSubStages(plan, installableInfos); err != nil {
		return nil, fmt.Errorf("add weighted substages: %w", err)
	}

	if err := addReleaseOperations(plan, releaseInfos); err != nil {
		return nil, fmt.Errorf("add release operations: %w", err)
	}

	if err := addDeleteResourcesOps(plan, deletableInfos); err != nil {
		return nil, fmt.Errorf("add delete resources operations: %w", err)
	}

	if err := addInstallResourceOps(plan, installableInfos); err != nil {
		return nil, fmt.Errorf("add install resource operations: %w", err)
	}

	if err := connectInternalDependencies(plan, installableInfos); err != nil {
		return nil, fmt.Errorf("connect internal dependencies: %w", err)
	}

	if err := plan.Optimize(); err != nil {
		return nil, fmt.Errorf("optimize plan: %w", err)
	}

	return &Plan{}, nil
}

func addMainStages(plan *Plan) error {
	chain := plan.AddOperationChain()
	for _, stage := range common.StagesOrdered {
		startOp := &operation.Operation{
			Type:    operation.OperationTypeNoop,
			Version: operation.OperationVersionNoop,
			Config: &operation.OperationConfigNoop{
				OpID: fmt.Sprintf("%s/%s", stage, common.StageStartSuffix),
			},
		}
		chain.AddOperation(startOp)

		endOp := &operation.Operation{
			Type:    operation.OperationTypeNoop,
			Version: operation.OperationVersionNoop,
			Config: &operation.OperationConfigNoop{
				OpID: fmt.Sprintf("%s/%s", stage, common.StageEndSuffix),
			},
		}
		chain.AddOperation(endOp)
	}

	if err := chain.Do(); err != nil {
		return fmt.Errorf("do add chain operations: %w", err)
	}

	return nil
}

func addReleaseOperations(plan *Plan, releaseInfos []*resinfo.ReleaseInfo) error {
	for _, info := range releaseInfos {
		switch info.Must {
		case resinfo.ReleaseTypeInstall:
			if err := addPendingAndDeployedReleaseOps(plan, info, helmrelease.StatusPendingInstall); err != nil {
				return fmt.Errorf("add pending/deployed ops for release install: %w", err)
			}
		case resinfo.ReleaseTypeUpgrade:
			if err := addPendingAndDeployedReleaseOps(plan, info, helmrelease.StatusPendingUpgrade); err != nil {
				return fmt.Errorf("add pending/deployed ops for release upgrade: %w", err)
			}
		case resinfo.ReleaseTypeRollback:
			if err := addPendingAndDeployedReleaseOps(plan, info, helmrelease.StatusPendingRollback); err != nil {
				return fmt.Errorf("add pending/deployed ops for release rollback: %w", err)
			}
		case resinfo.ReleaseTypeSupersede:
			if err := addSupersedeReleaseOps(plan, info); err != nil {
				return fmt.Errorf("add supersede ops for release: %w", err)
			}
		case resinfo.ReleaseTypeUninstall:
			if err := addUninstallReleaseOps(plan, info); err != nil {
				return fmt.Errorf("add uninstall ops for release: %w", err)
			}
		case resinfo.ReleaseTypeDelete:
			addDeleteReleaseOps(plan, info)
		case resinfo.ReleaseTypeNone:
		default:
			panic("unexpected release must condition")
		}
	}

	return nil
}

func addPendingAndDeployedReleaseOps(plan *Plan, info *resinfo.ReleaseInfo, pendingStatus helmrelease.Status) error {
	var pendingRel *helmrelease.Release
	if rel, err := copystructure.Copy(info.Release); err != nil {
		return fmt.Errorf("deep copy release: %w", err)
	} else {
		pendingRel = rel.(*helmrelease.Release)
	}

	pendingRel.Info.Status = pendingStatus

	pendingOp := &operation.Operation{
		Type:    operation.OperationTypeCreateRelease,
		Version: operation.OperationVersionCreateRelease,
		Config: &operation.OperationConfigCreateRelease{
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

	succeededOp := &operation.Operation{
		Type:    operation.OperationTypeUpdateRelease,
		Version: operation.OperationVersionUpdateRelease,
		Config: &operation.OperationConfigUpdateRelease{
			Release: succeededRel,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(succeededOp).Stage(common.StageFinal).Do())

	return nil
}

func addSupersedeReleaseOps(plan *Plan, info *resinfo.ReleaseInfo) error {
	var supersededRel *helmrelease.Release
	if rel, err := copystructure.Copy(info.Release); err != nil {
		return fmt.Errorf("deep copy release: %w", err)
	} else {
		supersededRel = rel.(*helmrelease.Release)
	}

	supersededRel.Info.Status = helmrelease.StatusSuperseded

	supersedeOp := &operation.Operation{
		Type:    operation.OperationTypeUpdateRelease,
		Version: operation.OperationVersionUpdateRelease,
		Config: &operation.OperationConfigUpdateRelease{
			Release: supersededRel,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(supersedeOp).Stage(common.StageFinal).Do())

	return nil
}

func addUninstallReleaseOps(plan *Plan, info *resinfo.ReleaseInfo) error {
	var uninstallingRel *helmrelease.Release
	if rel, err := copystructure.Copy(info.Release); err != nil {
		return fmt.Errorf("deep copy release: %w", err)
	} else {
		uninstallingRel = rel.(*helmrelease.Release)
	}

	uninstallingRel.Info.Status = helmrelease.StatusUninstalling

	uninstallingOp := &operation.Operation{
		Type:    operation.OperationTypeUpdateRelease,
		Version: operation.OperationVersionUpdateRelease,
		Config: &operation.OperationConfigUpdateRelease{
			Release: uninstallingRel,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(uninstallingOp).Stage(common.StageInit).Do())

	uninstalledOp := &operation.Operation{
		Type:    operation.OperationTypeDeleteRelease,
		Version: operation.OperationVersionDeleteRelease,
		Config: &operation.OperationConfigDeleteRelease{
			ReleaseName:      uninstallingRel.Name,
			ReleaseNamespace: uninstallingRel.Namespace,
			ReleaseRevision:  uninstallingRel.Version,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(uninstalledOp).Stage(common.StageFinal).Do())

	return nil
}

func addDeleteReleaseOps(plan *Plan, info *resinfo.ReleaseInfo) {
	deletedOp := &operation.Operation{
		Type:    operation.OperationTypeDeleteRelease,
		Version: operation.OperationVersionDeleteRelease,
		Config: &operation.OperationConfigDeleteRelease{
			ReleaseName:      info.Release.Name,
			ReleaseNamespace: info.Release.Namespace,
			ReleaseRevision:  info.Release.Version,
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(deletedOp).Stage(common.StageFinal).Do())
}

func addDeleteResourcesOps(plan *Plan, infos []*resinfo.DeletableResourceInfo) error {
	for _, info := range infos {
		if info.MustDelete {
			chain := plan.AddOperationChain()

			deleteOp := &operation.Operation{
				Type:    operation.OperationTypeDelete,
				Version: operation.OperationVersionDelete,
				Config: &operation.OperationConfigDelete{
					ResourceMeta: info.ResourceMeta,
				},
			}

			chain.AddOperation(deleteOp).Stage(info.LocalResource.Stage)

			if info.MustTrackAbsence {
				trackOp := &operation.Operation{
					Type:    operation.OperationTypeTrackAbsence,
					Version: operation.OperationVersionTrackAbsence,
					Config: &operation.OperationConfigTrackAbsence{
						ResourceMeta: info.ResourceMeta,
					},
				}
				chain.AddOperation(trackOp).Stage(info.LocalResource.Stage)
			}

			if err := chain.Do(); err != nil {
				return fmt.Errorf("do add chain operations: %w", err)
			}
		}
	}

	return nil
}

func addWeightedSubStages(plan *Plan, infos []*resinfo.InstallableResourceInfo) error {
	stageWeights := map[common.Stage][]int{}
	for _, info := range infos {
		if info.LocalResource.Weight == nil {
			continue
		}

		if _, found := stageWeights[info.LocalResource.Stage]; !found {
			stageWeights[info.LocalResource.Stage] = []int{}
		}

		stageWeights[info.LocalResource.Stage] = append(stageWeights[info.LocalResource.Stage], *info.LocalResource.Weight)
	}

	for stage := range stageWeights {
		sort.Ints(stageWeights[stage])
		stageWeights[stage] = lo.Uniq(stageWeights[stage])
	}

	for stage, weights := range stageWeights {
		chain := plan.AddOperationChain()

		for _, weight := range weights {
			weightedSubStage := common.SubStageWeighted(stage, weight)

			startOp := &operation.Operation{
				Type:    operation.OperationTypeNoop,
				Version: operation.OperationVersionNoop,
				Config: &operation.OperationConfigNoop{
					OpID: fmt.Sprintf("%s/%d", weightedSubStage, common.StageStartSuffix),
				},
			}
			chain.AddOperation(startOp).Stage(stage)

			endOp := &operation.Operation{
				Type:    operation.OperationTypeNoop,
				Version: operation.OperationVersionNoop,
				Config: &operation.OperationConfigNoop{
					OpID: fmt.Sprintf("%s/%s", weightedSubStage, common.StageEndSuffix),
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

func addInstallResourceOps(plan *Plan, infos []*resinfo.InstallableResourceInfo) error {
	for _, info := range infos {
		chain := plan.AddOperationChain()

		var stg common.Stage
		if info.LocalResource.Weight != nil {
			stg = common.SubStageWeighted(info.LocalResource.Stage, *info.LocalResource.Weight)
		} else {
			stg = info.LocalResource.Stage
		}

		if info.MustInstall != resinfo.ResourceInstallTypeNone {
			for _, extDep := range info.LocalResource.ExternalDependencies {
				trackOp := &operation.Operation{
					Type:    operation.OperationTypeTrackPresence,
					Version: operation.OperationVersionTrackPresence,
					Config: &operation.OperationConfigTrackPresence{
						ResourceMeta: extDep.ResourceMeta,
					},
				}
				chain.AddOperation(trackOp).Stage(stg).SkipOnDuplicate()
			}
		}

		switch info.MustInstall {
		case resinfo.ResourceInstallTypeCreate:
			createOp := &operation.Operation{
				Type:      operation.OperationTypeCreate,
				Version:   operation.OperationVersionCreate,
				Iteration: operation.OperationIteration(info.Iteration),
				Config: &operation.OperationConfigCreate{
					ResourceSpec:  info.LocalResource.ResourceSpec,
					ForceReplicas: info.LocalResource.DefaultReplicasOnCreation,
				},
			}
			chain.AddOperation(createOp).Stage(stg)
		case resinfo.ResourceInstallTypeRecreate:
			recreateOp := &operation.Operation{
				Type:      operation.OperationTypeRecreate,
				Version:   operation.OperationVersionRecreate,
				Iteration: operation.OperationIteration(info.Iteration),
				Config: &operation.OperationConfigRecreate{
					ResourceSpec:  info.LocalResource.ResourceSpec,
					ForceReplicas: info.LocalResource.DefaultReplicasOnCreation,
				},
			}
			chain.AddOperation(recreateOp).Stage(stg)
		case resinfo.ResourceInstallTypeUpdate:
			updateOp := &operation.Operation{
				Type:      operation.OperationTypeUpdate,
				Version:   operation.OperationVersionUpdate,
				Iteration: operation.OperationIteration(info.Iteration),
				Config: &operation.OperationConfigUpdate{
					ResourceSpec: info.LocalResource.ResourceSpec,
				},
			}
			chain.AddOperation(updateOp).Stage(stg)
		case resinfo.ResourceInstallTypeApply:
			applyOp := &operation.Operation{
				Type:      operation.OperationTypeApply,
				Version:   operation.OperationVersionApply,
				Iteration: operation.OperationIteration(info.Iteration),
				Config: &operation.OperationConfigApply{
					ResourceSpec: info.LocalResource.ResourceSpec,
				},
			}
			chain.AddOperation(applyOp).Stage(stg)
		case resinfo.ResourceInstallTypeNone:
		default:
			panic("unexpected resource must condition")
		}

		if info.MustTrackReadiness {
			trackOp := &operation.Operation{
				Type:      operation.OperationTypeTrackReadiness,
				Version:   operation.OperationVersionTrackReadiness,
				Iteration: operation.OperationIteration(info.Iteration),
				Config: &operation.OperationConfigTrackReadiness{
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
			deleteOp := &operation.Operation{
				Type:      operation.OperationTypeDelete,
				Version:   operation.OperationVersionDelete,
				Iteration: operation.OperationIteration(info.Iteration),
				Config: &operation.OperationConfigDelete{
					ResourceMeta: info.ResourceMeta,
				},
			}
			chain.AddOperation(deleteOp).Stage(stg)

			opTrack := &operation.Operation{
				Type:      operation.OperationTypeTrackAbsence,
				Version:   operation.OperationVersionTrackAbsence,
				Iteration: operation.OperationIteration(info.Iteration),
				Config: &operation.OperationConfigTrackAbsence{
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

func connectInternalDependencies(plan *Plan, infos []*resinfo.InstallableResourceInfo) error {
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
			var dependUponOp *operation.Operation
			var dependUponOpFound bool
			switch dep.ResourceState {
			case common.ResourceStatePresent:
				deployOps := getAllFirstIterationDeployOps(plan)

				dependUponOp, dependUponOpFound = lo.Find(deployOps, func(op *operation.Operation) bool {
					return dep.ResourceMatcher.Match(getOpMeta(op))
				})
			case common.ResourceStateReady:
				trackOps := getAllFirstIterationTrackReadinessOps(plan)

				dependUponOp, dependUponOpFound = lo.Find(trackOps, func(op *operation.Operation) bool {
					return dep.ResourceMatcher.Match(getOpMeta(op))
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

func getDeployOp(plan *Plan, info *resinfo.InstallableResourceInfo, iteration int) (op *operation.Operation, found bool) {
	var deployOpID string
	switch info.MustInstall {
	case resinfo.ResourceInstallTypeCreate:
		deployOpID = operation.OperationID(operation.OperationTypeCreate, operation.OperationVersionCreate, operation.OperationIteration(iteration), info.ResourceMeta.ID())
	case resinfo.ResourceInstallTypeRecreate:
		deployOpID = operation.OperationID(operation.OperationTypeRecreate, operation.OperationVersionRecreate, operation.OperationIteration(iteration), info.ResourceMeta.ID())
	case resinfo.ResourceInstallTypeUpdate:
		deployOpID = operation.OperationID(operation.OperationTypeUpdate, operation.OperationVersionUpdate, operation.OperationIteration(iteration), info.ResourceMeta.ID())
	case resinfo.ResourceInstallTypeApply:
		deployOpID = operation.OperationID(operation.OperationTypeApply, operation.OperationVersionApply, operation.OperationIteration(iteration), info.ResourceMeta.ID())
	case resinfo.ResourceInstallTypeNone:
		return nil, false
	default:
		panic("unexpected resource must condition")
	}

	return lo.Must(plan.Operation(deployOpID)), false
}

func getAllFirstIterationDeployOps(plan *Plan) []*operation.Operation {
	var deployOps []*operation.Operation
	for _, op := range plan.Operations() {
		if op.Iteration != 0 {
			continue
		}

		switch op.Type {
		case operation.OperationTypeCreate,
			operation.OperationTypeRecreate,
			operation.OperationTypeUpdate,
			operation.OperationTypeApply:
			deployOps = append(deployOps, op)
		default:
			continue
		}
	}

	return deployOps
}

func getAllFirstIterationTrackReadinessOps(plan *Plan) []*operation.Operation {
	var trackOps []*operation.Operation
	for _, op := range plan.Operations() {
		if op.Iteration != 0 {
			continue
		}

		switch op.Type {
		case operation.OperationTypeTrackReadiness:
			trackOps = append(trackOps, op)
		default:
			continue
		}
	}

	return trackOps
}

func getOpMeta(op *operation.Operation) *meta.ResourceMeta {
	switch cfg := op.Config.(type) {
	case *operation.OperationConfigCreate:
		return cfg.ResourceSpec.ResourceMeta
	case *operation.OperationConfigRecreate:
		return cfg.ResourceSpec.ResourceMeta
	case *operation.OperationConfigUpdate:
		return cfg.ResourceSpec.ResourceMeta
	case *operation.OperationConfigApply:
		return cfg.ResourceSpec.ResourceMeta
	case *operation.OperationConfigDelete:
		return cfg.ResourceMeta
	case *operation.OperationConfigTrackReadiness:
		return cfg.ResourceMeta
	case *operation.OperationConfigTrackPresence:
		return cfg.ResourceMeta
	case *operation.OperationConfigTrackAbsence:
		return cfg.ResourceMeta
	default:
		panic("unexpected op config")
	}
}
