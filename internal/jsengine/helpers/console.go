package helpers

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dop251/goja"

	"github.com/werf/nelm/pkg/log"
)

func SetupConsoleGlobal(ctx context.Context, runtime *goja.Runtime) {
	console := runtime.NewObject()

	console.Set("log", func(call goja.FunctionCall) goja.Value {
		args := make([]interface{}, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		message := formatConsoleArgs(args...)
		fmt.Fprintln(os.Stdout, message)
		return goja.Undefined()
	})

	console.Set("error", func(call goja.FunctionCall) goja.Value {
		args := make([]interface{}, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		message := formatConsoleArgs(args...)
		fmt.Fprintln(os.Stderr, message)
		return goja.Undefined()
	})

	console.Set("warn", func(call goja.FunctionCall) goja.Value {
		args := make([]interface{}, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		message := formatConsoleArgs(args...)
		log.Default.Warn(ctx, message)
		return goja.Undefined()
	})

	runtime.Set("console", console)
}

func formatConsoleArgs(args ...interface{}) string {
	if len(args) == 0 {
		return ""
	}

	var parts []string
	for _, arg := range args {
		parts = append(parts, fmt.Sprintf("%v", arg))
	}

	return strings.Join(parts, " ")
}
