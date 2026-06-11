package plan

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/samber/lo"

	"github.com/werf/nelm/pkg/common"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/release"
	"github.com/werf/nelm/pkg/resource"
	"github.com/werf/nelm/pkg/resource/spec"
)

type BuildPlanOptions struct {
	NoFinalTracking bool
}

type BuildFailurePlanOptions struct {
	NoFinalTracking bool
}

type matchedDeletableResource struct {
	*spec.ResourceMeta

	Iteration int
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

// Builds any kind of a plan, be it for install, upgrade, rollback or uninstall. The only exception
// is a failure plan (see BuildFailurePlan), because it's way too different. Any differences between
// different kinds of plans must be figured out earlier, e.g. at BuildResourceInfos level. This
// generic design must be preserved. Keep it simple: if something can be done on earlier stages, do
// it there.
func BuildPlan(ctx context.Context, installableInfos []*InstallableResourceInfo, deletableInfos []*DeletableResourceInfo, releaseInfos []*ReleaseInfo, releaseNamespace string, opts BuildPlanOptions) (*Plan, error) {
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

	if err := connectInternalDeployDependencies(ctx, plan, installableInfos, deletableInfos, releaseNamespace); err != nil {
		return plan, fmt.Errorf("connect internal dependencies: %w", err)
	}

	if err := connectInternalDeleteDependencies(ctx, plan, deletableInfos, installableInfos, releaseNamespace); err != nil {
		return plan, fmt.Errorf("connect internal delete dependencies: %w", err)
	}

	if err := plan.Optimize(opts.NoFinalTracking); err != nil {
		return plan, fmt.Errorf("optimize plan: %w", err)
	}

	return plan, nil
}

func connectInternalDeleteDependencies(ctx context.Context, plan *Plan, delInfos []*DeletableResourceInfo, instInfos []*InstallableResourceInfo, releaseNamespace string) error {
	for _, info := range delInfos {
		internalDeps := lo.Union(info.LocalResource.AutoInternalDependencies, info.LocalResource.ManualDependencies)
		if len(internalDeps) == 0 {
			continue
		}

		deleteOp, found := getDeleteOp(plan, info)
		if !found {
			continue
		}

		for _, dep := range internalDeps {
			var (
				dependUponOps []*Operation
				err           error
			)

			switch dep.ResourceState {
			case common.ResourceStateAbsent:
				dependUponOps, err = resolveTrackAbsenceOpInStage(ctx, plan, delInfos, instInfos, dep, info.Stage, releaseNamespace)
			default:
				panic("unexpected internal dependency resource state")
			}

			if err != nil {
				return fmt.Errorf("find internal dependency ops: %w", err)
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

func connectInternalDeployDependencies(ctx context.Context, plan *Plan, instInfos []*InstallableResourceInfo, delInfos []*DeletableResourceInfo, releaseNamespace string) error {
	for _, info := range instInfos {
		internalDeps := lo.Union(info.LocalResource.AutoInternalDependencies, info.LocalResource.ManualDependencies)
		if len(internalDeps) == 0 {
			continue
		}

		deployOp, found := getDeployOp(plan, info)
		if !found {
			continue
		}

		for _, dep := range internalDeps {
			var (
				dependUponOps []*Operation
				err           error
			)

			switch dep.ResourceState {
			case common.ResourceStatePresent:
				dependUponOps, err = resolveDeployOpInStage(ctx, plan, instInfos, dep, info.Stage, releaseNamespace)
			case common.ResourceStateReady:
				dependUponOps, err = resolveTrackReadinessOpInStage(ctx, plan, instInfos, dep, info.Stage, releaseNamespace)
			case common.ResourceStateAbsent:
				dependUponOps, err = resolveTrackAbsenceOpInStage(ctx, plan, delInfos, instInfos, dep, info.Stage, releaseNamespace)
			default:
				panic("unexpected internal dependency resource state")
			}

			if err != nil {
				return fmt.Errorf("find internal dependency ops: %w", err)
			}

			for _, dependUponOp := range dependUponOps {
				if err := plan.Connect(dependUponOp.ID(), deployOp.ID()); err != nil {
					return fmt.Errorf("depend %q from %q: %w", deployOp.ID(), dependUponOp.ID(), err)
				}
			}
		}
	}

	return nil
}

func addFailureReleaseOperations(failedPlan, plan *Plan, releaseInfos []*ReleaseInfo) error {
	for _, info := range releaseInfos {
		if !info.MustFailOnFailedDeploy {
			continue
		}

		infoAcc := lo.Must(helmrel.NewAccessor(info.Release.Releaser))

		if _, releaseCreated := lo.Find(failedPlan.Operations(), func(op *Operation) bool {
			if op.Status != OperationStatusCompleted {
				return false
			}

			switch config := op.Config.(type) {
			case *OperationConfigCreateRelease:
				configAcc := lo.Must(helmrel.NewAccessor(config.Release.Releaser))

				return configAcc.Namespace() == infoAcc.Namespace() &&
					configAcc.Name() == infoAcc.Name() &&
					configAcc.Version() == infoAcc.Version()
			case *OperationConfigUpdateRelease:
				configAcc := lo.Must(helmrel.NewAccessor(config.Release.Releaser))

				return configAcc.Namespace() == infoAcc.Namespace() &&
					configAcc.Name() == infoAcc.Name() &&
					configAcc.Version() == infoAcc.Version()
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

func addReleaseOperations(plan *Plan, releaseInfos []*ReleaseInfo) error {
	for _, info := range releaseInfos {
		switch info.Must {
		case ReleaseTypeInstall:
			if err := addPendingAndDeployedReleaseOps(plan, info, helmreleasecommon.StatusPendingInstall); err != nil {
				return fmt.Errorf("add pending/deployed ops for release install: %w", err)
			}
		case ReleaseTypeUpgrade:
			if err := addPendingAndDeployedReleaseOps(plan, info, helmreleasecommon.StatusPendingUpgrade); err != nil {
				return fmt.Errorf("add pending/deployed ops for release upgrade: %w", err)
			}
		case ReleaseTypeRollback:
			if err := addPendingAndDeployedReleaseOps(plan, info, helmreleasecommon.StatusPendingRollback); err != nil {
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

func resolveDeployOpInStage(ctx context.Context, plan *Plan, instInfos []*InstallableResourceInfo, dep *resource.Dependency, sourceStage common.Stage, releaseNamespace string) ([]*Operation, error) {
	if dep.External {
		resMeta := resource.NewResourceMetaFromDependency(dep, releaseNamespace)

		opID := OperationID(OperationTypeTrackPresence, OperationVersionTrackPresence, OperationIteration(0), resMeta.ID())
		if op, found := plan.Operation(opID); found {
			return []*Operation{op}, nil
		}

		trackOp := &Operation{
			Type:     OperationTypeTrackPresence,
			Version:  OperationVersionTrackPresence,
			Category: OperationCategoryTrack,
			Config: &OperationConfigTrackPresence{
				ResourceMeta: resMeta,
			},
		}

		if err := plan.AddOperationChain().AddOperation(trackOp).Stage(sourceStage).SkipOnDuplicate().Do(); err != nil {
			return nil, fmt.Errorf("add track presence operation: %w", err)
		}

		return []*Operation{trackOp}, nil
	}

	matchByID := make(map[string]*InstallableResourceInfo)
	for _, candidate := range instInfos {
		match, found := matchByID[candidate.ID()]
		if candidate.Stage != sourceStage ||
			!dep.Match(candidate.ResourceMeta) ||
			(found && candidate.Iteration >= match.Iteration) {
			continue
		}

		matchByID[candidate.ID()] = candidate
	}

	matched, err := truncateInstallableMatches(ctx, lo.Values(matchByID), dep)
	if err != nil {
		return nil, err
	}

	if len(matched) == 0 {
		return nil, nil
	}

	var foundOps []*Operation
	for _, match := range matched {
		if op, found := getDeployOp(plan, match); found {
			foundOps = append(foundOps, op)
		} else {
			trackOp := &Operation{
				Type:     OperationTypeTrackPresence,
				Version:  OperationVersionTrackPresence,
				Category: OperationCategoryTrack,
				Config: &OperationConfigTrackPresence{
					ResourceMeta: match.ResourceMeta,
				},
			}

			if err := plan.AddOperationChain().AddOperation(trackOp).Stage(sourceStage).SkipOnDuplicate().Do(); err != nil {
				return nil, fmt.Errorf("add track presence operation: %w", err)
			}

			foundOps = append(foundOps, trackOp)
		}
	}

	return foundOps, nil
}

func resolveTrackAbsenceOpInStage(ctx context.Context, plan *Plan, delInfos []*DeletableResourceInfo, instInfos []*InstallableResourceInfo, dep *resource.Dependency, sourceStage common.Stage, releaseNamespace string) ([]*Operation, error) {
	if dep.External {
		resMeta := resource.NewResourceMetaFromDependency(dep, releaseNamespace)

		opID := OperationID(OperationTypeTrackAbsence, OperationVersionTrackAbsence, OperationIteration(0), resMeta.ID())
		if op, found := plan.Operation(opID); found {
			return []*Operation{op}, nil
		}

		trackOp := &Operation{
			Type:     OperationTypeTrackAbsence,
			Version:  OperationVersionTrackAbsence,
			Category: OperationCategoryTrack,
			Config: &OperationConfigTrackAbsence{
				ResourceMeta: resMeta,
			},
		}

		if err := plan.AddOperationChain().AddOperation(trackOp).Stage(sourceStage).SkipOnDuplicate().Do(); err != nil {
			return nil, fmt.Errorf("add track absence operation: %w", err)
		}

		return []*Operation{trackOp}, nil
	}

	matchByID := make(map[string]*matchedDeletableResource)
	for _, candidate := range delInfos {
		if candidate.Stage != sourceStage ||
			!dep.Match(candidate.ResourceMeta) {
			continue
		}

		matchByID[candidate.ID()] = &matchedDeletableResource{
			ResourceMeta: candidate.ResourceMeta,
			Iteration:    0,
		}
	}

	for _, candidate := range instInfos {
		match, found := matchByID[candidate.ID()]
		if !candidate.MustDeleteOnSuccessfulInstall ||
			candidate.StageDeleteOnSuccessfulInstall != sourceStage ||
			!dep.Match(candidate.ResourceMeta) ||
			(found && candidate.Iteration >= match.Iteration) {
			continue
		}

		matchByID[candidate.ID()] = &matchedDeletableResource{
			ResourceMeta: candidate.ResourceMeta,
			Iteration:    candidate.Iteration,
		}
	}

	matched, err := truncateDeletableMatches(ctx, lo.Values(matchByID), dep)
	if err != nil {
		return nil, err
	}

	if len(matched) == 0 {
		return nil, nil
	}

	var foundOps []*Operation
	for _, match := range matched {
		opID := OperationID(OperationTypeTrackAbsence, OperationVersionTrackAbsence, OperationIteration(match.Iteration), match.ID())

		if op, found := plan.Operation(opID); found {
			foundOps = append(foundOps, op)
		} else {
			trackOp := &Operation{
				Type:     OperationTypeTrackAbsence,
				Version:  OperationVersionTrackAbsence,
				Category: OperationCategoryTrack,
				Config: &OperationConfigTrackAbsence{
					ResourceMeta: match.ResourceMeta,
				},
			}

			if err := plan.AddOperationChain().AddOperation(trackOp).Stage(sourceStage).SkipOnDuplicate().Do(); err != nil {
				return nil, fmt.Errorf("add track absence operation: %w", err)
			}

			deleteOpID := OperationID(OperationTypeDelete, OperationVersionDelete, OperationIteration(match.Iteration), match.ID())
			if deleteOp, deleteFound := plan.Operation(deleteOpID); deleteFound {
				if err := plan.Connect(deleteOp.ID(), trackOp.ID()); err != nil {
					return nil, fmt.Errorf("connect delete to track absence: %w", err)
				}
			}

			foundOps = append(foundOps, trackOp)
		}
	}

	return foundOps, nil
}

func resolveTrackReadinessOpInStage(ctx context.Context, plan *Plan, instInfos []*InstallableResourceInfo, dep *resource.Dependency, sourceStage common.Stage, releaseNamespace string) ([]*Operation, error) {
	if dep.External {
		resMeta := resource.NewResourceMetaFromDependency(dep, releaseNamespace)

		opID := OperationID(OperationTypeTrackReadiness, OperationVersionTrackReadiness, OperationIteration(0), resMeta.ID())
		if op, found := plan.Operation(opID); found {
			return []*Operation{op}, nil
		}

		trackOp := &Operation{
			Type:     OperationTypeTrackReadiness,
			Version:  OperationVersionTrackReadiness,
			Category: OperationCategoryTrack,
			Config: &OperationConfigTrackReadiness{
				ResourceMeta: resMeta,
			},
		}

		if err := plan.AddOperationChain().AddOperation(trackOp).Stage(sourceStage).SkipOnDuplicate().Do(); err != nil {
			return nil, fmt.Errorf("add track readiness operation: %w", err)
		}

		return []*Operation{trackOp}, nil
	}

	matchByID := make(map[string]*InstallableResourceInfo)
	for _, candidate := range instInfos {
		match, found := matchByID[candidate.ID()]
		if candidate.Stage != sourceStage ||
			!dep.Match(candidate.ResourceMeta) ||
			(found && candidate.Iteration >= match.Iteration) {
			continue
		}

		matchByID[candidate.ID()] = candidate
	}

	matched, err := truncateInstallableMatches(ctx, lo.Values(matchByID), dep)
	if err != nil {
		return nil, err
	}

	if len(matched) == 0 {
		return nil, nil
	}

	var foundOps []*Operation
	for _, match := range matched {
		opID := OperationID(OperationTypeTrackReadiness, OperationVersionTrackReadiness, OperationIteration(match.Iteration), match.ID())
		if op, found := plan.Operation(opID); found {
			foundOps = append(foundOps, op)
		} else {
			trackOp := &Operation{
				Type:     OperationTypeTrackReadiness,
				Version:  OperationVersionTrackReadiness,
				Category: OperationCategoryTrack,
				Config: &OperationConfigTrackReadiness{
					ResourceMeta: match.ResourceMeta,
				},
			}

			if err := plan.AddOperationChain().AddOperation(trackOp).Stage(sourceStage).SkipOnDuplicate().Do(); err != nil {
				return nil, fmt.Errorf("add track readiness operation: %w", err)
			}

			if deployOp, deployFound := getDeployOp(plan, match); deployFound {
				if err := plan.Connect(deployOp.ID(), trackOp.ID()); err != nil {
					return nil, fmt.Errorf("connect deploy to track readiness: %w", err)
				}
			}

			foundOps = append(foundOps, trackOp)
		}
	}

	return foundOps, nil
}

func addDeleteReleaseOps(plan *Plan, info *ReleaseInfo) {
	acc := lo.Must(helmrel.NewAccessor(info.Release.Releaser))

	deletedOp := &Operation{
		Type:     OperationTypeDeleteRelease,
		Version:  OperationVersionDeleteRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigDeleteRelease{
			ReleaseName:      acc.Name(),
			ReleaseNamespace: acc.Namespace(),
			ReleaseRevision:  acc.Version(),
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

func addFailedReleaseOps(plan *Plan, info *ReleaseInfo) error {
	failedReleaser, err := release.CopyReleaserWithStatus(info.Release.Releaser, helmreleasecommon.StatusFailed)
	if err != nil {
		return fmt.Errorf("copy release with status: %w", err)
	}

	failedOp := &Operation{
		Type:     OperationTypeUpdateRelease,
		Version:  OperationVersionUpdateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigUpdateRelease{
			Release: &release.StoredRelease{Releaser: failedReleaser},
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(failedOp).Stage(common.StageInit).Do())

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

func addInstallResourceOps(plan *Plan, infos []*InstallableResourceInfo) error {
	for _, info := range infos {
		chain := plan.AddOperationChain()

		var stg common.Stage
		if info.LocalResource.Weight != nil {
			stg = common.SubStageWeighted(info.Stage, *info.LocalResource.Weight)
		} else {
			stg = info.Stage
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

func addPendingAndDeployedReleaseOps(plan *Plan, info *ReleaseInfo, pendingStatus helmreleasecommon.Status) error {
	pendingReleaser, err := release.CopyReleaserWithStatus(info.Release.Releaser, pendingStatus)
	if err != nil {
		return fmt.Errorf("copy release with status: %w", err)
	}

	pendingOp := &Operation{
		Type:     OperationTypeCreateRelease,
		Version:  OperationVersionCreateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigCreateRelease{
			Release: &release.StoredRelease{Releaser: pendingReleaser},
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(pendingOp).Stage(common.StageInit).Do())

	succeededReleaser, err := release.CopyReleaserWithStatus(pendingReleaser, helmreleasecommon.StatusDeployed)
	if err != nil {
		return fmt.Errorf("copy release with status: %w", err)
	}

	succeededOp := &Operation{
		Type:     OperationTypeUpdateRelease,
		Version:  OperationVersionUpdateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigUpdateRelease{
			Release: &release.StoredRelease{Releaser: succeededReleaser},
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(succeededOp).Stage(common.StageFinal).Do())

	return nil
}

func addSupersedeReleaseOps(plan *Plan, info *ReleaseInfo) error {
	supersededReleaser, err := release.CopyReleaserWithStatus(info.Release.Releaser, helmreleasecommon.StatusSuperseded)
	if err != nil {
		return fmt.Errorf("copy release with status: %w", err)
	}

	supersedeOp := &Operation{
		Type:     OperationTypeUpdateRelease,
		Version:  OperationVersionUpdateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigUpdateRelease{
			Release: &release.StoredRelease{Releaser: supersededReleaser},
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(supersedeOp).Stage(common.StageFinal).Do())

	return nil
}

func addUninstallReleaseOps(plan *Plan, info *ReleaseInfo) error {
	uninstallingReleaser, err := release.CopyReleaserWithStatus(info.Release.Releaser, helmreleasecommon.StatusUninstalling)
	if err != nil {
		return fmt.Errorf("copy release with status: %w", err)
	}

	uninstallingOp := &Operation{
		Type:     OperationTypeUpdateRelease,
		Version:  OperationVersionUpdateRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigUpdateRelease{
			Release: &release.StoredRelease{Releaser: uninstallingReleaser},
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(uninstallingOp).Stage(common.StageInit).Do())

	uninstallingAcc := lo.Must(helmrel.NewAccessor(uninstallingReleaser))

	uninstalledOp := &Operation{
		Type:     OperationTypeDeleteRelease,
		Version:  OperationVersionDeleteRelease,
		Category: OperationCategoryRelease,
		Config: &OperationConfigDeleteRelease{
			ReleaseName:      uninstallingAcc.Name(),
			ReleaseNamespace: uninstallingAcc.Namespace(),
			ReleaseRevision:  uninstallingAcc.Version(),
		},
	}
	lo.Must0(plan.AddOperationChain().AddOperation(uninstalledOp).Stage(common.StageFinal).Do())

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

func getDeleteOp(plan *Plan, info *DeletableResourceInfo) (*Operation, bool) {
	if !info.MustDelete {
		return nil, false
	}

	operationID := OperationID(OperationTypeDelete, OperationVersionDelete, OperationIteration(0), info.ID())

	return lo.Must(plan.Operation(operationID)), true
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

func truncateDeletableMatches(ctx context.Context, matched []*matchedDeletableResource, dep *resource.Dependency) ([]*matchedDeletableResource, error) {
	if len(matched) < dep.MinMatches {
		return nil, errors.New("matched resources count is less than minimum required")
	}

	if dep.MaxMatches > 0 && len(matched) > dep.MaxMatches {
		log.Default.Warn(ctx, "Dependency matched %d resources, but maxMatches is %d, only first %d will be used", len(matched), dep.MaxMatches, dep.MaxMatches)

		sort.SliceStable(matched, func(i, j int) bool {
			return spec.ResourceMetaSortHandler(matched[i].ResourceMeta, matched[j].ResourceMeta)
		})

		matched = matched[:dep.MaxMatches]
	}

	return matched, nil
}

func truncateInstallableMatches(ctx context.Context, matched []*InstallableResourceInfo, dep *resource.Dependency) ([]*InstallableResourceInfo, error) {
	if len(matched) < dep.MinMatches {
		return nil, errors.New("matched resources count is less than minimum required")
	}

	if dep.MaxMatches > 0 && len(matched) > dep.MaxMatches {
		log.Default.Warn(ctx, "Dependency matched %d resources, but maxMatches is %d, only first %d will be used", len(matched), dep.MaxMatches, dep.MaxMatches)

		sort.SliceStable(matched, func(i, j int) bool {
			return spec.ResourceMetaSortHandler(matched[i].ResourceMeta, matched[j].ResourceMeta)
		})

		matched = matched[:dep.MaxMatches]
	}

	return matched, nil
}
