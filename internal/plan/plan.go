package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/werf/nelm/pkg/common"
)

// Wrapper over dominikbraun/graph to make it easier to use as a plan/graph of operations.
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

func (p *Plan) Optimize(noFinalTracking bool) error {
	var err error

	p.Graph, err = graph.TransitiveReduction(p.Graph)
	if err != nil {
		return fmt.Errorf("transitively reduce graph: %w", err)
	}

	squashUselessMetaOperations(p)

	if noFinalTracking {
		squashFinalTrackingOperations(p)
	}

	return nil
}

func (p *Plan) SquashOperation(op *Operation) {
	adjMap := lo.Must(p.Graph.AdjacencyMap())
	predMap := lo.Must(p.Graph.PredecessorMap())

	opPreds := predMap[op.ID()]
	opAdjacencies := adjMap[op.ID()]

	for predID := range opPreds {
		lo.Must0(p.Graph.RemoveEdge(predID, op.ID()))
	}

	for adjID := range opAdjacencies {
		lo.Must0(p.Graph.RemoveEdge(op.ID(), adjID))
	}

	for predID := range opPreds {
		for adjID := range opAdjacencies {
			lo.Must0(p.Connect(predID, adjID))
		}
	}

	lo.Must0(p.Graph.RemoveVertex(op.ID()))
}

func (p *Plan) ToDOT() ([]byte, error) {
	b := &bytes.Buffer{}
	if err := draw.DOT(p.Graph, b, draw.GraphAttribute("rankdir", "LR")); err != nil {
		return nil, fmt.Errorf("draw graph as DOT: %w", err)
	}

	return b.Bytes(), nil
}

type planJSON struct {
	Operations []*Operation   `json:"operations"`
	Edges      []planEdgeJSON `json:"edges"`
}

type planEdgeJSON struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (p *Plan) MarshalJSON() ([]byte, error) {
	ops := p.Operations()

	sort.Slice(ops, func(i, j int) bool {
		return ops[i].ID() < ops[j].ID()
	})

	adjMap, err := p.Graph.AdjacencyMap()
	if err != nil {
		return nil, fmt.Errorf("get adjacency map: %w", err)
	}

	var edges []planEdgeJSON
	for fromID, toMap := range adjMap {
		for toID := range toMap {
			edges = append(edges, planEdgeJSON{From: fromID, To: toID})
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}

		return edges[i].From < edges[j].From
	})

	data, err := json.Marshal(planJSON{
		Operations: ops,
		Edges:      edges,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal plan: %w", err)
	}

	return data, nil
}

func (p *Plan) UnmarshalJSON(data []byte) error {
	var raw planJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal plan: %w", err)
	}

	p.Graph = graph.New(func(t *Operation) string { return t.ID() }, graph.Acyclic(), graph.PreventCycles(), graph.Directed())

	for _, op := range raw.Operations {
		if err := p.Graph.AddVertex(op); err != nil {
			if !errors.Is(err, graph.ErrVertexAlreadyExists) {
				return fmt.Errorf("add vertex %q: %w", op.ID(), err)
			}
		}
	}

	for _, edge := range raw.Edges {
		if err := p.Connect(edge.From, edge.To); err != nil {
			return fmt.Errorf("connect edge from %q to %q: %w", edge.From, edge.To, err)
		}
	}

	return nil
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
		for _, op := range pair {
			p.SquashOperation(op)
		}
	}

	operationPairs = findMetaOperationPairs(p.Operations())
	uselessOperationPairs = findUselessMetaOperations(operationPairs, lo.Must(p.Graph.AdjacencyMap()))

	if len(uselessOperationPairs) > 0 {
		squashUselessMetaOperations(p)
	}
}

func squashFinalTrackingOperations(p *Plan) {
	ops := p.Operations()
	trackingOps := lo.Filter(ops, func(op *Operation, _ int) bool {
		return op.Category == OperationCategoryTrack
	})

	for _, trackingOp := range trackingOps {
		var foundDependentResourceOps bool
		lo.Must0(graph.BFS(p.Graph, trackingOp.ID(), func(opID string) bool {
			op := lo.Must(p.Operation(opID))
			if op.Category == OperationCategoryResource {
				foundDependentResourceOps = true
				return true
			}

			return false
		}))

		if !foundDependentResourceOps {
			p.SquashOperation(trackingOp)
		}
	}
}
