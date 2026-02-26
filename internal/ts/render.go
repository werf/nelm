package ts

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/nelm/pkg/deno"
	"github.com/werf/nelm/pkg/log"
)

func RenderChart(ctx context.Context, chart *helmchart.Chart, renderedValues chartutil.Values, rebuildBundle bool, chartPath, tempDirPath string) (map[string]string, error) {
	allRendered := make(map[string]string)

	denoRuntime := deno.NewDenoRuntime(rebuildBundle)
	if err := denoRuntime.BundleChartsRecursive(ctx, chart, chartPath); err != nil {
		return nil, fmt.Errorf("process chart for TypeScript rendering: %w", err)
	}

	if err := renderChartRecursive(ctx, chart, renderedValues, chart.Name(), chartPath, allRendered, tempDirPath, denoRuntime); err != nil {
		return nil, fmt.Errorf("render chart recursive: %w", err)
	}

	return allRendered, nil
}

func renderChartRecursive(ctx context.Context, chart *helmchart.Chart, values chartutil.Values, pathPrefix, chartPath string, results map[string]string, tempDirPath string, denoRuntime *deno.DenoRuntime) error {
	log.Default.Debug(ctx, "Rendering TypeScript for chart %q (path prefix: %s)", chart.Name(), pathPrefix)

	entrypoint, bundle := deno.GetEntrypointAndBundle(chart.RuntimeFiles)

	if entrypoint != "" && bundle != nil {
		content, err := renderChart(ctx, bundle, chart, values, tempDirPath, denoRuntime)
		if err != nil {
			return fmt.Errorf("render files for chart %q: %w", chart.Name(), err)
		}

		if content != "" {
			outputPath := path.Join(pathPrefix, deno.ChartTSSourceDir, entrypoint)
			results[outputPath] = content
			log.Default.Debug(ctx, "Rendered output: %s", outputPath)
		}
	}

	for _, dep := range chart.Dependencies() {
		depName := dep.Name()
		log.Default.Debug(ctx, "Processing dependency %q for chart %q", depName, chart.Name())

		err := renderChartRecursive(
			ctx,
			dep,
			scopeValuesForSubchart(values, depName, dep),
			path.Join(pathPrefix, "charts", depName),
			filepath.Join(chartPath, "charts", depName),
			results,
			tempDirPath,
			denoRuntime,
		)
		if err != nil {
			return fmt.Errorf("render dependency %q: %w", depName, err)
		}
	}

	return nil
}

func renderChart(ctx context.Context, bundle *helmchart.File, chart *helmchart.Chart, renderedValues chartutil.Values, tempDirPath string, denoRuntime *deno.DenoRuntime) (string, error) {
	renderDir := filepath.Join(tempDirPath, "typescript-render", chart.ChartFullPath())
	if err := os.MkdirAll(renderDir, 0o755); err != nil {
		return "", fmt.Errorf("create temp dir for render context: %w", err)
	}

	if err := writeInputRenderContext(renderedValues, chart, renderDir); err != nil {
		return "", fmt.Errorf("build render context: %w", err)
	}

	if err := denoRuntime.RunApp(ctx, bundle.Data, renderDir); err != nil {
		return "", fmt.Errorf("run deno app: %w", err)
	}

	resultBytes, err := os.ReadFile(filepath.Join(renderDir, deno.RenderOutputFileName))
	if err != nil {
		return "", fmt.Errorf("read output file: %w", err)
	}

	return strings.TrimSpace(string(resultBytes)), nil
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

func writeInputRenderContext(renderedValues chartutil.Values, chart *helmchart.Chart, renderDir string) error {
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

	yamlInput, err := yaml.Marshal(renderContext)
	if err != nil {
		return fmt.Errorf("marshal render context to yaml: %w", err)
	}

	if err := os.WriteFile(filepath.Join(renderDir, deno.RenderInputFileName), yamlInput, 0o644); err != nil {
		return fmt.Errorf("write render context to file: %w", err)
	}

	return nil
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
