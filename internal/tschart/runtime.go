package tschart

import (
	"context"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"

	"github.com/werf/nelm/internal/tschart/helpers"
)

func (e *Engine) createVM(ctx context.Context, bundleContent []byte) (*goja.Runtime, *require.RequireModule, error) {
	vm := goja.New()

	sourceLoader := func(requestedPath string) ([]byte, error) {
		if requestedPath == BundleFile || requestedPath == "./"+BundleFile {
			return bundleContent, nil
		}

		return nil, require.ModuleFileDoesNotExistError
	}

	registry := require.NewRegistryWithLoader(sourceLoader)

	requireModule := registry.Enable(vm)

	helpers.SetupConsoleGlobal(ctx, vm)

	return vm, requireModule, nil
}

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

	return fmt.Errorf("%s", stack)
}
