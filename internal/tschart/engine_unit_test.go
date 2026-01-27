//nolint:testpackage // White-box test needs access to internal functions
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
	var (
		fileList        []*chart.File
		runtimeFileList []*chart.File
	)

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
	testChart := newTestChart(map[string]string{
		"ts/src/index.ts": sourceContent,
	})
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
	renderedTemplates, err := engine.RenderFiles(ctx, "", testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)
	assert.Contains(t, renderedTemplates, DefaultOutputFile)

	yaml := renderedTemplates[DefaultOutputFile]
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
	testChart := newTestChart(map[string]string{
		"ts/src/index.ts": sourceContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Release": map[string]interface{}{
			"Name": "test",
		},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, "", testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)

	yaml := renderedTemplates[DefaultOutputFile]

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
	testChart := newTestChart(map[string]string{
		"ts/src/index.ts": sourceContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values": map[string]interface{}{
			"enabled": false,
		},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, "", testChart, renderedValues)
	require.NoError(t, err)

	assert.Empty(t, renderedTemplates)
}

func TestNoTypeScriptSource(t *testing.T) {
	// Create a chart without ts/ folder
	testChart := newTestChart(map[string]string{
		"templates/deployment.yaml": "apiVersion: v1\nkind: Deployment",
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, "", testChart, renderedValues)
	require.NoError(t, err)

	assert.Empty(t, renderedTemplates)
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
	testChart := newTestChart(map[string]string{
		"ts/src/index.ts": sourceContent,
	})

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderFiles(ctx, "", testChart, renderedValues)
	require.NoError(t, err)

	assert.Len(t, renderedTemplates, 1)
	yaml := renderedTemplates[DefaultOutputFile]
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
	yaml := renderedTemplates[DefaultOutputFile]
	assert.Contains(t, yaml, "kind: ConfigMap")
	assert.Contains(t, yaml, "name: packaged-chart-test")
}

// createTestChartWithSubchart creates a root chart with a TypeScript subchart dependency
func createTestChartWithSubchart(t *testing.T, rootContent, subchartContent string) *chart.Chart {
	// Build subchart object with RuntimeFiles
	subchart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "ts-subchart",
			Version: "0.1.0",
		},
		Files: []*chart.File{},
		RuntimeFiles: []*chart.File{
			{Name: "ts/src/index.ts", Data: []byte(subchartContent)},
		},
	}

	// Build root chart object with dependency and RuntimeFiles
	rootChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "root-chart",
			Version: "1.0.0",
		},
		Files: []*chart.File{},
		RuntimeFiles: []*chart.File{
			{Name: "ts/src/index.ts", Data: []byte(rootContent)},
		},
	}
	rootChart.SetDependencies(subchart)

	return rootChart
}

func TestRenderChartWithTSSubchart(t *testing.T) {
	rootContent := `
export function render(context: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'root-config' },
            data: {
                chartName: context.Chart.Name,
                message: context.Values.rootMessage || 'default'
            }
        }]
    };
}
`
	subchartContent := `
export function render(context: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'subchart-config' },
            data: {
                chartName: context.Chart.Name,
                message: context.Values.subMessage || 'default'
            }
        }]
    };
}
`
	rootChart := createTestChartWithSubchart(t, rootContent, subchartContent)

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values": map[string]interface{}{
			"rootMessage": "Hello from root",
			"ts-subchart": map[string]interface{}{
				"subMessage": "Hello from subchart",
			},
		},
		"Release": map[string]interface{}{
			"Name":      "test-release",
			"Namespace": "default",
		},
		"Capabilities": map[string]interface{}{},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderChartWithDependencies(ctx, "", rootChart, renderedValues)
	require.NoError(t, err)

	// Should have 2 outputs: root and subchart
	assert.Len(t, renderedTemplates, 2)

	// Check root chart output path
	rootOutputPath := "root-chart/" + DefaultOutputFile
	assert.Contains(t, renderedTemplates, rootOutputPath)
	rootYaml := renderedTemplates[rootOutputPath]
	assert.Contains(t, rootYaml, "name: root-config")
	assert.Contains(t, rootYaml, "chartName: root-chart")
	assert.Contains(t, rootYaml, "message: Hello from root")

	// Check subchart output path follows Helm convention
	subchartOutputPath := "root-chart/charts/ts-subchart/" + DefaultOutputFile
	assert.Contains(t, renderedTemplates, subchartOutputPath)
	subYaml := renderedTemplates[subchartOutputPath]
	assert.Contains(t, subYaml, "name: subchart-config")
	assert.Contains(t, subYaml, "chartName: ts-subchart")
	assert.Contains(t, subYaml, "message: Hello from subchart")
}

func TestRenderClassicRootWithTSSubchart(t *testing.T) {
	// Root chart has no ts/ directory (classic Go template chart)
	// Only the subchart has TypeScript
	subchartContent := `
export function render(context: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'ts-subchart-only' },
            data: {
                chartName: context.Chart.Name,
                releaseName: context.Release.Name
            }
        }]
    };
}
`
	subchart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "ts-subchart",
			Version: "0.1.0",
		},
		Files: []*chart.File{},
		RuntimeFiles: []*chart.File{
			{Name: "ts/src/index.ts", Data: []byte(subchartContent)},
		},
	}

	rootChart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "classic-root",
			Version: "1.0.0",
		},
		Files:        []*chart.File{},
		RuntimeFiles: []*chart.File{}, // No TypeScript in root
	}
	rootChart.SetDependencies(subchart)

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values": map[string]interface{}{
			"ts-subchart": map[string]interface{}{},
		},
		"Release": map[string]interface{}{
			"Name":      "my-release",
			"Namespace": "default",
		},
		"Capabilities": map[string]interface{}{},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderChartWithDependencies(ctx, "", rootChart, renderedValues)
	require.NoError(t, err)

	// Only subchart output (root has no ts/)
	assert.Len(t, renderedTemplates, 1)

	subchartOutputPath := "classic-root/charts/ts-subchart/" + DefaultOutputFile
	assert.Contains(t, renderedTemplates, subchartOutputPath)
	yaml := renderedTemplates[subchartOutputPath]
	assert.Contains(t, yaml, "name: ts-subchart-only")
	assert.Contains(t, yaml, "chartName: ts-subchart")
	assert.Contains(t, yaml, "releaseName: my-release")
}

func TestScopeValuesForSubchart(t *testing.T) {
	subchart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:        "my-subchart",
			Version:     "2.0.0",
			AppVersion:  "1.5.0",
			Description: "Test subchart",
		},
		Files: []*chart.File{
			{Name: "README.md", Data: []byte("# Subchart")},
		},
	}

	parentValues := chartutil.Values{
		"Values": map[string]interface{}{
			"rootKey": "rootValue",
			"my-subchart": map[string]interface{}{
				"subKey": "subValue",
				"nested": map[string]interface{}{
					"deep": "value",
				},
			},
		},
		"Release": map[string]interface{}{
			"Name":      "test-release",
			"Namespace": "prod",
		},
		"Capabilities": map[string]interface{}{
			"KubeVersion": map[string]interface{}{
				"Version": "v1.28.0",
			},
		},
	}

	scoped := scopeValuesForSubchart(parentValues, "my-subchart", subchart)

	// Release should be copied
	assert.Equal(t, "test-release", scoped["Release"].(map[string]interface{})["Name"])
	assert.Equal(t, "prod", scoped["Release"].(map[string]interface{})["Namespace"])

	// Capabilities should be copied
	assert.NotNil(t, scoped["Capabilities"])

	// Chart metadata should come from subchart
	chartMeta := scoped["Chart"].(map[string]interface{})
	assert.Equal(t, "my-subchart", chartMeta["Name"])
	assert.Equal(t, "2.0.0", chartMeta["Version"])
	assert.Equal(t, "1.5.0", chartMeta["AppVersion"])

	// Values should be scoped to subchart's values only
	scopedValues := scoped["Values"].(map[string]interface{})
	assert.Equal(t, "subValue", scopedValues["subKey"])
	assert.Equal(t, "value", scopedValues["nested"].(map[string]interface{})["deep"])
	// Root values should NOT be present
	assert.Nil(t, scopedValues["rootKey"])

	// Files should come from subchart
	assert.NotNil(t, scoped["Files"])
}

func TestScopeValuesForSubchartMissingValues(t *testing.T) {
	subchart := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:    "missing-values-subchart",
			Version: "1.0.0",
		},
	}

	// Parent values don't have an entry for this subchart
	parentValues := chartutil.Values{
		"Values": map[string]interface{}{
			"other-subchart": map[string]interface{}{
				"key": "value",
			},
		},
		"Release": map[string]interface{}{
			"Name": "test",
		},
	}

	scoped := scopeValuesForSubchart(parentValues, "missing-values-subchart", subchart)

	// Values should be empty map, not nil
	assert.NotNil(t, scoped["Values"])
	assert.Empty(t, scoped["Values"])
}

func TestRenderNestedDependencies(t *testing.T) {
	// Create a 3-level nested structure: root -> sub1 -> sub2
	rootContent := `
export function render(context: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'root' },
            data: { level: 'root' }
        }]
    };
}
`
	sub1Content := `
export function render(context: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'sub1' },
            data: { level: 'sub1' }
        }]
    };
}
`
	sub2Content := `
export function render(context: any) {
    return {
        manifests: [{
            apiVersion: 'v1',
            kind: 'ConfigMap',
            metadata: { name: 'sub2' },
            data: { level: 'sub2' }
        }]
    };
}
`
	// Build chart objects with RuntimeFiles
	sub2 := &chart.Chart{
		Metadata: &chart.Metadata{Name: "sub2", Version: "0.1.0"},
		RuntimeFiles: []*chart.File{
			{Name: "ts/src/index.ts", Data: []byte(sub2Content)},
		},
	}
	sub1 := &chart.Chart{
		Metadata: &chart.Metadata{Name: "sub1", Version: "0.1.0"},
		RuntimeFiles: []*chart.File{
			{Name: "ts/src/index.ts", Data: []byte(sub1Content)},
		},
	}
	sub1.SetDependencies(sub2)

	rootChart := &chart.Chart{
		Metadata: &chart.Metadata{Name: "nested-root", Version: "1.0.0"},
		RuntimeFiles: []*chart.File{
			{Name: "ts/src/index.ts", Data: []byte(rootContent)},
		},
	}
	rootChart.SetDependencies(sub1)

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values": map[string]interface{}{
			"sub1": map[string]interface{}{
				"sub2": map[string]interface{}{},
			},
		},
		"Release":      map[string]interface{}{"Name": "test"},
		"Capabilities": map[string]interface{}{},
	})

	engine := NewEngine()
	renderedTemplates, err := engine.RenderChartWithDependencies(ctx, "", rootChart, renderedValues)
	require.NoError(t, err)

	// Should have 3 outputs
	assert.Len(t, renderedTemplates, 3)

	// Verify paths follow Helm convention
	assert.Contains(t, renderedTemplates, "nested-root/"+DefaultOutputFile)
	assert.Contains(t, renderedTemplates, "nested-root/charts/sub1/"+DefaultOutputFile)
	assert.Contains(t, renderedTemplates, "nested-root/charts/sub1/charts/sub2/"+DefaultOutputFile)

	// Verify content
	assert.Contains(t, renderedTemplates["nested-root/"+DefaultOutputFile], "level: root")
	assert.Contains(t, renderedTemplates["nested-root/charts/sub1/"+DefaultOutputFile], "level: sub1")
	assert.Contains(t, renderedTemplates["nested-root/charts/sub1/charts/sub2/"+DefaultOutputFile], "level: sub2")
}

func TestRenderSubchartError(t *testing.T) {
	rootContent := `
export function render(context: any) {
    return { manifests: [{ apiVersion: 'v1', kind: 'ConfigMap', metadata: { name: 'root' } }] };
}
`
	// Subchart has invalid TypeScript that will cause runtime error
	subchartContent := `
export function render(context: any) {
    // This will cause a runtime error
    const x: any = null;
    x.nonExistent.deep;
    return { manifests: [] };
}
`
	rootChart := createTestChartWithSubchart(t, rootContent, subchartContent)

	ctx := context.Background()
	renderedValues := newTestValues(map[string]interface{}{
		"Values":       map[string]interface{}{"ts-subchart": map[string]interface{}{}},
		"Release":      map[string]interface{}{"Name": "test"},
		"Capabilities": map[string]interface{}{},
	})

	engine := NewEngine()
	_, err := engine.RenderChartWithDependencies(ctx, "", rootChart, renderedValues)

	// Should fail with error mentioning the subchart
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ts-subchart")
}
