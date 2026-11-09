package util

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

func TestProcessDependencies_ExportsRootGlobalsToNestedDependency(t *testing.T) {
	c := &chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion:   chart.APIVersionV2,
			Name:         "parent",
			Version:      "0.1.0",
			Dependencies: []*chart.Dependency{{Name: "mid", Version: "0.1.0"}},
		},
		Values:      map[string]any{"global": map[string]any{"root": "parent-values", "both": "parent-values"}},
		ExtraValues: map[string]any{"global": map[string]any{"service": "service-values"}},
	}
	mid := &chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion: chart.APIVersionV2,
			Name:       "mid",
			Version:    "0.1.0",
			Dependencies: []*chart.Dependency{{Name: "leaf", Version: "0.1.0", ExportValues: []interface{}{
				map[string]interface{}{"parent": "global.root", "child": "fromRoot"},
				map[string]interface{}{"parent": "global.mid", "child": "fromMid"},
				map[string]interface{}{"parent": "global.both", "child": "fromBoth"},
				map[string]interface{}{"parent": "global.user", "child": "fromUser"},
				map[string]interface{}{"parent": "global.service", "child": "fromService"},
			}}},
		},
		Values: map[string]any{"global": map[string]any{"mid": "mid-values", "both": "mid-values"}},
	}
	mid.AddDependency(&chart.Chart{Metadata: &chart.Metadata{APIVersion: chart.APIVersionV2, Name: "leaf", Version: "0.1.0"}})
	c.AddDependency(mid)

	vals := map[string]interface{}{"global": map[string]interface{}{"user": "user-values"}}
	require.NoError(t, ProcessDependencies(c, &vals))

	leafValues := c.Dependencies()[0].Dependencies()[0].Values
	assert.Equal(t, "parent-values", leafValues["fromRoot"])
	assert.Equal(t, "mid-values", leafValues["fromMid"])
	assert.Equal(t, "parent-values", leafValues["fromBoth"])

	leafUserValues, found := userValueAtPath(vals, "mid.leaf")
	require.True(t, found)
	assert.Equal(t, map[string]interface{}{"fromUser": "user-values", "fromService": "service-values"}, leafUserValues)
}

func TestProcessDependencies_ExportsIndependentOfDependencyOrder(t *testing.T) {
	reader := &chart.Dependency{Name: "a", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": "b.foo", "child": "fromB"}}}
	writer := &chart.Dependency{Name: "b", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": "g.foo", "child": "foo"}}}

	tests := []struct {
		name string
		deps []*chart.Dependency
	}{
		{name: "reader listed before writer", deps: []*chart.Dependency{reader, writer}},
		{name: "writer listed before reader", deps: []*chart.Dependency{writer, reader}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &chart.Chart{
				Metadata: &chart.Metadata{
					APIVersion:   chart.APIVersionV2,
					Name:         "parent",
					Version:      "0.1.0",
					Dependencies: lo.Map(tt.deps, func(d *chart.Dependency, _ int) *chart.Dependency { return lo.ToPtr(*d) }),
				},
				Values: map[string]any{"g": map[string]any{"foo": "parent-values"}},
			}
			c.AddDependency(&chart.Chart{Metadata: &chart.Metadata{APIVersion: chart.APIVersionV2, Name: "a", Version: "0.1.0"}, Values: map[string]any{"fromB": "a-values"}})
			c.AddDependency(&chart.Chart{Metadata: &chart.Metadata{APIVersion: chart.APIVersionV2, Name: "b", Version: "0.1.0"}, Values: map[string]any{"foo": "b-values"}})

			require.NoError(t, ProcessDependencies(c, &map[string]interface{}{}))

			a, found := lo.Find(c.Dependencies(), func(d *chart.Chart) bool { return d.Name() == "a" })
			require.True(t, found)
			assert.Equal(t, "parent-values", a.Values["fromB"])

			b, found := lo.Find(c.Dependencies(), func(d *chart.Chart) bool { return d.Name() == "b" })
			require.True(t, found)
			assert.Equal(t, "parent-values", b.Values["foo"])
		})
	}
}

func TestProcessDependencies_ExportsScalarThenMapToNestedDependencyRoot(t *testing.T) {
	c := &chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion: chart.APIVersionV2,
			Name:       "parent",
			Version:    "0.1.0",
			Dependencies: []*chart.Dependency{{Name: "mid", Version: "0.1.0", ExportValues: []interface{}{
				map[string]interface{}{"parent": "first", "child": "leaf"},
				map[string]interface{}{"parent": "second", "child": "leaf"},
			}}},
		},
		Values: map[string]any{"first": "scalar", "second": map[string]any{"k": "parent-values"}},
	}
	mid := &chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion:   chart.APIVersionV2,
			Name:         "mid",
			Version:      "0.1.0",
			Dependencies: []*chart.Dependency{{Name: "leaf", Version: "0.1.0"}},
		},
	}
	mid.AddDependency(&chart.Chart{Metadata: &chart.Metadata{APIVersion: chart.APIVersionV2, Name: "leaf", Version: "0.1.0"}})
	c.AddDependency(mid)

	require.NoError(t, ProcessDependencies(c, &map[string]interface{}{}))

	leafValue, found := userValueAtPath(c.Dependencies()[0].Values, "leaf.k")
	require.True(t, found)
	assert.Equal(t, "parent-values", leafValue)
}

func TestProcessDependencies_ExportsChainOfManyDependencies(t *testing.T) {
	c := chainOfExportsChart(200)
	c.Values = map[string]any{"src": "exported"}

	require.NoError(t, ProcessDependencies(c, &map[string]interface{}{}))

	for _, dep := range c.Dependencies() {
		assert.Equal(t, "exported", dep.Values["out"], dep.Name())
	}
}
