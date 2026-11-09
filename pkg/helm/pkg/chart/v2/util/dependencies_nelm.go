package util

import (
	"fmt"
	"log/slog"
	"slices"

	"github.com/samber/lo"

	"github.com/werf/nelm/v2/pkg/helm/intern/copystructure"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

type chartValuesExport struct {
	Chart          *chart.Chart
	DependencyPath []string
	Parent         string
	Child          string
	Paths          userValuesExport
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

	vals, err := rootVals.Table(joinPath(chartPathFromRoot(c)...))
	if err != nil {
		return nil, fmt.Errorf("get chart %q values from root chart values: %w", c.ChartPath(), err)
	}

	return vals, nil
}

func userValuesGlobalPaths(chartPath []string) [][]string {
	paths := [][]string{{common.GlobalKey}}
	for i := range chartPath {
		paths = append(paths, slices.Concat(chartPath[:i+1], []string{common.GlobalKey}))
	}

	return paths
}

func userValuesExportParentValue(vals map[string]interface{}, export userValuesExport) (interface{}, bool, error) {
	if len(export.GlobalPaths) == 0 {
		value, found := valueAtPath(vals, export.ParentPath)
		return value, found, nil
	}

	var globals map[string]interface{}
	for _, path := range export.GlobalPaths {
		value, found := valueAtPath(vals, path)
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
			return nil, false, fmt.Errorf("copy value %q: %w", joinPath(path...), err)
		}

		globals = mergeUserGlobals(globals, valueCopy.(map[string]interface{}))
	}

	if globals == nil {
		return nil, false, nil
	}

	value, found := valueAtPath(map[string]interface{}{common.GlobalKey: globals}, export.ParentPath)

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
// sibling dependencies regardless of their order. The exports to the same dependency are applied
// together where batchChartValuesExports allows it, so that the values in between, e.g. a scalar
// exported to a dependency root before a map exported there, are never coalesced.
func processDependencyExportValues(c *chart.Chart, merge bool) error {
	exports, err := prepareChartValuesExports(c, merge)
	if err != nil {
		return fmt.Errorf("prepare export values: %w", err)
	}

	for _, batch := range batchChartValuesExports(exports, dependencyPaths(c)) {
		if err := applyChartValuesExports(batch, merge); err != nil {
			return fmt.Errorf("apply export values of dependency %q: %w", batch[0].Chart.ChartPath(), err)
		}
	}

	return nil
}

// batchChartValuesExports returns the exports in the order of sortValuesExports, split into batches
// of exports to the same dependency. An export joins the last batch of its dependency unless it
// reads what the batch writes, or the batch is not independent of an export that follows it. The
// batch is then moved after the batches that follow it, which, unless the values in between are
// invalid, changes what no export reads. depPaths are the paths of all the dependencies in the
// chart values. If a dependency is named global, no batch is moved: its values are also globals,
// which coalescing propagates to the other charts, nesting them deeper on each pass.
func batchChartValuesExports(exports []chartValuesExport, depPaths [][]string) [][]chartValuesExport {
	moveBatches := !lo.ContainsBy(depPaths, func(depPath []string) bool {
		return depPath[len(depPath)-1] == common.GlobalKey
	})

	paths := lo.Map(exports, func(export chartValuesExport, _ int) userValuesExport {
		return export.Paths
	})

	var (
		batches [][]chartValuesExport
		// dependentsAfter[k] is the number of exports after batches[k] that are not independent of it.
		dependentsAfter []int
	)

	for _, i := range sortValuesExports(paths) {
		export := exports[i]

		batch, k, found := lo.FindLastIndexOf(batches, func(batch []chartValuesExport) bool {
			return batch[0].Chart == export.Chart
		})

		if found && dependentsAfter[k] == 0 && !lo.ContainsBy(batch, func(batchExport chartValuesExport) bool {
			return valuesExportReadsPath(export.Paths, batchExport.Paths.ChildPath)
		}) {
			batches = append(slices.Delete(batches, k, k+1), append(batch, export))
			dependentsAfter = append(slices.Delete(dependentsAfter, k, k+1), 0)
		} else {
			batches = append(batches, []chartValuesExport{export})
			dependentsAfter = append(dependentsAfter, 0)
		}

		for j, earlierBatch := range batches[:len(batches)-1] {
			if !lo.EveryBy(earlierBatch, func(batchExport chartValuesExport) bool {
				return moveBatches && chartValuesExportsIndependent(batchExport, export, depPaths)
			}) {
				dependentsAfter[j]++
			}
		}
	}

	return batches
}

// prepareChartValuesExports returns the export-values of the chart dependencies, recursively,
// parents before their dependencies. It normalizes the export-values in the chart metadata and, for
// each dependency in the chart metadata, replaces the values of the dependency and of its parents
// with their merged values, like applying export-values that export nothing.
func prepareChartValuesExports(c *chart.Chart, merge bool) ([]chartValuesExport, error) {
	var exports []chartValuesExport

	if r := dependencyForChart(c); r != nil {
		parentChartPath := chartPathFromRoot(c.Parent())
		depPath := appendPath(parentChartPath, c.Name())

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
				Chart:          c,
				DependencyPath: depPath,
				Parent:         parent,
				Child:          child,
				Paths:          newUserValuesExport(parentChartPath, depPath, parent, child),
			})
		}

		setDependencyExportValues(r, normalizedExportValues)

		if err := overrideChartValues(c, map[string]interface{}{}, merge); err != nil {
			return nil, fmt.Errorf("merge values of dependency %q: %w", c.ChartPath(), err)
		}
	} else if c.Parent() != nil {
		// A declared dependency gets its secret-values.yaml above, via overrideChartValues. An undeclared
		// one gets it at the same step, so that the export-values see it as for a declared one.
		if err := util.ApplyDefaultSecretValues(c); err != nil {
			return nil, fmt.Errorf("apply secret values of undeclared dependency %q: %w", c.ChartPath(), err)
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

func newUserValuesExport(chartPath, depPath []string, parent, child string) userValuesExport {
	export := userValuesExport{
		ChildPath:        appendPath(depPath, child),
		ParentPath:       appendPath(chartPath, parent),
		ToDependencyRoot: child == "",
	}

	if len(chartPath) > 0 && isGlobalValuesPath(parent) {
		export.GlobalPaths = userValuesGlobalPaths(chartPath)
		export.ParentPath = parsePath(parent)
	}

	return export
}

// valuesExportParentPaths returns the paths from the root chart that the export reads.
func valuesExportParentPaths(export userValuesExport) [][]string {
	parentPaths := [][]string{export.ParentPath}
	for _, globalPath := range export.GlobalPaths {
		parentPaths = append(parentPaths, slices.Concat(globalPath, export.ParentPath[1:]))
	}

	return parentPaths
}

// sortValuesExports returns the indexes of the exports in the order to apply them: each export comes
// after the exports that write to its parent path, e.g. when its parent path is the child path of an
// export to a sibling dependency listed after it, and after the exports listed before it that write
// to its child path. Otherwise the order is kept. A cycle is broken at its first listed export.
func sortValuesExports(exports []userValuesExport) []int {
	waitedBy := make([][]int, len(exports))
	waitsFor := make([]int, len(exports))
	for i := range exports {
		for j := range exports {
			if j != i && (valuesExportReadsPath(exports[i], exports[j].ChildPath) || (j < i && pathsOverlap(exports[j].ChildPath, exports[i].ChildPath))) {
				waitedBy[j] = append(waitedBy[j], i)
				waitsFor[i]++
			}
		}
	}

	sorted := make([]int, 0, len(exports))
	done := make([]bool, len(exports))

	for len(sorted) < len(exports) {
		next := -1
		for i := range exports {
			if done[i] {
				continue
			}

			if next == -1 {
				next = i
			}

			if waitsFor[i] == 0 {
				next = i
				break
			}
		}

		done[next] = true
		sorted = append(sorted, next)

		for _, i := range waitedBy[next] {
			waitsFor[i]--
		}
	}

	return sorted
}

// chartValuesExportsIndependent reports whether the exports can be applied in any order: the values
// of their dependencies don't overlap, neither of them reads the values of the other's dependency,
// and not both of them touch globals. An export might change the values of its dependency outside of
// its child path, e.g. the globals of the nested charts, or the defaults merged into a map that
// replaced a scalar. Coalescing also merges the globals of a dependency into the globals of its
// parents, because it copies the nested maps of the globals by reference.
func chartValuesExportsIndependent(a, b chartValuesExport, depPaths [][]string) bool {
	return !(valuesExportTouchesGlobals(a.Paths, depPaths) && valuesExportTouchesGlobals(b.Paths, depPaths)) &&
		!pathsOverlap(a.DependencyPath, b.DependencyPath) &&
		!valuesExportReadsPath(a.Paths, b.DependencyPath) &&
		!valuesExportReadsPath(b.Paths, a.DependencyPath)
}

// valuesExportTouchesGlobals reports whether the export reads or writes a path with globals: a path
// under a global, or a path of a dependency or of its parents, whose values include its globals.
func valuesExportTouchesGlobals(export userValuesExport, depPaths [][]string) bool {
	return lo.ContainsBy(append(valuesExportParentPaths(export), export.ChildPath), func(path []string) bool {
		return slices.Contains(path, common.GlobalKey) || lo.ContainsBy(depPaths, func(depPath []string) bool {
			return len(path) <= len(depPath) && slices.Equal(path, depPath[:len(path)])
		})
	})
}

// dependencyPaths returns the paths of the dependencies of the chart, recursively, in the chart values.
func dependencyPaths(c *chart.Chart) [][]string {
	var paths [][]string
	for _, d := range c.Dependencies() {
		paths = append(paths, chartPathFromRoot(d))
		paths = append(paths, dependencyPaths(d)...)
	}

	return paths
}

func valuesExportReadsPath(export userValuesExport, path []string) bool {
	return lo.ContainsBy(valuesExportParentPaths(export), func(parentPath []string) bool {
		return pathsOverlap(path, parentPath)
	})
}

func pathsOverlap(a, b []string) bool {
	n := min(len(a), len(b))

	return slices.Equal(a[:n], b[:n])
}

func valueAtPath(vals map[string]interface{}, path []string) (interface{}, bool) {
	var value interface{} = vals
	for _, key := range path {
		table, ok := value.(map[string]interface{})
		if !ok {
			return nil, false
		}

		if value, ok = table[key]; !ok {
			return nil, false
		}
	}

	return value, true
}

// appendPath returns a copy of path followed by the keys of each non-empty dot-separated subPath.
func appendPath(path []string, subPaths ...string) []string {
	result := slices.Clone(path)
	for _, subPath := range subPaths {
		if subPath != "" {
			result = append(result, parsePath(subPath)...)
		}
	}

	return result
}

// chartPathFromRoot returns the path of the chart values in the root chart values.
func chartPathFromRoot(c *chart.Chart) []string {
	return parsePath(c.ChartPath())[1:]
}

// ExportAndImportValuesNotFoundWarnings returns the warnings about the export-values of the enabled
// chart dependencies whose parent path is set neither in the chart values nor in any of the values
// layers, and about the import-values whose child path has a map neither in the chart values nor
// any value in the values layers, e.g. because of a typo in Chart.yaml. Each layer has the
// export-values and import-values applied separately, so only all of them together tell that a
// path is not set. Call it after ProcessDependenciesWithMergedValues with the values layers both
// before ExportUserValues and after ImportUserValues, and with the chart values coalesced before
// processing, or nil if they are invalid: an export might overwrite the path it reads, and
// an export might read the path that another export writes.
func ExportAndImportValuesNotFoundWarnings(c *chart.Chart, layers []map[string]interface{}, chartValuesBeforeProcessing map[string]interface{}) ([]string, error) {
	exportLayers := append(slices.Clone(layers), chartValuesBeforeProcessing)

	var warnings []string
	for _, export := range collectUserValuesExports(c, nil) {
		found, err := userValuesExportParentFound(exportLayers, export)
		if err != nil {
			return nil, err
		}

		if !found {
			if found, err = chartValuesExportParentFound(export); err != nil {
				return nil, err
			}
		}

		if !found {
			warnings = append(warnings, fmt.Sprintf("Skipping export-values of %q from chart %q to dependency %q: the path is not set in any values", export.Parent, export.Chart.ChartPath(), export.DependencyName))
		}
	}

	importWarnings, err := importValuesNotFoundWarnings(c, layers)
	if err != nil {
		return nil, err
	}

	return append(warnings, importWarnings...), nil
}

func userValuesExportParentFound(layers []map[string]interface{}, export userValuesExport) (bool, error) {
	for _, vals := range layers {
		_, found, err := userValuesExportParentValue(vals, export)
		if err != nil {
			return false, fmt.Errorf("get value %q: %w", joinPath(export.ParentPath...), err)
		}

		if found {
			return true, nil
		}
	}

	return false, nil
}

// chartValuesExportParentFound looks up the parent path in the same chart values as
// applyChartValuesExports.
func chartValuesExportParentFound(export userValuesExport) (bool, error) {
	var (
		pvals common.Values
		err   error
	)
	if isGlobalValuesPath(export.Parent) && export.Chart.Parent() != nil {
		pvals, err = valuesWithRootGlobalsForChart(export.Chart, true)
	} else {
		pvals, err = valuesForChart(export.Chart, true)
	}

	if err != nil {
		return false, fmt.Errorf("get values of chart %q: %w", export.Chart.ChartPath(), err)
	}

	_, found := valueAtPath(pvals, parsePath(export.Parent))

	return found, nil
}

// importValuesNotFoundWarnings looks up the child path in the same chart values as
// processImportValues. It counts a non-map in a values layer as set, because ImportUserValues
// already warns about it.
func importValuesNotFoundWarnings(c *chart.Chart, layers []map[string]interface{}) ([]string, error) {
	var warnings []string
	for _, d := range c.Dependencies() {
		depWarnings, err := importValuesNotFoundWarnings(d, layers)
		if err != nil {
			return nil, err
		}

		warnings = append(warnings, depWarnings...)
	}

	chartPath := chartPathFromRoot(c)

	var cvals common.Values
	for _, r := range c.Metadata.Dependencies {
		for _, importValue := range r.ImportValues {
			child, _, ok := parseImportValues(importValue)
			if !ok {
				continue
			}

			childPath := appendPath(chartPath, r.Name, child)

			if lo.ContainsBy(layers, func(vals map[string]interface{}) bool {
				_, found := valueAtPath(vals, childPath)
				return found
			}) {
				continue
			}

			if cvals == nil {
				var err error
				if cvals, err = valuesForChart(c, true); err != nil {
					return nil, fmt.Errorf("get values of chart %q: %w", c.ChartPath(), err)
				}
			}

			if _, err := cvals.Table(r.Name + "." + child); err == nil {
				continue
			}

			warnings = append(warnings, fmt.Sprintf("Skipping import-values of %q from dependency %q to chart %q: no map is set at the path in any values", child, r.Name, c.ChartPath()))
		}
	}

	return warnings, nil
}
