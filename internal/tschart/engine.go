package tschart

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	"sigs.k8s.io/yaml"
)

const (
	DefaultTimeout = 3 * time.Second
)

type Engine struct {
	timeout time.Duration
}

func NewEngine() *Engine {
	return &Engine{
		timeout: DefaultTimeout,
	}
}

func NewEngineWithTimeout(timeout time.Duration) *Engine {
	return &Engine{
		timeout: timeout,
	}
}

func (e *Engine) RenderFiles(ctx context.Context, chart *helmchart.Chart, renderedValues chartutil.Values) (map[string]string, error) {
	var bundleFile *helmchart.File
	for _, file := range chart.Files {
		if file.Name == BundleFile {
			bundleFile = file
			break
		}
	}

	if bundleFile == nil {
		return map[string]string{}, nil
	}

	renderContext := buildRenderContext(renderedValues, chart)

	yaml, err := e.executeBundle(ctx, bundleFile.Data, renderContext)
	if err != nil {
		return nil, fmt.Errorf("execute bundle: %w", err)
	}

	if strings.TrimSpace(yaml) == "" {
		return map[string]string{}, nil
	}

	return map[string]string{
		BundleFile: yaml,
	}, nil
}

func buildRenderContext(renderedValues chartutil.Values, chart *helmchart.Chart) map[string]interface{} {
	renderContext := renderedValues.AsMap()

	// Flatten nested Values
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

func (e *Engine) executeBundle(ctx context.Context, bundleContent []byte, renderCtx map[string]interface{}) (string, error) {
	vm, requireModule, err := e.createVM(ctx, bundleContent)
	if err != nil {
		return "", fmt.Errorf("create JS VM: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		select {
		case <-timeoutCtx.Done():
			vm.Interrupt("execution timeout exceeded")
		case <-done:
		}
	}()
	defer close(done)

	exportsVal, err := requireModule.Require("./" + BundleFile)
	if err != nil {
		return "", formatJSError(err, BundleFile)
	}

	if exportsVal == nil || goja.IsUndefined(exportsVal) || goja.IsNull(exportsVal) {
		return "", fmt.Errorf("bundle does not export anything")
	}

	exports := exportsVal.ToObject(vm)
	if exports == nil {
		return "", fmt.Errorf("exports is not an object")
	}

	renderFunc := exports.Get("render")
	if renderFunc == nil || goja.IsUndefined(renderFunc) || goja.IsNull(renderFunc) {
		return "", fmt.Errorf("bundle does not export 'render' function")
	}

	renderCallable, ok := goja.AssertFunction(renderFunc)
	if !ok {
		return "", fmt.Errorf("'render' export is not a function")
	}

	result, err := renderCallable(goja.Undefined(), vm.ToValue(renderCtx))
	if err != nil {
		return "", formatJSError(err, BundleFile)
	}

	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return "", nil // Null return means skip rendering
	}

	if export, ok := result.Export().(map[string]interface{}); ok {
		if manifests, exists := export["manifests"]; exists {
			yamlStr, err := convertToYAML(manifests)
			if err != nil {
				return "", fmt.Errorf("convert result to YAML: %w", err)
			}
			return yamlStr, nil
		}
		return "", fmt.Errorf("render result object does not contain 'manifests' field")
	}

	return "", fmt.Errorf("unexpected render result type: %T", result.Export())
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
