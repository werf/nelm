//go:build ai_tests

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
)

func TestAI_ProcessDependenciesCallable(t *testing.T) {
	parent := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:       "parent",
			Version:    "1.0.0",
			APIVersion: chart.APIVersionV2,
		},
		Values: map[string]interface{}{
			"key": "value",
		},
	}

	child := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:       "child",
			Version:    "0.1.0",
			APIVersion: chart.APIVersionV2,
		},
		Values: map[string]interface{}{},
	}
	parent.SetDependencies(child)

	vals := map[string]interface{}{
		"key": "value",
	}

	err := ProcessDependencies(parent, &vals)
	require.NoError(t, err)
	assert.NotNil(t, vals)
}

func TestAI_ProcessDependenciesRejectsNilVals(t *testing.T) {
	parent := &chart.Chart{
		Metadata: &chart.Metadata{
			Name:       "parent",
			Version:    "1.0.0",
			APIVersion: chart.APIVersionV2,
		},
	}

	err := ProcessDependencies(parent, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}
