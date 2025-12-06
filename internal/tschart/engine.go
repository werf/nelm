package tschart

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/nelm/pkg/log"
)

func mergeRuntimeFiles(runtimeFiles, runtimeDepsFiles []*helmchart.File) []*helmchart.File {
	if len(runtimeDepsFiles) == 0 {
		return runtimeFiles
	}

	merged := make([]*helmchart.File, 0, len(runtimeFiles)+len(runtimeDepsFiles))
	merged = append(merged, runtimeFiles...)
	merged = append(merged, runtimeDepsFiles...)

	return merged
}

type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) RenderChartWithDependencies(
	ctx context.Context,
	rootChartPath string,
	chart *helmchart.Chart,
	renderedValues chartutil.Values,
) (map[string]string, error) {
	allRendered := make(map[string]string)

	err := e.renderChartRecursive(ctx, rootChartPath, chart, renderedValues, chart.Name(), allRendered)
	if err != nil {
		return nil, err
	}

	return allRendered, nil
}

//nolint:funcorder // Helper method kept near the method that uses it for readability
func (e *Engine) renderChartRecursive(
	ctx context.Context,
	chartDirPath string, // Filesystem path (for local charts)
	chart *helmchart.Chart,
	values chartutil.Values,
	pathPrefix string, // Output path prefix (e.g., "root/charts/sub")
	results map[string]string,
) error {
	log.Default.Debug(ctx, "Rendering TypeScript for chart %q (path prefix: %s)", chart.Name(), pathPrefix)

	rendered, err := e.RenderFiles(ctx, chartDirPath, chart, values)
	if err != nil {
		return fmt.Errorf("render TypeScript for chart %q: %w", chart.Name(), err)
	}

	for filename, content := range rendered {
		outputPath := path.Join(pathPrefix, filename)
		results[outputPath] = content
		log.Default.Debug(ctx, "Added TypeScript output: %s", outputPath)
	}

	for _, dep := range chart.Dependencies() {
		depName := dep.Name()

		log.Default.Debug(ctx, "Processing dependency %q for chart %q", depName, chart.Name())

		depValues := scopeValuesForSubchart(values, depName, dep)

		depDirPath := filepath.Join(chartDirPath, "charts", depName)

		depPathPrefix := path.Join(pathPrefix, "charts", depName)

		err := e.renderChartRecursive(ctx, depDirPath, dep, depValues, depPathPrefix, results)
		if err != nil {
			return fmt.Errorf("render dependency %q: %w", depName, err)
		}
	}

	return nil
}

func scopeValuesForSubchart(parentValues chartutil.Values, subchartName string, subchart *helmchart.Chart) chartutil.Values {
	scoped := chartutil.Values{}

	if caps, ok := parentValues["Capabilities"]; ok {
		scoped["Capabilities"] = caps
	}

	if release, ok := parentValues["Release"]; ok {
		scoped["Release"] = release
	}

	if runtime, ok := parentValues["Runtime"]; ok {
		scoped["Runtime"] = runtime
	}

	scoped["Chart"] = buildChartMetadata(subchart)

	if parentVals, ok := parentValues["Values"]; ok {
		switch v := parentVals.(type) {
		case map[string]interface{}:
			if subVals, ok := v[subchartName]; ok {
				scoped["Values"] = subVals
			} else {
				scoped["Values"] = map[string]interface{}{}
			}
		case chartutil.Values:
			if subVals, ok := v[subchartName]; ok {
				scoped["Values"] = subVals
			} else {
				scoped["Values"] = map[string]interface{}{}
			}
		default:
			scoped["Values"] = map[string]interface{}{}
		}
	} else {
		scoped["Values"] = map[string]interface{}{}
	}

	files := make(map[string]interface{}, len(subchart.Files))
	for _, file := range subchart.Files {
		files[file.Name] = file.Data
	}

	scoped["Files"] = files

	return scoped
}

func buildChartMetadata(chart *helmchart.Chart) map[string]interface{} {
	metadata := map[string]interface{}{
		"Name":    chart.Name(),
		"Version": "",
	}

	if chart.Metadata != nil {
		metadata["Version"] = chart.Metadata.Version
		metadata["AppVersion"] = chart.Metadata.AppVersion
		metadata["Description"] = chart.Metadata.Description
		metadata["Keywords"] = chart.Metadata.Keywords
		metadata["Home"] = chart.Metadata.Home
		metadata["Sources"] = chart.Metadata.Sources
		metadata["Icon"] = chart.Metadata.Icon
		metadata["APIVersion"] = chart.Metadata.APIVersion
		metadata["Condition"] = chart.Metadata.Condition
		metadata["Tags"] = chart.Metadata.Tags
		metadata["Type"] = chart.Metadata.Type
		metadata["Annotations"] = chart.Metadata.Annotations

		if chart.Metadata.Maintainers != nil {
			maintainers := make([]map[string]interface{}, len(chart.Metadata.Maintainers))
			for i, m := range chart.Metadata.Maintainers {
				maintainers[i] = map[string]interface{}{
					"Name":  m.Name,
					"Email": m.Email,
					"URL":   m.URL,
				}
			}

			metadata["Maintainers"] = maintainers
		}
	}

	return metadata
}

func (e *Engine) RenderFiles(ctx context.Context, chartPath string, chart *helmchart.Chart, renderedValues chartutil.Values) (map[string]string, error) {
	mergedFiles := mergeRuntimeFiles(chart.RuntimeFiles, chart.RuntimeDepsFiles)

	var (
		vendorBundle string
		packages     []string
		entrypoint   string
	)

	vendorBundle, packages, err := GetVendorBundleFromFiles(mergedFiles)
	if err != nil {
		return nil, fmt.Errorf("get vendor bundle: %w", err)
	}

	sourceFiles := ExtractSourceFiles(mergedFiles)
	if len(sourceFiles) == 0 {
		return map[string]string{}, nil
	}

	entrypoint = findEntrypointFromFiles(sourceFiles)
	if entrypoint == "" {
		return map[string]string{}, nil
	}

	appBundle, err := BuildAppBundleFromChartFiles(ctx, mergedFiles, packages)
	if err != nil {
		return nil, fmt.Errorf("build app bundle from chart files: %w", err)
	}

	renderContext := buildRenderContext(renderedValues, chart)

	result, err := executeInGoja(vendorBundle, appBundle, renderContext)
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

	outputPath := path.Join(TSSourceDir, entrypoint)

	return map[string]string{
		outputPath: yamlOutput,
	}, nil
}

func findEntrypointFromFiles(sourceFiles map[string][]byte) string {
	for _, ep := range EntryPoints {
		if _, exists := sourceFiles[ep]; exists {
			return ep
		}
	}

	return ""
}

func buildRenderContext(renderedValues chartutil.Values, chart *helmchart.Chart) map[string]interface{} {
	renderContext := renderedValues.AsMap()

	if valuesInterface, ok := renderContext["Values"]; ok {
		if chartValues, ok := valuesInterface.(chartutil.Values); ok {
			renderContext["Values"] = chartValues.AsMap()
		}
	}

	renderContext["Chart"] = buildChartMetadata(chart)

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
