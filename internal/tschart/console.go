package tschart

import (
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/console"
	"github.com/dop251/goja_nodejs/require"
)

func SetupConsoleGlobal(runtime *goja.Runtime) {
	registry := require.NewRegistry()
	registry.Enable(runtime)

	console.Enable(runtime)
}
