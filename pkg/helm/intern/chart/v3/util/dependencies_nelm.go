package util

import (
	"fmt"
	"strings"

	chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	"github.com/werf/nelm/v2/pkg/helm/intern/copystructure"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
)

type ImportUserValuesOptions struct {
	// ReplacedPaths are deleted from the values before and after the imports of each chart, so that
	// the values at these paths are neither imported nor consumed by the next imports, e.g. the paths
	// at which a higher values layer replaces the values of this layer.
	ReplacedPaths [][]string
}

// ImportUserValues applies import-values of the chart dependencies to one layer of user-supplied
// values v, and returns the result without modifying v. Within the layer, an imported value is
// overridden by the value at the parent path. It also returns the messages about the imports skipped
// because of their values. Call it after ProcessDependenciesWithMergedValues.
func ImportUserValues(c *chart.Chart, v map[string]any, opts ImportUserValuesOptions) (map[string]any, []string, error) {
	vals, err := util.DeepCopyValues(v)
	if err != nil {
		return nil, nil, fmt.Errorf("copy user values: %w", err)
	}

	util.DeleteValuesAtPaths(vals, opts.ReplacedPaths)

	return importUserValues(c, vals, opts.ReplacedPaths)
}

func importUserValues(c *chart.Chart, vals map[string]any, replacedPaths [][]string) (map[string]any, []string, error) {
	var skipped []string
	for _, d := range c.Dependencies() {
		var (
			depSkipped []string
			err        error
		)
		if vals, depSkipped, err = importUserValues(d, vals, replacedPaths); err != nil {
			return nil, nil, err
		}

		skipped = append(skipped, depSkipped...)
	}

	chartPath := stripFirstPathPart(c.ChartPath())

	imported := map[string]any{}
	for _, r := range c.Metadata.Dependencies {
		for _, importValue := range r.ImportValues {
			child, parent, ok := parseImportValues(importValue)
			if !ok {
				continue
			}

			childPath := joinNonEmptyPath(chartPath, r.Name, child)

			value, found := userValueAtPath(vals, childPath)
			if !found {
				continue
			}

			if value == nil {
				if parent != "." {
					imported = util.MergeTables(imported, valueToPathMap(parsePath(parent), nil))
				}

				continue
			}

			if !istable(value) {
				skipped = append(skipped, fmt.Sprintf("Skipping import-values of %q: only a map can be imported", childPath))
				continue
			}

			valueCopy, err := copystructure.Copy(value)
			if err != nil {
				return nil, nil, fmt.Errorf("copy value %q: %w", childPath, err)
			}

			imported = util.MergeTables(imported, pathToMap(parent, valueCopy.(map[string]any)))
		}
	}

	if len(imported) == 0 {
		return vals, skipped, nil
	}

	if chartPath != "" {
		imported = valueToPathMap(parsePath(chartPath), imported)
	}

	vals = util.MergeTables(vals, imported)
	util.DeleteValuesAtPaths(vals, replacedPaths)

	return vals, skipped, nil
}

func joinNonEmptyPath(path ...string) string {
	var parts []string
	for _, p := range path {
		if p != "" {
			parts = append(parts, p)
		}
	}

	return strings.Join(parts, ".")
}

func mergeOrCoalesceValues(c *chart.Chart, vals map[string]any, merge bool) (common.Values, error) {
	if merge {
		return util.MergeValues(c, vals)
	}

	return util.CoalesceValues(c, vals)
}

// parseImportValues returns the child and parent paths of an import-values entry, in the original
// form or normalized by processImportValues.
func parseImportValues(importValue any) (string, string, bool) {
	switch iv := importValue.(type) {
	case map[string]any:
		return fmt.Sprintf("%v", iv["child"]), fmt.Sprintf("%v", iv["parent"]), true
	case map[string]string:
		return iv["child"], iv["parent"], true
	case string:
		return "exports." + iv, ".", true
	default:
		return "", "", false
	}
}

func stripFirstPathPart(path string) string {
	pathParts := parsePath(path)
	if len(pathParts) <= 1 {
		return ""
	}

	return strings.Join(pathParts[1:], ".")
}

func userValueAtPath(vals map[string]any, path string) (any, bool) {
	var value any = vals
	for _, key := range parsePath(path) {
		table, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}

		if value, ok = table[key]; !ok {
			return nil, false
		}
	}

	return value, true
}

func valueToPathMap(path []string, value any) map[string]any {
	result := map[string]any{path[len(path)-1]: value}
	for i := len(path) - 2; i >= 0; i-- {
		result = map[string]any{path[i]: result}
	}

	return result
}

// valuesForChart returns the chart values. The extra values and the custom secret values are user
// values, imported by ImportUserValues, so they are used only if the chart values are invalid
// without them, e.g. when they replace a scalar in values.yaml with a map for a subchart.
func valuesForChart(c *chart.Chart, merge bool) (common.Values, error) {
	vals, err := mergeOrCoalesceValues(c, nil, merge)
	if err == nil {
		return vals, nil
	}

	extraVals, extraErr := util.ServiceAndCustomSecretValues(c)
	if extraErr != nil {
		return nil, extraErr
	}

	if len(extraVals) == 0 {
		return nil, err
	}

	return mergeOrCoalesceValues(c, extraVals, merge)
}

func applyDependenciesDefaultSecretValues(c *chart.Chart) error {
	for _, d := range c.Dependencies() {
		if err := util.ApplyDefaultSecretValues(d); err != nil {
			return err
		}

		if err := applyDependenciesDefaultSecretValues(d); err != nil {
			return err
		}
	}

	return nil
}
