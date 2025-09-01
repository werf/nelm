package plan

import (
	"bytes"
	"fmt"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/plan/operation"
)

func NewPlan() *Plan {
	return &Plan{
		graph: graph.New(func(t *operation.Operation) string { return t.ID() }, graph.Acyclic(), graph.PreventCycles(), graph.Directed()),
	}
}

type Plan struct {
	graph graph.Graph[string, *operation.Operation]
}

func (p *Plan) Operation(id string) (op *operation.Operation, found bool) {
	vertex, err := p.graph.Vertex(id)
	if err != nil {
		if err == graph.ErrVertexNotFound {
			return nil, false
		} else {
			panic(fmt.Sprintf("unexpected error: %s", err))
		}
	}

	return vertex, true
}

func (p *Plan) Operations() ([]*operation.Operation, error) {
	var operations []*operation.Operation
	adjMap, err := p.graph.AdjacencyMap()
	if err != nil {
		return nil, fmt.Errorf("get adjacency map: %w", err)
	}

	for opID := range adjMap {
		operations = append(operations, lo.Must(p.Operation(opID)))
	}

	return operations, nil
}

func (p *Plan) PredecessorMap() (map[string]map[string]graph.Edge[string], error) {
	return p.graph.PredecessorMap()
}

func (p *Plan) AddOperation(op *operation.Operation) {
	if err := p.graph.AddVertex(op); err != nil && !errors.Is(err, graph.ErrVertexAlreadyExists) {
		panic(fmt.Sprintf("unexpected error: %s", err))
	}
}

func (p *Plan) AddOperationInStage(op *operation.Operation, stage common.Stage) {
	p.AddOperation(op)

	stageStartOp := lo.Must(p.Operation(fmt.Sprintf("%s/%s", stage, common.StageStartSuffix)))
	lo.Must0(p.Connect(stageStartOp.ID(), op.ID()))

	stageEndOp := lo.Must(p.Operation(fmt.Sprintf("%s/%s", stage, common.StageEndSuffix)))
	lo.Must0(p.Connect(op.ID(), stageEndOp.ID()))
}

func (p *Plan) Connect(fromID, toID string) error {
	if err := p.graph.AddEdge(fromID, toID); err != nil {
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
	p.graph, err = graph.TransitiveReduction(p.graph)
	if err != nil {
		return fmt.Errorf("transitively reduce graph: %w", err)
	}

	return nil
}

func (p *Plan) ToDOT() ([]byte, error) {
	b := &bytes.Buffer{}
	if err := draw.DOT(p.graph, b, draw.GraphAttribute("rankdir", "LR")); err != nil {
		return nil, fmt.Errorf("draw graph as DOT: %w", err)
	}

	return b.Bytes(), nil
}
