package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
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
		assert.True(t, strings.HasPrefix(code, "import { createRequire as __nelmCreateRequire } from 'node:module'; const require = __nelmCreateRequire(import.meta.url);"),
			"must give CommonJS dependencies require")
	})

	t.Run("bundles a JavaScript entrypoint", func(t *testing.T) {
		tsDir := filepath.Join(copyTestChart(t), "charts", "subchart", "ts")

		bundle, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointJS)
		require.NoError(t, err)
		assert.Contains(t, string(bundle), `name: "subchart"`)
	})

	t.Run("fails on imports that Node.js does not resolve to built-in modules", func(t *testing.T) {
		tsDir := filepath.Join(copyTestChart(t), "ts")
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "src", "index.ts"), []byte("import yaml from 'https://deno.land/std/yaml/mod.ts';\nimport cdn from '//cdn.example.com/x.js';\nimport fs from 'fs';\nimport { join } from 'node:path';\nconsole.log(yaml, cdn, fs, join);\n"), 0o644))

		_, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointTS)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `"https://deno.land/std/yaml/mod.ts" imported by src/index.ts`)
		assert.Contains(t, err.Error(), `"//cdn.example.com/x.js" imported by src/index.ts`)
		assert.Contains(t, err.Error(), "install the package with the package manager")
		assert.NotContains(t, err.Error(), `"fs"`)
		assert.NotContains(t, err.Error(), `"node:path"`)
	})

	t.Run("does not fail on unsupported imports left out of the bundle as unused", func(t *testing.T) {
		tsDir := filepath.Join(copyTestChart(t), "ts")
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "src", "helpers.ts"), []byte("export function configMapName(): string {\n  return 'ts-chart';\n}\n\nexport function unused() {\n  return import('https://example.com/x.js');\n}\n"), 0o644))

		bundle, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointTS)
		require.NoError(t, err)
		assert.NotContains(t, string(bundle), "https://example.com/x.js")
	})

	t.Run("keeps built-in modules external", func(t *testing.T) {
		tsDir := filepath.Join(copyTestChart(t), "ts")
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "src", "index.ts"), []byte("import fs from 'fs';\nimport { readFile } from 'fs/promises';\nimport { join } from 'node:path';\nconst tty = require('tty');\nconsole.log(fs, readFile, join, tty);\n"), 0o644))

		bundle, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointTS)
		require.NoError(t, err)
		assert.Contains(t, string(bundle), `from "fs/promises"`)
	})

	t.Run("fails on an unresolvable import", func(t *testing.T) {
		tsDir := filepath.Join(copyTestChart(t), "ts")
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "src", "index.ts"), []byte("import 'missing-package';\n"), 0o644))

		_, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointTS)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `Could not resolve "missing-package"`)
		assert.Contains(t, err.Error(), "install the dependencies of "+tsDir)
		assert.Contains(t, err.Error(), "lock file in ts/ or in the workspace root")
	})

	t.Run("does not suggest installing dependencies for other errors", func(t *testing.T) {
		tsDir := filepath.Join(copyTestChart(t), "ts")
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "src", "index.ts"), []byte("export const broken = ;\n"), 0o644))

		_, err := ts.BuildBundle(context.Background(), tsDir, common.ChartTSEntryPointTS)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "install the dependencies")
	})
}

func TestBundleChartsRecursive(t *testing.T) {
	t.Run("bundles the chart and its subcharts without installing what is installed", func(t *testing.T) {
		chartDir := copyTestChart(t)
		chart := loadChart(t, chartDir)

		require.NoError(t, ts.BundleChartsRecursive(context.Background(), chart, chartDir, false))

		assert.Contains(t, string(findBundle(t, chart)), "async function render(")

		subchart := findDependency(t, chart, "subchart")
		assert.Contains(t, string(findBundle(t, subchart)), `name: "subchart"`)
	})

	t.Run("reuses an existing bundle without sources or a package manager", func(t *testing.T) {
		stubPath(t)

		accessor, err := helmchart.NewAccessor(&v2chart.Chart{
			Metadata: &v2chart.Metadata{APIVersion: v2chart.APIVersionV2, Name: "mychart", Version: "1.0.0"},
			RuntimeFiles: []*chartcommon.File{
				{Name: common.ChartTSSourceDir + common.ChartTSEntryPointTS, Data: []byte("export {};\n")},
				{Name: common.ChartTSBundleFile, Data: []byte("bundle\n")},
			},
		})
		require.NoError(t, err)

		require.NoError(t, ts.BundleChartsRecursive(context.Background(), accessor, t.TempDir(), false))
		assert.Equal(t, "bundle\n", string(findBundle(t, accessor)))

		require.Error(t, ts.BundleChartsRecursive(context.Background(), accessor, t.TempDir(), true),
			"rebuilding must need the sources on disk")
	})

	t.Run("keeps an existing bundle unless asked to rebuild it", func(t *testing.T) {
		chartDir := copyTestChart(t)
		require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "ts", "dist"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(chartDir, "ts", "dist", "bundle.js"), []byte("// prebuilt"), 0o644))

		chart := loadChart(t, chartDir)

		require.NoError(t, ts.BundleChartsRecursive(context.Background(), chart, chartDir, false))
		assert.Equal(t, "// prebuilt", string(findBundle(t, chart)))

		require.NoError(t, ts.BundleChartsRecursive(context.Background(), chart, chartDir, true))
		assert.Contains(t, string(findBundle(t, chart)), "async function render(")
	})
}

func TestBundleChartsRecursiveInstallsDependencies(t *testing.T) {
	binDir := stubPath(t)
	writeStub(t, binDir, "npm", "")

	chartDir := copyTestChart(t)
	require.NoError(t, os.RemoveAll(filepath.Join(chartDir, "ts", "node_modules")))
	require.NoError(t, os.WriteFile(filepath.Join(chartDir, "ts", "package-lock.json"), []byte("{}"), 0o644))

	err := ts.BundleChartsRecursive(context.Background(), loadChart(t, chartDir), chartDir, false)
	require.Error(t, err, "the stub installs nothing, so the bundle must fail to resolve the dependency")
	assert.Contains(t, err.Error(), `Could not resolve "fake-chart-ts-sdk"`)

	run := readStubRun(t, binDir, "npm")
	assert.Equal(t, []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund", "--workspaces=false"}, run.args)
	assert.Equal(t, resolvePath(t, filepath.Join(chartDir, "ts")), run.dir)
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
