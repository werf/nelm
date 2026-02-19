package ts

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/gookit/color"
	"github.com/samber/lo"
	"sigs.k8s.io/yaml"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

func RenderChart(ctx context.Context, chart *helmchart.Chart, renderedValues chartutil.Values, rebuildBundle bool, chartPath string) (map[string]string, error) {
	allRendered := make(map[string]string)

	if err := renderChartRecursive(ctx, chart, renderedValues, chart.Name(), chartPath, allRendered, rebuildBundle); err != nil {
		return nil, err
	}

	return allRendered, nil
}

func renderChartRecursive(ctx context.Context, chart *helmchart.Chart, values chartutil.Values, pathPrefix, chartPath string, results map[string]string, rebuildBundle bool) error {
	log.Default.Debug(ctx, "Rendering TypeScript for chart %q (path prefix: %s)", chart.Name(), pathPrefix)

	rendered, err := renderFiles(ctx, chart, values, chartPath, rebuildBundle)
	if err != nil {
		return fmt.Errorf("render files for chart %q: %w", chart.Name(), err)
	}

	for filename, content := range rendered {
		outputPath := path.Join(pathPrefix, filename)
		results[outputPath] = content
		log.Default.Debug(ctx, "Rendered output: %s", outputPath)
	}

	for _, dep := range chart.Dependencies() {
		depName := dep.Name()
		log.Default.Debug(ctx, "Processing dependency %q for chart %q", depName, chart.Name())

		err := renderChartRecursive(
			ctx,
			dep,
			scopeValuesForSubchart(values, depName, dep),
			path.Join(pathPrefix, "charts", depName),
			path.Join(chartPath, "charts", depName),
			results,
			rebuildBundle,
		)
		if err != nil {
			return fmt.Errorf("render dependency %q: %w", depName, err)
		}
	}

	return nil
}

func renderFiles(ctx context.Context, chart *helmchart.Chart, renderedValues chartutil.Values, chartPath string, rebuildBundle bool) (map[string]string, error) {
	sourceFiles := extractSourceFiles(chart.RuntimeFiles)
	if len(sourceFiles) == 0 {
		return map[string]string{}, nil
	}

	entrypoint := findEntrypointInFiles(sourceFiles)
	if entrypoint == "" {
		return map[string]string{}, nil
	}

	bundleFile, foundBundle := lo.Find(chart.RuntimeFiles, func(f *helmchart.File) bool {
		return f.Name == common.ChartTSVendorBundleFile
	})

	var bundle []byte
	if rebuildBundle || !foundBundle {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Run bundle for ")+"%s", entrypoint)

		bundleRes, err := buildBundle(ctx, chartPath, entrypoint)
		if err != nil {
			return nil, fmt.Errorf("build bundle for chart %q: %w", chart.Name(), err)
		}

		bundle = bundleRes

		if err := saveBundleToFile(chartPath, bundle); err != nil {
			return nil, fmt.Errorf("save bundle: %w", err)
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Bundled: ")+"%s - %s", "dist/bundle.js", humanize.Bytes(uint64(len(bundle))))
	} else {
		bundle = bundleFile.Data
	}

	renderCtx, err := buildRenderContext(renderedValues, chart)
	if err != nil {
		return nil, fmt.Errorf("build render context: %w", err)
	}

	result, err := runApp(ctx, chartPath, bundle, renderCtx)
	if err != nil {
		return nil, fmt.Errorf("run deno app: %w", err)
	}

	if result == nil {
		return map[string]string{}, nil
	}

	yamlOutput, err := convertRenderResultToYAML(result)
	if err != nil {
		return nil, fmt.Errorf("convert render result to yaml: %w", err)
	}

	if strings.TrimSpace(yamlOutput) == "" {
		return map[string]string{}, nil
	}

	return map[string]string{
		path.Join(common.ChartTSSourceDir, entrypoint): yamlOutput,
	}, nil
}

func buildRenderContext(renderedValues chartutil.Values, chart *helmchart.Chart) (string, error) {
	renderContext := renderedValues.AsMap()

	if valuesInterface, ok := renderContext["Values"]; ok {
		if chartValues, ok := valuesInterface.(chartutil.Values); ok {
			renderContext["Values"] = chartValues.AsMap()
		}
	}

	renderContext["Chart"] = buildChartMetadata(chart)

	files := make(map[string]any, len(chart.Files))
	for _, file := range chart.Files {
		files[file.Name] = file.Data
	}

	renderContext["Files"] = files

	jsonInput, err := json.Marshal(renderContext)
	if err != nil {
		return "", fmt.Errorf("marshal render context to json: %w", err)
	}

	return string(jsonInput), nil
}

func convertRenderResultToYAML(result any) (string, error) {
	resultMap, ok := result.(map[string]any)
	if !ok {
		return "", fmt.Errorf("convert render result to yaml: unexpected type %T", result)
	}

	manifests, exists := resultMap["manifests"]
	if !exists {
		return "", fmt.Errorf("convert render result to yaml: missing 'manifests' field")
	}

	return marshalManifests(manifests)
}

func scopeValuesForSubchart(parentValues chartutil.Values, subchartName string, subchart *helmchart.Chart) chartutil.Values {
	scoped := chartutil.Values{
		"Chart":  buildChartMetadata(subchart),
		"Values": map[string]any{},
	}

	for _, key := range []string{"Capabilities", "Release", "Runtime"} {
		if v, ok := parentValues[key]; ok {
			scoped[key] = v
		}
	}

	if parentVals := parentValues["Values"]; parentVals != nil {
		var valuesMap map[string]any
		switch v := parentVals.(type) {
		case map[string]any:
			valuesMap = v
		case chartutil.Values:
			valuesMap = v
		}

		if valuesMap != nil {
			if subVals, ok := valuesMap[subchartName]; ok {
				scoped["Values"] = subVals
			}
		}
	}

	files := make(map[string]any, len(subchart.Files))
	for _, f := range subchart.Files {
		files[f.Name] = f.Data
	}

	scoped["Files"] = files

	return scoped
}

func buildChartMetadata(chart *helmchart.Chart) map[string]any {
	metadata := map[string]any{
		"Name":    chart.Name(),
		"Version": "",
	}

	if chart.Metadata == nil {
		return metadata
	}

	m := chart.Metadata
	metadata["Version"] = m.Version
	metadata["AppVersion"] = m.AppVersion
	metadata["Description"] = m.Description
	metadata["Keywords"] = m.Keywords
	metadata["Home"] = m.Home
	metadata["Sources"] = m.Sources
	metadata["Icon"] = m.Icon
	metadata["APIVersion"] = m.APIVersion
	metadata["Condition"] = m.Condition
	metadata["Tags"] = m.Tags
	metadata["Type"] = m.Type
	metadata["Annotations"] = m.Annotations

	if m.Maintainers != nil {
		maintainers := make([]map[string]any, len(m.Maintainers))
		for i, maint := range m.Maintainers {
			maintainers[i] = map[string]any{
				"Name":  maint.Name,
				"Email": maint.Email,
				"URL":   maint.URL,
			}
		}

		metadata["Maintainers"] = maintainers
	}

	return metadata
}

func marshalManifests(value any) (string, error) {
	arr, ok := value.([]any)
	if !ok {
		yamlBytes, err := yaml.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("marshal resource: %w", err)
		}

		return string(yamlBytes), nil
	}

	var results []string
	for _, item := range arr {
		if item == nil {
			continue
		}

		yamlBytes, err := yaml.Marshal(item)
		if err != nil {
			return "", fmt.Errorf("marshal manifest: %w", err)
		}

		results = append(results, string(yamlBytes))
	}

	return strings.Join(results, "---\n"), nil
}
