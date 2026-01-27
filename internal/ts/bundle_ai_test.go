//go:build ai_tests

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

func TestAI_VendorBundle(t *testing.T) {
	t.Run("scoped package in vendor bundle", func(t *testing.T) {
		vendorBundle := `
var __NELM_VENDOR_BUNDLE__ = (function() {
    var __NELM_VENDOR__ = {};
    __NELM_VENDOR__['@myorg/utils'] = {
        formatName: function(name) { return '@myorg:' + name; }
    };
    if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }
    return { __NELM_VENDOR__: __NELM_VENDOR__ };
})();
`
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: common.ChartTSVendorBundleFile, Data: []byte(vendorBundle)},
				{Name: "ts/src/index.ts", Data: []byte(`
const utils = require('@myorg/utils');
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: utils.formatName(ctx.Release.Name) }
        }]
    };
}
`)},
			},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "scoped-test"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "name: '@myorg:scoped-test'")
	})

	t.Run("multiple packages in vendor bundle", func(t *testing.T) {
		vendorBundle := `
var __NELM_VENDOR_BUNDLE__ = (function() {
    var __NELM_VENDOR__ = {};
    __NELM_VENDOR__['lodash'] = {
        merge: function(a, b) { return Object.assign({}, a, b); },
        get: function(obj, path, def) {
            return path.split('.').reduce((o, k) => o && o[k], obj) || def;
        }
    };
    __NELM_VENDOR__['yaml'] = {
        stringify: function(obj) { return JSON.stringify(obj); }
    };
    if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }
    return { __NELM_VENDOR__: __NELM_VENDOR__ };
})();
`
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: common.ChartTSVendorBundleFile, Data: []byte(vendorBundle)},
				{Name: "ts/src/index.ts", Data: []byte(`
const _ = require('lodash');
const yaml = require('yaml');
export function render(ctx: any) {
    const base = { app: 'test' };
    const merged = _.merge(base, ctx.Values.labels || {});
    const nested = _.get(ctx, 'Values.deeply.nested', 'default');
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'multi-pkg', labels: merged },
            data: { nested, serialized: yaml.stringify(merged) }
        }]
    };
}
`)},
			},
		}
		values := chartutil.Values{"Values": map[string]any{"labels": map[string]any{"env": "prod"}}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "app: test")
		assert.Contains(t, yaml, "env: prod")
		assert.Contains(t, yaml, "nested: default")
	})

	t.Run("package with subpath import", func(t *testing.T) {
		vendorBundle := `
var __NELM_VENDOR_BUNDLE__ = (function() {
    var __NELM_VENDOR__ = {};
    __NELM_VENDOR__['mylib'] = {
        core: { create: function(name) { return { name: name }; } },
        utils: { format: function(s) { return s.toUpperCase(); } }
    };
    __NELM_VENDOR__['mylib/core'] = __NELM_VENDOR__['mylib'].core;
    __NELM_VENDOR__['mylib/utils'] = __NELM_VENDOR__['mylib'].utils;
    if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }
    return { __NELM_VENDOR__: __NELM_VENDOR__ };
})();
`
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: common.ChartTSVendorBundleFile, Data: []byte(vendorBundle)},
				{Name: "ts/src/index.ts", Data: []byte(`
const core = require('mylib/core');
const utils = require('mylib/utils');
export function render(ctx: any) {
    const obj = core.create(ctx.Release.Name);
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: utils.format(obj.name) }
        }]
    };
}
`)},
			},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "subpath"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "name: SUBPATH")
	})

	t.Run("missing package gives clear error", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
const missing = require('nonexistent-package');
export function render(ctx: any) {
    return { manifests: [missing.create()] };
}
`)},
			},
		}

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent-package")
	})

	t.Run("vendor bundle with complex nested exports", func(t *testing.T) {
		vendorBundle := `
var __NELM_VENDOR_BUNDLE__ = (function() {
    var __NELM_VENDOR__ = {};
    __NELM_VENDOR__['k8s-helpers'] = {
        metadata: {
            createLabels: function(name, version) {
                return {
                    'app.kubernetes.io/name': name,
                    'app.kubernetes.io/version': version,
                    'app.kubernetes.io/managed-by': 'nelm'
                };
            },
            createAnnotations: function(opts) {
                return { 'description': opts.description || 'No description' };
            }
        },
        resources: {
            configMap: function(name, data) {
                return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: name }, data: data };
            }
        }
    };
    if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }
    return { __NELM_VENDOR__: __NELM_VENDOR__ };
})();
`
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "2.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: common.ChartTSVendorBundleFile, Data: []byte(vendorBundle)},
				{Name: "ts/src/index.ts", Data: []byte(`
const k8s = require('k8s-helpers');
export function render(ctx: any) {
    const labels = k8s.metadata.createLabels(ctx.Release.Name, ctx.Chart.Version);
    const annotations = k8s.metadata.createAnnotations({ description: 'Test config' });
    const cm = k8s.resources.configMap(ctx.Release.Name + '-config', { key: 'value' });
    cm.metadata.labels = labels;
    cm.metadata.annotations = annotations;
    return { manifests: [cm] };
}
`)},
			},
		}
		values := chartutil.Values{
			"Release": map[string]any{"Name": "nested-vendor"},
		}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "app.kubernetes.io/name: nested-vendor")
		assert.Contains(t, yaml, "app.kubernetes.io/version: 2.0.0")
		assert.Contains(t, yaml, "app.kubernetes.io/managed-by: nelm")
		assert.Contains(t, yaml, "description: Test config")
	})

	t.Run("CommonJS require from vendor", func(t *testing.T) {
		vendorBundle := `
var __NELM_VENDOR_BUNDLE__ = (function() {
    var __NELM_VENDOR__ = {};
    __NELM_VENDOR__['cjs-module'] = {
        config: { name: 'cjs-config' },
        namedExport: function() { return 'named'; },
        anotherNamed: 'value'
    };
    if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }
    return { __NELM_VENDOR__: __NELM_VENDOR__ };
})();
`
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: common.ChartTSVendorBundleFile, Data: []byte(vendorBundle)},
				{Name: "ts/src/index.ts", Data: []byte(`
const cjsModule = require('cjs-module');
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'cjs-imports' },
            data: {
                configName: cjsModule.config.name,
                named: cjsModule.namedExport(),
                another: cjsModule.anotherNamed
            }
        }]
    };
}
`)},
			},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "configName: cjs-config")
		assert.Contains(t, yaml, "named: named")
		assert.Contains(t, yaml, "another: value")
	})
}
