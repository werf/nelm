package ts_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/nelm/internal/ts"
	"github.com/werf/nelm/pkg/common"
)

func TestRenderChartWithDependencies(t *testing.T) {
	t.Run("root and subchart both rendered", func(t *testing.T) {
		rootChart := createTestChartWithSubchart(
			`export function render(context: any) {
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'root-config' },
        data: { chartName: context.Chart.Name, message: context.Values.rootMessage || 'default' } }] };
}`,
			`export function render(context: any) {
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'subchart-config' },
        data: { chartName: context.Chart.Name, message: context.Values.subMessage || 'default' } }] };
}`,
		)

		values := chartutil.Values{
			"Values": map[string]any{
				"rootMessage": "Hello from root",
				"ts-subchart": map[string]any{"subMessage": "Hello from subchart"},
			},
			"Release":      map[string]any{"Name": "test-release", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), rootChart, values)
		require.NoError(t, err)

		assert.Len(t, result, 2)

		rootPath := "root-chart/" + common.ChartTSSourceDir + common.ChartTSEntryPointTS
		assert.Contains(t, result, rootPath)
		assert.Contains(t, result[rootPath], "name: root-config")
		assert.Contains(t, result[rootPath], "chartName: root-chart")
		assert.Contains(t, result[rootPath], "message: Hello from root")

		subPath := "root-chart/charts/ts-subchart/" + common.ChartTSSourceDir + common.ChartTSEntryPointTS
		assert.Contains(t, result, subPath)
		assert.Contains(t, result[subPath], "name: subchart-config")
		assert.Contains(t, result[subPath], "chartName: ts-subchart")
		assert.Contains(t, result[subPath], "message: Hello from subchart")
	})

	t.Run("classic root with TS subchart only", func(t *testing.T) {
		subchart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "ts-subchart", Version: "0.1.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
export function render(context: any) {
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'ts-subchart-only' },
        data: { chartName: context.Chart.Name, releaseName: context.Release.Name } }] };
}
`)},
			},
		}
		rootChart := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "classic-root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{}, // No TypeScript in root
		}
		rootChart.SetDependencies(subchart)

		values := chartutil.Values{
			"Values":       map[string]any{"ts-subchart": map[string]any{}},
			"Release":      map[string]any{"Name": "my-release", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), rootChart, values)
		require.NoError(t, err)

		assert.Len(t, result, 1)

		subPath := "classic-root/charts/ts-subchart/" + common.ChartTSSourceDir + common.ChartTSEntryPointTS
		assert.Contains(t, result, subPath)
		assert.Contains(t, result[subPath], "name: ts-subchart-only")
		assert.Contains(t, result[subPath], "releaseName: my-release")
	})

	t.Run("nested dependencies (3 levels)", func(t *testing.T) {
		sub2 := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "sub2", Version: "0.1.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(c: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'sub2' }, data: { level: 'sub2' } }] }; }`)}},
		}
		sub1 := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "sub1", Version: "0.1.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(c: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'sub1' }, data: { level: 'sub1' } }] }; }`)}},
		}
		sub1.SetDependencies(sub2)

		rootChart := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "nested-root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(c: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'root' }, data: { level: 'root' } }] }; }`)}},
		}
		rootChart.SetDependencies(sub1)

		values := chartutil.Values{
			"Values":       map[string]any{"sub1": map[string]any{"sub2": map[string]any{}}},
			"Release":      map[string]any{"Name": "test"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), rootChart, values)
		require.NoError(t, err)

		assert.Len(t, result, 3)
		assert.Contains(t, result, "nested-root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
		assert.Contains(t, result, "nested-root/charts/sub1/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
		assert.Contains(t, result, "nested-root/charts/sub1/charts/sub2/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)

		assert.Contains(t, result["nested-root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "level: root")
		assert.Contains(t, result["nested-root/charts/sub1/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "level: sub1")
		assert.Contains(t, result["nested-root/charts/sub1/charts/sub2/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "level: sub2")
	})

	t.Run("subchart error includes chart name", func(t *testing.T) {
		rootChart := createTestChartWithSubchart(
			`export function render(c: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'root' } }] }; }`,
			`export function render(c: any) { const x: any = null; x.foo.bar; return { manifests: [] }; }`,
		)

		values := chartutil.Values{
			"Values":       map[string]any{"ts-subchart": map[string]any{}},
			"Release":      map[string]any{"Name": "test"},
			"Capabilities": map[string]any{},
		}

		_, err := ts.RenderChart(context.Background(), rootChart, values)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ts-subchart")
	})
}

func TestRenderFiles(t *testing.T) {
	t.Run("no TypeScript source returns empty", func(t *testing.T) {
		ch := newTestChart(nil)
		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("simple manifest", func(t *testing.T) {
		ch := newChartWithTS(`
export function render(context: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: {
                name: context.Release.Name + '-config',
                namespace: context.Release.Namespace
            },
            data: { replicas: String(context.Values.replicas) }
        }]
    };
}
`)
		values := chartutil.Values{
			"Values":  map[string]any{"replicas": 3},
			"Release": map[string]any{"Name": "test-release", "Namespace": "default"},
		}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)

		require.Len(t, result, 1)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "kind: ConfigMap")
		assert.Contains(t, yaml, "name: test-release-config")
		assert.Contains(t, yaml, "namespace: default")
		assert.Contains(t, yaml, `replicas: "3"`)
	})

	t.Run("multiple resources with separator", func(t *testing.T) {
		ch := newChartWithTS(`
export function render(context: any) {
    return {
        manifests: [
            { apiVersion: 'v1', kind: 'Service', metadata: { name: context.Release.Name + '-svc' } },
            { apiVersion: 'apps/v1', kind: 'Deployment', metadata: { name: context.Release.Name + '-deploy' } }
        ]
    };
}
`)
		values := chartutil.Values{"Release": map[string]any{"Name": "test"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)

		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "kind: Service")
		assert.Contains(t, yaml, "kind: Deployment")
		assert.Contains(t, yaml, "---")
	})

	t.Run("null result returns empty", func(t *testing.T) {
		ch := newChartWithTS(`
export function render(context: any) {
    if (!context.Values.enabled) return null;
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'test' } }] };
}
`)
		values := chartutil.Values{"Values": map[string]any{"enabled": false}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("module.exports.render pattern", func(t *testing.T) {
		ch := newChartWithTS(`
module.exports.render = function(context: any) {
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'module-exports-test' } }] };
};
`)
		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)

		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: module-exports-test")
	})

	t.Run("module.exports object pattern", func(t *testing.T) {
		ch := newChartWithTS(`
module.exports = {
    render: function(context: any) {
        return { manifests: [{ apiVersion: 'v1', kind: 'Secret', metadata: { name: 'object-pattern-test' } }] };
    }
};
`)
		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)

		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "kind: Secret")
		assert.Contains(t, yaml, "name: object-pattern-test")
	})

	t.Run("arrow function and array methods", func(t *testing.T) {
		ch := newChartWithTS(`
export const render = (context: any) => {
    const prefix = context.Release.Name;
    const resources = [1, 2, 3].map(i => ({
        apiVersion: 'v1',
        kind: 'ConfigMap',
        metadata: { name: prefix + '-config-' + i },
        data: { index: String(i) }
    }));
    return { manifests: resources };
};
`)
		values := chartutil.Values{"Release": map[string]any{"Name": "my-app"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)

		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: my-app-config-1")
		assert.Contains(t, yaml, "name: my-app-config-2")
		assert.Contains(t, yaml, "name: my-app-config-3")
	})

	t.Run("TypeScript interfaces compile correctly", func(t *testing.T) {
		ch := newChartWithTS(`
interface RenderContext {
    Release: { Name: string; Namespace: string };
    Values: { replicas?: number };
}

interface Manifest {
    apiVersion: string;
    kind: string;
    metadata: { name: string };
    spec?: any;
}

export function render(context: RenderContext) {
    const manifest: Manifest = {
        apiVersion: 'apps/v1',
        kind: 'Deployment',
        metadata: { name: context.Release.Name },
        spec: { replicas: context.Values.replicas || 1 }
    };
    return { manifests: [manifest] };
}
`)
		values := chartutil.Values{
			"Release": map[string]any{"Name": "typed-app", "Namespace": "production"},
			"Values":  map[string]any{"replicas": 5},
		}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)

		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "kind: Deployment")
		assert.Contains(t, yaml, "name: typed-app")
		assert.Contains(t, yaml, "replicas: 5")
	})

	t.Run("multiple files with imports", func(t *testing.T) {
		ch := createChartWithTSFiles(map[string]string{
			"src/index.ts": `
import { createConfigMap } from './helpers';
export function render(context: any) {
    return { manifests: [createConfigMap(context.Release.Name)] };
}
`,
			"src/helpers.ts": `
export function createConfigMap(name: string) {
    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        metadata: { name: name + '-config' },
        data: { source: 'helper-function' }
    };
}
`,
		})
		values := chartutil.Values{"Release": map[string]any{"Name": "multi-file-app"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)

		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: multi-file-app-config")
		assert.Contains(t, yaml, "source: helper-function")
	})

	t.Run("error when render function missing", func(t *testing.T) {
		ch := newChartWithTS(`export function notRender(context: any) { return { manifests: [] }; }`)

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no 'render' function exported")
	})

	t.Run("runtime error shows source location", func(t *testing.T) {
		ch := newChartWithTS(`
export function render(context: any) {
    const obj: any = null;
    obj.nonExistentProperty;
    return { manifests: [] };
}
`)
		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index.ts")
	})

	t.Run("vendor bundle provides npm dependencies", func(t *testing.T) {
		vendorBundle := `
var __NELM_VENDOR_BUNDLE__ = (function() {
    var __NELM_VENDOR__ = {};
    __NELM_VENDOR__['fake-lib'] = {
        helper: function(name) {
            return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: name + '-from-vendor' } };
        }
    };
    if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }
    return { __NELM_VENDOR__: __NELM_VENDOR__ };
})();
`
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test-chart", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: common.ChartTSVendorBundleFile, Data: []byte(vendorBundle)},
				{Name: "ts/src/index.ts", Data: []byte(`
const fakeLib = require('fake-lib');
export function render(context: any) {
    return { manifests: [fakeLib.helper(context.Release.Name)] };
}
`)},
			},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "vendor-test"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)

		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: vendor-test-from-vendor")
	})

	t.Run("node_modules in RuntimeDepsFiles", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test-chart", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
import { helper } from 'fake-lib';
export function render(context: any) {
    return { manifests: [helper(context.Release.Name)] };
}
`)},
			},
			RuntimeDepsFiles: []*chart.File{
				{Name: "ts/node_modules/fake-lib/package.json", Data: []byte(`{"name": "fake-lib", "version": "1.0.0", "main": "index.js"}`)},
				{Name: "ts/node_modules/fake-lib/index.js", Data: []byte(`
module.exports.helper = function(name) {
    return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: name + '-from-npm' } };
};
`)},
			},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "npm-test"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)

		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: npm-test-from-npm")
	})
}

func TestScopeValuesForSubchart(t *testing.T) {
	t.Run("scopes values correctly", func(t *testing.T) {
		subchart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "my-subchart", Version: "2.0.0", AppVersion: "1.5.0", Description: "Test subchart"},
			Files:    []*chart.File{{Name: "README.md", Data: []byte("# Subchart")}},
		}

		parentValues := chartutil.Values{
			"Values": map[string]any{
				"rootKey": "rootValue",
				"my-subchart": map[string]any{
					"subKey": "subValue",
					"nested": map[string]any{"deep": "value"},
				},
			},
			"Release":      map[string]any{"Name": "test-release", "Namespace": "prod"},
			"Capabilities": map[string]any{"KubeVersion": map[string]any{"Version": "v1.28.0"}},
		}

		scoped := ts.ScopeValuesForSubchart(parentValues, "my-subchart", subchart)

		// Release copied
		assert.Equal(t, "test-release", scoped["Release"].(map[string]any)["Name"])
		assert.Equal(t, "prod", scoped["Release"].(map[string]any)["Namespace"])

		// Capabilities copied
		assert.NotNil(t, scoped["Capabilities"])

		// Chart metadata from subchart
		chartMeta := scoped["Chart"].(map[string]any)
		assert.Equal(t, "my-subchart", chartMeta["Name"])
		assert.Equal(t, "2.0.0", chartMeta["Version"])
		assert.Equal(t, "1.5.0", chartMeta["AppVersion"])

		// Values scoped to subchart
		scopedValues := scoped["Values"].(map[string]any)
		assert.Equal(t, "subValue", scopedValues["subKey"])
		assert.Equal(t, "value", scopedValues["nested"].(map[string]any)["deep"])
		assert.Nil(t, scopedValues["rootKey"])

		// Files from subchart
		assert.NotNil(t, scoped["Files"])
	})

	t.Run("missing subchart values returns empty map", func(t *testing.T) {
		subchart := &chart.Chart{Metadata: &chart.Metadata{Name: "missing-values-subchart", Version: "1.0.0"}}
		parentValues := chartutil.Values{
			"Values":  map[string]any{"other-subchart": map[string]any{"key": "value"}},
			"Release": map[string]any{"Name": "test"},
		}

		scoped := ts.ScopeValuesForSubchart(parentValues, "missing-values-subchart", subchart)

		assert.NotNil(t, scoped["Values"])
		assert.Empty(t, scoped["Values"])
	})
}

func createChartWithTSFiles(files map[string]string) *chart.Chart {
	var runtimeFiles []*chart.File
	for name, content := range files {
		runtimeFiles = append(runtimeFiles, &chart.File{Name: "ts/" + name, Data: []byte(content)})
	}

	return &chart.Chart{
		Metadata:     &chart.Metadata{Name: "test-chart", Version: "1.0.0"},
		RuntimeFiles: runtimeFiles,
	}
}

func createTestChartWithSubchart(rootContent, subchartContent string) *chart.Chart {
	subchart := &chart.Chart{
		Metadata:     &chart.Metadata{Name: "ts-subchart", Version: "0.1.0"},
		RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(subchartContent)}},
	}
	rootChart := &chart.Chart{
		Metadata:     &chart.Metadata{Name: "root-chart", Version: "1.0.0"},
		RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(rootContent)}},
	}
	rootChart.SetDependencies(subchart)

	return rootChart
}

func newChartWithTS(sourceContent string) *chart.Chart {
	return &chart.Chart{
		Metadata:     &chart.Metadata{Name: "test-chart", Version: "1.0.0"},
		RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(sourceContent)}},
	}
}

// Test helpers

func newTestChart(files map[string]string) *chart.Chart {
	var runtimeFiles []*chart.File
	for name, content := range files {
		runtimeFiles = append(runtimeFiles, &chart.File{Name: name, Data: []byte(content)})
	}

	return &chart.Chart{
		Metadata:     &chart.Metadata{Name: "test-chart", Version: "1.0.0"},
		RuntimeFiles: runtimeFiles,
	}
}
