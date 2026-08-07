//go:build ai_tests

package engine

import (
	"context"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/helm/pkg/chart/common"
	chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
)

func TestAI_DebugFuncsExistInFuncMap(t *testing.T) {
	e := Engine{}
	tpl := template.New("test")
	e.initFunMap(context.Background(), tpl)

	funcs := []string{"dump_debug", "printf_debug", "include_debug", "tpl_debug"}
	for _, name := range funcs {
		found := false
		for _, tmpl := range tpl.Templates() {
			_ = tmpl
		}
		testTpl := template.New("check_" + name)
		testTpl.Funcs(template.FuncMap{name: func() string { return "" }})
		_ = testTpl

		execTpl, err := tpl.Parse("{{ " + name + " }}")
		if err == nil && execTpl != nil {
			found = true
		}
		assert.True(t, found, "function %q should be registered in FuncMap", name)
	}
}

func TestAI_WerfSecretFileFuncExists(t *testing.T) {
	e := Engine{}
	tpl := template.New("test")
	e.initFunMap(context.Background(), tpl)

	parsed, err := tpl.Parse(`{{ werf_secret_file "test.txt" }}`)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
}

func TestAI_EngineRendersSimpleTemplate(t *testing.T) {
	c := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:       "test-chart",
			Version:    "0.1.0",
			APIVersion: chart.APIVersionV2,
		},
		Templates: []*common.File{
			{Name: "templates/hello.yaml", Data: []byte("greeting: {{ .Values.hello }}")},
		},
		Values: map[string]interface{}{
			"hello": "world",
		},
	}

	vals := common.Values{
		"Values":  c.Values,
		"Release": map[string]interface{}{"Name": "test", "Namespace": "default", "IsInstall": true, "IsUpgrade": false, "Service": "Helm"},
		"Chart":   map[string]interface{}{"Name": "test-chart", "Version": "0.1.0"},
	}

	out, err := Render(context.Background(), c, vals)
	require.NoError(t, err)
	require.NotEmpty(t, out)

	found := false
	for _, v := range out {
		if v == "greeting: world" {
			found = true
			break
		}
	}
	assert.True(t, found, "rendered output should contain 'greeting: world', got %v", out)
}
