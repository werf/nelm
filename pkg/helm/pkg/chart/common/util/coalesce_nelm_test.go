package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepCopyValues(t *testing.T) {
	vals := map[string]any{"a": map[string]any{"b": []any{map[string]any{"c": "d"}}}}

	got, err := DeepCopyValues(vals)
	require.NoError(t, err)
	assert.Equal(t, vals, got)

	got["a"].(map[string]any)["b"].([]any)[0].(map[string]any)["c"] = "changed"
	assert.Equal(t, "d", vals["a"].(map[string]any)["b"].([]any)[0].(map[string]any)["c"])

	got, err = DeepCopyValues(nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, got)
}

func TestDeleteValuesAtPaths(t *testing.T) {
	vals := map[string]any{
		"a": map[string]any{"b": "deleted", "c": "kept"},
		"d": "scalar",
		"e": "deleted",
	}

	DeleteValuesAtPaths(vals, [][]string{{"a", "b"}, {"d", "x"}, {"missing", "x"}, {"e"}})

	assert.Equal(t, map[string]any{"a": map[string]any{"c": "kept"}, "d": "scalar"}, vals)
}
