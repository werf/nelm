package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/ts"
)

func TestBuildBundle(t *testing.T) {
	t.Run("bundles sources and dependencies into an ES module for Node.js", func(t *testing.T) {
		tsDir := filepath.Join(copyTestChart(t), "ts")

		bundle, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointTS)
		require.NoError(t, err)

		code := string(bundle)
		assert.Contains(t, code, `from "node:fs/promises"`, "node: builtins must stay external imports")
		assert.Contains(t, code, `from "node:util"`, "node: builtins must stay external imports")
		assert.Contains(t, code, "async function render(", "dependencies from node_modules must be inlined")
		assert.Contains(t, code, `return "ts-chart"`, "imported sources must be inlined")
		assert.Contains(t, code, "await render(", "top-level await must be kept for Node.js")
		assert.NotContains(t, code, "RenderContext", "types must be stripped")
		assert.NotContains(t, code, "require(", "must be an ES module")
	})

	t.Run("bundles a JavaScript entrypoint", func(t *testing.T) {
		tsDir := filepath.Join(copyTestChart(t), "charts", "subchart", "ts")

		bundle, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointJS)
		require.NoError(t, err)
		assert.Contains(t, string(bundle), `name: "subchart"`)
	})

	t.Run("fails on an unresolvable import", func(t *testing.T) {
		tsDir := filepath.Join(copyTestChart(t), "ts")
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "src", "index.ts"), []byte("import 'missing-package';\n"), 0o644))

		_, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointTS)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Could not resolve "missing-package"`)
	})
}

func TestBundleChartsRecursive(t *testing.T) {
	missingNode := filepath.Join(t.TempDir(), "missing-node")

	t.Run("bundles the chart and its subcharts without installing what is installed", func(t *testing.T) {
		chartDir := copyTestChart(t)
		chart := loadChart(t, chartDir)

		require.NoError(t, ts.BundleChartsRecursive(context.Background(), chart, chartDir, false, missingNode))

		assert.Contains(t, string(findBundle(t, chart)), "async function render(")

		subchart := findDependency(t, chart, "subchart")
		assert.Contains(t, string(findBundle(t, subchart)), `name: "subchart"`)
	})

	t.Run("keeps an existing bundle unless asked to rebuild it", func(t *testing.T) {
		chartDir := copyTestChart(t)
		require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "ts", "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(chartDir, "ts", "dist", "bundle.js"), []byte("// prebuilt"), 0o644))

		chart := loadChart(t, chartDir)

		require.NoError(t, ts.BundleChartsRecursive(context.Background(), chart, chartDir, false, missingNode))
		assert.Equal(t, "// prebuilt", string(findBundle(t, chart)))

		require.NoError(t, ts.BundleChartsRecursive(context.Background(), chart, chartDir, true, missingNode))
		assert.Contains(t, string(findBundle(t, chart)), "async function render(")
	})
}

func findBundle(t *testing.T, chart helmchart.Accessor) []byte {
	t.Helper()

	bundle, found := lo.Find(chart.RuntimeFiles(), func(file *chartcommon.File) bool {
		return file.Name == common.ChartTSBundleFile
	})
	require.True(t, found, "chart %q has no bundle", chart.Name())

	return bundle.Data
}

func findDependency(t *testing.T, chart helmchart.Accessor, name string) helmchart.Accessor {
	t.Helper()

	for _, dep := range chart.Dependencies() {
		accessor, err := helmchart.NewAccessor(dep)
		require.NoError(t, err)

		if accessor.Name() == name {
			return accessor
		}
	}

	require.Failf(t, "dependency not found", "chart %q has no dependency %q", chart.Name(), name)

	return nil
}
