//go:build ai_tests

package plan

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/legacy/progrep"
	"github.com/werf/nelm/pkg/resource"
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

func installableInfoToDeleteOnAnyOutcome(name string) *InstallableResourceInfo {
	info := installableInfoToDeleteOnSuccessfulInstall(name)
	info.MustDeleteOnFailedInstall = true

	return info
}

func createConfigMapOp(name string) *Operation {
	return &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec(name, "", gvkConfigMap)},
	}
}

func installableInfoToDeleteOnFailedInstall(name string) *InstallableResourceInfo {
	info := installableInfoNamed(name, ResourceInstallTypeApply)
	info.MustDeleteOnFailedInstall = true

	return info
}

func installableInfoToDeleteOnSuccessfulInstall(name string) *InstallableResourceInfo {
	info := installableInfoNamed(name, ResourceInstallTypeApply)
	info.MustDeleteOnSuccessfulInstall = true

	return info
}

func realInstallableInfo(name string, stage common.Stage) *InstallableResourceInfo {
	return &InstallableResourceInfo{
		ResourceMeta:       makeResourceMeta(name, "default", gvkConfigMap),
		LocalResource:      &resource.InstallableResource{ResourceSpec: makeResourceSpec(name, "default", gvkConfigMap)},
		MustInstall:        ResourceInstallTypeApply,
		MustTrackReadiness: true,
		Stage:              stage,
	}
}

func deletableInfoWithUID(uid types.UID) *DeletableResourceInfo {
	return &DeletableResourceInfo{GetResult: unstructuredWithUID(uid)}
}

func installableInfoNamed(name string, installType ResourceInstallType, policies ...common.ResourcePolicy) *InstallableResourceInfo {
	return &InstallableResourceInfo{
		ResourceMeta:  makeResourceMeta(name, "test-namespace", gvkConfigMap),
		LocalResource: &resource.InstallableResource{ResourcePolicies: policies},
		MustInstall:   installType,
	}
}

func installableInfoWithUID(uid types.UID) *InstallableResourceInfo {
	return &InstallableResourceInfo{GetResult: unstructuredWithUID(uid)}
}

func lastReportOperations(t *testing.T, ch <-chan progrep.ProgressReport) []progrep.Operation {
	t.Helper()

	reports := drainChannel(ch)
	require.NotEmpty(t, reports, "expected at least one report")

	return reports[len(reports)-1].Operations
}

func makeResourceSpec(name, namespace string, gvk schema.GroupVersionKind) *spec.ResourceSpec {
	return &spec.ResourceSpec{
		ResourceMeta: makeResourceMeta(name, namespace, gvk),
	}
}

func makeUntouchedInfo(name, namespace string, gvk schema.GroupVersionKind) *InstallableResourceInfo {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(namespace)

	return &InstallableResourceInfo{
		ResourceMeta: makeResourceMeta(name, namespace, gvk),
		MustInstall:  ResourceInstallTypeNone,
		GetResult:    obj,
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

// graphRootsAndSinks treats the given operations as a self-contained graph: roots have no
// dependencies among them, sinks are not depended upon by any of them.
func graphRootsAndSinks(ops []progrep.Operation) ([]string, []string) {
	ids := lo.SliceToMap(ops, func(op progrep.Operation) (string, struct{}) {
		return op.ID, struct{}{}
	})

	dependedUpon := make(map[string]struct{})

	var roots []string
	for _, op := range ops {
		internalDeps := lo.Filter(op.DependsOn, func(id string, _ int) bool {
			return lo.HasKey(ids, id)
		})

		if len(internalDeps) == 0 {
			roots = append(roots, op.ID)
		}

		for _, id := range internalDeps {
			dependedUpon[id] = struct{}{}
		}
	}

	var sinks []string
	for _, op := range ops {
		if !lo.HasKey(dependedUpon, op.ID) {
			sinks = append(sinks, op.ID)
		}
	}

	sort.Strings(roots)
	sort.Strings(sinks)

	return roots, sinks
}

func makeResourceMeta(name, namespace string, gvk schema.GroupVersionKind) *spec.ResourceMeta {
	return &spec.ResourceMeta{
		Name:             name,
		Namespace:        namespace,
		GroupVersionKind: gvk,
	}
}

func operationIDs(ops []progrep.Operation) []string {
	return lo.Map(ops, func(op progrep.Operation, _ int) string {
		return op.ID
	})
}

func operationsByID(ops []progrep.Operation) map[string]progrep.Operation {
	return lo.SliceToMap(ops, func(op progrep.Operation) (string, progrep.Operation) {
		return op.ID, op
	})
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

func reachesThroughDependsOn(byID map[string]progrep.Operation, from, to string) bool {
	visited := make(map[string]struct{})
	queue := []string{from}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		if id == to {
			return true
		}

		if _, ok := visited[id]; ok {
			continue
		}

		visited[id] = struct{}{}
		queue = append(queue, byID[id].DependsOn...)
	}

	return false
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

func stageMetaOp(opID string) *Operation {
	return &Operation{
		Type: OperationTypeNoop, Version: OperationVersionNoop, Category: OperationCategoryMeta,
		Config: &OperationConfigNoop{OpID: opID},
	}
}

func startTestPlan(reporter *LegacyProgressReporter, p *Plan, untouched []*InstallableResourceInfo, opts StartPlanOptions) {
	reporter.StartPlan(p, "default", untouched, newFakeRESTMapper(), opts)
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

func unstructuredWithUID(uid types.UID) *unstructured.Unstructured {
	unstruct := &unstructured.Unstructured{Object: map[string]interface{}{}}
	unstruct.SetUID(uid)

	return unstruct
}
