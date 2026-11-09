package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

func TestProcessDependencies_ExportsRootGlobalsToNestedDependency(t *testing.T) {
	c := &chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion:   chart.APIVersionV2,
			Name:         "parent",
			Version:      "0.1.0",
			Dependencies: []*chart.Dependency{{Name: "mid", Version: "0.1.0"}},
		},
		Values:      map[string]any{"global": map[string]any{"root": "parent-values", "both": "parent-values"}},
		ExtraValues: map[string]any{"global": map[string]any{"service": "service-values"}},
	}
	mid := &chart.Chart{
		Metadata: &chart.Metadata{
			APIVersion: chart.APIVersionV2,
			Name:       "mid",
			Version:    "0.1.0",
			Dependencies: []*chart.Dependency{{Name: "leaf", Version: "0.1.0", ExportValues: []interface{}{
				map[string]interface{}{"parent": "global.root", "child": "fromRoot"},
				map[string]interface{}{"parent": "global.mid", "child": "fromMid"},
				map[string]interface{}{"parent": "global.both", "child": "fromBoth"},
				map[string]interface{}{"parent": "global.user", "child": "fromUser"},
				map[string]interface{}{"parent": "global.service", "child": "fromService"},
			}}},
		},
		Values: map[string]any{"global": map[string]any{"mid": "mid-values", "both": "mid-values"}},
	}
	mid.AddDependency(&chart.Chart{Metadata: &chart.Metadata{APIVersion: chart.APIVersionV2, Name: "leaf", Version: "0.1.0"}})
	c.AddDependency(mid)

	vals := map[string]interface{}{"global": map[string]interface{}{"user": "user-values"}}
	require.NoError(t, ProcessDependencies(c, &vals))

	leafValues := c.Dependencies()[0].Dependencies()[0].Values
	assert.Equal(t, "parent-values", leafValues["fromRoot"])
	assert.Equal(t, "mid-values", leafValues["fromMid"])
	assert.Equal(t, "parent-values", leafValues["fromBoth"])

	leafUserValues, found := userValueAtPath(vals, "mid.leaf")
	require.True(t, found)
	assert.Equal(t, map[string]interface{}{"fromUser": "user-values", "fromService": "service-values"}, leafUserValues)
}
