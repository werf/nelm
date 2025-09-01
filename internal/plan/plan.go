package plan

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/werf/nelm/internal/common"
)

type Plan struct {
	Graph graph.Graph[string, *Operation]
}

func NewPlan() *Plan {
	return &Plan{
		Graph: graph.New(func(t *Operation) string { return t.ID() }, graph.Acyclic(), graph.PreventCycles(), graph.Directed()),
	}
}

func (p *Plan) Operation(id string) (op *Operation, found bool) {
	vertex, err := p.Graph.Vertex(id)
	if err != nil {
		if errors.Is(err, graph.ErrVertexNotFound) {
			return nil, false
		} else {
			panic(fmt.Sprintf("unexpected error: %s", err))
		}
	}

	return vertex, true
}

func (p *Plan) Operations() []*Operation {
	var operations []*Operation

	adjMap := lo.Must(p.Graph.AdjacencyMap())

	for opID := range adjMap {
		operations = append(operations, lo.Must(p.Operation(opID)))
	}

	return operations
}

func (p *Plan) AddOperationChain() *planChainBuilder {
	return &planChainBuilder{
		plan: p,
	}
}

func (p *Plan) Connect(fromID, toID string) error {
	if err := p.Graph.AddEdge(fromID, toID); err != nil {
		if errors.Is(err, graph.ErrEdgeAlreadyExists) {
			return nil
		} else {
			return fmt.Errorf("add edge from %q to %q: %w", fromID, toID, err)
		}
	}

	return nil
}

func (p *Plan) Optimize() error {
	var err error

	p.Graph, err = graph.TransitiveReduction(p.Graph)
	if err != nil {
		return fmt.Errorf("transitively reduce graph: %w", err)
	}

	squashUselessMetaOperations(p)

	return nil
}

func (p *Plan) ToDOT() ([]byte, error) {
	b := &bytes.Buffer{}
	if err := draw.DOT(p.Graph, b, draw.GraphAttribute("rankdir", "LR")); err != nil {
		return nil, fmt.Errorf("draw graph as DOT: %w", err)
	}

	return b.Bytes(), nil
}

type planBuilderStep struct {
	operation       *Operation
	skipOnDuplicate bool
	stage           common.Stage
}

type planChainBuilder struct {
	plan  *Plan
	steps []*planBuilderStep
	err   error
}

func (b *planChainBuilder) AddOperation(op *Operation) *planChainBuilder {
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
		var vertexAdded bool
		if err := b.plan.Graph.AddVertex(step.operation); err != nil {
			if !errors.Is(err, graph.ErrVertexAlreadyExists) || !step.skipOnDuplicate {
				return fmt.Errorf("add vertex: %w", err)
			}
		} else {
			vertexAdded = true
		}

		operations := b.plan.Operations()

		if step.stage != "" && vertexAdded {
			stageStartOp := lo.Must(lo.Find(operations, func(op *Operation) bool {
				config, ok := op.Config.(*OperationConfigNoop)
				if !ok {
					return false
				}

				return config.OpID == fmt.Sprintf("%s/%s/%s", common.StagePrefix, step.stage, common.StageStartSuffix)
			}))
			if err := b.plan.Connect(stageStartOp.ID(), step.operation.ID()); err != nil {
				return fmt.Errorf("connect starting stage: %w", err)
			}

			stageEndOp := lo.Must(lo.Find(operations, func(op *Operation) bool {
				config, ok := op.Config.(*OperationConfigNoop)
				if !ok {
					return false
				}

				return config.OpID == fmt.Sprintf("%s/%s/%s", common.StagePrefix, step.stage, common.StageEndSuffix)
			}))
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

func findMetaOperationPairs(operations []*Operation) [][]*Operation {
	var (
		startOps []*Operation
		endOps   []*Operation
	)
	for _, op := range operations {
		if op.Type != OperationTypeNoop || op.Category != OperationCategoryMeta {
			continue
		}

		if strings.HasSuffix(op.ID(), "/"+common.StageStartSuffix) {
			startOps = append(startOps, op)
		} else if strings.HasSuffix(op.ID(), "/"+common.StageEndSuffix) {
			endOps = append(endOps, op)
		}
	}

	var pairOps [][]*Operation
	for _, startOp := range startOps {
		endOpID := lo.Must(strings.CutSuffix(startOp.ID(), common.StageStartSuffix)) + common.StageEndSuffix

		endOp := lo.Must(lo.Find(endOps, func(op *Operation) bool {
			return op.ID() == endOpID
		}))

		pairOps = append(pairOps, []*Operation{startOp, endOp})
	}

	return pairOps
}

func findUselessMetaOperations(operationPairs [][]*Operation, adjMap map[string]map[string]graph.Edge[string]) [][]*Operation {
	var uselessPairs [][]*Operation
	for _, pair := range operationPairs {
		startOp := pair[0]
		endOp := pair[1]

		adjacencies := adjMap[startOp.ID()]
		if len(adjacencies) != 1 {
			continue
		}

		if _, ok := adjacencies[endOp.ID()]; !ok {
			continue
		}

		uselessPairs = append(uselessPairs, pair)
	}

	return uselessPairs
}

func squashUselessMetaOperations(p *Plan) {
	operationPairs := findMetaOperationPairs(p.Operations())
	uselessOperationPairs := findUselessMetaOperations(operationPairs, lo.Must(p.Graph.AdjacencyMap()))

	for _, pair := range uselessOperationPairs {
		startOp := pair[0]
		endOp := pair[1]

		adjMap := lo.Must(p.Graph.AdjacencyMap())
		predMap := lo.Must(p.Graph.PredecessorMap())

		startPreds := predMap[startOp.ID()]
		endAdjacencies := adjMap[endOp.ID()]

		lo.Must0(p.Graph.RemoveEdge(startOp.ID(), endOp.ID()))

		for predID := range startPreds {
			lo.Must0(p.Graph.RemoveEdge(predID, startOp.ID()))
		}

		for adjID := range endAdjacencies {
			lo.Must0(p.Graph.RemoveEdge(endOp.ID(), adjID))
		}

		for predID := range startPreds {
			for adjID := range endAdjacencies {
				lo.Must0(p.Connect(predID, adjID))
			}
		}

		lo.Must0(p.Graph.RemoveVertex(startOp.ID()))
		lo.Must0(p.Graph.RemoveVertex(endOp.ID()))
	}

	operationPairs = findMetaOperationPairs(p.Operations())
	uselessOperationPairs = findUselessMetaOperations(operationPairs, lo.Must(p.Graph.AdjacencyMap()))

	if len(uselessOperationPairs) > 0 {
		squashUselessMetaOperations(p)
	}
}
