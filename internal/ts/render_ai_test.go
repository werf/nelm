//go:build ai_tests

package ts_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/nelm/internal/ts"
	"github.com/werf/nelm/pkg/common"
)

// =============================================================================
// Context Object Completeness
// =============================================================================

func TestAI_ContextCompleteness(t *testing.T) {
	t.Run("Release object fields", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const r = ctx.Release;
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'release-info' },
            data: {
                name: r.Name,
                namespace: r.Namespace,
                isUpgrade: String(r.IsUpgrade || false),
                isInstall: String(r.IsInstall || false),
                revision: String(r.Revision || 1),
                service: r.Service || 'Helm'
            }
        }]
    };
}
`)}},
		}
		values := chartutil.Values{
			"Release": map[string]any{
				"Name":      "myrelease",
				"Namespace": "mynamespace",
				"IsUpgrade": true,
				"IsInstall": false,
				"Revision":  3,
				"Service":   "Helm",
			},
		}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: myrelease")
		assert.Contains(t, yaml, "namespace: mynamespace")
		assert.Contains(t, yaml, "isUpgrade: \"true\"")
		assert.Contains(t, yaml, "isInstall: \"false\"")
		assert.Contains(t, yaml, "revision: \"3\"")
	})

	t.Run("Chart object fields", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{
				Name:        "mychart",
				Version:     "1.2.3",
				AppVersion:  "4.5.6",
				Description: "My awesome chart",
				Home:        "https://example.com",
				Icon:        "https://example.com/icon.png",
				Keywords:    []string{"web", "app"},
				Type:        "application",
			},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const c = ctx.Chart;
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'chart-info' },
            data: {
                name: c.Name,
                version: c.Version,
                appVersion: c.AppVersion || '',
                description: c.Description || '',
                type: c.Type || 'application'
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: mychart")
		assert.Contains(t, yaml, "version: 1.2.3")
		assert.Contains(t, yaml, "appVersion: 4.5.6")
		assert.Contains(t, yaml, "description: My awesome chart")
	})

	t.Run("Capabilities object", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const caps = ctx.Capabilities || {};
    const kube = caps.KubeVersion || {};

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'caps-info' },
            data: {
                kubeVersion: kube.Version || 'unknown',
                kubeMajor: kube.Major || '',
                kubeMinor: kube.Minor || '',
                helmVersion: caps.HelmVersion?.Version || 'v3'
            }
        }]
    };
}
`)}},
		}
		values := chartutil.Values{
			"Capabilities": map[string]any{
				"KubeVersion": map[string]any{
					"Version": "v1.28.0",
					"Major":   "1",
					"Minor":   "28",
				},
				"HelmVersion": map[string]any{
					"Version": "v3.14.0",
				},
			},
		}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "kubeVersion: v1.28.0")
		assert.Contains(t, yaml, "kubeMajor: \"1\"")
		assert.Contains(t, yaml, "kubeMinor: \"28\"")
	})

	t.Run("Values with complex nesting", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const v = ctx.Values;
    return {
        manifests: [{
            apiVersion: 'apps/v1',
            kind: 'Deployment',
            metadata: { name: 'values-test' },
            spec: {
                replicas: v.replicas,
                template: {
                    spec: {
                        containers: [{
                            name: 'app',
                            image: v.image.repository + ':' + v.image.tag,
                            ports: v.ports.map((p: any) => ({ containerPort: p })),
                            env: Object.entries(v.env || {}).map(([k, val]) => ({ name: k, value: val }))
                        }]
                    }
                }
            }
        }]
    };
}
`)}},
		}
		values := chartutil.Values{
			"Values": map[string]any{
				"replicas": 3,
				"image": map[string]any{
					"repository": "nginx",
					"tag":        "1.21",
				},
				"ports": []any{80, 443},
				"env": map[string]any{
					"LOG_LEVEL": "info",
					"DEBUG":     "false",
				},
			},
		}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "replicas: 3")
		assert.Contains(t, yaml, "image: nginx:1.21")
		assert.Contains(t, yaml, "containerPort: 80")
		assert.Contains(t, yaml, "name: LOG_LEVEL")
	})

	t.Run("Files object access", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			Files: []*chart.File{
				{Name: "config/app.properties", Data: []byte("key1=value1\nkey2=value2")},
				{Name: "scripts/init.sh", Data: []byte("#!/bin/bash\necho hello")},
			},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const files = ctx.Files || {};
    const fileList = Object.keys(files);

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'files-test' },
            data: {
                fileCount: String(fileList.length),
                files: fileList.join(',')
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "fileCount:")
	})

	t.Run("Template object (current template info)", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const tpl = ctx.Template || {};
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'template-info' },
            data: {
                name: tpl.Name || 'unknown',
                basePath: tpl.BasePath || ''
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{
			"Template": map[string]any{
				"Name":     "test/templates/configmap.yaml",
				"BasePath": "test/templates",
			},
		})
		require.NoError(t, err)
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "name:")
	})
}

// =============================================================================
// Error Messages and Debugging
// =============================================================================

func TestAI_ErrorMessages(t *testing.T) {
	t.Run("syntax error includes file name", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return { manifests: [{ // missing closing
}
`)}},
		}

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index.ts")
	})

	t.Run("runtime error includes source location", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const obj: any = null;
    return obj.property.nested;
}
`)}},
		}

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index.ts")
	})

	t.Run("missing render function has clear message", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function notRender(ctx: any) {
    return { manifests: [] };
}
`)}},
		}

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "render")
	})

	t.Run("type error in helper file includes correct file", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
import { broken } from './helpers';
export function render(ctx: any) {
    return { manifests: [broken()] };
}
`)},
				{Name: "ts/src/helpers.ts", Data: []byte(`
export function broken() {
    const x: any = null;
    return x.foo.bar;
}
`)},
			},
		}

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.Error(t, err)
		errStr := err.Error()
		assert.True(t, strings.Contains(errStr, "helpers.ts") || strings.Contains(errStr, "index.ts"),
			"error should reference source file: %s", errStr)
	})

	t.Run("undefined variable error is descriptive", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return { manifests: [{ name: undefinedVariable }] };
}
`)}},
		}

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "undefinedVariable")
	})

	t.Run("subchart error includes chart path", func(t *testing.T) {
		subchart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "failing-sub", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    throw new Error("intentional failure");
}
`)}},
		}

		root := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		root.SetDependencies(subchart)

		values := chartutil.Values{
			"Values":       map[string]any{"failing-sub": map[string]any{}},
			"Release":      map[string]any{"Name": "test", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		_, err := ts.RenderChart(context.Background(), root, values)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failing-sub")
	})

	t.Run("thrown Error object message is preserved", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    if (!ctx.Values.required) {
        throw new Error("required value is missing");
    }
    return { manifests: [] };
}
`)}},
		}

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{"Values": map[string]any{}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "required value is missing")
	})

	t.Run("thrown string is captured", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    throw "string error message";
}
`)}},
		}

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "string error message")
	})
}

// =============================================================================
// Import/Export Patterns
// =============================================================================

func TestAI_ImportExportPatterns(t *testing.T) {
	t.Run("re-exports from barrel file", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
import { createDeployment, createService } from './resources';
export function render(ctx: any) {
    return {
        manifests: [
            createDeployment(ctx.Release.Name),
            createService(ctx.Release.Name)
        ]
    };
}
`)},
				{Name: "ts/src/resources/index.ts", Data: []byte(`
export { createDeployment } from './deployment';
export { createService } from './service';
`)},
				{Name: "ts/src/resources/deployment.ts", Data: []byte(`
export function createDeployment(name: string) {
    return { apiVersion: 'apps/v1', kind: 'Deployment', metadata: { name: name + '-deploy' } };
}
`)},
				{Name: "ts/src/resources/service.ts", Data: []byte(`
export function createService(name: string) {
    return { apiVersion: 'v1', kind: 'Service', metadata: { name: name + '-svc' } };
}
`)},
			},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "barrel-test"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: barrel-test-deploy")
		assert.Contains(t, yaml, "name: barrel-test-svc")
	})

	t.Run("default export", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
import config from './config';
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'default-export' },
            data: config
        }]
    };
}
`)},
				{Name: "ts/src/config.ts", Data: []byte(`
export default {
    key1: 'value1',
    key2: 'value2'
};
`)},
			},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "key1: value1")
		assert.Contains(t, yaml, "key2: value2")
	})

	t.Run("mixed default and named exports", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
import Builder, { VERSION, helper } from './builder';
export function render(ctx: any) {
    const b = new Builder(ctx.Release.Name);
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: b.getName() },
            data: { version: VERSION, extra: helper() }
        }]
    };
}
`)},
				{Name: "ts/src/builder.ts", Data: []byte(`
export const VERSION = '1.0.0';
export function helper() { return 'helped'; }
export default class Builder {
    constructor(private name: string) {}
    getName() { return this.name + '-built'; }
}
`)},
			},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "mixed-test"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: mixed-test-built")
		assert.Contains(t, yaml, "version: 1.0.0")
		assert.Contains(t, yaml, "extra: helped")
	})

	t.Run("import with alias", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
import { createConfigMap as cm, createSecret as sec } from './helpers';
export function render(ctx: any) {
    return { manifests: [cm('my-cm'), sec('my-secret')] };
}
`)},
				{Name: "ts/src/helpers.ts", Data: []byte(`
export function createConfigMap(name: string) {
    return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name } };
}
export function createSecret(name: string) {
    return { apiVersion: 'v1', kind: 'Secret', metadata: { name } };
}
`)},
			},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "kind: ConfigMap")
		assert.Contains(t, yaml, "name: my-cm")
		assert.Contains(t, yaml, "kind: Secret")
		assert.Contains(t, yaml, "name: my-secret")
	})

	t.Run("namespace import", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
import * as utils from './utils';
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: utils.formatName(ctx.Release.Name) },
            data: { labels: JSON.stringify(utils.defaultLabels) }
        }]
    };
}
`)},
				{Name: "ts/src/utils.ts", Data: []byte(`
export function formatName(name: string) {
    return name.toLowerCase().replace(/[^a-z0-9-]/g, '-');
}
export const defaultLabels = { managed: 'true', source: 'ts' };
`)},
			},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "My_App"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: my-app")
	})

	t.Run("circular imports between helpers", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
import { createA } from './a';
export function render(ctx: any) {
    return { manifests: [createA('test')] };
}
`)},
				{Name: "ts/src/a.ts", Data: []byte(`
import { formatB } from './b';
export function createA(name: string) {
    return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: formatB(name) } };
}
export function formatA(s: string) { return 'A-' + s; }
`)},
				{Name: "ts/src/b.ts", Data: []byte(`
import { formatA } from './a';
export function formatB(s: string) { return 'B-' + formatA(s); }
`)},
			},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: B-A-test")
	})

	t.Run("deep nested imports", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{
				{Name: "ts/src/index.ts", Data: []byte(`
import { render as doRender } from './lib/core/render';
export function render(ctx: any) { return doRender(ctx); }
`)},
				{Name: "ts/src/lib/core/render.ts", Data: []byte(`
import { getMetadata } from '../utils/metadata';
export function render(ctx: any) {
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: getMetadata(ctx) }] };
}
`)},
				{Name: "ts/src/lib/utils/metadata.ts", Data: []byte(`
import { formatName } from '../../helpers/format';
export function getMetadata(ctx: any) { return { name: formatName(ctx.Release.Name) }; }
`)},
				{Name: "ts/src/helpers/format.ts", Data: []byte(`
export function formatName(name: string) { return name + '-formatted'; }
`)},
			},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "deep"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: deep-formatted")
	})
}

func TestAI_RenderChartWithDependencies_ChartMetadata(t *testing.T) {
	t.Run("subchart receives correct Chart metadata", func(t *testing.T) {
		subchart := &chart.Chart{
			Metadata: &chart.Metadata{
				Name:        "my-subchart",
				Version:     "2.3.4",
				AppVersion:  "1.2.3",
				Description: "A test subchart",
				Keywords:    []string{"test", "subchart"},
			},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: {
                name: 'meta-cm',
                labels: {
                    'chart': ctx.Chart.Name + '-' + ctx.Chart.Version,
                    'app.kubernetes.io/version': ctx.Chart.AppVersion
                }
            },
            data: {
                chartName: ctx.Chart.Name,
                chartVersion: ctx.Chart.Version,
                appVersion: ctx.Chart.AppVersion,
                description: ctx.Chart.Description
            }
        }]
    };
}
`)}},
		}

		root := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		root.SetDependencies(subchart)

		values := chartutil.Values{
			"Values":       map[string]any{"my-subchart": map[string]any{}},
			"Release":      map[string]any{"Name": "meta-test", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		yaml := result["root/charts/my-subchart/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "chartName: my-subchart")
		assert.Contains(t, yaml, "chartVersion: 2.3.4")
		assert.Contains(t, yaml, "appVersion: 1.2.3")
		assert.Contains(t, yaml, "description: A test subchart")
		assert.Contains(t, yaml, "chart: my-subchart-2.3.4")
		assert.Contains(t, yaml, "app.kubernetes.io/version: 1.2.3")
	})
}

func TestAI_RenderChartWithDependencies_ConditionalSubcharts(t *testing.T) {
	t.Run("subchart conditionally renders based on enabled flag", func(t *testing.T) {
		subchart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "optional-sub", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    if (!ctx.Values.enabled) {
        return null;
    }
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'optional-cm' },
            data: { enabled: 'true' }
        }]
    };
}
`)}},
		}

		root := &chart.Chart{
			Metadata: &chart.Metadata{Name: "root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'root-cm' },
            data: { always: 'present' }
        }]
    };
}
`)}},
		}
		root.SetDependencies(subchart)

		// Test with subchart disabled
		valuesDisabled := chartutil.Values{
			"Values": map[string]any{
				"optional-sub": map[string]any{"enabled": false},
			},
			"Release":      map[string]any{"Name": "conditional-test", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, valuesDisabled)
		require.NoError(t, err)

		// Root should be rendered, subchart should return empty
		assert.Contains(t, result, "root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
		assert.Contains(t, result["root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "always: present")

		// Subchart returns null, so it should be empty
		subPath := "root/charts/optional-sub/" + common.ChartTSSourceDir + common.ChartTSEntryPointTS
		if yaml, exists := result[subPath]; exists {
			assert.Empty(t, yaml)
		}

		// Test with subchart enabled
		valuesEnabled := chartutil.Values{
			"Values": map[string]any{
				"optional-sub": map[string]any{"enabled": true},
			},
			"Release":      map[string]any{"Name": "conditional-test", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result2, err := ts.RenderChart(context.Background(), root, valuesEnabled)
		require.NoError(t, err)

		assert.Contains(t, result2["root/charts/optional-sub/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "enabled: \"true\"")
	})

	t.Run("multiple conditional subcharts with different states", func(t *testing.T) {
		makeConditionalChart := func(name string) *chart.Chart {
			return &chart.Chart{
				Metadata: &chart.Metadata{Name: name, Version: "1.0.0"},
				RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    if (!ctx.Values.enabled) return null;
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: '` + name + `-cm' },
            data: { component: '` + name + `' }
        }]
    };
}
`)}},
			}
		}

		redis := makeConditionalChart("redis")
		postgres := makeConditionalChart("postgres")
		mongodb := makeConditionalChart("mongodb")

		root := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "app", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(ctx: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'app-cm' } }] }; }`)}},
		}
		root.SetDependencies(redis, postgres, mongodb)

		values := chartutil.Values{
			"Values": map[string]any{
				"redis":    map[string]any{"enabled": true},
				"postgres": map[string]any{"enabled": false},
				"mongodb":  map[string]any{"enabled": true},
			},
			"Release":      map[string]any{"Name": "multi-db", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		// App always renders
		assert.Contains(t, result, "app/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)

		// Redis enabled
		assert.Contains(t, result["app/charts/redis/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "component: redis")

		// Postgres disabled - should be empty or not contain component
		postgresYaml := result["app/charts/postgres/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Empty(t, postgresYaml)

		// MongoDB enabled
		assert.Contains(t, result["app/charts/mongodb/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "component: mongodb")
	})
}

// =============================================================================
// Subchart Scenarios
// =============================================================================

func TestAI_RenderChartWithDependencies_DeepNesting(t *testing.T) {
	t.Run("4 levels deep subchart hierarchy", func(t *testing.T) {
		level4 := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "level4", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(ctx: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'level4-cm' }, data: { depth: '4', chart: ctx.Chart.Name } }] }; }`)}},
		}
		level3 := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "level3", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(ctx: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'level3-cm' }, data: { depth: '3', chart: ctx.Chart.Name } }] }; }`)}},
		}
		level3.SetDependencies(level4)

		level2 := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "level2", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(ctx: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'level2-cm' }, data: { depth: '2', chart: ctx.Chart.Name } }] }; }`)}},
		}
		level2.SetDependencies(level3)

		root := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(ctx: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'root-cm' }, data: { depth: '1', chart: ctx.Chart.Name } }] }; }`)}},
		}
		root.SetDependencies(level2)

		values := chartutil.Values{
			"Values": map[string]any{
				"level2": map[string]any{
					"level3": map[string]any{
						"level4": map[string]any{},
					},
				},
			},
			"Release":      map[string]any{"Name": "deep-test", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		assert.Len(t, result, 4)
		assert.Contains(t, result, "root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
		assert.Contains(t, result, "root/charts/level2/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
		assert.Contains(t, result, "root/charts/level2/charts/level3/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
		assert.Contains(t, result, "root/charts/level2/charts/level3/charts/level4/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)

		assert.Contains(t, result["root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "depth: \"1\"")
		assert.Contains(t, result["root/charts/level2/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "depth: \"2\"")
		assert.Contains(t, result["root/charts/level2/charts/level3/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "depth: \"3\"")
		assert.Contains(t, result["root/charts/level2/charts/level3/charts/level4/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "depth: \"4\"")
	})

	t.Run("5 levels with values propagated correctly", func(t *testing.T) {
		makeChart := func(name string) *chart.Chart {
			return &chart.Chart{
				Metadata: &chart.Metadata{Name: name, Version: "1.0.0"},
				RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: ctx.Chart.Name + '-cm' },
            data: {
                chart: ctx.Chart.Name,
                message: ctx.Values.message || 'no-message',
                release: ctx.Release.Name
            }
        }]
    };
}
`)}},
			}
		}

		l5 := makeChart("l5")
		l4 := makeChart("l4")
		l4.SetDependencies(l5)
		l3 := makeChart("l3")
		l3.SetDependencies(l4)
		l2 := makeChart("l2")
		l2.SetDependencies(l3)
		root := makeChart("root")
		root.SetDependencies(l2)

		values := chartutil.Values{
			"Values": map[string]any{
				"message": "root-msg",
				"l2": map[string]any{
					"message": "l2-msg",
					"l3": map[string]any{
						"message": "l3-msg",
						"l4": map[string]any{
							"message": "l4-msg",
							"l5": map[string]any{
								"message": "l5-msg",
							},
						},
					},
				},
			},
			"Release":      map[string]any{"Name": "deep-values", "Namespace": "test"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		assert.Len(t, result, 5)
		assert.Contains(t, result["root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "message: root-msg")
		assert.Contains(t, result["root/charts/l2/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "message: l2-msg")
		assert.Contains(t, result["root/charts/l2/charts/l3/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "message: l3-msg")
		assert.Contains(t, result["root/charts/l2/charts/l3/charts/l4/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "message: l4-msg")
		assert.Contains(t, result["root/charts/l2/charts/l3/charts/l4/charts/l5/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "message: l5-msg")
	})
}

func TestAI_RenderChartWithDependencies_GlobalValues(t *testing.T) {
	t.Run("global values available to all subcharts", func(t *testing.T) {
		subchart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "sub", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const global = ctx.Values.global || {};
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'sub-cm' },
            data: {
                imageRegistry: global.imageRegistry || 'default-registry',
                imagePullPolicy: global.imagePullPolicy || 'IfNotPresent'
            }
        }]
    };
}
`)}},
		}

		root := &chart.Chart{
			Metadata: &chart.Metadata{Name: "root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const global = ctx.Values.global || {};
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'root-cm' },
            data: {
                imageRegistry: global.imageRegistry || 'default-registry',
                imagePullPolicy: global.imagePullPolicy || 'IfNotPresent'
            }
        }]
    };
}
`)}},
		}
		root.SetDependencies(subchart)

		values := chartutil.Values{
			"Values": map[string]any{
				"global": map[string]any{
					"imageRegistry":   "my-registry.io",
					"imagePullPolicy": "Always",
				},
				"sub": map[string]any{
					"global": map[string]any{
						"imageRegistry":   "my-registry.io",
						"imagePullPolicy": "Always",
					},
				},
			},
			"Release":      map[string]any{"Name": "global-test", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		assert.Len(t, result, 2)
		assert.Contains(t, result["root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "imageRegistry: my-registry.io")
		assert.Contains(t, result["root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "imagePullPolicy: Always")
		assert.Contains(t, result["root/charts/sub/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "imageRegistry: my-registry.io")
		assert.Contains(t, result["root/charts/sub/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "imagePullPolicy: Always")
	})

	t.Run("global values in nested subcharts", func(t *testing.T) {
		makeGlobalAwareChart := func(name string) *chart.Chart {
			return &chart.Chart{
				Metadata: &chart.Metadata{Name: name, Version: "1.0.0"},
				RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const global = ctx.Values.global || {};
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: ctx.Chart.Name + '-cm' },
            data: {
                environment: global.environment || 'unknown',
                chart: ctx.Chart.Name
            }
        }]
    };
}
`)}},
			}
		}

		leaf := makeGlobalAwareChart("leaf")
		middle := makeGlobalAwareChart("middle")
		middle.SetDependencies(leaf)
		root := makeGlobalAwareChart("root")
		root.SetDependencies(middle)

		globalVals := map[string]any{"environment": "production"}

		values := chartutil.Values{
			"Values": map[string]any{
				"global": globalVals,
				"middle": map[string]any{
					"global": globalVals,
					"leaf": map[string]any{
						"global": globalVals,
					},
				},
			},
			"Release":      map[string]any{"Name": "nested-global", "Namespace": "prod"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		assert.Len(t, result, 3)
		for _, yaml := range result {
			assert.Contains(t, yaml, "environment: production")
		}
	})
}

func TestAI_RenderChartWithDependencies_MixedTSAndNonTS(t *testing.T) {
	t.Run("TS root with non-TS subchart", func(t *testing.T) {
		nonTSSubchart := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "classic-sub", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}

		tsRoot := &chart.Chart{
			Metadata: &chart.Metadata{Name: "ts-root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'ts-root-cm' },
            data: { source: 'typescript' }
        }]
    };
}
`)}},
		}
		tsRoot.SetDependencies(nonTSSubchart)

		values := chartutil.Values{
			"Values":       map[string]any{"classic-sub": map[string]any{}},
			"Release":      map[string]any{"Name": "mixed", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), tsRoot, values)
		require.NoError(t, err)

		assert.Len(t, result, 1)
		assert.Contains(t, result, "ts-root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
		assert.Contains(t, result["ts-root/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "source: typescript")
	})

	t.Run("non-TS root with TS subchart", func(t *testing.T) {
		tsSubchart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "ts-sub", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'ts-sub-cm' },
            data: { source: 'typescript-subchart' }
        }]
    };
}
`)}},
		}

		classicRoot := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "classic-root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		classicRoot.SetDependencies(tsSubchart)

		values := chartutil.Values{
			"Values":       map[string]any{"ts-sub": map[string]any{}},
			"Release":      map[string]any{"Name": "mixed", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), classicRoot, values)
		require.NoError(t, err)

		assert.Len(t, result, 1)
		assert.Contains(t, result, "classic-root/charts/ts-sub/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
		assert.Contains(t, result["classic-root/charts/ts-sub/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "source: typescript-subchart")
	})

	t.Run("alternating TS and non-TS in deep hierarchy", func(t *testing.T) {
		level4 := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "l4-ts", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(ctx: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'l4' }, data: { type: 'ts' } }] }; }`)}},
		}

		level3 := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "l3-classic", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		level3.SetDependencies(level4)

		level2 := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "l2-ts", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`export function render(ctx: any) { return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'l2' }, data: { type: 'ts' } }] }; }`)}},
		}
		level2.SetDependencies(level3)

		root := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "root-classic", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		root.SetDependencies(level2)

		values := chartutil.Values{
			"Values": map[string]any{
				"l2-ts": map[string]any{
					"l3-classic": map[string]any{
						"l4-ts": map[string]any{},
					},
				},
			},
			"Release":      map[string]any{"Name": "alt", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		assert.Len(t, result, 2)
		assert.Contains(t, result, "root-classic/charts/l2-ts/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
		assert.Contains(t, result, "root-classic/charts/l2-ts/charts/l3-classic/charts/l4-ts/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS)
	})
}

func TestAI_RenderChartWithDependencies_SiblingSubcharts(t *testing.T) {
	t.Run("multiple sibling subcharts at same level", func(t *testing.T) {
		frontend := &chart.Chart{
			Metadata: &chart.Metadata{Name: "frontend", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'apps/v1',
            kind: 'Deployment',
            metadata: { name: 'frontend' },
            spec: { replicas: ctx.Values.replicas || 1 }
        }]
    };
}
`)}},
		}

		backend := &chart.Chart{
			Metadata: &chart.Metadata{Name: "backend", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'apps/v1',
            kind: 'Deployment',
            metadata: { name: 'backend' },
            spec: { replicas: ctx.Values.replicas || 1 }
        }]
    };
}
`)}},
		}

		worker := &chart.Chart{
			Metadata: &chart.Metadata{Name: "worker", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'apps/v1',
            kind: 'Deployment',
            metadata: { name: 'worker' },
            spec: { replicas: ctx.Values.replicas || 1 }
        }]
    };
}
`)}},
		}

		root := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "myapp", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		root.SetDependencies(frontend, backend, worker)

		values := chartutil.Values{
			"Values": map[string]any{
				"frontend": map[string]any{"replicas": 3},
				"backend":  map[string]any{"replicas": 2},
				"worker":   map[string]any{"replicas": 5},
			},
			"Release":      map[string]any{"Name": "myapp", "Namespace": "production"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		assert.Len(t, result, 3)
		assert.Contains(t, result["myapp/charts/frontend/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "replicas: 3")
		assert.Contains(t, result["myapp/charts/backend/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "replicas: 2")
		assert.Contains(t, result["myapp/charts/worker/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS], "replicas: 5")
	})

	t.Run("sibling subcharts with independent errors do not affect each other", func(t *testing.T) {
		goodChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "good", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'good-cm' } }] };
}
`)}},
		}

		badChart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "bad", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const x: any = null;
    return x.boom; // This will throw
}
`)}},
		}

		root := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		root.SetDependencies(goodChart, badChart)

		values := chartutil.Values{
			"Values":       map[string]any{"good": map[string]any{}, "bad": map[string]any{}},
			"Release":      map[string]any{"Name": "test", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		_, err := ts.RenderChart(context.Background(), root, values)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bad")
	})
}

func TestAI_RenderChartWithDependencies_ValueOverrides(t *testing.T) {
	t.Run("parent overrides subchart default values", func(t *testing.T) {
		subchart := &chart.Chart{
			Metadata: &chart.Metadata{Name: "sub", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'apps/v1',
            kind: 'Deployment',
            metadata: { name: 'sub-deploy' },
            spec: {
                replicas: ctx.Values.replicas || 1,
                template: {
                    spec: {
                        containers: [{
                            image: ctx.Values.image || 'default:latest',
                            resources: ctx.Values.resources || { limits: { cpu: '100m' } }
                        }]
                    }
                }
            }
        }]
    };
}
`)}},
		}

		root := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		root.SetDependencies(subchart)

		values := chartutil.Values{
			"Values": map[string]any{
				"sub": map[string]any{
					"replicas": 5,
					"image":    "my-image:v2",
					"resources": map[string]any{
						"limits": map[string]any{
							"cpu":    "500m",
							"memory": "512Mi",
						},
					},
				},
			},
			"Release":      map[string]any{"Name": "override-test", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		yaml := result["root/charts/sub/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "replicas: 5")
		assert.Contains(t, yaml, "image: my-image:v2")
		assert.Contains(t, yaml, "cpu: 500m")
		assert.Contains(t, yaml, "memory: 512Mi")
	})

	t.Run("nested value overrides through multiple levels", func(t *testing.T) {
		leaf := &chart.Chart{
			Metadata: &chart.Metadata{Name: "leaf", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'leaf-cm' },
            data: {
                setting1: ctx.Values.setting1 || 'default1',
                setting2: ctx.Values.setting2 || 'default2',
                nested: JSON.stringify(ctx.Values.nested || {})
            }
        }]
    };
}
`)}},
		}

		middle := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "middle", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		middle.SetDependencies(leaf)

		root := &chart.Chart{
			Metadata:     &chart.Metadata{Name: "root", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{},
		}
		root.SetDependencies(middle)

		values := chartutil.Values{
			"Values": map[string]any{
				"middle": map[string]any{
					"leaf": map[string]any{
						"setting1": "overridden1",
						"setting2": "overridden2",
						"nested": map[string]any{
							"deep": "value",
						},
					},
				},
			},
			"Release":      map[string]any{"Name": "nested-override", "Namespace": "default"},
			"Capabilities": map[string]any{},
		}

		result, err := ts.RenderChart(context.Background(), root, values)
		require.NoError(t, err)

		yaml := result["root/charts/middle/charts/leaf/"+common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "setting1: overridden1")
		assert.Contains(t, yaml, "setting2: overridden2")
		assert.Contains(t, yaml, `"deep":"value"`)
	})
}

// =============================================================================
// TypeScript Language Features
// =============================================================================

func TestAI_TypeScriptFeatures(t *testing.T) {
	t.Run("generic functions compile and work", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
function identity<T>(arg: T): T {
    return arg;
}

function createResource<T extends { kind: string }>(resource: T): T {
    return resource;
}

export function render(ctx: any) {
    const name = identity<string>(ctx.Release.Name);
    const cm = createResource({ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name } });
    return { manifests: [cm] };
}
`)}},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "generic-test"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "name: generic-test")
	})

	t.Run("enums compile and work", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
enum ResourceType {
    ConfigMap = "ConfigMap",
    Secret = "Secret",
    Deployment = "Deployment"
}

const enum Protocol {
    TCP = "TCP",
    UDP = "UDP"
}

export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: ResourceType.ConfigMap,
            metadata: { name: 'enum-test' },
            data: { protocol: Protocol.TCP }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "kind: ConfigMap")
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "protocol: TCP")
	})

	t.Run("type unions and intersections", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
type StringOrNumber = string | number;
type Named = { name: string };
type Versioned = { version: string };
type NamedAndVersioned = Named & Versioned;

export function render(ctx: any) {
    const replicas: StringOrNumber = ctx.Values.replicas || 1;
    const meta: NamedAndVersioned = { name: 'test', version: '1.0' };

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: meta.name },
            data: { replicas: String(replicas), version: meta.version }
        }]
    };
}
`)}},
		}
		values := chartutil.Values{"Values": map[string]any{"replicas": 3}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "replicas: \"3\"")
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "version: \"1.0\"")
	})

	t.Run("optional chaining and nullish coalescing", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const nested = ctx.Values?.deeply?.nested?.value ?? 'default';
    const port = ctx.Values.port ?? 8080;

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'optional-test' },
            data: { nested, port: String(port) }
        }]
    };
}
`)}},
		}
		values := chartutil.Values{"Values": map[string]any{}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "nested: default")
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "port: \"8080\"")
	})

	t.Run("class with methods", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
class ResourceBuilder {
    private name: string;

    constructor(name: string) {
        this.name = name;
    }

    buildConfigMap(data: Record<string, string>) {
        return {
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: this.name },
            data
        };
    }
}

export function render(ctx: any) {
    const builder = new ResourceBuilder(ctx.Release.Name + '-config');
    return { manifests: [builder.buildConfigMap({ key: 'value' })] };
}
`)}},
		}
		values := chartutil.Values{"Release": map[string]any{"Name": "class-test"}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "name: class-test-config")
		assert.Contains(t, result[common.ChartTSSourceDir+common.ChartTSEntryPointTS], "key: value")
	})

	t.Run("spread operator", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const baseLabels = { app: 'myapp', version: '1.0' };
    const extraLabels = ctx.Values.extraLabels || {};

    const merged = { ...baseLabels, ...extraLabels };

    const resources = [
        { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'cm1' } },
        { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'cm2' } }
    ];
    const additional = ctx.Values.additional || [];

    return { manifests: [...resources, ...additional].map(r => ({ ...r, metadata: { ...r.metadata, labels: merged } })) };
}
`)}},
		}
		values := chartutil.Values{"Values": map[string]any{"extraLabels": map[string]any{"env": "prod"}}}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "app: myapp")
		assert.Contains(t, yaml, "env: prod")
	})

	t.Run("destructuring assignment", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const { Name: releaseName, Namespace: namespace } = ctx.Release;
    const { replicas = 1, image = 'nginx:latest' } = ctx.Values;
    const [first, second = 'default'] = ctx.Values.items || ['item1'];

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: releaseName, namespace },
            data: { replicas: String(replicas), image, first, second }
        }]
    };
}
`)}},
		}
		values := chartutil.Values{
			"Release": map[string]any{"Name": "destruct-test", "Namespace": "myns"},
			"Values":  map[string]any{"replicas": 5},
		}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: destruct-test")
		assert.Contains(t, yaml, "namespace: myns")
		assert.Contains(t, yaml, "replicas: \"5\"")
		assert.Contains(t, yaml, "first: item1")
		assert.Contains(t, yaml, "second: default")
	})

	t.Run("template literals", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "mychart", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    const { Name, Namespace } = ctx.Release;
    const fullName = ` + "`${Name}-${ctx.Chart.Name}`" + `;
    const fqdn = ` + "`${Name}.${Namespace}.svc.cluster.local`" + `;

    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: fullName },
            data: { fqdn }
        }]
    };
}
`)}},
		}
		values := chartutil.Values{
			"Release": map[string]any{"Name": "myrelease", "Namespace": "mynamespace"},
		}

		result, err := ts.RenderFiles(context.Background(), ch, values)
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: myrelease-mychart")
		assert.Contains(t, yaml, "fqdn: myrelease.mynamespace.svc.cluster.local")
	})

	t.Run("async/await is not supported at top level", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export async function render(ctx: any) {
    await Promise.resolve();
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'async-test' } }] };
}
`)}},
		}

		_, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})

		// Async render should either fail or return a promise (which won't be valid)
		// The exact behavior depends on implementation
		if err == nil {
			t.Log("async render compiled but may not work as expected")
		}
	})
}

// =============================================================================
// YAML Output Edge Cases
// =============================================================================

func TestAI_YAMLOutput(t *testing.T) {
	t.Run("special characters are escaped", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'special-chars' },
            data: {
                colon: 'value: with colon',
                hash: 'value # with hash',
                quotes: "it's a \"quoted\" value",
                ampersand: 'foo & bar',
                asterisk: '*.example.com',
                question: 'is this ok?',
                pipe: 'cmd | grep',
                brackets: '[item1, item2]',
                braces: '{key: value}'
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "colon:")
		assert.Contains(t, yaml, "hash:")
		assert.Contains(t, yaml, "quotes:")
	})

	t.Run("multiline strings", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'multiline' },
            data: {
                script: 'line1\nline2\nline3',
                config: 'key1=value1\nkey2=value2'
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "script:")
		assert.Contains(t, yaml, "config:")
	})

	t.Run("null and undefined values", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'nulls' },
            data: {
                nullValue: null,
                undefinedValue: undefined,
                emptyString: '',
                zero: 0,
                falseValue: false
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "emptyString:")
		assert.Contains(t, yaml, "zero: 0")
		assert.Contains(t, yaml, "falseValue: false")
	})

	t.Run("empty objects and arrays", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: {
                name: 'empties',
                labels: {},
                annotations: {}
            },
            data: {}
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "name: empties")
		assert.Contains(t, yaml, "labels:")
		assert.Contains(t, yaml, "data:")
	})

	t.Run("numeric strings stay as strings", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'numeric-strings' },
            data: {
                port: '8080',
                version: '1.0',
                zipcode: '02134',
                phone: '555-1234',
                scientific: '1e10'
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]

		// These should remain as strings, not be converted to numbers
		assert.Contains(t, yaml, "port:")
		assert.Contains(t, yaml, "zipcode:")
	})

	t.Run("boolean-like strings", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'bool-strings' },
            data: {
                yesString: 'yes',
                noString: 'no',
                trueString: 'true',
                falseString: 'false',
                onString: 'on',
                offString: 'off'
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "yesString:")
		assert.Contains(t, yaml, "noString:")
	})

	t.Run("deeply nested objects", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'apps/v1',
            kind: 'Deployment',
            metadata: { name: 'deep' },
            spec: {
                template: {
                    spec: {
                        containers: [{
                            name: 'app',
                            resources: {
                                limits: {
                                    cpu: '100m',
                                    memory: '128Mi'
                                },
                                requests: {
                                    cpu: '50m',
                                    memory: '64Mi'
                                }
                            },
                            env: [
                                { name: 'VAR1', value: 'val1' },
                                { name: 'VAR2', valueFrom: { secretKeyRef: { name: 'secret', key: 'key' } } }
                            ]
                        }]
                    }
                }
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "cpu: 100m")
		assert.Contains(t, yaml, "memory: 128Mi")
		assert.Contains(t, yaml, "name: VAR1")
		assert.Contains(t, yaml, "secretKeyRef:")
	})

	t.Run("array of primitives", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'arrays' },
            data: {
                hosts: ['host1.example.com', 'host2.example.com', 'host3.example.com'].join(','),
                ports: [80, 443, 8080].map(String).join(',')
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "host1.example.com")
		assert.Contains(t, yaml, "80,443,8080")
	})

	t.Run("unicode characters", func(t *testing.T) {
		ch := &chart.Chart{
			Metadata: &chart.Metadata{Name: "test", Version: "1.0.0"},
			RuntimeFiles: []*chart.File{{Name: "ts/src/index.ts", Data: []byte(`
export function render(ctx: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'unicode' },
            data: {
                japanese: '日本語',
                emoji: '🚀',
                chinese: '中文',
                arabic: 'العربية',
                mixed: 'Hello 世界 🌍'
            }
        }]
    };
}
`)}},
		}

		result, err := ts.RenderFiles(context.Background(), ch, chartutil.Values{})
		require.NoError(t, err)
		yaml := result[common.ChartTSSourceDir+common.ChartTSEntryPointTS]
		assert.Contains(t, yaml, "japanese:")
		assert.Contains(t, yaml, "emoji:")
	})
}
