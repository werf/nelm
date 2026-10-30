/*
Copyright The Helm Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/werf/nelm/v2/pkg/helm/intern/copystructure"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/v2/pkg/log"
)

// ProcessDependencies checks through this chart's dependencies, processing accordingly. The service
// values and the custom secret values of the chart override v.
func ProcessDependencies(c *chart.Chart, v *map[string]interface{}) error {
	if v == nil {
		return fmt.Errorf("process dependencies: values map pointer is nil")
	}

	extraVals, err := util.ServiceAndCustomSecretValues(c)
	if err != nil {
		return err
	}

	var chartValsBeforeProcessing map[string]interface{}
	if chartVals, err := util.CoalesceValues(c, nil); err == nil {
		chartValsBeforeProcessing = chartVals.AsMap()
	}

	exportedVals, exportSkipped, err := ExportUserValues(c, *v, ExportUserValuesOptions{})
	if err != nil {
		return fmt.Errorf("export user values: %w", err)
	}

	exportedExtraVals, extraExportSkipped, err := ExportUserValues(c, extraVals, ExportUserValuesOptions{})
	if err != nil {
		return fmt.Errorf("export service and custom secret values: %w", err)
	}

	if err := ProcessDependenciesWithMergedValues(c, common.MergeMaps(exportedVals, exportedExtraVals)); err != nil {
		return err
	}

	importedVals, importSkipped, err := ImportUserValues(c, exportedVals, ImportUserValuesOptions{})
	if err != nil {
		return fmt.Errorf("import user values: %w", err)
	}

	importedExtraVals, extraImportSkipped, err := ImportUserValues(c, exportedExtraVals, ImportUserValuesOptions{})
	if err != nil {
		return fmt.Errorf("import service and custom secret values: %w", err)
	}

	notFound, err := ExportAndImportValuesNotFoundWarnings(c, []map[string]interface{}{*v, extraVals, importedVals, importedExtraVals}, chartValsBeforeProcessing)
	if err != nil {
		return fmt.Errorf("check export and import values paths: %w", err)
	}

	*v = common.MergeMaps(importedVals, importedExtraVals)

	for _, msg := range lo.Uniq(slices.Concat(exportSkipped, extraExportSkipped, importSkipped, extraImportSkipped, notFound)) {
		log.Default.Warn(context.Background(), "%s", msg)
	}

	return nil
}

type ExportUserValuesOptions struct {
	// ReplacedPaths are deleted from the values before and after each export, so that the values at
	// these paths are neither exported nor consumed by the next exports, e.g. the paths at which a
	// higher values layer replaces the values of this layer.
	ReplacedPaths [][]string
}

// ExportUserValues applies export-values of the chart dependencies to one layer of user-supplied
// values v, and returns the result without modifying v, and the messages about the exports skipped
// because of their values. Within the layer, an exported value, including null, overrides the value
// at the child path. Call it before ProcessDependenciesWithMergedValues, so that dependency
// conditions see the exported values, and after it, so that the values are not exported to the
// disabled dependencies.
func ExportUserValues(c *chart.Chart, v map[string]interface{}, opts ExportUserValuesOptions) (map[string]interface{}, []string, error) {
	vals, err := util.DeepCopyValues(v)
	if err != nil {
		return nil, nil, fmt.Errorf("copy user values: %w", err)
	}

	util.DeleteValuesAtPaths(vals, opts.ReplacedPaths)

	exports := collectUserValuesExports(c, nil)

	var skipped []string
	for _, i := range sortValuesExports(exports) {
		export := exports[i]
		value, found, err := userValuesExportParentValue(vals, export)
		if err != nil {
			return nil, nil, fmt.Errorf("get value %q: %w", joinPath(export.ParentPath), err)
		}

		if !found {
			continue
		}

		if export.ToDependencyRoot && !istable(value) {
			skipped = append(skipped, fmt.Sprintf("Skipping export-values of %q to dependency %q: only a map can be exported to the dependency root", joinPath(export.ParentPath), joinPath(export.ChildPath)))
			continue
		}

		var valueCopy interface{}
		if value != nil {
			if valueCopy, err = copystructure.Copy(value); err != nil {
				return nil, nil, fmt.Errorf("copy value %q: %w", joinPath(export.ParentPath), err)
			}
		}

		vals = util.MergeTables(valueToPathMap(export.ChildPath, valueCopy), vals)
		util.DeleteValuesAtPaths(vals, opts.ReplacedPaths)
	}

	return vals, skipped, nil
}

// ProcessDependenciesWithMergedValues processes dependencies like ProcessDependencies, except that v
// must already contain the exported user values (see ExportUserValues), the service values and the
// custom secret values, and import-values are not applied to v (see ImportUserValues).
func ProcessDependenciesWithMergedValues(c *chart.Chart, v map[string]interface{}) error {
	if err := processDependencyEnabled(c, v, ""); err != nil {
		return fmt.Errorf("process dependency enabled: %w", err)
	}

	return processDependencyImportExportValues(c, true)
}

// processDependencyConditions disables charts based on condition path value in values
func processDependencyConditions(reqs []*chart.Dependency, cvals common.Values, cpath string) {
	if reqs == nil {
		return
	}
	for _, r := range reqs {
		for c := range strings.SplitSeq(strings.TrimSpace(r.Condition), ",") {
			if len(c) > 0 {
				// retrieve value
				vv, err := cvals.PathValue(cpath + c)
				var errNoValue common.ErrNoValue
				if err == nil {
					// if not bool, warn
					if bv, ok := vv.(bool); ok {
						r.Enabled = bv
						break
					}
					slog.Warn("returned non-bool value", "path", c, "chart", r.Name)
				} else if !errors.As(err, &errNoValue) {
					// this is a real error
					slog.Warn("the method PathValue returned error", slog.Any("error", err))
				}
			}
		}
	}
}

// processDependencyTags disables charts based on tags in values
func processDependencyTags(reqs []*chart.Dependency, cvals common.Values) {
	if reqs == nil {
		return
	}
	vt, err := cvals.Table("tags")
	if err != nil {
		return
	}
	for _, r := range reqs {
		var hasTrue, hasFalse bool
		for _, k := range r.Tags {
			if b, ok := vt[k]; ok {
				// if not bool, warn
				if bv, ok := b.(bool); ok {
					if bv {
						hasTrue = true
					} else {
						hasFalse = true
					}
				} else {
					slog.Warn("returned non-bool value", "tag", k, "chart", r.Name)
				}
			}
		}
		if !hasTrue && hasFalse {
			r.Enabled = false
		} else if hasTrue || !hasTrue && !hasFalse {
			r.Enabled = true
		}
	}
}

// getAliasDependency finds the chart for an alias dependency and copies parts that will be modified
func getAliasDependency(charts []*chart.Chart, dep *chart.Dependency) *chart.Chart {
	for _, c := range charts {
		if c == nil {
			continue
		}
		if c.Name() != dep.Name {
			continue
		}
		if !IsCompatibleRange(dep.Version, c.Metadata.Version) {
			continue
		}

		out := *c
		out.Metadata = copyMetadata(c.Metadata)

		// empty dependencies and shallow copy all dependencies, otherwise parent info may be corrupted if
		// there is more than one dependency aliasing this chart
		out.SetDependencies()
		for _, dependency := range c.Dependencies() {
			cpy := *dependency
			out.AddDependency(&cpy)
		}

		if dep.Alias != "" {
			out.Metadata.Name = dep.Alias
		}
		return &out
	}
	return nil
}

func copyMetadata(metadata *chart.Metadata) *chart.Metadata {
	md := *metadata

	if md.Dependencies != nil {
		dependencies := make([]*chart.Dependency, len(md.Dependencies))
		for i := range md.Dependencies {
			dependency := *md.Dependencies[i]
			dependencies[i] = &dependency
		}
		md.Dependencies = dependencies
	}
	return &md
}

// processDependencyEnabled removes disabled charts from dependencies
func processDependencyEnabled(c *chart.Chart, v map[string]any, path string) error {
	if c.Metadata.Dependencies == nil {
		return nil
	}

	var chartDependencies []*chart.Chart
	// If any dependency is not a part of Chart.yaml
	// then this should be added to chartDependencies.
	// However, if the dependency is already specified in Chart.yaml
	// we should not add it, as it would be processed from Chart.yaml anyway.

Loop:
	for _, existing := range c.Dependencies() {
		for _, req := range c.Metadata.Dependencies {
			if existing.Name() == req.Name && IsCompatibleRange(req.Version, existing.Metadata.Version) {
				continue Loop
			}
		}
		chartDependencies = append(chartDependencies, existing)
	}

	for _, req := range c.Metadata.Dependencies {
		if req == nil {
			continue
		}
		if chartDependency := getAliasDependency(c.Dependencies(), req); chartDependency != nil {
			chartDependencies = append(chartDependencies, chartDependency)
		}
		if req.Alias != "" {
			req.Name = req.Alias
		}
	}
	c.SetDependencies(chartDependencies...)

	// set all to true
	for _, lr := range c.Metadata.Dependencies {
		lr.Enabled = true
	}
	cvals, err := util.CoalesceValues(c, v)
	if err != nil {
		return err
	}
	// flag dependencies as enabled/disabled
	processDependencyTags(c.Metadata.Dependencies, cvals)
	processDependencyConditions(c.Metadata.Dependencies, cvals, path)
	// make a map of charts to remove
	rm := map[string]struct{}{}
	for _, r := range c.Metadata.Dependencies {
		if !r.Enabled {
			// remove disabled chart
			rm[r.Name] = struct{}{}
		}
	}
	// don't keep disabled charts in new slice
	cd := []*chart.Chart{}
	copy(cd, c.Dependencies()[:0])
	for _, n := range c.Dependencies() {
		if _, ok := rm[n.Metadata.Name]; !ok {
			cd = append(cd, n)
		}
	}
	// don't keep disabled charts in metadata
	cdMetadata := []*chart.Dependency{}
	copy(cdMetadata, c.Metadata.Dependencies[:0])
	for _, n := range c.Metadata.Dependencies {
		if _, ok := rm[n.Name]; !ok {
			cdMetadata = append(cdMetadata, n)
		}
	}

	// recursively call self to process sub dependencies
	for _, t := range cd {
		subpath := path + t.Metadata.Name + "."
		if err := processDependencyEnabled(t, cvals, subpath); err != nil {
			return err
		}
	}
	// set the correct dependencies in metadata
	c.Metadata.Dependencies = nil
	c.Metadata.Dependencies = append(c.Metadata.Dependencies, cdMetadata...)
	c.SetDependencies(cd...)

	return nil
}

// pathToMap creates a nested map given a YAML path in dot notation.
func pathToMap(path string, data map[string]any) map[string]any {
	if path == "." {
		return data
	}
	return set(parsePath(path), data)
}

func parsePath(key string) []string { return strings.Split(key, ".") }

func set(path []string, data map[string]any) map[string]any {
	if len(path) == 0 {
		return nil
	}
	cur := data
	for i := len(path) - 1; i >= 0; i-- {
		cur = map[string]any{path[i]: cur}
	}
	return cur
}

// processImportValues merges values from child to parent based on the chart's dependencies' ImportValues field.
func processImportValues(c *chart.Chart, merge bool) error {
	if c.Metadata.Dependencies == nil {
		return nil
	}
	cvals, err := valuesForChart(c, merge)
	if err != nil {
		return err
	}
	b := make(map[string]any)
	// import values from each dependency if specified in import-values
	for _, r := range c.Metadata.Dependencies {
		var outiv []any
		for _, riv := range r.ImportValues {
			switch iv := riv.(type) {
			case map[string]any:
				child := fmt.Sprintf("%v", iv["child"])
				parent := fmt.Sprintf("%v", iv["parent"])

				outiv = append(outiv, map[string]string{
					"child":  child,
					"parent": parent,
				})

				// The table may be set only in user-supplied or service values, imported by
				// ImportUserValues, see ExportAndImportValuesNotFoundWarnings.
				vv, err := cvals.Table(r.Name + "." + child)
				if err != nil {
					continue
				}
				// create value map from child to be merged into parent
				if merge {
					b = util.MergeTables(b, pathToMap(parent, vv.AsMap()))
				} else {
					b = util.CoalesceTables(b, pathToMap(parent, vv.AsMap()))
				}
			case string:
				child := "exports." + iv
				outiv = append(outiv, map[string]string{
					"child":  child,
					"parent": ".",
				})
				vm, err := cvals.Table(r.Name + "." + child)
				if err != nil {
					continue
				}
				if merge {
					b = util.MergeTables(b, vm.AsMap())
				} else {
					b = util.CoalesceTables(b, vm.AsMap())
				}
			}
		}
		r.ImportValues = outiv
	}

	// Imported values from a child to a parent chart have a lower priority than
	// the parents values. This enables parent charts to import a large section
	// from a child and then override select parts. This is why b is merged into
	// cvals in the code below and not the other way around.
	if merge {
		// deep copying the cvals as there are cases where pointers can end
		// up in the cvals when they are copied onto b in ways that break things.
		cvals = deepCopyMap(cvals)
		c.Values = util.MergeTables(cvals, b)
	} else {
		// Trimming the nil values from cvals is needed for backwards compatibility.
		// Previously, the b value had been populated with cvals along with some
		// overrides. This caused the coalescing functionality to remove the
		// nil/null values. This trimming is for backwards compat.
		cvals = trimNilValues(cvals)
		c.Values = util.CoalesceTables(cvals, b)
	}

	return nil
}

func deepCopyMap(vals map[string]any) map[string]any {
	valsCopy, err := copystructure.Copy(vals)
	if err != nil {
		return vals
	}
	return valsCopy.(map[string]any)
}

func trimNilValues(vals map[string]any) map[string]any {
	valsCopy, err := copystructure.Copy(vals)
	if err != nil {
		return vals
	}
	valsCopyMap := valsCopy.(map[string]any)
	for key, val := range valsCopyMap {
		if val == nil {
			// Iterate over the values and remove nil keys
			delete(valsCopyMap, key)
		} else if istable(val) {
			// Recursively call into ourselves to remove keys from inner tables
			valsCopyMap[key] = trimNilValues(val.(map[string]any))
		}
	}

	return valsCopyMap
}

// istable is a special-purpose function to see if the present thing matches the definition of a YAML table.
func istable(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

// processDependencyImportValues imports specified chart values from child to parent.
func processDependencyImportValues(c *chart.Chart, merge bool) error {
	for _, d := range c.Dependencies() {
		// recurse
		if err := processDependencyImportValues(d, merge); err != nil {
			return err
		}
	}
	return processImportValues(c, merge)
}

func processDependencyImportExportValues(c *chart.Chart, merge bool) error {
	if err := processDependencyExportValues(c, merge); err != nil {
		return fmt.Errorf("process dependency export values: %w", err)
	}

	if err := processDependencyImportValues(c, merge); err != nil {
		return fmt.Errorf("process dependency import values: %w", err)
	}

	return nil
}

type chartValuesExport struct {
	Chart          *chart.Chart
	DependencyPath []string
	Parent         string
	Child          string
	Paths          userValuesExport
	// TouchesGlobals is set by batchChartValuesExports.
	TouchesGlobals bool
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

	exports = slices.Clone(exports)
	for i := range exports {
		exports[i].TouchesGlobals = valuesExportTouchesGlobals(exports[i].Paths, depPaths)
	}

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
				return moveBatches && chartValuesExportsIndependent(batchExport, export)
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

func valueMapFromExport(parentChartName, dependencyName string, pvals common.Values, parent, child string) (map[string]interface{}, bool) {
	vm, err := pvals.Table(parent)
	if err == nil {
		if child == "" {
			return vm.AsMap(), true
		}
		return pathToMap(child, vm.AsMap()), true
	}

	value, valueErr := pvals.PathValue(parent)
	if valueErr != nil {
		// Not a warning: the parent path may be set only in user-supplied or service values, which
		// are exported by ExportUserValues.
		slog.Debug(
			"ExportValues parent path not found in chart values",
			slog.String("chart", parentChartName),
			slog.String("dependency", dependencyName),
			slog.String("path", parent),
			slog.Any("error", valueErr),
		)
		return nil, false
	}

	childSlice := parsePath(child)
	if len(childSlice) == 1 && childSlice[0] == "" {
		slog.Warn(
			"ExportValues cannot map primitive to root",
			slog.String("chart", parentChartName),
			slog.String("dependency", dependencyName),
		)
		return nil, false
	}

	childPath := joinPath(childSlice[:len(childSlice)-1])
	childMap := map[string]interface{}{childSlice[len(childSlice)-1]: value}
	if childPath == "" {
		return childMap, true
	}

	return pathToMap(childPath, childMap), true
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

	vals, err := rootVals.Table(joinPath(chartPathFromRoot(c)))
	if err != nil {
		return nil, fmt.Errorf("get chart %q values from root chart values: %w", c.ChartPath(), err)
	}

	return vals, nil
}

func parseExportValues(rev interface{}) (string, string, error) {
	var parent string
	var child string

	switch ev := rev.(type) {
	case map[string]string:
		return parseExportValues(map[string]interface{}{"parent": ev["parent"], "child": ev["child"]})
	case map[string]interface{}:
		parentValue, ok := ev["parent"].(string)
		if !ok {
			return "", "", fmt.Errorf("parent must be a string")
		}

		childValue, ok := ev["child"].(string)
		if !ok {
			return "", "", fmt.Errorf("child must be a string")
		}

		parent = strings.TrimSpace(parentValue)
		child = strings.TrimSpace(childValue)
		if parent == "" || parent == "." {
			return "", "", fmt.Errorf("parent %q is not allowed", parentValue)
		}
		if child == "." {
			child = ""
		}
	case string:
		s := strings.TrimSpace(ev)
		switch s {
		case "", ".":
			parent = "exports"
		default:
			parent = "exports." + s
		}
		child = ""
	default:
		return "", "", fmt.Errorf("invalid format of ExportValues")
	}

	return parent, child, nil
}

func syncChartOverridesToParentsValues(c *chart.Chart, overrides map[string]interface{}, merge bool) error {
	if c.Parent() == nil {
		return nil
	}

	pvals, err := valuesForChart(c.Parent(), merge)
	if err != nil {
		return fmt.Errorf("get parent values: %w", err)
	}

	parentOverrides := pathToMap(c.Name(), deepCopyMap(overrides))
	if merge {
		c.Parent().Values = util.MergeTables(deepCopyMap(parentOverrides), deepCopyMap(pvals))
	} else {
		c.Parent().Values = util.CoalesceTables(deepCopyMap(parentOverrides), deepCopyMap(pvals))
	}

	if err := syncChartOverridesToParentsValues(c.Parent(), parentOverrides, merge); err != nil {
		return fmt.Errorf("sync chart overrides recursively: %w", err)
	}

	return nil
}

// valuesForChart returns the chart values. The service values and the custom secret values are user
// values, exported and imported by ExportUserValues and ImportUserValues, so they are used only if
// the chart values are invalid without them, e.g. when they replace a scalar in values.yaml with a map
// for a subchart.
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

func mergeOrCoalesceValues(c *chart.Chart, vals map[string]any, merge bool) (common.Values, error) {
	if merge {
		return util.MergeValues(c, vals)
	}

	return util.CoalesceValues(c, vals)
}

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
func ImportUserValues(c *chart.Chart, v map[string]interface{}, opts ImportUserValuesOptions) (map[string]interface{}, []string, error) {
	vals, err := util.DeepCopyValues(v)
	if err != nil {
		return nil, nil, fmt.Errorf("copy user values: %w", err)
	}

	util.DeleteValuesAtPaths(vals, opts.ReplacedPaths)

	return importUserValues(c, vals, opts.ReplacedPaths)
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
			return false, fmt.Errorf("get value %q: %w", joinPath(export.ParentPath), err)
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

type userValuesExport struct {
	// Chart, DependencyName and Parent are the export-values definition in Chart.yaml, set only by
	// collectUserValuesExports.
	Chart          *chart.Chart
	ChildPath      []string
	DependencyName string
	// GlobalPaths are set for an export of a global value from a subchart: the paths of the globals of
	// the root chart and of each chart down to the subchart, from the highest precedence. ParentPath is
	// then the path in these globals merged like in coalesceGlobals.
	GlobalPaths      [][]string
	Parent           string
	ParentPath       []string
	ToDependencyRoot bool
}

// collectUserValuesExports returns the export-values of the chart dependencies, recursively, with
// the paths from the root chart, parents before their dependencies.
func collectUserValuesExports(c *chart.Chart, chartPath []string) []userValuesExport {
	var exports []userValuesExport

	for _, existing := range c.Dependencies() {
		if chartMatchesDependency(existing, c.Metadata.Dependencies) {
			continue
		}

		exports = append(exports, collectUserValuesExports(existing, appendPath(chartPath, existing.Name()))...)
	}

	for _, req := range c.Metadata.Dependencies {
		if req == nil {
			continue
		}

		dep := getAliasDependency(c.Dependencies(), req)
		inRange := dep != nil
		// processDependencyEnabled keeps a chart that matches no dependency by name and version range,
		// and prepareChartValuesExports collects the export-values of the non-alias dependency with its
		// name for it, so they are applied here too. Such a chart is recursed into in the loop above.
		if !inRange && req.Alias == "" {
			dep, _ = lo.Find(c.Dependencies(), func(d *chart.Chart) bool {
				return d.Name() == req.Name && !chartMatchesDependency(d, c.Metadata.Dependencies)
			})
		}

		if dep == nil {
			continue
		}

		depPath := appendPath(chartPath, dep.Name())

		for _, exportValue := range dependencyExportValues(req) {
			parent, child, err := parseExportValues(exportValue)
			if err != nil {
				continue
			}

			export := newUserValuesExport(chartPath, depPath, parent, child)
			export.Chart = c
			export.DependencyName = dep.Name()
			export.Parent = parent
			exports = append(exports, export)
		}

		if inRange {
			exports = append(exports, collectUserValuesExports(dep, depPath)...)
		}
	}

	return exports
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

func userValuesGlobalPaths(chartPath []string) [][]string {
	paths := [][]string{{common.GlobalKey}}
	for i := range chartPath {
		paths = append(paths, slices.Concat(chartPath[:i+1], []string{common.GlobalKey}))
	}

	return paths
}

func chartMatchesDependency(c *chart.Chart, reqs []*chart.Dependency) bool {
	return lo.ContainsBy(reqs, func(r *chart.Dependency) bool {
		return r != nil && r.Name == c.Name() && IsCompatibleRange(r.Version, c.Metadata.Version)
	})
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
func chartValuesExportsIndependent(a, b chartValuesExport) bool {
	return !(a.TouchesGlobals && b.TouchesGlobals) &&
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

// valuesExportParentPaths returns the paths from the root chart that the export reads.
func valuesExportParentPaths(export userValuesExport) [][]string {
	parentPaths := [][]string{export.ParentPath}
	for _, globalPath := range export.GlobalPaths {
		parentPaths = append(parentPaths, slices.Concat(globalPath, export.ParentPath[1:]))
	}

	return parentPaths
}

func pathsOverlap(a, b []string) bool {
	n := min(len(a), len(b))

	return slices.Equal(a[:n], b[:n])
}

func importUserValues(c *chart.Chart, vals map[string]interface{}, replacedPaths [][]string) (map[string]interface{}, []string, error) {
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

	chartPath := chartPathFromRoot(c)

	imported := map[string]interface{}{}
	for _, r := range c.Metadata.Dependencies {
		for _, importValue := range r.ImportValues {
			child, parent, ok := parseImportValues(importValue)
			if !ok {
				continue
			}

			childPath := appendPath(chartPath, r.Name, child)

			value, found := valueAtPath(vals, childPath)
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
				skipped = append(skipped, fmt.Sprintf("Skipping import-values of %q: only a map can be imported", joinPath(childPath)))
				continue
			}

			valueCopy, err := copystructure.Copy(value)
			if err != nil {
				return nil, nil, fmt.Errorf("copy value %q: %w", joinPath(childPath), err)
			}

			imported = util.MergeTables(imported, pathToMap(parent, valueCopy.(map[string]interface{})))
		}
	}

	if len(imported) == 0 {
		return vals, skipped, nil
	}

	if len(chartPath) > 0 {
		imported = valueToPathMap(chartPath, imported)
	}

	vals = util.MergeTables(vals, imported)
	util.DeleteValuesAtPaths(vals, replacedPaths)

	return vals, skipped, nil
}

// parseImportValues returns the child and parent paths of an import-values entry, in the original
// form or normalized by processImportValues.
func parseImportValues(importValue interface{}) (string, string, bool) {
	switch iv := importValue.(type) {
	case map[string]interface{}:
		return fmt.Sprintf("%v", iv["child"]), fmt.Sprintf("%v", iv["parent"]), true
	case map[string]string:
		return iv["child"], iv["parent"], true
	case string:
		return "exports." + iv, ".", true
	default:
		return "", "", false
	}
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
			return nil, false, fmt.Errorf("copy value %q: %w", joinPath(path), err)
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

func valueToPathMap(path []string, value interface{}) map[string]interface{} {
	result := map[string]interface{}{path[len(path)-1]: value}
	for i := len(path) - 2; i >= 0; i-- {
		result = map[string]interface{}{path[i]: result}
	}

	return result
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

func dependencyForChart(c *chart.Chart) *chart.Dependency {
	if c.Parent() == nil || c.Parent().Metadata == nil {
		return nil
	}

	for _, r := range c.Parent().Metadata.Dependencies {
		if r != nil && r.Name == c.Name() {
			return r
		}
	}

	return nil
}

func dependencyExportValues(r *chart.Dependency) []interface{} {
	if r == nil {
		return nil
	}

	rv := reflect.ValueOf(r)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil
	}

	elem := rv.Elem()
	if !elem.IsValid() {
		return nil
	}

	field := elem.FieldByName("ExportValues")
	if !field.IsValid() || field.Kind() != reflect.Slice {
		return nil
	}

	result := make([]interface{}, 0, field.Len())
	for i := range field.Len() {
		result = append(result, field.Index(i).Interface())
	}

	return result
}

func setDependencyExportValues(r *chart.Dependency, values []interface{}) {
	if r == nil {
		return
	}

	rv := reflect.ValueOf(r)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return
	}

	elem := rv.Elem()
	if !elem.IsValid() {
		return
	}

	field := elem.FieldByName("ExportValues")
	if !field.IsValid() || !field.CanSet() || field.Kind() != reflect.Slice {
		return
	}

	field.Set(reflect.ValueOf(values))
}

// chartPathFromRoot returns the path of the chart values in the root chart values.
func chartPathFromRoot(c *chart.Chart) []string {
	return parsePath(c.ChartPath())[1:]
}

func joinPath(path []string) string {
	return strings.Join(path, ".")
}
