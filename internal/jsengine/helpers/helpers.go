package helpers

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

// RegisterHelpersModule registers the helm:helpers module
// Provides essential helper functions for JavaScript charts
func RegisterHelpersModule(registry *require.Registry) {
	registry.RegisterNativeModule("helm:helpers", func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").ToObject(runtime)

		// b64enc encodes a string to base64
		exports.Set("b64enc", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("b64enc requires 1 argument"))
			}

			str := call.Argument(0).String()
			encoded := base64.StdEncoding.EncodeToString([]byte(str))
			return runtime.ToValue(encoded)
		})

		// b64dec decodes a base64 string
		exports.Set("b64dec", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("b64dec requires 1 argument"))
			}

			str := call.Argument(0).String()
			decoded, err := base64.StdEncoding.DecodeString(str)
			if err != nil {
				panic(runtime.NewGoError(fmt.Errorf("decode base64: %w", err)))
			}

			return runtime.ToValue(string(decoded))
		})

		// sha256sum computes the SHA256 hash of a string
		exports.Set("sha256sum", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("sha256sum requires 1 argument"))
			}

			str := call.Argument(0).String()
			hash := sha256.Sum256([]byte(str))
			hashStr := fmt.Sprintf("%x", hash)

			return runtime.ToValue(hashStr)
		})

		// quote wraps a string in double quotes
		exports.Set("quote", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("quote requires 1 argument"))
			}

			str := call.Argument(0).String()
			quoted := fmt.Sprintf("%q", str)

			return runtime.ToValue(quoted)
		})

		// bytesToString converts a byte array to UTF-8 string
		exports.Set("bytesToString", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("bytesToString requires 1 argument"))
			}

			bytesInterface := call.Argument(0).Export()

			var bytes []byte
			switch v := bytesInterface.(type) {
			case []byte:
				bytes = v
			case []interface{}:
				// Handle array of numbers from JavaScript
				bytes = make([]byte, len(v))
				for i, item := range v {
					if num, ok := item.(int64); ok {
						bytes[i] = byte(num)
					} else if num, ok := item.(float64); ok {
						bytes[i] = byte(num)
					} else {
						panic(runtime.NewTypeError(fmt.Sprintf("bytesToString: invalid byte at index %d", i)))
					}
				}
			default:
				panic(runtime.NewTypeError("bytesToString requires a byte array"))
			}

			str := string(bytes)
			if !utf8.ValidString(str) {
				panic(runtime.NewGoError(fmt.Errorf("bytesToString: invalid UTF-8 data")))
			}

			return runtime.ToValue(str)
		})

		// stringToBytes converts a string to byte array
		exports.Set("stringToBytes", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("stringToBytes requires 1 argument"))
			}

			str := call.Argument(0).String()
			bytes := []byte(str)

			return runtime.ToValue(bytes)
		})

		// b64encBytes encodes a byte array to base64 string
		exports.Set("b64encBytes", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("b64encBytes requires 1 argument"))
			}

			bytesInterface := call.Argument(0).Export()

			var bytes []byte
			switch v := bytesInterface.(type) {
			case []byte:
				bytes = v
			case []interface{}:
				// Handle array of numbers from JavaScript
				bytes = make([]byte, len(v))
				for i, item := range v {
					if num, ok := item.(int64); ok {
						bytes[i] = byte(num)
					} else if num, ok := item.(float64); ok {
						bytes[i] = byte(num)
					} else {
						panic(runtime.NewTypeError(fmt.Sprintf("b64encBytes: invalid byte at index %d", i)))
					}
				}
			default:
				panic(runtime.NewTypeError("b64encBytes requires a byte array"))
			}

			encoded := base64.StdEncoding.EncodeToString(bytes)
			return runtime.ToValue(encoded)
		})

		// b64decBytes decodes a base64 string to byte array
		exports.Set("b64decBytes", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("b64decBytes requires 1 argument"))
			}

			str := call.Argument(0).String()
			decoded, err := base64.StdEncoding.DecodeString(str)
			if err != nil {
				panic(runtime.NewGoError(fmt.Errorf("decode base64: %w", err)))
			}

			return runtime.ToValue(decoded)
		})

		// sha256sumBytes computes the SHA256 hash of a byte array
		exports.Set("sha256sumBytes", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 {
				panic(runtime.NewTypeError("sha256sumBytes requires 1 argument"))
			}

			bytesInterface := call.Argument(0).Export()

			var bytes []byte
			switch v := bytesInterface.(type) {
			case []byte:
				bytes = v
			case []interface{}:
				// Handle array of numbers from JavaScript
				bytes = make([]byte, len(v))
				for i, item := range v {
					if num, ok := item.(int64); ok {
						bytes[i] = byte(num)
					} else if num, ok := item.(float64); ok {
						bytes[i] = byte(num)
					} else {
						panic(runtime.NewTypeError(fmt.Sprintf("sha256sumBytes: invalid byte at index %d", i)))
					}
				}
			default:
				panic(runtime.NewTypeError("sha256sumBytes requires a byte array"))
			}

			hash := sha256.Sum256(bytes)
			hashStr := fmt.Sprintf("%x", hash)

			return runtime.ToValue(hashStr)
		})
	})
}
