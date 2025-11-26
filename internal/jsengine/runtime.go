package jsengine

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"

	"github.com/werf/nelm/internal/jsengine/helpers"
)

func (e *Engine) createVM(ctx context.Context, chartPath string, jsTemplateFiles map[string][]byte) (*goja.Runtime, *require.RequireModule, error) {
	vm := goja.New()

	// Disable eval for security
	vm.Set("eval", nil)

	sourceLoader := func(requestedPath string) ([]byte, error) {
		if strings.HasPrefix(requestedPath, "helm:") || strings.Contains(requestedPath, "/helm:") {
			// Native modules are not files, return "not found" so require tries registry
			return nil, require.ModuleFileDoesNotExistError
		}

		if path.IsAbs(requestedPath) {
			return nil, fmt.Errorf("absolute paths not allowed: %q (security restriction)", requestedPath)
		}

		cleanPath := path.Clean(requestedPath)

		if !strings.HasPrefix(cleanPath, JSTemplatesDir+"/") && cleanPath != JSTemplatesDir {
			return nil, fmt.Errorf("module %q is outside js-templates/ directory (security restriction)", requestedPath)
		}

		if strings.Contains(cleanPath, "..") {
			return nil, fmt.Errorf("module %q contains path traversal (security restriction)", requestedPath)
		}

		if content, ok := jsTemplateFiles[requestedPath]; ok {
			return content, nil
		}

		// File not found - return the special error that tells require to try other extensions
		return nil, require.ModuleFileDoesNotExistError
	}

	registry := require.NewRegistryWithLoader(sourceLoader)

	helpers.RegisterYamlModule(registry)
	helpers.RegisterHelpersModule(registry)

	requireModule := registry.Enable(vm)

	helpers.SetupConsoleGlobal(ctx, vm)

	return vm, requireModule, nil
}

// formatJSError formats a JavaScript error with filtered stack trace
// Shows error message and all user JavaScript files in call stack, hiding goja internals
func formatJSError(err error, currentFile string) error {
	if err == nil {
		return nil
	}

	gojaErr, ok := err.(*goja.Exception)
	if !ok {
		return err
	}

	errMsg := gojaErr.Error()

	stackProp := gojaErr.Value().ToObject(nil).Get("stack")
	if stackProp == nil || goja.IsUndefined(stackProp) || goja.IsNull(stackProp) {
		return fmt.Errorf("%s\n  at %s", errMsg, currentFile)
	}

	stack := stackProp.String()

	filteredStack := filterStackTrace(stack, currentFile)

	if filteredStack == "" {
		return fmt.Errorf("%s\n  at %s", errMsg, currentFile)
	}

	return fmt.Errorf("%s\n%s", errMsg, filteredStack)
}

// filterStackTrace filters a JavaScript stack trace to show only user code
func filterStackTrace(stack string, currentFile string) string {
	lines := strings.Split(stack, "\n")
	var filteredLines []string

	// Regex patterns to match user code frames
	// Look for patterns like: "at functionName (file.js:line:col)" or "at file.js:line:col"
	userFramePattern := regexp.MustCompile(`at\s+(?:(\w+)\s+\()?([^:]+\.js):(\d+):(\d+)\)?`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "at ") {
			continue
		}

		matches := userFramePattern.FindStringSubmatch(line)
		if matches != nil {
			// matches[1] = function name (optional)
			// matches[2] = file path
			// matches[3] = line number
			// matches[4] = column number

			filePath := matches[2]
			lineNum := matches[3]
			funcName := matches[1]

			if strings.Contains(filePath, "goja") || strings.Contains(filePath, "runtime") {
				continue
			}

			var frame string
			if funcName != "" {
				frame = fmt.Sprintf("  at %s (%s:%s)", funcName, path.Base(filePath), lineNum)
			} else {
				frame = fmt.Sprintf("  at %s:%s", path.Base(filePath), lineNum)
			}

			filteredLines = append(filteredLines, frame)
		}
	}

	return strings.Join(filteredLines, "\n")
}
