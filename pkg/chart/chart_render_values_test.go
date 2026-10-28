package chart

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderChart_ValuesLayersExportUserValues(t *testing.T) {
	tests := []struct {
		name              string
		secretValuesFiles []string
		want              interface{}
	}{
		{
			name: "exported values file table replaces scalar in values file",
			want: map[string]interface{}{"child": map[string]interface{}{"x": "values-file"}},
		},
		{
			name:              "exported secret values file table overrides values file table",
			secretValuesFiles: []string{layeredSecretValuesExportFile},
			want:              map[string]interface{}{"child": map[string]interface{}{"x": "secret-values-file"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
				ChartDir:          "export-over-scalar",
				ValuesFiles:       []string{layeredValuesExportFile},
				SecretValuesFiles: tt.secretValuesFiles,
			})

			assert.Equal(t, tt.want, renderedChartValues(t, result)["sub"].(map[string]interface{})["target"])
			assert.Equal(t, tt.want, result.ReleaseConfig["sub"].(map[string]interface{})["target"], "release config must have exported values")
		})
	}
}

func TestRenderChart_ValuesLayersExportValues(t *testing.T) {
	tests := []struct {
		name          string
		serviceValues map[string]interface{}
		want          string
	}{
		{name: "secret-values.yaml value is exported", want: "secret-values-yaml"},
		{name: "service value is exported", serviceValues: map[string]interface{}{"exp": "service"}, want: "service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderLayeredValuesChart(t, layeredValuesRenderOptions{ServiceValues: tt.serviceValues})
			vals := renderedChartValues(t, result)

			assert.Equal(t, tt.want, vals["sub"].(map[string]interface{})["e"])
		})
	}
}

func TestRenderChart_ValuesLayersPrecedence(t *testing.T) {
	result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
		ServiceValues:     map[string]interface{}{"p56": "service", "all": "service"},
		Set:               []string{"p45=set", "p56=set", "all=set", "sub.s3=set", "list[1]=set"},
		ValuesFiles:       []string{layeredValuesFile},
		SecretValuesFiles: []string{layeredSecretValuesFile},
	})
	vals := renderedChartValues(t, result)
	sub := vals["sub"].(map[string]interface{})

	tests := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{name: "secret-values.yaml overrides values.yaml", got: vals["p12"], want: "secret-values-yaml"},
		{name: "values file overrides secret-values.yaml", got: vals["p23"], want: "values-file"},
		{name: "secret values file overrides values file", got: vals["p34"], want: "secret-values-file"},
		{name: "set overrides secret values file", got: vals["p45"], want: "set"},
		{name: "service values override set", got: vals["p56"], want: "service"},
		{name: "service values override all layers", got: vals["all"], want: "service"},
		{name: "maps from different layers are merged", got: vals["nested"], want: map[string]interface{}{"a": "values-yaml", "b": "secret-values-yaml"}},
		{name: "set patches a list element from values file", got: vals["list"], want: []interface{}{"values-file-0", "set"}},
		{name: "subchart secret-values.yaml overrides subchart values.yaml", got: sub["s1"], want: "sub-secret-values-yaml"},
		{name: "parent values.yaml overrides subchart secret-values.yaml", got: sub["s2"], want: "root-values-yaml"},
		{name: "parent secret-values.yaml overrides parent values.yaml for subchart", got: sub["s4"], want: "root-secret-values-yaml"},
		{name: "key only in secret-values.yaml is set", got: vals["rootSecret"], want: "TOPSECRET-ROOT"},
		{name: "key only in subchart secret-values.yaml is set", got: sub["subSecret"], want: "TOPSECRET-SUB"},
		{name: "key only in secret values file is set", got: vals["customSecret"], want: "TOPSECRET-CUSTOM"},
		{name: "set overrides subchart secret-values.yaml", got: sub["s3"], want: "set"},
		{name: "secret-values.yaml enables subchart", got: dependencyNames(t, result), want: []string{"sub", "sub2"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.got)
		})
	}

	assert.Equal(t, map[string]interface{}{
		"p23":          "values-file",
		"p34":          "secret-values-file",
		"p45":          "set",
		"p56":          "set",
		"all":          "set",
		"customSecret": "TOPSECRET-CUSTOM",
		"del3":         "values-file",
		"del4":         "secret-values-file",
		"list":         []interface{}{"values-file-0", "set"},
		"sub":          map[string]interface{}{"s3": "set"},
	}, result.ReleaseConfig, "release config must have --values, --secret-values and --set values, without service values")
}

func TestRenderChart_ValuesLayersPrecedence_V3Chart(t *testing.T) {
	result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
		ChartDir:          "chart-v3",
		SecretValuesFiles: []string{layeredSecretValuesFile},
		ServiceValues:     map[string]interface{}{"p56": "service"},
		Set:               []string{"p45=set", "p56=set"},
	})
	vals := renderedChartValues(t, result)

	assert.Equal(t, "secret-values-yaml", vals["p12"], "secret-values.yaml overrides values.yaml")
	assert.Equal(t, "secret-values-file", vals["p34"], "secret values file overrides secret-values.yaml")
	assert.Equal(t, "set", vals["p45"], "set overrides secret values file")
	assert.Equal(t, "service", vals["p56"], "service values override set")
	assert.Equal(t, "TOPSECRET-ROOT", vals["rootSecret"], "key only in secret-values.yaml is set")
	assert.Equal(t, map[string]interface{}{
		"p34":          "secret-values-file",
		"p45":          "set",
		"p56":          "set",
		"all":          "secret-values-file",
		"customSecret": "TOPSECRET-CUSTOM",
		"del4":         "secret-values-file",
	}, result.ReleaseConfig)
}

func TestRenderChart_ValuesLayersSecretValuesReplaceMapFromValuesFile(t *testing.T) {
	result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
		ValuesFiles:       []string{layeredValuesShapeMapFile},
		SecretValuesFiles: []string{layeredSecretValuesShapeScalarFile},
		SetJSON:           []string{`{"shape":{"new":"set"}}`},
	})

	assert.Equal(t, map[string]interface{}{"new": "set"}, renderedChartValues(t, result)["shape"])
}

func TestRenderChart_ValuesLayersSetDisablesSubchartEnabledBySecretValues(t *testing.T) {
	result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
		Set: []string{"sub2.enabled=false"},
	})

	assert.Equal(t, []string{"sub"}, dependencyNames(t, result))
}

func TestRenderChart_ValuesLayersSetNullDeletes(t *testing.T) {
	result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
		Set:               []string{"del1=null", "del2=null", "del3=null", "del4=null", "nested.b=null", "sub.s1=null"},
		ValuesFiles:       []string{layeredValuesFile},
		SecretValuesFiles: []string{layeredSecretValuesFile, layeredSecretValuesNullFile},
	})
	vals := renderedChartValues(t, result)

	assert.NotContains(t, vals, "del1", "key from values.yaml")
	assert.NotContains(t, vals, "del2", "key from secret-values.yaml")
	assert.Nil(t, vals["del3"], "key from values file")
	assert.Nil(t, vals["del4"], "key from secret values file")
	assert.Nil(t, vals["p34"], "key from values file and secret values file, deleted by a later secret values file")
	assert.Equal(t, map[string]interface{}{"a": "values-yaml"}, vals["nested"], "nested key from values.yaml and secret-values.yaml")
	assert.NotContains(t, vals["sub"], "s1", "key from subchart secret-values.yaml")
}

func TestRenderChart_ValuesLayersSetValidOnlyOverSecretValues(t *testing.T) {
	result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
		ValuesFiles:       []string{layeredValuesShapeFile},
		SecretValuesFiles: []string{layeredSecretValuesShapeFile},
		Set:               []string{"shape.a=set"},
	})

	assert.Equal(t, map[string]interface{}{"a": "set", "b": "secret"}, renderedChartValues(t, result)["shape"])
	assert.Equal(t, map[string]interface{}{"shape": map[string]interface{}{"a": "set", "b": "secret"}}, result.ReleaseConfig)
}
