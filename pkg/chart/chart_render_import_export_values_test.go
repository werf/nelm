package chart

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/helm/pkg/cli/values"
	"github.com/werf/nelm/v2/pkg/helm/pkg/getter"
)

func TestRenderChart_ExportValuesFromValuesLayers(t *testing.T) {
	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want map[string]interface{}
	}{
		{
			name: "values.yaml",
			want: map[string]interface{}{
				"sub.image.repository":     "parent-values-yaml",
				"sub.deep.repo":            "parent-values-yaml",
				"sub.m":                    map[string]interface{}{"a": "parent-values-yaml"},
				"aliased.image.repository": "parent-values-yaml",
			},
		},
		{
			name: "plain value from set",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=set"}},
			want: map[string]interface{}{
				"sub.image.repository":     "set",
				"sub.deep.repo":            "set",
				"aliased.image.repository": "parent-values-yaml",
			},
		},
		{
			name: "map from set is merged with lower layers",
			opts: layeredValuesRenderOptions{Set: []string{"g.map.b=set"}},
			want: map[string]interface{}{
				"sub.m": map[string]interface{}{"a": "parent-values-yaml", "b": "set"},
			},
		},
		{
			name: "plain value from set to aliased subchart",
			opts: layeredValuesRenderOptions{Set: []string{"g.aliasRepo=set"}},
			want: map[string]interface{}{
				"sub.image.repository":     "parent-values-yaml",
				"aliased.image.repository": "set",
				"aliased.deep.repo":        "set",
			},
		},
		{
			name: "value from set in subchart to its subchart",
			opts: layeredValuesRenderOptions{Set: []string{"sub.image.repository=set"}},
			want: map[string]interface{}{
				"sub.image.repository": "set",
				"sub.deep.repo":        "set",
			},
		},
		{
			name: "null from set is exported",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=null"}},
			want: map[string]interface{}{
				"sub.image.repository":     absentValue{},
				"sub.deep.repo":            absentValue{},
				"aliased.image.repository": "parent-values-yaml",
			},
		},
		{
			name: "exported value from set overrides child value from set",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=set-parent", "sub.image.repository=set-child"}},
			want: map[string]interface{}{
				"sub.image.repository": "set-parent",
				"sub.deep.repo":        "set-parent",
			},
		},
		{
			name: "null from set for subchart",
			opts: layeredValuesRenderOptions{Set: []string{"sub=null"}},
			want: map[string]interface{}{
				"sub.image.repository": "parent-values-yaml",
				"sub.deep.repo":        "parent-values-yaml",
			},
		},
		{
			name: "exported value from set replaces null from set for subchart",
			opts: layeredValuesRenderOptions{
				Set:         []string{"sub=null", "g.repo=set"},
				ValuesFiles: []string{layeredImportExportValuesFile},
			},
			want: map[string]interface{}{
				"sub.image.repository": "set",
				"sub.out.k":            "values-file",
			},
		},
		{
			name: "values file",
			opts: layeredValuesRenderOptions{ValuesFiles: []string{layeredImportExportValuesFile}},
			want: map[string]interface{}{
				"sub.image.repository":     "values-file",
				"sub.deep.repo":            "values-file",
				"sub.m":                    map[string]interface{}{"a": "parent-values-yaml", "b": "values-file"},
				"aliased.image.repository": "values-file",
			},
		},
		{
			name: "secret values file",
			opts: layeredValuesRenderOptions{SecretValuesFiles: []string{layeredImportExportSecretValuesFile}},
			want: map[string]interface{}{
				"sub.image.repository": "secret-values-file",
				"sub.deep.repo":        "secret-values-file",
				"sub.m":                map[string]interface{}{"a": "parent-values-yaml", "b": "secret-values-file"},
			},
		},
		{
			name: "service values",
			opts: layeredValuesRenderOptions{ServiceValues: map[string]interface{}{"g": map[string]interface{}{"repo": "service", "map": map[string]interface{}{"b": "service"}}}},
			want: map[string]interface{}{
				"sub.image.repository": "service",
				"sub.deep.repo":        "service",
				"sub.m":                map[string]interface{}{"a": "parent-values-yaml", "b": "service"},
			},
		},
		{
			name: "child value from set overrides exported value from values file",
			opts: layeredValuesRenderOptions{
				Set:         []string{"sub.image.repository=set"},
				ValuesFiles: []string{layeredImportExportValuesFile},
			},
			want: map[string]interface{}{
				"sub.image.repository": "set",
				"sub.deep.repo":        "set",
			},
		},
		{
			name: "exported value from service values overrides child value from set",
			opts: layeredValuesRenderOptions{
				ServiceValues: map[string]interface{}{"g": map[string]interface{}{"repo": "service"}},
				Set:           []string{"sub.image.repository=set"},
			},
			want: map[string]interface{}{
				"sub.image.repository": "service",
				"sub.deep.repo":        "service",
			},
		},
		{
			name: "exported value from set overrides exported value from secret values file",
			opts: layeredValuesRenderOptions{
				SecretValuesFiles: []string{layeredImportExportSecretValuesFile},
				Set:               []string{"g.repo=set"},
			},
			want: map[string]interface{}{
				"sub.image.repository": "set",
				"sub.m":                map[string]interface{}{"a": "parent-values-yaml", "b": "secret-values-file"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ChartDir = "import-export-chart"
			vals := renderedChartValues(t, renderLayeredValuesChart(t, tt.opts))

			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}
		})
	}
}

func TestRenderChart_ExportedValueEnablesSubchart(t *testing.T) {
	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want []string
	}{
		{name: "not exported", want: []string{"aliased", "sub"}},
		{name: "exported from set", opts: layeredValuesRenderOptions{Set: []string{"toggle=true"}}, want: []string{"aliased", "sub", "toggled"}},
		{name: "exported from values file", opts: layeredValuesRenderOptions{ValuesFiles: []string{layeredImportExportValuesFile}}, want: []string{"aliased", "sub", "toggled"}},
		{
			name: "child value from set overrides exported value from values file",
			opts: layeredValuesRenderOptions{Set: []string{"toggled.enabled=false"}, ValuesFiles: []string{layeredImportExportValuesFile}},
			want: []string{"aliased", "sub"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ChartDir = "import-export-chart"

			assert.Equal(t, tt.want, dependencyNames(t, renderLayeredValuesChart(t, tt.opts)))
		})
	}
}

func TestRenderChart_ImportValuesFromValuesLayers(t *testing.T) {
	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want map[string]interface{}
	}{
		{
			name: "values.yaml",
			want: map[string]interface{}{
				"imported":      map[string]interface{}{"k": "sub-values-yaml", "fromDeep": map[string]interface{}{"k": "deep-values-yaml"}},
				"importedAlias": map[string]interface{}{"k": "sub-values-yaml", "fromDeep": map[string]interface{}{"k": "deep-values-yaml"}},
				"d":             "sub-values-yaml",
			},
		},
		{
			name: "map from set is merged with lower layers",
			opts: layeredValuesRenderOptions{Set: []string{"sub.out.k=set", "sub.out.extra=set"}},
			want: map[string]interface{}{
				"imported":        map[string]interface{}{"k": "set", "extra": "set", "fromDeep": map[string]interface{}{"k": "deep-values-yaml"}},
				"importedAlias.k": "sub-values-yaml",
			},
		},
		{
			name: "map from set in aliased subchart",
			opts: layeredValuesRenderOptions{Set: []string{"aliased.out.k=set"}},
			want: map[string]interface{}{
				"imported.k":      "sub-values-yaml",
				"importedAlias.k": "set",
			},
		},
		{
			name: "exports from set",
			opts: layeredValuesRenderOptions{Set: []string{"sub.exports.data.d=set"}},
			want: map[string]interface{}{
				"d": "set",
			},
		},
		{
			name: "map from set in subchart of subchart",
			opts: layeredValuesRenderOptions{Set: []string{"sub.deep.out.k=set"}},
			want: map[string]interface{}{
				"sub.out.fromDeep.k":       "set",
				"imported.fromDeep.k":      "set",
				"importedAlias.fromDeep.k": "deep-values-yaml",
			},
		},
		{
			name: "null from set",
			opts: layeredValuesRenderOptions{Set: []string{"sub.out=null"}},
			want: map[string]interface{}{
				"imported":        absentValue{},
				"importedAlias.k": "sub-values-yaml",
			},
		},
		{
			name: "parent value from set overrides imported value from set",
			opts: layeredValuesRenderOptions{Set: []string{"sub.out.k=set-child", "imported.k=set-parent"}},
			want: map[string]interface{}{
				"imported.k": "set-parent",
			},
		},
		{
			name: "values file",
			opts: layeredValuesRenderOptions{ValuesFiles: []string{layeredImportExportValuesFile}},
			want: map[string]interface{}{
				"imported.k":      "values-file",
				"importedAlias.k": "values-file",
			},
		},
		{
			name: "secret values file",
			opts: layeredValuesRenderOptions{SecretValuesFiles: []string{layeredImportExportSecretValuesFile}},
			want: map[string]interface{}{
				"imported.k": "secret-values-file",
			},
		},
		{
			name: "service values",
			opts: layeredValuesRenderOptions{ServiceValues: map[string]interface{}{"sub": map[string]interface{}{"out": map[string]interface{}{"k": "service"}}}},
			want: map[string]interface{}{
				"imported.k": "service",
			},
		},
		{
			name: "imported value from service values overrides parent value from set",
			opts: layeredValuesRenderOptions{
				ServiceValues: map[string]interface{}{"sub": map[string]interface{}{"out": map[string]interface{}{"k": "service"}}},
				Set:           []string{"imported.k=set"},
			},
			want: map[string]interface{}{
				"imported.k": "service",
			},
		},
		{
			name: "parent value from service values overrides imported value from set",
			opts: layeredValuesRenderOptions{
				ServiceValues: map[string]interface{}{"imported": map[string]interface{}{"k": "service"}},
				Set:           []string{"sub.out.k=set"},
			},
			want: map[string]interface{}{
				"imported.k": "service",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ChartDir = "import-export-chart"
			vals := renderedChartValues(t, renderLayeredValuesChart(t, tt.opts))

			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}
		})
	}
}

func TestRenderChart_ReleaseConfigWithExportedValuesWithoutServiceAndImportedValues(t *testing.T) {
	result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
		ChartDir:          "import-export-chart",
		SecretValuesFiles: []string{layeredImportExportSecretValuesFile},
		ServiceValues:     map[string]interface{}{"g": map[string]interface{}{"aliasRepo": "service"}, "service": "service"},
		Set:               []string{"g.repo=set", "sub.out.k=set"},
		ValuesFiles:       []string{layeredImportExportValuesFile},
	})

	for path, want := range map[string]interface{}{
		"g.repo":                   "set",
		"g.aliasRepo":              "values-file",
		"g.map":                    map[string]interface{}{"b": "secret-values-file"},
		"sub.image.repository":     "set",
		"sub.deep.repo":            "set",
		"sub.m":                    map[string]interface{}{"b": "secret-values-file"},
		"sub.out.k":                "set",
		"aliased.image.repository": "values-file",
		"toggled.enabled":          true,
		"secretObj.old":            "secret-values-file",
		"service":                  absentValue{},
		"imported":                 absentValue{},
		"importedAlias":            absentValue{},
	} {
		assert.Equal(t, want, valueAtPath(t, result.ReleaseConfig, path), path)
	}
}

func TestRenderChart_SetReplacesOrMergesMapsFromFiles(t *testing.T) {
	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want map[string]interface{}
	}{
		{
			name: "set merges into map from values file",
			opts: layeredValuesRenderOptions{Set: []string{"obj.new=set"}},
			want: map[string]interface{}{"obj": map[string]interface{}{"old": "values-file", "new": "set"}},
		},
		{
			name: "set-json object merges into map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`{"obj":{"new":"set"}}`}},
			want: map[string]interface{}{"obj": map[string]interface{}{"old": "values-file", "new": "set"}},
		},
		{
			name: "set-json replaces map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`obj={"new":"set"}`}},
			want: map[string]interface{}{"obj": map[string]interface{}{"new": "set"}},
		},
		{
			name: "set-json replaces map from values file with empty map",
			opts: layeredValuesRenderOptions{SetJSON: []string{`obj={}`}},
			want: map[string]interface{}{"obj": map[string]interface{}{}},
		},
		{
			name: "set-json replaces map from secret values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`secretObj={"new":"set"}`}},
			want: map[string]interface{}{"secretObj": map[string]interface{}{"new": "set"}},
		},
		{
			name: "set-json replaces parent map of exported map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`sub={"m":{"c":"set"}}`}},
			want: map[string]interface{}{"sub.m": map[string]interface{}{"a": "parent-values-yaml", "c": "set"}},
		},
		{
			name: "set-json replaces exported map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`sub.m={"c":"set"}`}},
			want: map[string]interface{}{"sub.m": map[string]interface{}{"a": "parent-values-yaml", "c": "set"}},
		},
		{
			name: "set-json replaces export parent map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`g.map={"c":"set"}`}},
			want: map[string]interface{}{"sub.m": map[string]interface{}{"a": "parent-values-yaml", "c": "set"}},
		},
		{
			name: "set-json replaces import child map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`sub.out={"z":"set"}`}},
			want: map[string]interface{}{"imported.k": "sub-values-yaml", "imported.z": "set"},
		},
		{
			name: "set-json replaces imported map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`imported={"z":"set"}`}},
			want: map[string]interface{}{"imported.k": "sub-values-yaml", "imported.z": "set"},
		},
		{
			name: "set-json object merges into map that replaced map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`obj=null`, `{"obj":{"new":"set"}}`}},
			want: map[string]interface{}{"obj": map[string]interface{}{"new": "set"}},
		},
		{
			name: "set replaces imported scalar from values file with map",
			opts: layeredValuesRenderOptions{Set: []string{"imported.k.n=set"}},
			want: map[string]interface{}{"imported.k": map[string]interface{}{"n": "set"}},
		},
		{
			name: "set-json map replaces list set before",
			opts: layeredValuesRenderOptions{SetJSON: []string{`newObj[0]=1`, `newObj={"new":"set"}`}},
			want: map[string]interface{}{"newObj": map[string]interface{}{"new": "set"}},
		},
		{
			name: "set-json merges into map below replaced map",
			opts: layeredValuesRenderOptions{SetJSON: []string{`obj={"new":"set"}`, `obj.newer={"x":"set"}`}},
			want: map[string]interface{}{"obj": map[string]interface{}{"new": "set", "newer": map[string]interface{}{"x": "set"}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ChartDir = "import-export-chart"
			tt.opts.ValuesFiles = []string{layeredImportExportValuesFile}
			tt.opts.SecretValuesFiles = []string{layeredImportExportSecretValuesFile}
			vals := renderedChartValues(t, renderLayeredValuesChart(t, tt.opts))

			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}
		})
	}
}

func TestSetValuesReplacedPaths(t *testing.T) {
	tests := []struct {
		name string
		opts values.Options
		want [][]string
	}{
		{name: "set-json map", opts: values.Options{JSONValues: []string{`a={"x":1}`}}, want: [][]string{{"a"}}},
		{name: "set-json nested map", opts: values.Options{JSONValues: []string{`a.b={}`}}, want: [][]string{{"a", "b"}}},
		{name: "set-json several maps", opts: values.Options{JSONValues: []string{`a={"x":1},b.c={"y":{"z":2}}`}}, want: [][]string{{"a"}, {"b", "c"}}},
		{name: "set-json object", opts: values.Options{JSONValues: []string{`{"a":{"b":1}}`}}},
		{name: "set-json null then object", opts: values.Options{JSONValues: []string{`a=null`, `{"a":{"b":1}}`}}, want: [][]string{{"a"}}},
		{name: "set-json map then set into it", opts: values.Options{JSONValues: []string{`a={"x":1}`}, Values: []string{"a.y=2"}}, want: [][]string{{"a"}}},
		{name: "set", opts: values.Options{Values: []string{"a.b=1", "c=null"}}},
		{name: "set-json list", opts: values.Options{JSONValues: []string{`a=[{"x":1}]`}}},
		{name: "set-json list then map", opts: values.Options{JSONValues: []string{`a[0]=1`, `a={"b":{"c":2}}`}}, want: [][]string{{"a"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setFilesCache := map[string]string{}
			applySet := func(base map[string]interface{}) (map[string]interface{}, error) {
				return tt.opts.MergeSetValues(context.Background(), getter.Providers{}, base, values.MergeSetValuesOptions{SetFilesCache: setFilesCache})
			}

			setPaths, err := applySet(map[string]interface{}{})
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.want, setValuesReplacedPaths(setPaths, applySet, nil))
		})
	}
}
