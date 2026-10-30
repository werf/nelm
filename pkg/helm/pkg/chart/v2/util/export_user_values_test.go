package util

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

func TestExportUserValues(t *testing.T) {
	tests := []struct {
		name         string
		deps         []*chart.Dependency
		chartVersion string
		vals         map[string]interface{}
		want         map[string]interface{}
	}{
		{
			name: "dependency listed before exporting dependency",
			deps: []*chart.Dependency{
				{Name: "b", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": "a.image", "child": "repo"}}},
				{Name: "a", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": "g", "child": "image"}}},
			},
			vals: map[string]interface{}{"g": "exported"},
			want: map[string]interface{}{
				"g": "exported",
				"a": map[string]interface{}{"image": "exported"},
				"b": map[string]interface{}{"repo": "exported"},
			},
		},
		{
			name: "dependency listed before dependency exporting null to its parent path",
			deps: []*chart.Dependency{
				{Name: "b", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": "a.image.repository", "child": "repo"}}},
				{Name: "a", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": "g", "child": "image"}}},
			},
			vals: map[string]interface{}{"g": nil, "a": map[string]interface{}{"image": map[string]interface{}{"repository": "child"}}},
			want: map[string]interface{}{"g": nil, "a": map[string]interface{}{"image": nil}},
		},
		{
			name: "later export to same child path wins",
			deps: []*chart.Dependency{
				{Name: "a", Version: "0.1.0", ExportValues: []interface{}{
					map[string]interface{}{"parent": "b.out", "child": "image"},
					map[string]interface{}{"parent": "g", "child": "image"},
				}},
				{Name: "b", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": "h", "child": "out"}}},
			},
			vals: map[string]interface{}{"g": "g", "h": "h"},
			want: map[string]interface{}{
				"g": "g",
				"h": "h",
				"a": map[string]interface{}{"image": "g"},
				"b": map[string]interface{}{"out": "h"},
			},
		},
		{
			name: "exports to child paths under their parent path are applied once",
			deps: []*chart.Dependency{
				{Name: "a", Version: "0.1.0", ExportValues: []interface{}{
					map[string]interface{}{"parent": "a", "child": "x"},
					map[string]interface{}{"parent": "a", "child": "y"},
				}},
			},
			vals: map[string]interface{}{"a": map[string]interface{}{"seed": "seed"}},
			want: map[string]interface{}{"a": map[string]interface{}{
				"seed": "seed",
				"x":    map[string]interface{}{"seed": "seed"},
				"y":    map[string]interface{}{"seed": "seed", "x": map[string]interface{}{"seed": "seed"}},
			}},
		},
		{
			name: "dependency chart out of version range",
			deps: []*chart.Dependency{
				{Name: "a", Version: "~1.0.0", ExportValues: []interface{}{map[string]interface{}{"parent": "g", "child": "image"}}},
			},
			chartVersion: "2.0.0",
			vals:         map[string]interface{}{"g": "exported"},
			want:         map[string]interface{}{"g": "exported", "a": map[string]interface{}{"image": "exported"}},
		},
		{
			name: "alias dependency chart out of version range",
			deps: []*chart.Dependency{
				{Name: "a", Alias: "al", Version: "~1.0.0", ExportValues: []interface{}{map[string]interface{}{"parent": "g", "child": "image"}}},
			},
			chartVersion: "2.0.0",
			vals:         map[string]interface{}{"g": "exported"},
			want:         map[string]interface{}{"g": "exported"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &chart.Chart{Metadata: &chart.Metadata{Name: "parent", Version: "0.1.0", APIVersion: chart.APIVersionV2, Dependencies: tt.deps}}
			for _, dep := range tt.deps {
				parent.AddDependency(&chart.Chart{Metadata: &chart.Metadata{Name: dep.Name, Version: lo.Ternary(tt.chartVersion != "", tt.chartVersion, dep.Version), APIVersion: chart.APIVersionV2}})
			}

			got, _, err := ExportUserValues(parent, tt.vals, ExportUserValuesOptions{})
			require.NoError(t, err)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExportUserValuesCycle(t *testing.T) {
	deps := []*chart.Dependency{
		{Name: "a", Version: "0.1.0", ExportValues: []interface{}{
			map[string]interface{}{"parent": "a.z", "child": "x"},
			map[string]interface{}{"parent": "a.y", "child": "z"},
			map[string]interface{}{"parent": "a.x", "child": "y"},
		}},
	}
	parent := &chart.Chart{Metadata: &chart.Metadata{Name: "parent", Version: "0.1.0", APIVersion: chart.APIVersionV2, Dependencies: deps}}
	parent.AddDependency(&chart.Chart{Metadata: &chart.Metadata{Name: "a", Version: "0.1.0", APIVersion: chart.APIVersionV2}})

	got, _, err := ExportUserValues(parent, map[string]interface{}{"a": map[string]interface{}{"x": "x", "y": "y", "z": "z"}}, ExportUserValuesOptions{})
	require.NoError(t, err)

	assert.Equal(t, map[string]interface{}{"a": map[string]interface{}{"x": "z", "y": "z", "z": "z"}}, got)
}

func TestExportUserValuesDependencyChartsInAndOutOfVersionRange(t *testing.T) {
	deps := []*chart.Dependency{
		{Name: "a", Version: "~1.0.0", ExportValues: []interface{}{map[string]interface{}{"parent": "a", "child": "x"}}},
	}
	parent := &chart.Chart{Metadata: &chart.Metadata{Name: "parent", Version: "0.1.0", APIVersion: chart.APIVersionV2, Dependencies: deps}}
	parent.AddDependency(&chart.Chart{Metadata: &chart.Metadata{Name: "a", Version: "1.0.0", APIVersion: chart.APIVersionV2}})
	parent.AddDependency(&chart.Chart{Metadata: &chart.Metadata{Name: "a", Version: "2.0.0", APIVersion: chart.APIVersionV2}})

	got, _, err := ExportUserValues(parent, map[string]interface{}{"a": map[string]interface{}{"seed": "seed"}}, ExportUserValuesOptions{})
	require.NoError(t, err)

	assert.Equal(t, map[string]interface{}{"a": map[string]interface{}{"seed": "seed", "x": map[string]interface{}{"seed": "seed"}}}, got)
}

func TestExportUserValuesDependencyChartOutOfVersionRangeUsedByAlias(t *testing.T) {
	deps := []*chart.Dependency{
		{Name: "a", Version: "~1.0.0", ExportValues: []interface{}{map[string]interface{}{"parent": "g", "child": "enabled"}}},
		{Name: "a", Alias: "al", Version: "~2.0.0", Condition: "a.enabled"},
	}
	parent := &chart.Chart{Metadata: &chart.Metadata{Name: "parent", Version: "0.1.0", APIVersion: chart.APIVersionV2, Dependencies: deps}}
	parent.AddDependency(&chart.Chart{Metadata: &chart.Metadata{Name: "a", Version: "2.0.0", APIVersion: chart.APIVersionV2}})

	got, _, err := ExportUserValues(parent, map[string]interface{}{"g": true, "a": map[string]interface{}{"enabled": false}}, ExportUserValuesOptions{})
	require.NoError(t, err)

	assert.Equal(t, map[string]interface{}{"g": true, "a": map[string]interface{}{"enabled": false}}, got)
}

func TestExportUserValuesGlobalValuesOfNestedChart(t *testing.T) {
	tests := []struct {
		name            string
		parent          string
		vals            map[string]interface{}
		want            interface{}
		wantNotExported bool
	}{
		{
			name:   "root chart global",
			parent: "global.img",
			vals:   map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": "root"}}},
			want:   map[string]interface{}{"repo": "root"},
		},
		{
			name:   "intermediate chart global",
			parent: "global.img",
			vals:   map[string]interface{}{"a": map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": "a"}}}},
			want:   map[string]interface{}{"repo": "a"},
		},
		{
			name:   "higher chart globals are merged over lower chart globals",
			parent: "global.img",
			vals: map[string]interface{}{
				"global": map[string]interface{}{"img": map[string]interface{}{"repo": "root"}},
				"a": map[string]interface{}{
					"global": map[string]interface{}{"img": map[string]interface{}{"repo": "a", "tag": "a"}},
					"b":      map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": "b", "tag": "b", "pullPolicy": "b"}}},
				},
			},
			want: map[string]interface{}{"repo": "root", "tag": "a", "pullPolicy": "b"},
		},
		{
			name:   "higher chart global scalar overrides lower chart global map",
			parent: "global.img",
			vals: map[string]interface{}{
				"global": map[string]interface{}{"img": map[string]interface{}{"repo": "root"}},
				"a":      map[string]interface{}{"b": map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": map[string]interface{}{"name": "b"}}}}},
			},
			want: map[string]interface{}{"repo": "root"},
		},
		{
			name:   "higher chart global null overrides lower chart global map",
			parent: "global.img",
			vals: map[string]interface{}{
				"global": map[string]interface{}{"img": map[string]interface{}{"repo": nil}},
				"a":      map[string]interface{}{"b": map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": map[string]interface{}{"name": "b"}}}}},
			},
			want: map[string]interface{}{"repo": nil},
		},
		{
			name:   "higher chart global null shadows lower chart global value",
			parent: "global.img.repo.name",
			vals: map[string]interface{}{
				"global": map[string]interface{}{"img": map[string]interface{}{"repo": nil}},
				"a":      map[string]interface{}{"b": map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": map[string]interface{}{"name": "b"}}}}},
			},
			wantNotExported: true,
		},
		{
			name:   "higher chart global scalar shadows lower chart global value",
			parent: "global.img.repo.name",
			vals: map[string]interface{}{
				"global": map[string]interface{}{"img": map[string]interface{}{"repo": "root"}},
				"a":      map[string]interface{}{"b": map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": map[string]interface{}{"name": "b"}}}}},
			},
			wantNotExported: true,
		},
		{
			name:   "lower chart global map is kept on type conflict of top-level global key",
			parent: "global.img",
			vals: map[string]interface{}{
				"global": map[string]interface{}{"img": "root"},
				"a":      map[string]interface{}{"b": map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": "b"}}}},
			},
			want: map[string]interface{}{"repo": "b"},
		},
		{
			name:   "lower chart global scalar is kept on type conflict of top-level global key",
			parent: "global.img",
			vals: map[string]interface{}{
				"global": map[string]interface{}{"img": map[string]interface{}{"repo": "root"}},
				"a":      map[string]interface{}{"b": map[string]interface{}{"global": map[string]interface{}{"img": "b"}}},
			},
			want: "b",
		},
		{
			name:   "non-map intermediate chart global stops root chart global",
			parent: "global.img",
			vals: map[string]interface{}{
				"global": map[string]interface{}{"img": map[string]interface{}{"repo": "root", "tag": "root"}},
				"a": map[string]interface{}{
					"global": "a",
					"b":      map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": "b"}}},
				},
			},
			want: map[string]interface{}{"repo": "b"},
		},
		{
			name:   "non-map chart global is not exported",
			parent: "global.img",
			vals: map[string]interface{}{
				"global": map[string]interface{}{"img": map[string]interface{}{"repo": "root"}},
				"a":      map[string]interface{}{"b": map[string]interface{}{"global": nil}},
			},
			wantNotExported: true,
		},
		{
			name:   "non-map root chart global is ignored",
			parent: "global.img",
			vals: map[string]interface{}{
				"global": "root",
				"a":      map[string]interface{}{"b": map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": "b"}}}},
			},
			want: map[string]interface{}{"repo": "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &chart.Chart{Metadata: &chart.Metadata{Name: "b", Version: "0.1.0", APIVersion: chart.APIVersionV2, Dependencies: []*chart.Dependency{
				{Name: "leaf", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": tt.parent, "child": "image"}}},
			}}}
			b.AddDependency(&chart.Chart{Metadata: &chart.Metadata{Name: "leaf", Version: "0.1.0", APIVersion: chart.APIVersionV2}})

			a := &chart.Chart{Metadata: &chart.Metadata{Name: "a", Version: "0.1.0", APIVersion: chart.APIVersionV2, Dependencies: []*chart.Dependency{{Name: "b", Version: "0.1.0"}}}}
			a.AddDependency(b)

			parent := &chart.Chart{Metadata: &chart.Metadata{Name: "parent", Version: "0.1.0", APIVersion: chart.APIVersionV2, Dependencies: []*chart.Dependency{{Name: "a", Version: "0.1.0"}}}}
			parent.AddDependency(a)

			valsBefore, err := copyUserValues(tt.vals)
			require.NoError(t, err)

			got, _, err := ExportUserValues(parent, tt.vals, ExportUserValuesOptions{})
			require.NoError(t, err)

			value, found := userValueAtPath(got, "a.b.leaf.image")
			assert.Equal(t, !tt.wantNotExported, found)
			assert.Equal(t, tt.want, value)
			assert.Equal(t, valsBefore, tt.vals)
		})
	}
}

func TestExportUserValuesGlobalValueOfNestedChartBeforeExportReadingIt(t *testing.T) {
	a := &chart.Chart{Metadata: &chart.Metadata{Name: "a", Version: "0.1.0", APIVersion: chart.APIVersionV2, Dependencies: []*chart.Dependency{
		{Name: "b", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": "global.foo", "child": "bar"}}},
	}}}
	a.AddDependency(&chart.Chart{Metadata: &chart.Metadata{Name: "b", Version: "0.1.0", APIVersion: chart.APIVersionV2}})

	parent := &chart.Chart{Metadata: &chart.Metadata{Name: "parent", Version: "0.1.0", APIVersion: chart.APIVersionV2, Dependencies: []*chart.Dependency{
		{Name: "a", Version: "0.1.0", ExportValues: []interface{}{map[string]interface{}{"parent": "a.b.bar", "child": "global.baz"}}},
	}}}
	parent.AddDependency(a)

	got, _, err := ExportUserValues(parent, map[string]interface{}{"global": map[string]interface{}{"foo": "foo"}}, ExportUserValuesOptions{})
	require.NoError(t, err)

	assert.Equal(t, map[string]interface{}{
		"global": map[string]interface{}{"foo": "foo"},
		"a":      map[string]interface{}{"global": map[string]interface{}{"baz": "foo"}, "b": map[string]interface{}{"bar": "foo"}},
	}, got)
}
