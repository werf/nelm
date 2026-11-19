package ts

import (
	"os"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
