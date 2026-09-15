//go:build ai_tests

package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAI_StripSchemaBytes(t *testing.T) {
	t.Run("drops description annotations", func(t *testing.T) {
		stripped := strip(t, `{
			"description": "Deployment enables declarative updates.",
			"type": "object",
			"properties": {
				"spec": {
					"description": "Specification of the desired behavior.",
					"properties": {
						"replicas": {"description": "Number of desired pods.", "format": "int32", "type": "integer"}
					}
				}
			}
		}`)

		assert.NotContains(t, stripped, "description")

		spec := stripped["properties"].(map[string]any)["spec"].(map[string]any)
		assert.NotContains(t, spec, "description")

		replicas := spec["properties"].(map[string]any)["replicas"].(map[string]any)
		assert.Equal(t, "int32", replicas["format"])
		assert.NotContains(t, replicas, "description")
	})

	t.Run("keeps description used as a property name", func(t *testing.T) {
		// PriorityClass really does have a "description" field, so a naive strip corrupts it.
		stripped := strip(t, `{
			"description": "PriorityClass defines mapping from a priority class name to the priority integer value.",
			"properties": {
				"description": {
					"description": "description is an arbitrary string.",
					"type": ["string", "null"]
				},
				"value": {"type": "integer"}
			},
			"required": ["value"]
		}`)

		assert.NotContains(t, stripped, "description", "the top level annotation must be gone")

		properties := stripped["properties"].(map[string]any)
		require.Contains(t, properties, "description", "the property named description must survive")

		descriptionProperty := properties["description"].(map[string]any)
		assert.Equal(t, []any{"string", "null"}, descriptionProperty["type"])
		assert.NotContains(t, descriptionProperty, "description", "its own annotation must be gone")
	})

	t.Run("keeps names in every name-keyed map", func(t *testing.T) {
		stripped := strip(t, `{
			"definitions": {"description": {"type": "string"}},
			"$defs": {"description": {"type": "string"}},
			"patternProperties": {"description": {"type": "string"}},
			"dependencies": {"description": ["value"]}
		}`)

		for _, keyword := range []string{"definitions", "$defs", "patternProperties", "dependencies"} {
			assert.Contains(t, stripped[keyword].(map[string]any), "description", "%s lost its key", keyword)
		}
	})

	t.Run("keeps instance data verbatim", func(t *testing.T) {
		stripped := strip(t, `{
			"enum": [{"description": "a literal value"}],
			"default": {"description": "a literal default"},
			"x-kubernetes-group-version-kind": [{"description": "extension data", "kind": "Deployment"}]
		}`)

		assert.Equal(t, []any{map[string]any{"description": "a literal value"}}, stripped["enum"])
		assert.Equal(t, map[string]any{"description": "a literal default"}, stripped["default"])
		assert.Equal(t,
			[]any{map[string]any{"description": "extension data", "kind": "Deployment"}},
			stripped["x-kubernetes-group-version-kind"])
	})

	t.Run("recurses through schema combinators", func(t *testing.T) {
		stripped := strip(t, `{
			"oneOf": [{"description": "first", "type": "string"}],
			"items": {"description": "item", "type": "string"},
			"not": {"description": "not", "type": "string"}
		}`)

		assert.NotContains(t, stripped["oneOf"].([]any)[0].(map[string]any), "description")
		assert.NotContains(t, stripped["items"].(map[string]any), "description")
		assert.NotContains(t, stripped["not"].(map[string]any), "description")
	})

	t.Run("rejects malformed json", func(t *testing.T) {
		_, err := stripSchemaBytes([]byte("{"))
		require.Error(t, err)
	})
}

func strip(t *testing.T, schema string) map[string]any {
	t.Helper()

	strippedBytes, err := stripSchemaBytes([]byte(schema))
	require.NoError(t, err)

	var stripped map[string]any

	require.NoError(t, json.Unmarshal(strippedBytes, &stripped))

	return stripped
}
