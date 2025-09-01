package plan

import (
	"fmt"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/plan/operation"
)

func BuildPlan() *Plan {
	plan := NewPlan()

	for _, stage := range []common.Stage{
		common.StageInit,
		common.StagePrePreUninstall,
		common.StagePrePreInstall,
		common.StagePreInstall,
		common.StagePreUninstall,
		common.StageInstall,
		common.StageUninstall,
		common.StagePostInstall,
		common.StagePostUninstall,
		common.StagePostPostInstall,
		common.StagePostPostUninstall,
		common.StageFinal,
	} {
		startOp := &operation.Operation{
			Type:    operation.OperationTypeNoop,
			Version: operation.OperationVersionNoop,
			Config: &operation.OperationConfigNoop{
				OpID: fmt.Sprintf("%s/%s", stage, common.StageStartSuffix),
			},
		}
		plan.AddOperation(startOp)

		endOp := &operation.Operation{
			Type:    operation.OperationTypeNoop,
			Version: operation.OperationVersionNoop,
			Config: &operation.OperationConfigNoop{
				OpID: fmt.Sprintf("%s/%s", stage, common.StageEndSuffix),
			},
		}
		plan.AddOperation(endOp)

		plan.Connect(startOp.ID(), endOp.ID())
	}

	return &Plan{}
}
