package tschart

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
)

func newTestChart(files map[string]string) *chart.Chart {
	var fileList []*chart.File
	var runtimeFileList []*chart.File
	for name, content := range files {
		fileList = append(fileList, &chart.File{
			Name: name,
			Data: []byte(content),
		})
		runtimeFileList = append(runtimeFileList, &chart.File{
			Name: name,
			Data: []byte(content),
		})
	}

	testChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "test-chart",
			Version: "1.0.0",
		},
		Files:        fileList,
		RuntimeFiles: runtimeFileList,
	}

	return testChart
}

func newTestValues(data map[string]interface{}) chartutil.Values {
	return chartutil.Values(data)
}

// createTestChartDir creates a temporary chart directory with TypeScript source files
func createTestChartDir(t *testing.T, sourceContent string) string {
	tempDir, err := os.MkdirTemp("", "tschart-engine-test-*")
	require.NoError(t, err)

	tsDir := filepath.Join(tempDir, "ts", "src")
	require.NoError(t, os.MkdirAll(tsDir, 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(tsDir, "index.ts"),
		[]byte(sourceContent),
		0644,
	))

	return tempDir
}

func TestRenderSimpleManifest(t *testing.T) {
	sourceContent := `
export function render(context: any) {
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
}
`
	chartDir := createTestChartDir(t, sourceContent)
	defer os.RemoveAll(chartDir)

	testChart := newTestChart(map[string]string{})
	testChart.Metadata = &chart.Metadata{
		Name:    "test-chart",
		Version: "1.0.0",
	}

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
	renderedTemplates, err := engine.RenderFiles(ctx, chartDir, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)
	assert.Contains(t, renderedTemplates, OutputFile)

	yaml := renderedTemplates[OutputFile]
	assert.Contains(t, yaml, "kind: ConfigMap")
	assert.Contains(t, yaml, "name: test-release-config")
	assert.Contains(t, yaml, "namespace: default")
	assert.Contains(t, yaml, "replicas: \"3\"")
}

func TestRenderMultipleResources(t *testing.T) {
	sourceContent := `
export function render(context: any) {
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
}
`
	chartDir := createTestChartDir(t, sourceContent)
	defer os.RemoveAll(chartDir)

	testChart := newTestChart(map[string]string{})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Release": map[string]interface{}{
			"Name": "test",
		},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, chartDir, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)

	yaml := renderedTemplates[OutputFile]

	assert.Contains(t, yaml, "kind: Service")
	assert.Contains(t, yaml, "kind: Deployment")
	assert.Contains(t, yaml, "---")
}

func TestRenderReturnsNull(t *testing.T) {
	sourceContent := `
export function render(context: any) {
    if (!context.Values.enabled) {
        return null;
    }
    return {
        manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'test' } }]
    };
}
`
	chartDir := createTestChartDir(t, sourceContent)
	defer os.RemoveAll(chartDir)

	testChart := newTestChart(map[string]string{})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values": map[string]interface{}{
			"enabled": false,
		},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, chartDir, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 0)
}

func TestNoTypeScriptSource(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tschart-no-ts-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a chart directory without ts/ folder
	testChart := newTestChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: v1\nkind: Deployment",
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, tempDir, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 0)
}

func TestRenderWithModuleExportsObject(t *testing.T) {
	sourceContent := `
module.exports = {
    render: function(context: any) {
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
	chartDir := createTestChartDir(t, sourceContent)
	defer os.RemoveAll(chartDir)

	testChart := newTestChart(map[string]string{})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, chartDir, testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)
	yaml := renderedTemplates[OutputFile]
	assert.Contains(t, yaml, "kind: ConfigMap")
	assert.Contains(t, yaml, "name: test-object-pattern")
}

func TestRenderFromPackagedChart(t *testing.T) {
	// Simulate a packaged chart (not a local directory) with source files
	sourceContent := `
export function render(context: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'packaged-chart-test' }
        }]
    };
}
`
	testChart := newTestChart(map[string]string{
		"ts/src/index.ts": sourceContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	// Use a non-existent path to simulate packaged chart
	renderedTemplates, err := engine.RenderFiles(ctx, "./non-existent-chart.tgz", testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)
	yaml := renderedTemplates[OutputFile]
	assert.Contains(t, yaml, "kind: ConfigMap")
	assert.Contains(t, yaml, "name: packaged-chart-test")
}
