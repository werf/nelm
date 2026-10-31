package loader

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nelmcommon "github.com/werf/nelm/v2/pkg/common"
	v3chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	v3util "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3/util"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	v2util "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2/util"
)

func TestLoad_WerfDataFiles(t *testing.T) {
	contexts := map[string]context.Context{
		"vanilla": context.Background(),
		"werf": nelmcommon.ContextWithHelmOptions(context.Background(), nelmcommon.HelmOptions{
			ChartLoadOpts: nelmcommon.ChartLoadOptions{ChartType: nelmcommon.LegacyChartTypeChart, NoSecrets: true},
		}),
	}

	for _, apiVersion := range []string{"v1", "v2", "v3"} {
		for ctxName, ctx := range contexts {
			t.Run(apiVersion+"/"+ctxName, func(t *testing.T) {
				testLoadWerfDataFiles(t, ctx, apiVersion)
			})
		}
	}
}

func testLoadWerfDataFiles(t *testing.T, ctx context.Context, apiVersion string) {
	t.Helper()

	dir := writeWerfDataChart(t, apiVersion)

	loaded, err := LoadDir(ctx, dir)
	require.NoError(t, err)
	assertWerfDataFiles(t, loaded)

	archivePath := saveChart(t, loaded)
	fromArchive, err := LoadFile(ctx, archivePath)
	require.NoError(t, err)
	assertWerfDataFiles(t, fromArchive)

	archiveData, err := os.ReadFile(archivePath)
	require.NoError(t, err)
	fromReader, err := LoadArchive(ctx, bytes.NewReader(archiveData))
	require.NoError(t, err)
	assertWerfDataFiles(t, fromReader)
}

func writeWerfDataChart(t *testing.T, apiVersion string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "mychart")
	writeTestFile(t, filepath.Join(dir, "Chart.yaml"), "apiVersion: "+apiVersion+"\nname: mychart\nversion: 1.0.0\n")
	writeTestFile(t, filepath.Join(dir, "values.yaml"), "key: chart\n")
	writeTestFile(t, filepath.Join(dir, "templates", "cm.yaml"), "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cm\n")
	writeTestFile(t, filepath.Join(dir, "README.md"), "readme\n")
	writeTestFile(t, filepath.Join(dir, ".helmignore"), ".*\n")
	writeTestFile(t, filepath.Join(dir, ".werf-data", "metadata.yaml"), "apiVersion: v2\nvalues:\n  values: true\n")
	writeTestFile(t, filepath.Join(dir, ".werf-data", "values.yaml"), "key: published\n")
	writeTestFile(t, filepath.Join(dir, ".werf-data", "set-files", "0"), "contents\n")
	writeTestFile(t, filepath.Join(dir, "charts", "dep", "Chart.yaml"), "apiVersion: v2\nname: dep\nversion: 1.0.0\n")
	writeTestFile(t, filepath.Join(dir, "charts", "dep", ".werf-data", "metadata.yaml"), "apiVersion: v2\n")
	writeTestFile(t, filepath.Join(dir, "charts", "dep", "charts", "nested", "Chart.yaml"), "apiVersion: v2\nname: nested\nversion: 1.0.0\n")
	writeTestFile(t, filepath.Join(dir, "charts", "dep", "charts", "nested", ".werf-data", "metadata.yaml"), "apiVersion: v2\n")

	return dir
}

func assertWerfDataFiles(t *testing.T, loaded helmchart.Charter) {
	t.Helper()

	accessor, err := helmchart.NewAccessor(loaded)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{
		".werf-data/metadata.yaml",
		".werf-data/set-files/0",
		".werf-data/values.yaml",
	}, testFileNames(accessor.WerfDataFiles()))
	assert.Equal(t, []string{"README.md"}, testFileNames(accessor.Files()))

	require.Len(t, accessor.Dependencies(), 1)
	dep, err := helmchart.NewAccessor(accessor.Dependencies()[0])
	require.NoError(t, err)
	assert.Equal(t, []string{".werf-data/metadata.yaml"}, testFileNames(dep.WerfDataFiles()))

	require.Len(t, dep.Dependencies(), 1)
	nested, err := helmchart.NewAccessor(dep.Dependencies()[0])
	require.NoError(t, err)
	assert.Equal(t, []string{".werf-data/metadata.yaml"}, testFileNames(nested.WerfDataFiles()))
}

func saveChart(t *testing.T, loaded helmchart.Charter) string {
	t.Helper()

	outDir := t.TempDir()

	switch c := loaded.(type) {
	case *v2chart.Chart:
		archivePath, err := v2util.Save(c, outDir)
		require.NoError(t, err)

		dirCopy := filepath.Join(t.TempDir(), "dircopy")
		require.NoError(t, v2util.SaveDir(c, dirCopy))
		assert.FileExists(t, filepath.Join(dirCopy, c.Name(), ".werf-data", "set-files", "0"))

		return archivePath
	case *v3chart.Chart:
		archivePath, err := v3util.Save(c, outDir)
		require.NoError(t, err)

		dirCopy := filepath.Join(t.TempDir(), "dircopy")
		require.NoError(t, v3util.SaveDir(c, dirCopy))
		assert.FileExists(t, filepath.Join(dirCopy, c.Name(), ".werf-data", "set-files", "0"))

		return archivePath
	default:
		t.Fatalf("unexpected chart type %T", loaded)

		return ""
	}
}

func writeTestFile(t *testing.T, filePath, data string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, []byte(data), 0o644))
}

func testFileNames(files []*common.File) []string {
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
	}

	return names
}
