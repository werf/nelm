package helpers

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	"sigs.k8s.io/yaml"
)

// RegisterYamlModule registers the helm:yaml module
// Provides YAML/JSON serialization and data manipulation utilities
func RegisterYamlModule(registry *require.Registry) {
	registry.RegisterNativeModule("helm:yaml", func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").ToObject(runtime)

		// toYaml converts an object to YAML string
		exports.Set("toYaml", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("toYaml requires 1 argument"))
			}

			obj := call.Argument(0).Export()
			bytes, err := yaml.Marshal(obj)
			if err != nil {
				panic(runtime.NewGoError(fmt.Errorf("marshal to YAML: %w", err)))
			}

			return runtime.ToValue(string(bytes))
		})

		// fromYaml parses a YAML string to an object
		exports.Set("fromYaml", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("fromYaml requires 1 argument"))
			}

			yamlStr := call.Argument(0).String()
			var result interface{}
			err := yaml.Unmarshal([]byte(yamlStr), &result)
			if err != nil {
				panic(runtime.NewGoError(fmt.Errorf("unmarshal YAML: %w", err)))
			}

			return runtime.ToValue(result)
		})
	})
}
