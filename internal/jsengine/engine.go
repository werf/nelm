package jsengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	"sigs.k8s.io/yaml"
)

const (
	JSTemplatesDir    = "js-templates"
	ManifestJSSuffix  = ".manifest.js"
	ManifestsJSSuffix = ".manifests.js"
	DefaultTimeout    = 3 * time.Second
)

type Engine struct {
	timeout time.Duration
}

func New() Engine {
	return Engine{
		timeout: DefaultTimeout,
	}
}

func NewWithTimeout(timeout time.Duration) Engine {
	return Engine{
		timeout: timeout,
	}
}

func (e *Engine) RenderFiles(ctx context.Context, chart *chart.Chart, renderedValues chartutil.Values) (map[string]string, error) {

	chartPath := chart.ChartFullPath()

	jsTemplateFiles := make(map[string][]byte)
	var manifestFiles []string

	for _, file := range chart.Files {
		if !strings.HasPrefix(file.Name, JSTemplatesDir+"/") {
			continue
		}

		jsTemplateFiles[file.Name] = file.Data

		if strings.HasSuffix(file.Name, ManifestJSSuffix) || strings.HasSuffix(file.Name, ManifestsJSSuffix) {
			manifestFiles = append(manifestFiles, file.Name)
		}
	}

	if len(manifestFiles) == 0 {
		return map[string]string{}, nil
	}

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

	renderedTemplates := make(map[string]string)
	for _, filePath := range manifestFiles {
		yaml, err := e.renderFile(ctx, chartPath, filePath, jsTemplateFiles, renderContext)
		if err != nil {
			return nil, fmt.Errorf("render %q: %w", filePath, err)
		}

		if strings.TrimSpace(yaml) != "" {
			renderedTemplates[filePath] = yaml
		}
	}

	return renderedTemplates, nil
}

func (e *Engine) RenderFile(ctx context.Context, fileName string, fileContent []byte, helperFiles map[string][]byte, renderCtx map[string]interface{}) (string, error) {
	jsTemplateFiles := make(map[string][]byte)

	mainFilePath := fileName
	if !strings.HasPrefix(mainFilePath, JSTemplatesDir+"/") {
		mainFilePath = JSTemplatesDir + "/" + fileName
	}
	jsTemplateFiles[mainFilePath] = fileContent

	for name, content := range helperFiles {
		helperPath := name
		if !strings.HasPrefix(helperPath, JSTemplatesDir+"/") {
			helperPath = JSTemplatesDir + "/" + name
		}
		jsTemplateFiles[helperPath] = content
	}

	chartPath := "/test-chart"

	return e.renderFile(ctx, chartPath, mainFilePath, jsTemplateFiles, renderCtx)
}

func (e *Engine) renderFile(ctx context.Context, chartPath string, filePath string, jsTemplateFiles map[string][]byte, renderCtx map[string]interface{}) (string, error) {
	vm, requireModule, err := e.createVM(ctx, chartPath, jsTemplateFiles)
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
			// Execution completed before timeout
		}
	}()
	defer close(done)

	requirePath := "./" + filePath

	exportsVal, err := requireModule.Require(requirePath)
	if err != nil {
		return "", formatJSError(err, filePath)
	}

	if exportsVal == nil || goja.IsUndefined(exportsVal) || goja.IsNull(exportsVal) {
		return "", fmt.Errorf("file does not export anything")
	}

	exports := exportsVal.ToObject(vm)
	if exports == nil {
		return "", fmt.Errorf("exports is not an object")
	}

	renderFunc := exports.Get("render")
	if renderFunc == nil || goja.IsUndefined(renderFunc) || goja.IsNull(renderFunc) {
		return "", fmt.Errorf("file does not export a 'render' function")
	}

	renderCallable, ok := goja.AssertFunction(renderFunc)
	if !ok {
		return "", fmt.Errorf("'render' export is not a function")
	}

	result, err := renderCallable(goja.Undefined(), vm.ToValue(renderCtx))
	if err != nil {
		return "", formatJSError(err, filePath)
	}

	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return "", nil
	}

	// FIXME: check that returned value is an object/array

	yamlStr, err := convertToYAML(result.Export())
	if err != nil {
		return "", fmt.Errorf("convert result to YAML: %w", err)
	}

	return yamlStr, nil
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
