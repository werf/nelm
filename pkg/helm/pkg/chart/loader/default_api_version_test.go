package loader

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nelmcommon "github.com/werf/nelm/v2/pkg/common"
	v3chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	v2util "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2/util"
)

const (
	defaultAPIVersionTestChartYAML = "name: mychart\nversion: 1.0.0\ndependencies:\n  - name: sub\n    version: 0.1.0\n"
	defaultAPIVersionTestChartLock = "dependencies:\n  - name: sub\n    version: 0.1.0\n    repository: \"\"\ndigest: sha256:0\ngenerated: \"2024-01-01T00:00:00Z\"\n"
)

func TestLoad_DefaultChartAPIVersion(t *testing.T) {
	tests := []struct {
		name               string
		ctx                context.Context
		expectedAPIVersion string
		expectedV3         bool
	}{
		{
			name:               "vanilla",
			ctx:                context.Background(),
			expectedAPIVersion: v2chart.APIVersionV1,
		},
		{
			name:               "no default",
			ctx:                newDefaultAPIVersionTestContext(nelmcommon.LegacyChartTypeChart, ""),
			expectedAPIVersion: v2chart.APIVersionV1,
		},
		{
			name:               "default v2",
			ctx:                newDefaultAPIVersionTestContext(nelmcommon.LegacyChartTypeChart, v2chart.APIVersionV2),
			expectedAPIVersion: v2chart.APIVersionV2,
		},
		{
			name:               "default v3",
			ctx:                newDefaultAPIVersionTestContext(nelmcommon.LegacyChartTypeChart, v3chart.APIVersionV3),
			expectedAPIVersion: v3chart.APIVersionV3,
			expectedV3:         true,
		},
		{
			name:               "default ignored for a bundle",
			ctx:                newDefaultAPIVersionTestContext(nelmcommon.LegacyChartTypeBundle, v2chart.APIVersionV2),
			expectedAPIVersion: v2chart.APIVersionV1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "mychart")
			writeTestFile(t, filepath.Join(dir, "Chart.yaml"), defaultAPIVersionTestChartYAML)
			writeTestFile(t, filepath.Join(dir, "Chart.lock"), defaultAPIVersionTestChartLock)
			writeTestFile(t, filepath.Join(dir, "charts", "sub", "Chart.yaml"), "apiVersion: v2\nname: sub\nversion: 0.1.0\n")

			fromDir, err := LoadDir(tt.ctx, dir)
			require.NoError(t, err)
			assertDefaultAPIVersionTestChart(t, fromDir, tt.expectedAPIVersion, tt.expectedV3)

			fromArchive, err := LoadArchive(tt.ctx, createChartArchive(t, "mychart", "", map[string][]byte{
				"Chart.yaml":            []byte(defaultAPIVersionTestChartYAML),
				"Chart.lock":            []byte(defaultAPIVersionTestChartLock),
				"charts/sub/Chart.yaml": []byte("apiVersion: v2\nname: sub\nversion: 0.1.0\n"),
			}, false))
			require.NoError(t, err)
			assertDefaultAPIVersionTestChart(t, fromArchive, tt.expectedAPIVersion, tt.expectedV3)
		})
	}
}

func TestLoad_DefaultChartAPIVersionKeepsRequirementsOfV1Chart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mychart")
	writeTestFile(t, filepath.Join(dir, "Chart.yaml"), "name: mychart\nversion: 1.0.0\n")
	writeTestFile(t, filepath.Join(dir, "requirements.yaml"), "dependencies:\n  - name: sub\n    version: 0.1.0\n")
	writeTestFile(t, filepath.Join(dir, "charts", "sub", "Chart.yaml"), "apiVersion: v2\nname: sub\nversion: 0.1.0\n")

	for apiVersion, expectedFiles := range map[string][]string{
		"":                   {"requirements.yaml"},
		v2chart.APIVersionV2: nil,
	} {
		t.Run("default "+apiVersion, func(t *testing.T) {
			loaded, err := LoadDir(newDefaultAPIVersionTestContext(nelmcommon.LegacyChartTypeChart, apiVersion), dir)
			require.NoError(t, err)

			c, ok := loaded.(*v2chart.Chart)
			require.True(t, ok)
			assert.Equal(t, expectedFiles, testFileNames(c.Files))
			require.Len(t, c.Metadata.Dependencies, 1)
		})
	}
}

func assertDefaultAPIVersionTestChart(t *testing.T, loaded helmchart.Charter, expectedAPIVersion string, expectedV3 bool) {
	t.Helper()

	if expectedV3 {
		c, ok := loaded.(*v3chart.Chart)
		require.True(t, ok, "loaded %T", loaded)
		assert.Equal(t, expectedAPIVersion, c.Metadata.APIVersion)
		assert.NotNil(t, c.Lock)

		return
	}

	c, ok := loaded.(*v2chart.Chart)
	require.True(t, ok, "loaded %T", loaded)
	assert.Equal(t, expectedAPIVersion, c.Metadata.APIVersion)

	savedDir := t.TempDir()
	require.NoError(t, v2util.SaveDir(c, savedDir))

	if expectedAPIVersion == v2chart.APIVersionV1 {
		assert.NoFileExists(t, filepath.Join(savedDir, "mychart", "Chart.lock"))
		return
	}

	assert.FileExists(t, filepath.Join(savedDir, "mychart", "Chart.lock"))

	saved, err := LoadDir(context.Background(), filepath.Join(savedDir, "mychart"))
	require.NoError(t, err)

	savedAccessor, err := helmchart.NewAccessor(saved)
	require.NoError(t, err)
	assert.Len(t, savedAccessor.MetaDependencies(), 1)
}

func newDefaultAPIVersionTestContext(chartType nelmcommon.LegacyChartType, defaultAPIVersion string) context.Context {
	return nelmcommon.ContextWithHelmOptions(context.Background(), nelmcommon.HelmOptions{
		ChartLoadOpts: nelmcommon.ChartLoadOptions{
			ChartType:              chartType,
			DefaultChartAPIVersion: defaultAPIVersion,
			NoDependenciesLoad:     true,
			NoSecrets:              true,
		},
	})
}
