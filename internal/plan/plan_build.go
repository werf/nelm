package plan

import (
	"fmt"
	"sort"

	"github.com/mitchellh/copystructure"
	"github.com/samber/lo"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/pkg/common"
)

type BuildPlanOptions struct {
	NoFinalTracking bool
}

// Builds any kind of a plan, be it for install, upgrade, rollback or uninstall. The only exception
// is a failure plan (see BuildFailurePlan), because it's way too different. Any differences between
// different kinds of plans must be figured out earlier, e.g. at BuildResourceInfos level. This
// generic design must be preserved. Keep it simple: if something can be done on earlier stages, do
// it there.
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

	if err := connectInternalDeployDependencies(plan, installableInfos, deletableInfos); err != nil {
		return plan, fmt.Errorf("connect internal dependencies: %w", err)
	}

	if err := connectInternalDeleteDependencies(plan, deletableInfos, installableInfos); err != nil {
		return plan, fmt.Errorf("connect internal delete dependencies: %w", err)
	}

	if err := plan.Optimize(opts.NoFinalTracking); err != nil {
		return plan, fmt.Errorf("optimize plan: %w", err)
	}

	return plan, nil
}

type BuildFailurePlanOptions struct {
	NoFinalTracking bool
}

// When the main plan fails, the failure plan must be built and executed.
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
			if op.Status != OperationStatusCompleted {
				return false
			}

			switch config := op.Config.(type) {
			case *OperationConfigCreateRelease:
				return config.Release.ID() == info.Release.ID()
			case *OperationConfigUpdateRelease:
				return config.Release.ID() == info.Release.ID()
			default:
				return false
			}
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
					ResourceMeta:      info.ResourceMeta,
					DeletePropagation: info.LocalResource.DeletePropagation,
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
					ResourceSpec:      info.LocalResource.ResourceSpec,
					ForceReplicas:     info.LocalResource.DefaultReplicasOnCreation,
					DeletePropagation: info.LocalResource.DeletePropagation,
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
					IgnoreLogsByRegex:                        info.LocalResource.SkipLogsRegex,
					IgnoreLogsByRegexForContainers:           info.LocalResource.SkipLogsRegexForContainers,
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
					ResourceMeta:      info.ResourceMeta,
					DeletePropagation: info.LocalResource.DeletePropagation,
				},
			}
			chain.AddOperation(deleteOp).Stage(info.StageDeleteOnSuccessfulInstall)

			opTrack := &Operation{
				Type:      OperationTypeTrackAbsence,
				Version:   OperationVersionTrackAbsence,
				Category:  OperationCategoryTrack,
				Iteration: OperationIteration(info.Iteration),
				Config: &OperationConfigTrackAbsence{
					ResourceMeta: info.ResourceMeta,
				},
			}
			chain.AddOperation(opTrack).Stage(info.StageDeleteOnSuccessfulInstall)
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
				ResourceMeta:      info.ResourceMeta,
				DeletePropagation: info.LocalResource.DeletePropagation,
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

func connectInternalDeployDependencies(plan *Plan, instInfos []*InstallableResourceInfo, delInfos []*DeletableResourceInfo) error {
	for _, info := range instInfos {
		internalDeps := lo.Union(info.LocalResource.AutoInternalDependencies, info.LocalResource.ManualInternalDependencies)
		if len(internalDeps) == 0 {
			continue
		}

		deployOp, found := getDeployOp(plan, info)
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
				dependUponOp, dependUponOpFound = findDeployOpInStage(plan, instInfos, dep, info.Stage)
			case common.ResourceStateReady:
				dependUponOp, dependUponOpFound = findTrackReadinessOpInStage(plan, instInfos, dep, info.Stage)
			case common.ResourceStateAbsent:
				// TODO(v2): all deploy/delete dependencies must depend upon all matched operations, not a single one
				dependUponOps := findTrackAbsenceOpInStage(plan, delInfos, instInfos, dep, info.Stage)
				if len(dependUponOps) > 0 {
					dependUponOp = dependUponOps[0]
					dependUponOpFound = true
				}
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

func getDeployOp(plan *Plan, info *InstallableResourceInfo) (op *Operation, found bool) {
	var deployOpID string
	switch info.MustInstall {
	case ResourceInstallTypeCreate:
		deployOpID = OperationID(OperationTypeCreate, OperationVersionCreate, OperationIteration(info.Iteration), info.ID())
	case ResourceInstallTypeRecreate:
		deployOpID = OperationID(OperationTypeRecreate, OperationVersionRecreate, OperationIteration(info.Iteration), info.ID())
	case ResourceInstallTypeUpdate:
		deployOpID = OperationID(OperationTypeUpdate, OperationVersionUpdate, OperationIteration(info.Iteration), info.ID())
	case ResourceInstallTypeApply:
		deployOpID = OperationID(OperationTypeApply, OperationVersionApply, OperationIteration(info.Iteration), info.ID())
	case ResourceInstallTypeNone:
		return nil, false
	default:
		panic("unexpected resource must condition")
	}

	return lo.Must(plan.Operation(deployOpID)), true
}

func connectInternalDeleteDependencies(plan *Plan, delInfos []*DeletableResourceInfo, instInfos []*InstallableResourceInfo) error {
	for _, info := range delInfos {
		internalDeps := lo.Union(info.LocalResource.AutoInternalDependencies, info.LocalResource.ManualInternalDependencies)
		if len(internalDeps) == 0 {
			continue
		}

		deleteOp, found := getDeleteOp(plan, info)
		if !found {
			continue
		}

		for _, dep := range internalDeps {
			var dependUponOps []*Operation

			switch dep.ResourceState {
			case common.ResourceStateAbsent:
				dependUponOps = findTrackAbsenceOpInStage(plan, delInfos, instInfos, dep, info.Stage)
			default:
				panic("unexpected internal dependency resource state")
			}

			for _, dependUponOp := range dependUponOps {
				if err := plan.Connect(dependUponOp.ID(), deleteOp.ID()); err != nil {
					return fmt.Errorf("depend %q from %q: %w", deleteOp.ID(), dependUponOp.ID(), err)
				}
			}
		}
	}

	return nil
}

func getDeleteOp(plan *Plan, info *DeletableResourceInfo) (*Operation, bool) {
	if !info.MustDelete {
		return nil, false
	}

	operationID := OperationID(OperationTypeDelete, OperationVersionDelete, OperationIteration(0), info.ID())

	return lo.Must(plan.Operation(operationID)), true
}

func findDeployOpInStage(plan *Plan, instInfos []*InstallableResourceInfo, dep *resource.InternalDependency, sourceStage common.Stage) (*Operation, bool) {
	var match *InstallableResourceInfo
	for _, candidate := range instInfos {
		if candidate.MustInstall == ResourceInstallTypeNone ||
			candidate.Stage != sourceStage ||
			!dep.Match(candidate.ResourceMeta) ||
			(match != nil && candidate.Iteration >= match.Iteration) {
			continue
		}

		match = candidate
	}

	if match == nil {
		return nil, false
	}

	return getDeployOp(plan, match)
}

func findTrackReadinessOpInStage(plan *Plan, instInfos []*InstallableResourceInfo, dep *resource.InternalDependency, sourceStage common.Stage) (*Operation, bool) {
	var match *InstallableResourceInfo
	for _, candidate := range instInfos {
		if !candidate.MustTrackReadiness ||
			candidate.Stage != sourceStage ||
			!dep.Match(candidate.ResourceMeta) ||
			(match != nil && candidate.Iteration >= match.Iteration) {
			continue
		}

		match = candidate
	}

	if match == nil {
		return nil, false
	}

	opID := OperationID(OperationTypeTrackReadiness, OperationVersionTrackReadiness, OperationIteration(match.Iteration), match.ID())

	return plan.Operation(opID)
}

func findTrackAbsenceOpInStage(plan *Plan, delInfos []*DeletableResourceInfo, instInfos []*InstallableResourceInfo, dep *resource.InternalDependency, sourceStage common.Stage) []*Operation {
	var foundOps []*Operation
	for _, candidate := range delInfos {
		if !candidate.MustTrackAbsence ||
			candidate.Stage != sourceStage ||
			!dep.Match(candidate.ResourceMeta) {
			continue
		}

		opID := OperationID(OperationTypeTrackAbsence, OperationVersionTrackAbsence, 0, candidate.ID())

		if op, found := plan.Operation(opID); found {
			foundOps = append(foundOps, op)
		}
	}

	if len(foundOps) > 0 {
		return foundOps
	}

	var match *InstallableResourceInfo
	for _, candidate := range instInfos {
		if !candidate.MustDeleteOnSuccessfulInstall ||
			candidate.StageDeleteOnSuccessfulInstall != sourceStage ||
			!dep.Match(candidate.ResourceMeta) ||
			(match != nil && candidate.Iteration >= match.Iteration) {
			continue
		}

		match = candidate
	}

	if match == nil {
		return nil
	}

	opID := OperationID(OperationTypeTrackAbsence, OperationVersionTrackAbsence, OperationIteration(match.Iteration), match.ID())

	op, found := plan.Operation(opID)
	if !found {
		return nil
	}

	return []*Operation{op}
}
