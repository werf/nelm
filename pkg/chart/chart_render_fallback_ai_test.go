//go:build ai_tests

package chart

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	nelmcommon "github.com/werf/nelm/pkg/common"
	v3chart "github.com/werf/nelm/pkg/helm/intern/chart/v3"
	chartcommon "github.com/werf/nelm/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	v2chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	chartv2util "github.com/werf/nelm/pkg/helm/pkg/chart/v2/util"
	"github.com/werf/nelm/pkg/log"
)

type fallbackChartFilesAI struct {
	childSchema     string
	dependencyBlock string
	rootSchema      string
	userValuesYAML  string
}

type capturingLoggerAI struct {
	log.Logger

	messages bytes.Buffer
	mu       sync.Mutex
}

func (l *capturingLoggerAI) Debug(ctx context.Context, format string, a ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.messages.WriteString(format)
}

func (l *capturingLoggerAI) sawFallbackDebug() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	return bytes.Contains(l.messages.Bytes(), []byte("injected service values"))
}

func TestAI_ServiceValuesFallback_ChartWithSubchart_StrictRootSchema_Renders(t *testing.T) {
	chartPath := writeFallbackChartAI(t, fallbackChartFilesAI{rootSchema: strictRootSchemaJSONAI(), userValuesYAML: "foo: bar\n"})
	ctx, c := loadChartWithServiceValuesAI(t, chartPath, serviceExtraValuesAI())
	require.NotEmpty(t, c.ExtraValues)

	vals, err := runFallbackAI(t, ctx, chartPath, c, map[string]any{"foo": "override"})
	require.NoError(t, err)

	rendered := vals.AsMap()["Values"]
	renderedMap, ok := rendered.(chartcommon.Values)
	if !ok {
		renderedMap = chartcommon.Values(rendered.(map[string]any))
	}
	assert.Contains(t, renderedMap, "werf", "service values must remain in the rendered values")
	assert.Contains(t, renderedMap, "global")
}

func TestAI_ServiceValuesFallback_ConditionOnServiceValue_DivergentGraph_Refused(t *testing.T) {
	chartPath := writeFallbackChartAI(t, fallbackChartFilesAI{
		dependencyBlock: "dependencies:\n  - name: child\n    version: 0.1.0\n    condition: werf.is_stub\n",
		rootSchema:      strictRootSchemaJSONAI(),
		userValuesYAML:  "foo: bar\nwerf:\n  is_stub: false\n",
	})
	extra := map[string]any{
		"werf":   map[string]any{"name": "proj", "is_stub": true},
		"global": map[string]any{"werf": map[string]any{"name": "proj"}},
	}
	ctx, c := loadChartWithServiceValuesAI(t, chartPath, extra)

	_, err := runFallbackAI(t, ctx, chartPath, c, map[string]any{"foo": "override"})
	require.Error(t, err, "when a service value drives dependency enablement, the fallback must refuse rather than validate a different graph")
	assert.Contains(t, err.Error(), "--no-values-schema-validation")
}

func TestAI_ServiceValuesFallback_DebugMessage_OnlyOnAccept(t *testing.T) {
	original := log.Default
	captured := &capturingLoggerAI{Logger: original}
	log.Default = captured
	t.Cleanup(func() { log.Default = original })

	chartPath := writeFallbackChartAI(t, fallbackChartFilesAI{rootSchema: strictRootSchemaJSONAI(), userValuesYAML: "foo: bar\n"})

	ctx, c := loadChartWithServiceValuesAI(t, chartPath, serviceExtraValuesAI())
	_, err := runFallbackAI(t, ctx, chartPath, c, map[string]any{"foo": "override"})
	require.NoError(t, err)
	assert.True(t, captured.sawFallbackDebug(), "accepted fallback must emit the debug message")

	captured.messages.Reset()

	ctx2, c2 := loadChartWithServiceValuesAI(t, chartPath, serviceExtraValuesAI())
	_, err = runFallbackAI(t, ctx2, chartPath, c2, map[string]any{"foo": "override", "fooo": "typo"})
	require.Error(t, err)
	assert.False(t, captured.sawFallbackDebug(), "rejected fallback must not emit the accept debug message")
}

func TestAI_ServiceValuesFallback_DeliberateStringConstraintOnServiceKey_Bypassed(t *testing.T) {
	cases := map[string]string{
		"root werf as string":         `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"foo":{"type":"string"},"werf":{"type":"string"}}}`,
		"global.werf as string":       `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"foo":{"type":"string"},"global":{"type":"object","properties":{"werf":{"type":"string"}}}}}`,
		"global.werf.name as integer": `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"foo":{"type":"string"},"global":{"type":"object","properties":{"werf":{"type":"object","properties":{"name":{"type":"integer"}}}}}}}`,
	}

	for name, schema := range cases {
		t.Run(name, func(t *testing.T) {
			chartPath := writeFallbackChartAI(t, fallbackChartFilesAI{rootSchema: schema, userValuesYAML: "foo: bar\n"})
			ctx, c := loadChartWithServiceValuesAI(t, chartPath, serviceExtraValuesAI())

			_, err := runFallbackAI(t, ctx, chartPath, c, map[string]any{"foo": "override"})
			require.NoError(t, err, "a deliberate but unsatisfiable-under-werf constraint on a service key is bypassed by design")
		})
	}
}

func TestAI_ServiceValuesFallback_KeywordAgnostic_PassThrough(t *testing.T) {
	cases := map[string]string{
		"unevaluatedProperties false": `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"foo":{"type":"string"},"child":{"type":"object"}},"unevaluatedProperties":false}`,
		"not required werf":           `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","not":{"required":["werf"]}}`,
		"propertyNames restricted":    `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","propertyNames":{"enum":["foo","child"]}}`,
	}

	for name, schema := range cases {
		t.Run(name, func(t *testing.T) {
			chartPath := writeFallbackChartAI(t, fallbackChartFilesAI{rootSchema: schema, userValuesYAML: "foo: bar\n"})
			ctx, c := loadChartWithServiceValuesAI(t, chartPath, serviceExtraValuesAI())

			_, err := runFallbackAI(t, ctx, chartPath, c, map[string]any{"foo": "override"})
			require.NoError(t, err, "the fallback re-runs the real validator, so it tolerates service values under any rejection keyword")
		})
	}
}

func TestAI_ServiceValuesFallback_NoExtraValues_NotEntered(t *testing.T) {
	original := log.Default
	captured := &capturingLoggerAI{Logger: original}
	log.Default = captured
	t.Cleanup(func() { log.Default = original })

	chartPath := writeFallbackChartAI(t, fallbackChartFilesAI{rootSchema: strictRootSchemaJSONAI(), userValuesYAML: "foo: bar\n"})
	ctx, c := loadChartWithServiceValuesAI(t, chartPath, nil)
	require.Empty(t, c.ExtraValues)
	require.False(t, chartHasExtraValues(c), "guard must report no service values")

	_, err := runFallbackAI(t, ctx, chartPath, c, map[string]any{"foo": "override", "fooo": "typo"})
	require.Error(t, err, "plain-nelm schema violations must still surface, unchanged")
	assert.Contains(t, err.Error(), "fooo")
	assert.False(t, captured.sawFallbackDebug(), "the fallback must not be entered when the chart carries no service values")
}

func TestAI_ServiceValuesFallback_RequiredWerf_ForbiddenGlobal_StillFails(t *testing.T) {
	schema := `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"required":["werf"],"properties":{"foo":{"type":"string"},"child":{"type":"object"},"werf":{"type":"object"}}}`
	chartPath := writeFallbackChartAI(t, fallbackChartFilesAI{rootSchema: schema, userValuesYAML: "foo: bar\n"})
	ctx, c := loadChartWithServiceValuesAI(t, chartPath, serviceExtraValuesAI())

	_, err := runFallbackAI(t, ctx, chartPath, c, map[string]any{"foo": "override"})
	require.Error(t, err, "a schema that requires werf but forbids global is unsatisfiable under werf; the service-free fallback drops werf and fails")
	assert.Contains(t, err.Error(), "werf")
}

func TestAI_ServiceValuesFallback_SubchartOwnStrictSchema_ReceivesPropagatedGlobal_Renders(t *testing.T) {
	childSchema := `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"properties":{"a":{"type":"integer"},"global":{"type":"object"}}}`
	chartPath := writeFallbackChartAI(t, fallbackChartFilesAI{
		childSchema:    childSchema,
		rootSchema:     strictRootSchemaJSONAI(),
		userValuesYAML: "foo: bar\n",
	})
	ctx, c := loadChartWithServiceValuesAI(t, chartPath, serviceExtraValuesAI())

	_, err := runFallbackAI(t, ctx, chartPath, c, map[string]any{"foo": "override"})
	require.NoError(t, err, "subchart with its own strict schema must pass once service-injected global is stripped")
}

func TestAI_ServiceValuesFallback_UserTypo_StillFails_NamingOnlyTypo(t *testing.T) {
	chartPath := writeFallbackChartAI(t, fallbackChartFilesAI{rootSchema: strictRootSchemaJSONAI(), userValuesYAML: "foo: bar\n"})
	ctx, c := loadChartWithServiceValuesAI(t, chartPath, serviceExtraValuesAI())

	_, err := runFallbackAI(t, ctx, chartPath, c, map[string]any{"foo": "override", "fooo": "typo"})
	require.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "fooo")
	assert.NotContains(t, msg, "werf")
	assert.NotContains(t, msg, "global")
}

func TestAI_chartHasExtraValues(t *testing.T) {
	assert.False(t, chartHasExtraValues(&v2chart.Chart{}), "v2 chart with nil ExtraValues")
	assert.False(t, chartHasExtraValues(&v2chart.Chart{ExtraValues: map[string]any{}}), "v2 chart with empty ExtraValues")
	assert.True(t, chartHasExtraValues(&v2chart.Chart{ExtraValues: map[string]any{"werf": map[string]any{}}}), "v2 chart with populated ExtraValues")
	assert.False(t, chartHasExtraValues(&v3chart.Chart{}), "v3 chart with nil ExtraValues")
	assert.True(t, chartHasExtraValues(&v3chart.Chart{ExtraValues: map[string]any{"werf": map[string]any{}}}), "v3 chart with populated ExtraValues")
}

func runFallbackAI(t *testing.T, ctx context.Context, chartPath string, c *v2chart.Chart, overrideValues map[string]any) (chartcommon.Values, error) {
	t.Helper()

	pristine, err := deepCopyValues(overrideValues)
	require.NoError(t, err)

	require.NoError(t, chartv2util.ProcessDependencies(c, &overrideValues))

	return renderValuesToleratingServiceValues(ctx, chartPath, c, overrideValues, pristine, renderReleaseOptionsAI(), nil, false)
}

func loadChartWithServiceValuesAI(t *testing.T, chartPath string, extra map[string]any) (context.Context, *v2chart.Chart) {
	t.Helper()

	ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())

	helmOpts := nelmcommon.HelmOptions{}
	helmOpts.ChartLoadOpts.ExtraValues = extra
	ctx = nelmcommon.ContextWithHelmOptions(ctx, helmOpts)

	loaded, err := loader.Load(ctx, chartPath)
	require.NoError(t, err)

	c, ok := loaded.(*v2chart.Chart)
	require.True(t, ok, "expected v2 chart, got %T", loaded)

	return ctx, c
}

func renderReleaseOptionsAI() chartcommon.ReleaseOptions {
	return chartcommon.ReleaseOptions{Name: "rel", Namespace: "ns", Revision: 1, IsInstall: true}
}

func serviceExtraValuesAI() map[string]any {
	return map[string]any{
		"werf":   map[string]any{"name": "proj"},
		"global": map[string]any{"werf": map[string]any{"name": "proj"}},
	}
}

func strictRootSchemaJSONAI() string {
	return `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"properties":{"foo":{"type":"string"},"child":{"type":"object"}}}`
}

func writeFallbackChartAI(t *testing.T, files fallbackChartFilesAI) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "templates"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "charts", "child", "templates"), 0o755))

	dependencyBlock := files.dependencyBlock
	if dependencyBlock == "" {
		dependencyBlock = "dependencies:\n  - name: child\n    version: 0.1.0\n"
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "Chart.yaml"),
		[]byte("apiVersion: v2\nname: parent\nversion: 0.1.0\n"+dependencyBlock), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "values.yaml"), []byte(files.userValuesYAML), 0o644))
	if files.rootSchema != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "values.schema.json"), []byte(files.rootSchema), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "templates", "cm.yaml"),
		[]byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: p\n"), 0o644))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "charts", "child", "Chart.yaml"),
		[]byte("apiVersion: v2\nname: child\nversion: 0.1.0\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "charts", "child", "values.yaml"), []byte("a: 1\n"), 0o644))
	if files.childSchema != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "charts", "child", "values.schema.json"), []byte(files.childSchema), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "charts", "child", "templates", "cm.yaml"),
		[]byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: c\n"), 0o644))

	return dir
}
