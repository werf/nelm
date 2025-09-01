package plan

import (
	"bytes"
	"fmt"
	"os"
	"regexp"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/werf/nelm/internal/plan/operation"
)

func NewFixmePlan() *FixmePlan {
	planGraph := graph.New(func(t operation.FixmeOperation) string { return t.ID() }, graph.Acyclic(), graph.PreventCycles(), graph.Directed())

	return &FixmePlan{
		graph: planGraph,
	}
}

type FixmePlan struct {
	graph graph.Graph[string, operation.FixmeOperation]
}

func (p *FixmePlan) Operation(idFormat string, a ...any) (op operation.FixmeOperation, found bool) {
	opID := fmt.Sprintf(idFormat, a...)

	vertex, err := p.graph.Vertex(opID)
	if err != nil {
		if errors.Is(err, graph.ErrVertexNotFound) {
			return nil, false
		} else {
			panic(fmt.Sprintf("unexpected error: %s", err))
		}
	}

	return vertex, true
}

func (p *FixmePlan) OperationsMatch(regex *regexp.Regexp) (ops []operation.FixmeOperation, found bool, err error) {
	operations, found, err := p.Operations()
	if err != nil {
		return nil, false, fmt.Errorf("error getting operations: %w", err)
	} else if !found {
		return nil, false, nil
	}

	for _, op := range operations {
		if regex.MatchString(op.ID()) {
			ops = append(ops, op)
		}
	}

	return ops, len(ops) > 0, nil
}

func (p *FixmePlan) Operations() (operations []operation.FixmeOperation, found bool, err error) {
	adjMap, err := p.graph.AdjacencyMap()
	if err != nil {
		return nil, false, fmt.Errorf("error getting adjacency map: %w", err)
	}

	for opID := range adjMap {
		operations = append(operations, lo.Must(p.Operation(opID)))
	}

	return operations, len(operations) > 0, nil
}

func (p *FixmePlan) CompletedOperations() (completedOps []operation.FixmeOperation, found bool, err error) {
	ops, found, err := p.Operations()
	if err != nil {
		return nil, false, fmt.Errorf("error getting operations: %w", err)
	} else if !found {
		return nil, false, nil
	}

	for _, op := range ops {
		if op.Status() == operation.StatusCompleted {
			completedOps = append(completedOps, op)
		}
	}

	return completedOps, len(completedOps) > 0, nil
}

func (p *FixmePlan) FailedOperations() (failedOps []operation.FixmeOperation, found bool, err error) {
	ops, found, err := p.Operations()
	if err != nil {
		return nil, false, fmt.Errorf("error getting operations: %w", err)
	} else if !found {
		return nil, false, nil
	}

	for _, op := range ops {
		if op.Status() == operation.StatusFailed {
			failedOps = append(failedOps, op)
		}
	}

	return failedOps, len(failedOps) > 0, nil
}

func (p *FixmePlan) CanceledOperations() (canceledOps []operation.FixmeOperation, found bool, err error) {
	ops, found, err := p.Operations()
	if err != nil {
		return nil, false, fmt.Errorf("error getting operations: %w", err)
	} else if !found {
		return nil, false, nil
	}

	for _, op := range ops {
		if op.Status() == operation.StatusUnknown {
			canceledOps = append(canceledOps, op)
		}
	}

	return canceledOps, len(canceledOps) > 0, nil
}

func (p *FixmePlan) WorthyCompletedOperations() (worthyCompletedOps []operation.FixmeOperation, found bool, err error) {
	completedOps, found, err := p.CompletedOperations()
	if err != nil {
		return nil, false, fmt.Errorf("error getting completed operations: %w", err)
	} else if !found {
		return nil, false, nil
	}

	for _, op := range completedOps {
		switch op.Type() {
		case operation.TypeCreateResourceOperation,
			operation.TypeRecreateResourceOperation,
			operation.TypeUpdateResourceOperation,
			operation.TypeApplyResourceOperation,
			operation.TypeDeleteResourceOperation,
			operation.TypeExtraPostCreateResourceOperation,
			operation.TypeExtraPostRecreateResourceOperation,
			operation.TypeExtraPostApplyResourceOperation,
			operation.TypeExtraPostUpdateResourceOperation,
			operation.TypeExtraPostDeleteResourceOperation:
			worthyCompletedOps = append(worthyCompletedOps, op)
		}
	}

	return worthyCompletedOps, len(worthyCompletedOps) > 0, nil
}

func (p *FixmePlan) WorthyFailedOperations() (worthyFailedOps []operation.FixmeOperation, found bool, err error) {
	failedOps, found, err := p.FailedOperations()
	if err != nil {
		return nil, false, fmt.Errorf("error getting failed operations: %w", err)
	} else if !found {
		return nil, false, nil
	}

	for _, op := range failedOps {
		worthyFailedOps = append(worthyFailedOps, op)
	}

	return worthyFailedOps, len(worthyFailedOps) > 0, nil
}

func (p *FixmePlan) WorthyCanceledOperations() (worthyCanceledOps []operation.FixmeOperation, found bool, err error) {
	canceledOps, found, err := p.CanceledOperations()
	if err != nil {
		return nil, false, fmt.Errorf("error getting canceled operations: %w", err)
	} else if !found {
		return nil, false, nil
	}

	for _, op := range canceledOps {
		switch op.Type() {
		case operation.TypeCreateResourceOperation,
			operation.TypeRecreateResourceOperation,
			operation.TypeUpdateResourceOperation,
			operation.TypeApplyResourceOperation,
			operation.TypeDeleteResourceOperation:
			worthyCanceledOps = append(worthyCanceledOps, op)
		}
	}

	return worthyCanceledOps, len(worthyCanceledOps) > 0, nil
}

func (p *FixmePlan) PredecessorMap() (map[string]map[string]graph.Edge[string], error) {
	return p.graph.PredecessorMap()
}

func (p *FixmePlan) AddOperation(op operation.FixmeOperation) {
	err := p.graph.AddVertex(op)
	if err != nil && !errors.Is(err, graph.ErrVertexAlreadyExists) {
		panic(fmt.Sprintf("unexpected error: %s", err))
	}
}

func (p *FixmePlan) AddStagedOperation(op operation.FixmeOperation, stageInID, stageOutID string) {
	p.AddOperation(op)

	if _, found := p.Operation(stageInID); !found {
		op := operation.NewStageOperation(stageInID)
		p.AddOperation(op)
	}
	if _, found := p.Operation(stageOutID); !found {
		op := operation.NewStageOperation(stageOutID)
		p.AddOperation(op)
	}
	lo.Must0(p.AddDependency(stageInID, stageOutID))

	lo.Must0(p.AddDependency(stageInID, op.ID()))
	lo.Must0(p.AddDependency(op.ID(), stageOutID))
}

func (p *FixmePlan) AddInStagedOperation(op operation.FixmeOperation, stageInID string) {
	p.AddOperation(op)

	if _, found := p.Operation(stageInID); !found {
		op := operation.NewStageOperation(stageInID)
		p.AddOperation(op)
	}

	lo.Must0(p.AddDependency(stageInID, op.ID()))
}

func (p *FixmePlan) AddOutStagedOperation(op operation.FixmeOperation, stageOutID string) {
	p.AddOperation(op)

	if _, found := p.Operation(stageOutID); !found {
		op := operation.NewStageOperation(stageOutID)
		p.AddOperation(op)
	}

	lo.Must0(p.AddDependency(op.ID(), stageOutID))
}

func (p *FixmePlan) AddDependency(fromOpID, toOpID string) error {
	if err := p.graph.AddEdge(fromOpID, toOpID); err != nil {
		if errors.Is(err, graph.ErrEdgeAlreadyExists) {
			return nil
		} else {
			return fmt.Errorf("error adding edge from %q to %q: %w", fromOpID, toOpID, err)
		}
	}

	return nil
}

func (p *FixmePlan) Optimize() error {
	var err error

	p.graph, err = graph.TransitiveReduction(p.graph)
	if err != nil {
		return fmt.Errorf("error transitively reducing graph: %w", err)
	}

	return nil
}

func (p *FixmePlan) DOT() ([]byte, error) {
	b := &bytes.Buffer{}

	if err := draw.DOT(
		p.graph,
		b,
		draw.GraphAttribute("rankdir", "LR"),
	); err != nil {
		return nil, fmt.Errorf("error drawing DOT graph: %w", err)
	}

	return b.Bytes(), nil
}

func (p *FixmePlan) SaveDOT(path string) error {
	dot, err := p.DOT()
	if err != nil {
		return fmt.Errorf("error getting DOT graph: %w", err)
	}

	if err := os.WriteFile(path, dot, 0o644); err != nil {
		return fmt.Errorf("error writing DOT graph file at %q: %w", path, err)
	}

	return nil
}

func (p *FixmePlan) Useless() (bool, error) {
	ops, found, err := p.Operations()
	if err != nil {
		return false, fmt.Errorf("error getting operations: %w", err)
	} else if !found {
		return true, nil
	}

	for _, op := range ops {
		switch op.Type() {
		case operation.TypeCreateResourceOperation,
			operation.TypeRecreateResourceOperation,
			operation.TypeUpdateResourceOperation,
			operation.TypeApplyResourceOperation,
			operation.TypeDeleteResourceOperation,
			operation.TypeFailReleaseOperation,
			operation.TypeTrackResourceReadinessOperation,
			operation.TypeTrackResourcePresenceOperation,
			operation.TypeTrackResourceAbsenceOperation,
			operation.TypeExtraPostCreateResourceOperation,
			operation.TypeExtraPostRecreateResourceOperation,
			operation.TypeExtraPostApplyResourceOperation,
			operation.TypeExtraPostUpdateResourceOperation,
			operation.TypeExtraPostDeleteResourceOperation:
			if !op.Empty() {
				return false, nil
			}
		}
	}

	return true, nil
}
