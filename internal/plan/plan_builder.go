package plan

import (
	"errors"
	"fmt"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/plan/operation"
)

type planChainBuilder struct {
	plan  *Plan
	steps []*planBuilderStep
	err   error
}

func (b *planChainBuilder) AddOperation(op *operation.Operation) *planChainBuilder {
	if b.err != nil {
		return b
	}

	b.steps = append(b.steps, &planBuilderStep{operation: op})

	return b
}

func (b *planChainBuilder) Stage(stage common.Stage) *planChainBuilder {
	if b.err != nil {
		return b
	}

	lastStep := lo.Must(lo.Last(b.steps))
	lastStep.stage = stage

	return b
}

func (b *planChainBuilder) SkipOnDuplicate() *planChainBuilder {
	if b.err != nil {
		return b
	}

	lastStep := lo.Must(lo.Last(b.steps))
	lastStep.skipOnDuplicate = true

	return b
}

func (b *planChainBuilder) Do() error {
	if b.err != nil {
		return fmt.Errorf("plan chain build: %w", b.err)
	}

	for i, step := range b.steps {
		if err := b.plan.graph.AddVertex(step.operation); err != nil {
			if !errors.Is(err, graph.ErrVertexAlreadyExists) || !step.skipOnDuplicate {
				return fmt.Errorf("add vertex: %w", err)
			}
		}

		if step.stage != "" {
			stageStartOp := lo.Must(b.plan.Operation(fmt.Sprintf("%s/%s", step.stage, common.StageStartSuffix)))
			if err := b.plan.Connect(stageStartOp.ID(), step.operation.ID()); err != nil {
				return fmt.Errorf("connect starting stage: %w", err)
			}

			stageEndOp := lo.Must(b.plan.Operation(fmt.Sprintf("%s/%s", step.stage, common.StageEndSuffix)))
			if err := b.plan.Connect(step.operation.ID(), stageEndOp.ID()); err != nil {
				return fmt.Errorf("connect ending stage: %w", err)
			}
		}

		if i > 0 {
			prevStep := b.steps[i-1]
			if err := b.plan.Connect(prevStep.operation.ID(), step.operation.ID()); err != nil {
				return fmt.Errorf("connect operations in chain: %w", err)
			}
		}
	}

	return nil
}

type planBuilderStep struct {
	operation       *operation.Operation
	skipOnDuplicate bool
	stage           common.Stage
}
