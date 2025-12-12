package tschart

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
)

func newTestChart(files map[string]string) *chart.Chart {
	var fileList []*chart.File
	for name, content := range files {
		fileList = append(fileList, &chart.File{
			Name: name,
			Data: []byte(content),
		})
	}

	testChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Files: fileList,
	}

	return testChart
}

func newTestValues(data map[string]interface{}) chartutil.Values {
	return chartutil.Values(data)
}

func TestRenderSimpleManifest(t *testing.T) {
	bundleContent := `
module.exports.render = function(context) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: {
                name: context.Release.Name + '-config',
                namespace: context.Release.Namespace
            },
            data: {
                replicas: String(context.Values.replicas)
            }
        }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": bundleContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values": map[string]interface{}{
			"replicas": 3,
		},
		"Release": map[string]interface{}{
			"Name":      "test-release",
			"Namespace": "default",
			"Revision":  1,
			"IsInstall": true,
			"IsUpgrade": false,
			"Service":   "Nelm",
		},
		"Chart": map[string]interface{}{
			"Name":    "test-chart",
			"Version": "1.0.0",
		},
		"Capabilities": map[string]interface{}{
			"APIVersions": []string{"v1", "apps/v1"},
			"KubeVersion": map[string]interface{}{
				"Version": "v1.29.0",
				"Major":   "1",
				"Minor":   "29",
			},
		},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)

	assert.Contains(t, renderedTemplates, "ts/chart_render_main.js")

	yaml := renderedTemplates["ts/chart_render_main.js"]
	assert.Contains(t, yaml, "kind: ConfigMap")
	assert.Contains(t, yaml, "name: test-release-config")
	assert.Contains(t, yaml, "namespace: default")
	assert.Contains(t, yaml, "replicas: \"3\"")
}

func TestRenderMultipleResources(t *testing.T) {
	bundleContent := `
module.exports.render = function(context) {
    return {
        manifests: [
            {
                apiVersion: 'v1',
                kind: 'Service',
                metadata: { name: context.Release.Name + '-svc' }
            },
            {
                apiVersion: 'apps/v1',
                kind: 'Deployment',
                metadata: { name: context.Release.Name + '-deploy' }
            }
        ]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": bundleContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Release": map[string]interface{}{
			"Name": "test",
		},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)

	yaml := renderedTemplates["ts/chart_render_main.js"]

	assert.Contains(t, yaml, "kind: Service")
	assert.Contains(t, yaml, "kind: Deployment")
	assert.Contains(t, yaml, "---")
}

func TestRenderReturnsNull(t *testing.T) {
	bundleContent := `
module.exports.render = function(context) {
    if (!context.Values.enabled) {
        return null;
    }
    return {
        manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'test' } }]
    };
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": bundleContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values": map[string]interface{}{
			"enabled": false,
		},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 0)
}

func TestNoJSBundle(t *testing.T) {
	testChart := newTestChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: v1\nkind: Deployment",
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 0)
}

func TestRenderWithModuleExportsObject(t *testing.T) {
	bundleContent := `
module.exports = {
    render: function(context) {
        return {
            manifests: [{
                apiVersion: 'v1',
                kind: 'ConfigMap',
                metadata: { name: 'test-object-pattern' }
            }]
        };
    }
};
`
	testChart := newTestChart(map[string]string{
		"ts/chart_render_main.js": bundleContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)
	yaml := renderedTemplates["ts/chart_render_main.js"]
	assert.Contains(t, yaml, "kind: ConfigMap")
	assert.Contains(t, yaml, "name: test-object-pattern")
}
