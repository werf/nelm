package util

import (
	"fmt"

	v3 "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	"github.com/werf/nelm/v2/pkg/helm/intern/copystructure"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	v2 "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

// DeepCopyValues returns a deep copy of vals, or an empty map if vals is nil.
func DeepCopyValues(vals map[string]any) (map[string]any, error) {
	valsCopy, err := copystructure.Copy(vals)
	if err != nil {
		return nil, fmt.Errorf("deep copy values: %w", err)
	}

	result := valsCopy.(map[string]any)
	if result == nil {
		return map[string]any{}, nil
	}

	return result, nil
}

// DeleteValuesAtPaths deletes the values at the paths from vals. A path whose parent is not a map is
// skipped.
func DeleteValuesAtPaths(vals map[string]any, paths [][]string) {
	for _, path := range paths {
		table := vals
		for _, key := range path[:len(path)-1] {
			if table, _ = table[key].(map[string]any); table == nil {
				break
			}
		}

		if table != nil {
			delete(table, path[len(path)-1])
		}
	}
}

// ApplyDefaultSecretValues merges the decrypted secret-values.yaml of the chart over its values.yaml
// in place, so that it has the priority of the chart defaults. Use it for subcharts: CoalesceValues
// and MergeValues apply the secret-values.yaml of the chart passed to them, but not of its subcharts.
func ApplyDefaultSecretValues(chrt chart.Charter) error {
	switch c := chrt.(type) {
	case *v2.Chart:
		vals, err := mergeDefaultSecretValues(c.Values, c.SecretsRuntimeData)
		if err != nil {
			return fmt.Errorf("merge secret values of chart %q: %w", c.Name(), err)
		}

		if vals != nil {
			c.Values = vals
		}
	case *v3.Chart:
		vals, err := mergeDefaultSecretValues(c.Values, c.SecretsRuntimeData)
		if err != nil {
			return fmt.Errorf("merge secret values of chart %q: %w", c.Name(), err)
		}

		if vals != nil {
			c.Values = vals
		}
	}

	return nil
}
