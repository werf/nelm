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
	content, err := os.ReadFile("scaffold/input.example.yaml")
	require.NoError(t, err)

	for _, contextType := range []string{"RenderContext", "WerfRenderContext"} {
		t.Run(contextType, func(t *testing.T) {
			result, err := scaffoldInput(content, "false", contextType)
			require.NoError(t, err)
			assert.Less(t, strings.Index(string(result), "Chart:"), strings.Index(string(result), "Files:"))
			assert.Less(t, strings.Index(string(result), "Files:"), strings.Index(string(result), "Release:"))
			assert.Less(t, strings.Index(string(result), "Release:"), strings.Index(string(result), "Values:"))

			var input map[string]any
			require.NoError(t, yaml.Unmarshal(result, &input))
			assert.Equal(t, "false", input["Chart"].(map[string]any)["Name"])
			assert.Equal(t, "false", input["Release"].(map[string]any)["Name"])
		})
	}
}
