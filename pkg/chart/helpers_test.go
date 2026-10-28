package chart

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
)

const layeredValuesSecretKey = "eb1988e0ffd7272892a851f4c20f6512" //nolint:gosec // Test-only key that encrypts testdata/layered-values.

var (
	layeredValuesFile                  = filepath.Join("testdata", "layered-values", "values-file.yaml")
	layeredValuesExportFile            = filepath.Join("testdata", "layered-values", "values-file-export.yaml")
	layeredSecretValuesExportFile      = filepath.Join("testdata", "layered-values", "secret-values-file-export.yaml")
	layeredSecretValuesFile            = filepath.Join("testdata", "layered-values", "secret-values-file.yaml")
	layeredSecretValuesNullFile        = filepath.Join("testdata", "layered-values", "secret-values-file-null.yaml")
	layeredSecretValuesShapeFile       = filepath.Join("testdata", "layered-values", "secret-values-file-shape.yaml")
	layeredSecretValuesShapeScalarFile = filepath.Join("testdata", "layered-values", "secret-values-file-shape-scalar.yaml")
	layeredValuesShapeFile             = filepath.Join("testdata", "layered-values", "values-file-shape.yaml")
	layeredValuesShapeMapFile          = filepath.Join("testdata", "layered-values", "values-file-shape-map.yaml")
)

type layeredValuesRenderOptions struct {
	ChartDir          string
	SecretValuesFiles []string
	ServiceValues     map[string]interface{}
	Set               []string
	SetJSON           []string
	ValuesFiles       []string
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
