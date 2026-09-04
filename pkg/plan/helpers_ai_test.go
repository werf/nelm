//go:build ai_tests

package plan

import (
	"fmt"
	"math/rand"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/pkg/legacy/progrep"
	"github.com/werf/nelm/pkg/resource/spec"
)

var (
	gvkConfigMap  = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
	gvkService    = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}
	gvkDeployment = schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	gvkNamespace  = schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Namespace"}
	gvkCRD        = schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}
)

type fakeRESTMapper struct {
	meta.RESTMapper

	mappings map[schema.GroupKind]*meta.RESTMapping
}

func newFakeRESTMapper() *fakeRESTMapper {
	namespacedScope := meta.RESTScopeNamespace
	clusterScope := meta.RESTScopeRoot

	return &fakeRESTMapper{
		mappings: map[schema.GroupKind]*meta.RESTMapping{
			{Group: "", Kind: "ConfigMap"}:      {Scope: namespacedScope},
			{Group: "", Kind: "Service"}:        {Scope: namespacedScope},
			{Group: "apps", Kind: "Deployment"}: {Scope: namespacedScope},
			{Group: "", Kind: "Namespace"}:      {Scope: clusterScope},
		},
	}
}

func (m *fakeRESTMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error) {
	if mapping, ok := m.mappings[gk]; ok {
		return mapping, nil
	}

	return nil, fmt.Errorf("no mapping for %v", gk)
}

func makeResourceSpec(name, namespace string, gvk schema.GroupVersionKind) *spec.ResourceSpec {
	return &spec.ResourceSpec{
		ResourceMeta: makeResourceMeta(name, namespace, gvk),
	}
}

func buildTestPlan(ops []*Operation, deps map[int][]int) *Plan {
	p := NewPlan()

	for _, op := range ops {
		if err := p.Graph.AddVertex(op); err != nil {
			panic(fmt.Sprintf("add vertex: %v", err))
		}
	}

	if deps != nil {
		for toIdx, fromIdxs := range deps {
			for _, fromIdx := range fromIdxs {
				if err := p.Graph.AddEdge(ops[fromIdx].ID(), ops[toIdx].ID()); err != nil {
					panic(fmt.Sprintf("add edge: %v", err))
				}
			}
		}
	}

	return p
}

func drainChannel(ch <-chan progrep.ProgressReport) []progrep.ProgressReport {
	var reports []progrep.ProgressReport

	for {
		select {
		case r := <-ch:
			reports = append(reports, r)
		default:
			return reports
		}
	}
}

func makeResourceMeta(name, namespace string, gvk schema.GroupVersionKind) *spec.ResourceMeta {
	return &spec.ResourceMeta{
		Name:             name,
		Namespace:        namespace,
		GroupVersionKind: gvk,
	}
}

func randomTrackingGraph(rnd *rand.Rand, opsCount int, edgeChance float64) ([]OperationCategory, map[int][]int) {
	allCategories := []OperationCategory{OperationCategoryResource, OperationCategoryTrack, OperationCategoryMeta}

	categories := make([]OperationCategory, opsCount)
	for i := range categories {
		categories[i] = allCategories[rnd.Intn(len(allCategories))]
	}

	deps := map[int][]int{}
	for from := 0; from < opsCount; from++ {
		for to := from + 1; to < opsCount; to++ {
			if rnd.Float64() < edgeChance {
				deps[to] = append(deps[to], from)
			}
		}
	}

	return categories, deps
}

// Pre-optimization implementation of squashFinalTrackingOperations, kept as a reference to assert
// the optimized one against.
func squashFinalTrackingOperationsReference(p *Plan) {
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

func trackingTestOperations(categories []OperationCategory) []*Operation {
	return lo.Map(categories, func(category OperationCategory, i int) *Operation {
		return &Operation{
			Type:     OperationTypeNoop,
			Version:  OperationVersionNoop,
			Category: category,
			Config:   &OperationConfigNoop{OpID: fmt.Sprintf("op-%d", i)},
		}
	})
}
