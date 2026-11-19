package ts

import (
	"os"
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatScaffoldTestValues(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: `{}`, expected: `{}`},
		{name: "objects", input: `{"global":{"name":"myapp","enabled":true,"count":2,"empty":null}}`, expected: `{global:{name:'myapp',enabled:true,count:2,empty:null}}`},
		{name: "keys", input: `{"$name":"app","_tag":"v1","with-dash":"value","a\"b":"value","a'b":"value"}`, expected: `{$name:'app',_tag:'v1','with-dash':'value','a"b':'value','a\'b':'value'}`},
		{name: "quotes", input: `{"value":"it's \"quoted\""}`, expected: `{value:'it\'s "quoted"'}`},
		{name: "backslashes", input: `{"value":"C:\\it's\\\"quoted\""}`, expected: `{value:'C:\\it\'s\\"quoted"'}`},
		{name: "escapes", input: `{"value":"line\nnext\t\u2028\u2029"}`, expected: `{value:'line\nnext\t\u2028\u2029'}`},
		{name: "arrays", input: `{"values":["a","b",true,false,null,1]}`, expected: `{values:['a','b',true,false,null,1]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := formatScaffoldTestValues([]byte(test.input))
			assert.Equal(t, test.expected, string(result))

			original := api.Transform("const values = "+test.input+";", api.TransformOptions{Loader: api.LoaderTS, MinifySyntax: true, MinifyWhitespace: true})
			require.Empty(t, original.Errors)

			formatted := api.Transform("const values = "+string(result)+";", api.TransformOptions{Loader: api.LoaderTS, MinifySyntax: true, MinifyWhitespace: true})
			require.Empty(t, formatted.Errors)
			assert.Equal(t, string(original.Code), string(formatted.Code))
		})
	}
}

func TestScaffoldInputPreservesValuesAndKeyOrder(t *testing.T) {
	content, err := os.ReadFile("testdata/scaffold-input.yaml")
	require.NoError(t, err)

	var original map[string]any
	require.NoError(t, yaml.Unmarshal(content, &original))

	for _, contextType := range []string{"RenderContext", "WerfRenderContext"} {
		t.Run(contextType, func(t *testing.T) {
			result, err := scaffoldInput(content, "false", contextType)
			require.NoError(t, err)
			assert.Less(t, strings.Index(string(result), "Values:"), strings.Index(string(result), "Release:"))
			assert.Less(t, strings.Index(string(result), "Release:"), strings.Index(string(result), "Chart:"))
			assert.Less(t, strings.Index(string(result), "Chart:"), strings.Index(string(result), "Capabilities:"))
			assert.Less(t, strings.Index(string(result), "z-last:"), strings.Index(string(result), "a-first:"))
			assert.Contains(t, string(result), "    z: last\n    a: first")

			var input map[string]any
			require.NoError(t, yaml.Unmarshal(result, &input))
			assert.Equal(t, "false", input["Chart"].(map[string]any)["Name"])
			assert.Equal(t, "false", input["Release"].(map[string]any)["Name"])
			assert.Equal(t, original["Capabilities"], input["Capabilities"])

			for _, key := range []string{"IsInstall", "IsUpgrade", "Revision", "Service"} {
				assert.Equal(t, original["Release"].(map[string]any)[key], input["Release"].(map[string]any)[key])
			}

			values := input["Values"].(map[string]any)
			delete(values, "global")
			assert.Equal(t, original["Values"], values)
		})
	}
}
