package plan

import (
	"fmt"

	"github.com/mitchellh/copystructure"
	"github.com/samber/lo"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/plan/resourceinfo"
)

func BuildFailurePlan(prevPlan *Plan, installableInfos []*resourceinfo.InstallableResourceInfo, releaseInfos []*resourceinfo.ReleaseInfo) (*Plan, error) {
	plan := NewPlan()

	if err := addMainStages(plan); err != nil {
		return nil, fmt.Errorf("add main stages: %w", err)
	}

	if err := addFailedReleaseOperations(prevPlan, plan, releaseInfos); err != nil {
		return nil, fmt.Errorf("add failed release operations: %w", err)
	}

	if err := addDeleteResourceOps(prevPlan, plan, installableInfos); err != nil {
		return nil, fmt.Errorf("add delete-on-failure resource operations: %w", err)
	}

	if err := plan.Optimize(); err != nil {
		return nil, fmt.Errorf("optimize plan: %w", err)
	}

	return plan, nil
}

func addFailedReleaseOperations(prevPlan *Plan, plan *Plan, releaseInfos []*resourceinfo.ReleaseInfo) error {
	for _, info := range releaseInfos {
		if !info.MustFailOnFailedDeploy {
			continue
		}

		releaseOpFromPrevPlan := findReleaseOperation(prevPlan, info)
		if releaseOpFromPrevPlan == nil {
			continue
		}

		if releaseOpFromPrevPlan.Status != operation.OperationStatusCompleted {
			continue
		}

		var failedRel *helmrelease.Release
		if rel, err := copystructure.Copy(info.Release); err != nil {
			return fmt.Errorf("deep copy release: %w", err)
		} else {
			failedRel = rel.(*helmrelease.Release)
		}

		failedRel.Info.Status = helmrelease.StatusFailed

		failedOp := &operation.Operation{
			Type:    operation.OperationTypeUpdateRelease,
			Version: operation.OperationVersionUpdateRelease,
			Config: &operation.OperationConfigUpdateRelease{
				Release: failedRel,
			},
		}
		lo.Must0(plan.AddOperationChain().AddOperation(failedOp).Stage(common.StageInit).Do())
	}

	return nil
}

func addDeleteResourceOps(prevPlan *Plan, plan *Plan, installableInfos []*resourceinfo.InstallableResourceInfo) error {
	for _, info := range installableInfos {
		if !info.MustDeleteOnSuccessfulInstall && !info.MustDeleteOnFailedInstall {
			continue
		}

		trackOpFromPrevPlanID := operation.OperationID(operation.OperationTypeTrackReadiness, operation.OperationVersionTrackReadiness, operation.OperationIteration(info.Iteration), info.ResourceMeta.ID())

		trackOpFromPrevPlan, found := lo.Find(prevPlan.Operations(), func(op *operation.Operation) bool {
			return op.ID() == trackOpFromPrevPlanID
		})
		if !found {
			continue
		}

		switch trackOpFromPrevPlan.Status {
		case operation.OperationStatusCompleted:
			if !info.MustDeleteOnSuccessfulInstall {
				continue
			}
		case operation.OperationStatusFailed:
			if !info.MustDeleteOnFailedInstall {
				continue
			}
		default:
			continue
		}

		deleteOp := &operation.Operation{
			Type:    operation.OperationTypeDelete,
			Version: operation.OperationVersionDelete,
			Config: &operation.OperationConfigDelete{
				ResourceMeta: info.ResourceMeta,
			},
		}
		lo.Must0(plan.AddOperationChain().AddOperation(deleteOp).Stage(common.StageUninstall).SkipOnDuplicate().Do())
	}

	return nil
}

func findReleaseOperation(plan *Plan, info *resourceinfo.ReleaseInfo) *operation.Operation {
	var opType operation.OperationType
	switch info.Must {
	case resourceinfo.ReleaseTypeInstall,
		resourceinfo.ReleaseTypeUpgrade,
		resourceinfo.ReleaseTypeRollback:
		opType = operation.OperationTypeCreateRelease
	case resourceinfo.ReleaseTypeUninstall:
		opType = operation.OperationTypeUpdateRelease
	case resourceinfo.ReleaseTypeDelete,
		resourceinfo.ReleaseTypeNone,
		resourceinfo.ReleaseTypeSupersede:
		return nil
	default:
		panic("unexpected release must condition")
	}

	for _, op := range plan.Operations() {
		if op.Type == opType {
			return op
		}
	}

	return nil
}
