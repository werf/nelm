package util

import (
	"fmt"

	"github.com/werf/nelm/v2/pkg/helm/intern/copystructure"
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
