package chart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/log"
)

const layeredValuesSecretKey = "eb1988e0ffd7272892a851f4c20f6512" //nolint:gosec // Test-only key that encrypts testdata/layered-values.

var (
	layeredImportExportSecretValuesFile = filepath.Join("testdata", "layered-values", "ie-secret-values-file.yaml")
	layeredImportExportValuesFile       = filepath.Join("testdata", "layered-values", "ie-values-file.yaml")
	layeredValuesFile                   = filepath.Join("testdata", "layered-values", "values-file.yaml")
	layeredValuesExportFile             = filepath.Join("testdata", "layered-values", "values-file-export.yaml")
	layeredSecretValuesExportFile       = filepath.Join("testdata", "layered-values", "secret-values-file-export.yaml")
	layeredSecretValuesFile             = filepath.Join("testdata", "layered-values", "secret-values-file.yaml")
	layeredSecretValuesNullFile         = filepath.Join("testdata", "layered-values", "secret-values-file-null.yaml")
	layeredSecretValuesShapeFile        = filepath.Join("testdata", "layered-values", "secret-values-file-shape.yaml")
	layeredSecretValuesShapeScalarFile  = filepath.Join("testdata", "layered-values", "secret-values-file-shape-scalar.yaml")
	layeredSecretValuesSubFile          = filepath.Join("testdata", "layered-values", "secret-values-file-sub.yaml")
	layeredValuesShapeFile              = filepath.Join("testdata", "layered-values", "values-file-shape.yaml")
	layeredValuesShapeMapFile           = filepath.Join("testdata", "layered-values", "values-file-shape-map.yaml")
)

type absentValue struct{}

type warningsLogger struct {
	log.Logger

	warnings []string
}

func (l *warningsLogger) Warn(_ context.Context, format string, a ...interface{}) {
	l.warnings = append(l.warnings, fmt.Sprintf(format, a...))
}

type layeredValuesRenderOptions struct {
	ChartDir                   string
	DefaultSecretValuesDisable bool
	SecretValuesFiles          []string
	ServiceValues              map[string]interface{}
	Set                        []string
	SetJSON                    []string
	ValuesFiles                []string
}

func dependencyNames(t *testing.T, result *RenderChartResult) []string {
	t.Helper()

	var names []string
	for _, dep := range result.Chart.Dependencies() {
		depAccessor, err := helmchart.NewAccessor(dep)
		require.NoError(t, err)

		names = append(names, depAccessor.Name())
	}

	sort.Strings(names)

	return names
}

func flattenToSetValues(vals map[string]interface{}, prefix string) []string {
	var result []string
	for key, value := range vals {
		if table, ok := value.(map[string]interface{}); ok {
			result = append(result, flattenToSetValues(table, prefix+key+".")...)
			continue
		}

		result = append(result, prefix+key+"="+fmt.Sprint(value))
	}

	return result
}

func readValuesFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var vals map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &vals))

	return vals
}

func renderLayeredValuesChart(t *testing.T, opts layeredValuesRenderOptions) *RenderChartResult {
	t.Helper()

	t.Setenv("WERF_SECRET_KEY", layeredValuesSecretKey)

	chartDir := opts.ChartDir
	if chartDir == "" {
		chartDir = "chart"
	}

	chartPath, err := filepath.Abs(filepath.Join("testdata", "layered-values", chartDir))
	require.NoError(t, err)

	renderOpts := RenderChartOptions{
		HelmOptions: common.HelmOptions{
			ChartLoadOpts: common.ChartLoadOptions{
				ExtraValues: opts.ServiceValues,
			},
		},
		TempDirPath: t.TempDir(),
	}
	renderOpts.ValuesSet = opts.Set
	renderOpts.ValuesSetJSON = opts.SetJSON
	renderOpts.ValuesFiles = opts.ValuesFiles
	renderOpts.HelmOptions.ChartLoadOpts.SecretValuesFiles = opts.SecretValuesFiles
	renderOpts.HelmOptions.ChartLoadOpts.DefaultSecretValuesDisable = opts.DefaultSecretValuesDisable

	ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())

	result, err := RenderChart(ctx, chartPath, "rel", "ns", 1, common.DeployTypeInitial, nil, nil, renderOpts)
	require.NoError(t, err)

	return result
}

func renderedChartValues(t *testing.T, result *RenderChartResult) map[string]interface{} {
	t.Helper()

	vals, ok := result.Values["Values"].(chartcommon.Values)
	require.True(t, ok, "unexpected rendered values type %T", result.Values["Values"])

	return vals.AsMap()
}

// valueAtPath returns the value at the dot-separated path, or absentValue if there is none.
func valueAtPath(t *testing.T, vals map[string]interface{}, path string) interface{} {
	t.Helper()

	var value interface{} = vals
	for key := range strings.SplitSeq(path, ".") {
		table, ok := value.(map[string]interface{})
		require.True(t, ok, "value at %q is not a map", path)

		if value, ok = table[key]; !ok {
			return absentValue{}
		}
	}

	return value
}
