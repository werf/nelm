package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
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

func TestCoalesceValuesTypeMismatchNamesSubchartPath(t *testing.T) {
	c := withDeps(&chart.Chart{Metadata: &chart.Metadata{Name: "p"}},
		withDeps(&chart.Chart{Metadata: &chart.Metadata{Name: "mid"}},
			&chart.Chart{Metadata: &chart.Metadata{Name: "leaf"}},
		),
	)

	_, err := CoalesceValues(c, map[string]any{
		"mid": map[string]any{"leaf": "some-string"},
	})

	assert.EqualError(t, err, "type mismatch on p.mid.leaf: expected a table, got string (some-string)")
}
