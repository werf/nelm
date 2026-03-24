package ts

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"
)

const requireShim = `
(function() {
    var vendorRegistry = (typeof global !== 'undefined' && global.__NELM_VENDOR__) ||
                         (typeof __NELM_VENDOR__ !== 'undefined' && __NELM_VENDOR__) ||
                         (typeof __NELM_VENDOR_BUNDLE__ !== 'undefined' && __NELM_VENDOR_BUNDLE__.__NELM_VENDOR__) ||
                         {};

    function require(moduleName) {
        if (vendorRegistry[moduleName]) {
            return vendorRegistry[moduleName];
        }
        throw new Error("Module '" + moduleName + "' not found in vendor bundle. Available modules: " + Object.keys(vendorRegistry).join(", "));
    }

    return require;
})()
`

func runBundle(vendorBundle, appBundle string, renderCtx map[string]any) (any, error) {
	vm, err := createVM()
	if err != nil {
		return nil, fmt.Errorf("create VM: %w", err)
	}

	if vendorBundle != "" {
		if _, err := vm.RunString(vendorBundle); err != nil {
			return nil, fmt.Errorf("execute vendor bundle: %w", formatJSError(vm, err, "vendor/libs.js"))
		}
	}

	requireFn, err := vm.RunString(requireShim)
	if err != nil {
		return nil, fmt.Errorf("execute require shim: %w", err)
	}

	if err := vm.Set("require", requireFn); err != nil {
		return nil, fmt.Errorf("set require: %w", err)
	}

	module := vm.NewObject()
	exports := vm.NewObject()

	if err := module.Set("exports", exports); err != nil {
		return nil, fmt.Errorf("set module.exports: %w", err)
	}

	if err := vm.Set("module", module); err != nil {
		return nil, fmt.Errorf("set module: %w", err)
	}

	if err := vm.Set("exports", exports); err != nil {
		return nil, fmt.Errorf("set exports: %w", err)
	}

	if _, err := vm.RunString(appBundle); err != nil {
		return nil, fmt.Errorf("execute app bundle: %w", formatJSError(vm, err, "app bundle"))
	}

	moduleExports := vm.Get("module").ToObject(vm).Get("exports")
	if moduleExports == nil || goja.IsUndefined(moduleExports) || goja.IsNull(moduleExports) {
		return nil, fmt.Errorf("run bundle: no exports")
	}

	renderFn := moduleExports.ToObject(vm).Get("render")
	if renderFn == nil || goja.IsUndefined(renderFn) || goja.IsNull(renderFn) {
		return nil, fmt.Errorf("run bundle: no 'render' function exported")
	}

	callable, ok := goja.AssertFunction(renderFn)
	if !ok {
		return nil, fmt.Errorf("run bundle: 'render' export is not a function")
	}

	result, err := callable(goja.Undefined(), vm.ToValue(renderCtx))
	if err != nil {
		return nil, fmt.Errorf("call render function: %w", formatJSError(vm, err, "render()"))
	}

	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return nil, nil //nolint:nilnil // Returning nil result with nil error indicates render produced no output
	}

	return result.Export(), nil
}

func createVM() (*goja.Runtime, error) {
	vm := goja.New()

	global := vm.NewObject()
	if err := vm.Set("global", global); err != nil {
		return nil, fmt.Errorf("set global: %w", err)
	}

	setupConsole(vm)

	return vm, nil
}

func formatJSError(vm *goja.Runtime, err error, currentFile string) error {
	if err == nil {
		return nil
	}

	gojaErr, ok := err.(*goja.Exception)
	if !ok {
		return err
	}

	errMsg := gojaErr.Error()

	stackProp := gojaErr.Value().ToObject(vm).Get("stack")
	if stackProp == nil || goja.IsUndefined(stackProp) || goja.IsNull(stackProp) {
		return fmt.Errorf("%s\n  at %s", errMsg, currentFile)
	}

	stack := stackProp.String()

	return fmt.Errorf("%s", stack)
}

func setupConsole(runtime *goja.Runtime) {
	registry := require.NewRegistry()
	registry.Enable(runtime)

	console.Enable(runtime)
}
