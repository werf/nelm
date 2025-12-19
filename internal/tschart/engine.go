package tschart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	"sigs.k8s.io/yaml"
)

const OutputFile = "ts/render_output.yaml"

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) RenderFiles(ctx context.Context, chartPath string, chart *helmchart.Chart, renderedValues chartutil.Values) (map[string]string, error) {
	isLocalDir := isLocalDirectory(chartPath)

	var vendorBundle string
	var packages []string
	var appBundle string
	var err error

	if isLocalDir {
		absChartPath, err := filepath.Abs(chartPath)
		if err != nil {
			return nil, fmt.Errorf("get absolute path: %w", err)
		}
		tsDir := filepath.Join(absChartPath, TSSourceDir)
		if _, err := os.Stat(tsDir); os.IsNotExist(err) {
			return map[string]string{}, nil
		}

		vendorBundle, packages, err = GetVendorBundleFromDir(ctx, chartPath)
		if err != nil {
			return nil, fmt.Errorf("get vendor bundle: %w", err)
		}

		appBundle, err = BuildAppBundleFromDir(ctx, chartPath, packages)
		if err != nil {
			if strings.Contains(err.Error(), "no TypeScript entrypoint found") {
				return map[string]string{}, nil
			}
			return nil, fmt.Errorf("build app bundle: %w", err)
		}
	} else {
		vendorBundle, packages = GetVendorBundleFromFiles(chart.RuntimeFiles)

		sourceFiles := ExtractSourceFiles(chart.RuntimeFiles)
		if len(sourceFiles) == 0 {
			return map[string]string{}, nil
		}

		appBundle, err = BuildAppBundleFromChartFiles(ctx, chart.RuntimeFiles, packages)
		if err != nil {
			return nil, fmt.Errorf("build app bundle from chart files: %w", err)
		}
	}

	renderContext := buildRenderContext(renderedValues, chart)
	result, err := executeInGoja(ctx, vendorBundle, appBundle, renderContext)
	if err != nil {
		return nil, fmt.Errorf("execute bundle: %w", err)
	}

	if result == nil {
		return map[string]string{}, nil
	}

	yamlOutput, err := resultToYAML(result)
	if err != nil {
		return nil, fmt.Errorf("convert result to YAML: %w", err)
	}

	if strings.TrimSpace(yamlOutput) == "" {
		return map[string]string{}, nil
	}

	return map[string]string{
		OutputFile: yamlOutput,
	}, nil
}

func isLocalDirectory(chartPath string) bool {
	stat, err := os.Stat(chartPath)
	if err != nil {
		return false
	}
	return stat.IsDir()
}

func buildRenderContext(renderedValues chartutil.Values, chart *helmchart.Chart) map[string]interface{} {
	renderContext := renderedValues.AsMap()

	if valuesInterface, ok := renderContext["Values"]; ok {
		if chartValues, ok := valuesInterface.(chartutil.Values); ok {
			renderContext["Values"] = chartValues.AsMap()
		}
	}

	files := make(map[string]interface{}, len(chart.Files))
	for _, file := range chart.Files {
		files[file.Name] = file.Data
	}
	renderContext["Files"] = files

	return renderContext
}

func resultToYAML(result interface{}) (string, error) {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected render result type: %T", result)
	}

	manifests, exists := resultMap["manifests"]
	if !exists {
		return "", fmt.Errorf("render result object does not contain 'manifests' field")
	}

	return convertToYAML(manifests)
}

func convertToYAML(value interface{}) (string, error) {
	if arr, ok := value.([]interface{}); ok {
		var results []string
		for i, item := range arr {
			if item == nil {
				continue
			}

			yamlBytes, err := yaml.Marshal(item)
			if err != nil {
				return "", fmt.Errorf("marshal resource at index %d: %w", i, err)
			}

			results = append(results, string(yamlBytes))
		}

		return strings.Join(results, "---\n"), nil
	}

	yamlBytes, err := yaml.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal resource: %w", err)
	}

	return string(yamlBytes), nil
}
