package tschart

import (
	"fmt"

	"github.com/dop251/goja"
)

const RequireShim = `
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

func createVM() (*goja.Runtime, error) {
	vm := goja.New()

	global := vm.NewObject()
	if err := vm.Set("global", global); err != nil {
		return nil, fmt.Errorf("set global: %w", err)
	}

	SetupConsoleGlobal(vm)

	return vm, nil
}

func executeInGoja(vendorBundle, appBundle string, renderCtx map[string]interface{}) (interface{}, error) {
	vm, err := createVM()
	if err != nil {
		return nil, fmt.Errorf("create VM: %w", err)
	}

	if vendorBundle != "" {
		if _, err := vm.RunString(vendorBundle); err != nil {
			return nil, fmt.Errorf("vendor bundle failed: %w", formatJSError(vm, err, "vendor/libs.js"))
		}
	}

	requireFn, err := vm.RunString(RequireShim)
	if err != nil {
		return nil, fmt.Errorf("require shim failed: %w", err)
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
		return nil, fmt.Errorf("app bundle failed: %w", formatJSError(vm, err, "app bundle"))
	}

	moduleExports := vm.Get("module").ToObject(vm).Get("exports")
	if moduleExports == nil || goja.IsUndefined(moduleExports) || goja.IsNull(moduleExports) {
		return nil, fmt.Errorf("bundle does not export anything")
	}

	renderFn := moduleExports.ToObject(vm).Get("render")
	if renderFn == nil || goja.IsUndefined(renderFn) || goja.IsNull(renderFn) {
		return nil, fmt.Errorf("bundle does not export 'render' function")
	}

	callable, ok := goja.AssertFunction(renderFn)
	if !ok {
		return nil, fmt.Errorf("'render' export is not a function")
	}

	result, err := callable(goja.Undefined(), vm.ToValue(renderCtx))
	if err != nil {
		return nil, fmt.Errorf("render failed: %w", formatJSError(vm, err, "render()"))
	}

	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return nil, nil //nolint:nilnil // Returning nil result with nil error indicates render produced no output
	}

	return result.Export(), nil
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
