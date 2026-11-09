package util

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/werf/nelm/v2/pkg/helm/intern/copystructure"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

type chartValuesExport struct {
	Chart  *chart.Chart
	Parent string
	Child  string
	Paths  userValuesExport
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

// processDependencyExportValues applies export-values of the chart dependencies, recursively, in the
// order of sortValuesExports, so that an export reads the values written by the exports to the
// sibling dependencies regardless of their order. Consecutive exports to the same dependency are
// applied together, unless one reads what another writes, so that the values in between, e.g. a
// scalar exported to a dependency root before a map exported there, are never coalesced.
func processDependencyExportValues(c *chart.Chart, merge bool) error {
	exports, err := prepareChartValuesExports(c, merge)
	if err != nil {
		return fmt.Errorf("prepare export values: %w", err)
	}

	paths := lo.Map(exports, func(export chartValuesExport, _ int) userValuesExport {
		return export.Paths
	})

	var batch []chartValuesExport
	for _, i := range sortValuesExports(paths) {
		if len(batch) > 0 && !joinsChartValuesExports(batch, exports[i]) {
			if err := applyChartValuesExports(batch, merge); err != nil {
				return fmt.Errorf("apply export values of dependency %q: %w", batch[0].Chart.ChartPath(), err)
			}

			batch = nil
		}

		batch = append(batch, exports[i])
	}

	if len(batch) > 0 {
		if err := applyChartValuesExports(batch, merge); err != nil {
			return fmt.Errorf("apply export values of dependency %q: %w", batch[0].Chart.ChartPath(), err)
		}
	}

	return nil
}

func joinsChartValuesExports(batch []chartValuesExport, export chartValuesExport) bool {
	if export.Chart != batch[0].Chart {
		return false
	}

	parentPaths := valuesExportParentPaths(export.Paths)

	return !lo.ContainsBy(batch, func(batchExport chartValuesExport) bool {
		return lo.ContainsBy(parentPaths, func(parentPath string) bool {
			return pathsOverlap(batchExport.Paths.ChildPath, parentPath)
		})
	})
}

// prepareChartValuesExports returns the export-values of the chart dependencies, recursively,
// parents before their dependencies. It normalizes the export-values in the chart metadata and, for
// each dependency in the chart metadata, replaces the values of the dependency and of its parents
// with their merged values, like applying export-values that export nothing.
func prepareChartValuesExports(c *chart.Chart, merge bool) ([]chartValuesExport, error) {
	var exports []chartValuesExport

	if r := dependencyForChart(c); r != nil {
		parentChartPath := stripFirstPathPart(c.Parent().ChartPath())
		depPath := joinNonEmptyPath(parentChartPath, c.Name())

		var normalizedExportValues []interface{}
		for _, rev := range dependencyExportValues(r) {
			parent, child, err := parseExportValues(rev)
			if err != nil {
				slog.Warn(
					"invalid ExportValues definition",
					slog.String("chart", c.Parent().Name()),
					slog.String("dependency", r.Name),
					slog.Any("error", err),
				)
				continue
			}

			normalizedExportValues = append(normalizedExportValues, map[string]string{
				"parent": parent,
				"child":  child,
			})

			exports = append(exports, chartValuesExport{
				Chart:  c,
				Parent: parent,
				Child:  child,
				Paths:  newUserValuesExport(parentChartPath, depPath, parent, child),
			})
		}

		setDependencyExportValues(r, normalizedExportValues)

		if err := overrideChartValues(c, map[string]interface{}{}, merge); err != nil {
			return nil, fmt.Errorf("merge values of dependency %q: %w", c.ChartPath(), err)
		}
	}

	for _, d := range c.Dependencies() {
		depExports, err := prepareChartValuesExports(d, merge)
		if err != nil {
			return nil, fmt.Errorf("prepare export values recursively: %w", err)
		}

		exports = append(exports, depExports...)
	}

	return exports, nil
}

// applyChartValuesExports applies the exports to the same dependency, reading the parent values once.
func applyChartValuesExports(exports []chartValuesExport, merge bool) error {
	c := exports[0].Chart

	pvals, err := valuesForChart(c.Parent(), merge)
	if err != nil {
		return fmt.Errorf("get parent values: %w", err)
	}

	globalPvals := pvals
	if c.Parent().Parent() != nil && lo.ContainsBy(exports, func(export chartValuesExport) bool { return isGlobalValuesPath(export.Parent) }) {
		if globalPvals, err = valuesWithRootGlobalsForChart(c.Parent(), merge); err != nil {
			return fmt.Errorf("get parent values with root chart globals: %w", err)
		}
	}

	exportedValues := map[string]interface{}{}
	for _, export := range exports {
		childValMap, ok := valueMapFromExport(c.Parent().Name(), c.Name(), lo.Ternary(isGlobalValuesPath(export.Parent), globalPvals, pvals), export.Parent, export.Child)
		if !ok {
			continue
		}

		if merge {
			exportedValues = util.MergeTables(deepCopyMap(childValMap), exportedValues)
		} else {
			exportedValues = util.CoalesceTables(deepCopyMap(childValMap), exportedValues)
		}
	}

	return overrideChartValues(c, exportedValues, merge)
}

func overrideChartValues(c *chart.Chart, overrides map[string]interface{}, merge bool) error {
	cvals, err := valuesForChart(c, merge)
	if err != nil {
		return fmt.Errorf("get chart values: %w", err)
	}

	if merge {
		c.Values = util.MergeTables(deepCopyMap(overrides), deepCopyMap(cvals))
	} else {
		c.Values = util.CoalesceTables(deepCopyMap(overrides), deepCopyMap(cvals))
	}

	if err := syncChartOverridesToParentsValues(c, deepCopyMap(overrides), merge); err != nil {
		return fmt.Errorf("sync chart overrides to parent values: %w", err)
	}

	return nil
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
