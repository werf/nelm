package util

import (
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/werf/nelm/v2/pkg/helm/intern/copystructure"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

func exportsGlobalValues(r *chart.Dependency) bool {
	return lo.ContainsBy(dependencyExportValues(r), func(rev interface{}) bool {
		parent, _, err := parseExportValues(rev)
		return err == nil && isGlobalValuesPath(parent)
	})
}

func isGlobalValuesPath(path string) bool {
	return parsePath(path)[0] == common.GlobalKey
}

// valuesWithRootGlobalsForChart returns the chart values coalesced from the root chart, so that their
// globals are the globals of the root chart merged over the globals of each chart down to this one,
// like when the chart is rendered.
func valuesWithRootGlobalsForChart(c *chart.Chart, merge bool) (common.Values, error) {
	root := c
	for root.Parent() != nil {
		root = root.Parent()
	}

	rootVals, err := valuesForChart(root, merge)
	if err != nil {
		return nil, fmt.Errorf("get root chart values: %w", err)
	}

	vals, err := rootVals.Table(stripFirstPathPart(c.ChartPath()))
	if err != nil {
		return nil, fmt.Errorf("get chart %q values from root chart values: %w", c.ChartPath(), err)
	}

	return vals, nil
}

func userValuesGlobalPaths(chartPath string) []string {
	paths := []string{common.GlobalKey}
	chartPathParts := parsePath(chartPath)
	for i := range chartPathParts {
		paths = append(paths, joinPath(append(slices.Clone(chartPathParts[:i+1]), common.GlobalKey)...))
	}

	return paths
}

func userValuesExportParentValue(vals map[string]interface{}, export userValuesExport) (interface{}, bool, error) {
	if len(export.GlobalPaths) == 0 {
		value, found := userValueAtPath(vals, export.ParentPath)
		return value, found, nil
	}

	var globals map[string]interface{}
	for _, path := range export.GlobalPaths {
		value, found := userValueAtPath(vals, path)
		if !found {
			continue
		}

		// coalesceGlobals propagates no globals into or out of a non-map global.
		if !istable(value) {
			globals = nil
			continue
		}

		valueCopy, err := copystructure.Copy(value)
		if err != nil {
			return nil, false, fmt.Errorf("copy value %q: %w", path, err)
		}

		globals = mergeUserGlobals(globals, valueCopy.(map[string]interface{}))
	}

	if globals == nil {
		return nil, false, nil
	}

	value, found := userValueAtPath(map[string]interface{}{common.GlobalKey: globals}, export.ParentPath)

	return value, found, nil
}

// mergeUserGlobals merges the higher chart globals over the lower chart globals like coalesceGlobals,
// which keeps the lower chart value of a top-level key if only one of the values is a map.
func mergeUserGlobals(higher, lower map[string]interface{}) map[string]interface{} {
	for key, value := range higher {
		lowerValue, found := lower[key]
		switch {
		case found && istable(value) != istable(lowerValue):
		case istable(value) && found:
			lower[key] = util.MergeTables(value.(map[string]interface{}), lowerValue.(map[string]interface{}))
		default:
			lower[key] = value
		}
	}

	return lower
}

func newUserValuesExport(chartPath, depPath, parent, child string) userValuesExport {
	export := userValuesExport{
		ChildPath:        joinNonEmptyPath(depPath, child),
		ParentPath:       joinNonEmptyPath(chartPath, parent),
		ToDependencyRoot: child == "",
	}

	if chartPath != "" && isGlobalValuesPath(parent) {
		export.GlobalPaths = userValuesGlobalPaths(chartPath)
		export.ParentPath = parent
	}

	return export
}

// valuesExportParentPaths returns the paths from the root chart that the export reads.
func valuesExportParentPaths(export userValuesExport) []string {
	parentPaths := []string{export.ParentPath}
	for _, globalPath := range export.GlobalPaths {
		parentPaths = append(parentPaths, globalPath+strings.TrimPrefix(export.ParentPath, common.GlobalKey))
	}

	return parentPaths
}
