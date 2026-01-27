package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/internal/ts"
	"github.com/werf/nelm/pkg/common"
)

func TestBuildVendorBundleToDir(t *testing.T) {
	t.Run("skips non-existent path", func(t *testing.T) {
		err := ts.BuildVendorBundleToDir(context.Background(), "./non-existent-path")
		require.NoError(t, err)
	})

	t.Run("error for file path", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "chart.tgz")
		require.NoError(t, os.WriteFile(filePath, []byte("dummy"), 0o644))

		err := ts.BuildVendorBundleToDir(context.Background(), filePath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a directory")
	})

	t.Run("skips chart without ts directory", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-chart")
		require.NoError(t, os.MkdirAll(chartPath, 0o755))

		err := ts.BuildVendorBundleToDir(context.Background(), chartPath)
		require.NoError(t, err)

		vendorPath := filepath.Join(chartPath, common.ChartTSVendorBundleFile)
		_, err = os.Stat(vendorPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("skips ts directory without entrypoint", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-chart")
		tsDir := filepath.Join(chartPath, "ts", "src")
		require.NoError(t, os.MkdirAll(tsDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "helpers.ts"), []byte("export const foo = 'bar';"), 0o644))

		err := ts.BuildVendorBundleToDir(context.Background(), chartPath)
		require.NoError(t, err)

		vendorPath := filepath.Join(chartPath, common.ChartTSVendorBundleFile)
		_, err = os.Stat(vendorPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("skips entrypoint without node_modules", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-chart")
		tsDir := filepath.Join(chartPath, "ts", "src")
		require.NoError(t, os.MkdirAll(tsDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "index.ts"), []byte(`
export function render(context: any) {
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'test' } }] };
}
`), 0o644))

		err := ts.BuildVendorBundleToDir(context.Background(), chartPath)
		require.NoError(t, err)

		vendorPath := filepath.Join(chartPath, common.ChartTSVendorBundleFile)
		_, err = os.Stat(vendorPath)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("creates vendor bundle with dependencies", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-chart")
		tsDir := filepath.Join(chartPath, "ts", "src")
		require.NoError(t, os.MkdirAll(tsDir, 0o755))

		// Source with import
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "index.ts"), []byte(`
import { helper } from 'fake-lib';
export function render(context: any) { return { manifests: [helper(context)] }; }
`), 0o644))

		// Fake node_modules
		fakeLibDir := filepath.Join(chartPath, "ts", "node_modules", "fake-lib")
		require.NoError(t, os.MkdirAll(fakeLibDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(fakeLibDir, "package.json"), []byte(`{"name": "fake-lib", "version": "1.0.0", "main": "index.js"}`), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(fakeLibDir, "index.js"), []byte(`
module.exports.helper = function(ctx) {
    return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: ctx.Release.Name } };
};
`), 0o644))

		err := ts.BuildVendorBundleToDir(context.Background(), chartPath)
		require.NoError(t, err)

		vendorPath := filepath.Join(chartPath, common.ChartTSVendorBundleFile)
		vendorContent, err := os.ReadFile(vendorPath)
		require.NoError(t, err)
		assert.Contains(t, string(vendorContent), "__NELM_VENDOR__")
		assert.Contains(t, string(vendorContent), "fake-lib")
	})

	t.Run("error on TypeScript syntax error", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-chart")
		tsDir := filepath.Join(chartPath, "ts", "src")
		require.NoError(t, os.MkdirAll(tsDir, 0o755))

		// Create node_modules to trigger build
		nodeModulesDir := filepath.Join(chartPath, "ts", "node_modules", "some-lib")
		require.NoError(t, os.MkdirAll(nodeModulesDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(nodeModulesDir, "package.json"), []byte(`{"name": "some-lib", "version": "1.0.0", "main": "index.js"}`), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(nodeModulesDir, "index.js"), []byte(`module.exports = {};`), 0o644))

		// Syntax error in source
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "index.ts"), []byte(`
import 'some-lib';
export function render(context: any) { return { manifests: [ }
`), 0o644))

		err := ts.BuildVendorBundleToDir(context.Background(), chartPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "TypeScript transpilation failed")
	})

	t.Run("detects multiple dependencies", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-chart")
		tsDir := filepath.Join(chartPath, "ts", "src")
		require.NoError(t, os.MkdirAll(tsDir, 0o755))

		// Source with multiple imports
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "index.ts"), []byte(`
import { helper } from './helpers';
import { util } from 'fake-util';
export function render(context: any) { return { manifests: [helper(context, util)] }; }
`), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "helpers.ts"), []byte(`
export function helper(context: any, util: any) {
    return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: context.Release.Name } };
}
`), 0o644))

		// Fake node_modules
		fakeUtilDir := filepath.Join(chartPath, "ts", "node_modules", "fake-util")
		require.NoError(t, os.MkdirAll(fakeUtilDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(fakeUtilDir, "package.json"), []byte(`{"name": "fake-util", "version": "1.0.0", "main": "index.js"}`), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(fakeUtilDir, "index.js"), []byte(`module.exports.util = function() { return 'utility'; };`), 0o644))

		err := ts.BuildVendorBundleToDir(context.Background(), chartPath)
		require.NoError(t, err)

		vendorPath := filepath.Join(chartPath, common.ChartTSVendorBundleFile)
		vendorContent, err := os.ReadFile(vendorPath)
		require.NoError(t, err)
		assert.Contains(t, string(vendorContent), "fake-util")
	})

	t.Run("works with JavaScript entrypoint", func(t *testing.T) {
		chartPath := filepath.Join(t.TempDir(), "my-chart")
		tsDir := filepath.Join(chartPath, "ts", "src")
		require.NoError(t, os.MkdirAll(tsDir, 0o755))

		require.NoError(t, os.WriteFile(filepath.Join(tsDir, "index.js"), []byte(`
const lib = require('js-lib');
exports.render = function(context) { return { manifests: [{ apiVersion: 'v1', kind: 'Pod' }] }; };
`), 0o644))

		// Fake node_modules
		jsLibDir := filepath.Join(chartPath, "ts", "node_modules", "js-lib")
		require.NoError(t, os.MkdirAll(jsLibDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(jsLibDir, "package.json"), []byte(`{"name": "js-lib", "version": "1.0.0", "main": "index.js"}`), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(jsLibDir, "index.js"), []byte(`module.exports = { hello: 'world' };`), 0o644))

		err := ts.BuildVendorBundleToDir(context.Background(), chartPath)
		require.NoError(t, err)

		vendorPath := filepath.Join(chartPath, common.ChartTSVendorBundleFile)
		vendorContent, err := os.ReadFile(vendorPath)
		require.NoError(t, err)
		assert.Contains(t, string(vendorContent), "js-lib")
	})
}
