package jsengine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
)

// newTestChart creates a test chart with the given files
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

// newTestValues creates chartutil.Values for testing
func newTestValues(data map[string]interface{}) chartutil.Values {
	return chartutil.Values(data)
}

func TestRenderSimpleManifest(t *testing.T) {
	// Create a test chart with a simple manifest
	manifestContent := `
exports.render = function(context) {
    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        metadata: {
            name: context.Release.Name + '-config',
            namespace: context.Release.Namespace
        },
        data: {
            replicas: String(context.Values.replicas)
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/configmap.manifest.js": manifestContent,
	})

	// Create render values
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

	// Render the chart
	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	// Verify results
	assert.Len(t, renderedTemplates, 1)

	// Check that the manifest was rendered
	assert.Contains(t, renderedTemplates, "js-templates/configmap.manifest.js")

	yaml := renderedTemplates["js-templates/configmap.manifest.js"]
	assert.Contains(t, yaml, "kind: ConfigMap")
	assert.Contains(t, yaml, "name: test-release-config")
	assert.Contains(t, yaml, "namespace: default")
	assert.Contains(t, yaml, "replicas: \"3\"")
}

func TestRenderMultipleResources(t *testing.T) {
	// Create a test chart with a manifest that returns multiple resources
	manifestContent := `
exports.render = function(context) {
    return [
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
    ];
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/resources.manifests.js": manifestContent,
	})

	// Create render values
	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Release": map[string]interface{}{
			"Name": "test",
		},
	})

	// Render the chart
	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	// Verify results
	assert.Len(t, renderedTemplates, 1)

	yaml := renderedTemplates["js-templates/resources.manifests.js"]

	// Should contain both resources separated by ---
	assert.Contains(t, yaml, "kind: Service")
	assert.Contains(t, yaml, "kind: Deployment")
	assert.Contains(t, yaml, "---")
}

func TestRenderWithYAMLModule(t *testing.T) {
	// Create a test chart with a manifest that uses helm:yaml
	manifestContent := `
const { toYaml, fromYaml } = require('helm:yaml');

exports.render = function(context) {
    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        metadata: { name: 'test' },
        data: {
            config: toYaml(context.Values.config)
        }
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/test.manifest.js": manifestContent,
	})

	// Create render values
	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values": map[string]interface{}{
			"config": map[string]interface{}{
				"database": "postgres",
				"port":     5432,
			},
		},
	})

	// Render the chart
	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	// Verify results
	assert.Len(t, renderedTemplates, 1)

	yaml := renderedTemplates["js-templates/test.manifest.js"]
	assert.Contains(t, yaml, "database: postgres")
	assert.Contains(t, yaml, "port: 5432")
}

func TestRenderReturnsNull(t *testing.T) {
	// Create a test chart with a manifest that returns null (skip resource)
	manifestContent := `
exports.render = function(context) {
    if (!context.Values.enabled) {
        return null;
    }
    return { apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'test' } };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/conditional.manifest.js": manifestContent,
	})

	// Create render values with enabled=false
	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values": map[string]interface{}{
			"enabled": false,
		},
	})

	// Render the chart
	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	// Should have no rendered templates since render() returned null
	assert.Len(t, renderedTemplates, 0)
}

func TestNoJSTemplatesDirectory(t *testing.T) {
	// Create a test chart without js-templates files
	testChart := newTestChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: v1\nkind: Deployment",
	})

	// Create render values
	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	// Render the chart
	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	// Should return empty map, not an error
	assert.Len(t, renderedTemplates, 0)
}

func TestRenderWithRequire(t *testing.T) {
	// Create a test chart with a manifest that requires a helper
	manifestContent := `
const { makeLabels } = require('./helpers.js');

exports.render = function(context) {
    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        metadata: {
            name: 'test',
            labels: makeLabels(context.Chart, context.Release)
        }
    };
};
`
	helperContent := `
exports.makeLabels = function(chart, release) {
    return {
        'app.kubernetes.io/name': chart.Name,
        'app.kubernetes.io/instance': release.Name,
        'app.kubernetes.io/version': chart.Version
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/deployment.manifest.js": manifestContent,
		"js-templates/helpers.js":             helperContent,
	})

	// Create render values
	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Release": map[string]interface{}{
			"Name": "my-release",
		},
		"Chart": map[string]interface{}{
			"Name":    "my-chart",
			"Version": "1.0.0",
		},
	})

	// Render the chart
	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	// Verify results
	assert.Len(t, renderedTemplates, 1)

	yaml := renderedTemplates["js-templates/deployment.manifest.js"]
	assert.Contains(t, yaml, "app.kubernetes.io/name: my-chart")
	assert.Contains(t, yaml, "app.kubernetes.io/instance: my-release")
	assert.Contains(t, yaml, "app.kubernetes.io/version: 1.0.0")
}

func TestRenderWithNestedRequire(t *testing.T) {
	// Create a test chart with nested directory structure
	manifestContent := `
const { makeLabels } = require('../helpers.js');

exports.render = function(context) {
    return {
        apiVersion: 'v1',
        kind: 'Service',
        metadata: {
            name: 'test',
            labels: makeLabels(context.Release.Name)
        }
    };
};
`
	helperContent := `
exports.makeLabels = function(name) {
    return {
        'app': name
    };
};
`
	testChart := newTestChart(map[string]string{
		"js-templates/subdir/service.manifest.js": manifestContent,
		"js-templates/helpers.js":                 helperContent,
	})

	// Create render values
	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Release": map[string]interface{}{
			"Name": "test-app",
		},
	})

	// Render the chart
	engine := New()
	renderedTemplates, err := engine.RenderFiles(ctx, testChart, renderedValues)
	require.NoError(t, err)

	// Verify results
	assert.Len(t, renderedTemplates, 1)

	yaml := renderedTemplates["js-templates/subdir/service.manifest.js"]
	assert.Contains(t, yaml, "app: test-app")
}

func TestRenderFile(t *testing.T) {
	// Test the RenderFile method (for debugging/testing single files)
	manifestContent := `
exports.render = function(context) {
    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        metadata: { name: context.Release.Name }
    };
};
`

	ctx := context.Background()
	renderCtx := map[string]interface{}{
		"Release": map[string]interface{}{
			"Name": "test",
		},
	}

	engine := New()
	yaml, err := engine.RenderFile(ctx, "deployment.manifest.js", []byte(manifestContent), nil, renderCtx)
	require.NoError(t, err)

	assert.Contains(t, yaml, "kind: ConfigMap")
	assert.Contains(t, yaml, "name: test")
}

func TestRenderFileWithHelpers(t *testing.T) {
	// Test RenderFile with helper files
	manifestContent := `
const { getName } = require('./helper.js');

exports.render = function(context) {
    return {
        apiVersion: 'v1',
        kind: 'ConfigMap',
        metadata: { name: getName(context.Release.Name) }
    };
};
`
	helperContent := `
exports.getName = function(base) {
    return base + '-config';
};
`

	ctx := context.Background()
	renderCtx := map[string]interface{}{
		"Release": map[string]interface{}{
			"Name": "test",
		},
	}

	helperFiles := map[string][]byte{
		"helper.js": []byte(helperContent),
	}

	engine := New()
	// RenderFile automatically prefixes with js-templates/, so we just pass the filename
	yaml, err := engine.RenderFile(ctx, "deployment.manifest.js", []byte(manifestContent), helperFiles, renderCtx)
	require.NoError(t, err)

	assert.Contains(t, yaml, "name: test-config")
}
