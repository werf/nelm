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

	*v = common.MergeMaps(importedVals, importedExtraVals)

	for _, msg := range lo.Uniq(slices.Concat(exportSkipped, extraExportSkipped, importSkipped, extraImportSkipped)) {
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
	vals, err := copyUserValues(v)
	if err != nil {
		return nil, nil, err
	}

	deleteUserValuesAtPaths(vals, opts.ReplacedPaths)

	var skipped []string
	for _, export := range sortUserValuesExports(collectUserValuesExports(c, "")) {
		value, found := userValueAtPath(vals, export.ParentPath)
		if !found {
			continue
		}

		if export.ToDependencyRoot && !istable(value) {
			skipped = append(skipped, fmt.Sprintf("Skipping export-values of %q to dependency %q: only a map can be exported to the dependency root", export.ParentPath, export.ChildPath))
			continue
		}

		var valueCopy interface{}
		if value != nil {
			if valueCopy, err = copystructure.Copy(value); err != nil {
				return nil, nil, fmt.Errorf("copy value %q: %w", export.ParentPath, err)
			}
		}

		vals = util.MergeTables(valueToPathMap(parsePath(export.ChildPath), valueCopy), vals)
		deleteUserValuesAtPaths(vals, opts.ReplacedPaths)
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

				// get child table
				vv, err := cvals.Table(r.Name + "." + child)
				if err != nil {
					slog.Warn(
						"ImportValues missing table from chart",
						slog.String("chart", r.Name),
						slog.Any("error", err),
					)
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
					slog.Warn("ImportValues missing table", slog.Any("error", err))
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

func processDependencyExportValues(c *chart.Chart, merge bool) error {
	if err := processExportValues(c, merge); err != nil {
		return fmt.Errorf("process export values: %w", err)
	}

	for _, d := range c.Dependencies() {
		if err := processDependencyExportValues(d, merge); err != nil {
			return fmt.Errorf("process dependency export values recursively: %w", err)
		}
	}

	return nil
}

func processExportValues(c *chart.Chart, merge bool) error {
	if c.Parent() == nil || c.Parent().Metadata.Dependencies == nil {
		return nil
	}

	r := dependencyForChart(c)
	if r == nil {
		return nil
	}

	pvals, err := valuesForChart(c.Parent(), merge)
	if err != nil {
		return fmt.Errorf("get parent values: %w", err)
	}

	cvals, err := valuesForChart(c, merge)
	if err != nil {
		return fmt.Errorf("get chart values: %w", err)
	}

	exportedValues, err := getExportedValues(c.Parent().Name(), r, pvals, merge)
	if err != nil {
		return fmt.Errorf("get exported values: %w", err)
	}

	if merge {
		c.Values = util.MergeTables(deepCopyMap(exportedValues), deepCopyMap(cvals))
	} else {
		c.Values = util.CoalesceTables(deepCopyMap(exportedValues), deepCopyMap(cvals))
	}

	if err := syncChartOverridesToParentsValues(c, deepCopyMap(exportedValues), merge); err != nil {
		return fmt.Errorf("sync chart overrides to parent values: %w", err)
	}

	return nil
}

func getExportedValues(parentName string, r *chart.Dependency, pvals common.Values, merge bool) (map[string]interface{}, error) {
	b := make(map[string]interface{})
	var normalizedExportValues []interface{}

	for _, rev := range dependencyExportValues(r) {
		parent, child, err := parseExportValues(rev)
		if err != nil {
			slog.Warn(
				"invalid ExportValues definition",
				slog.String("chart", parentName),
				slog.String("dependency", r.Name),
				slog.Any("error", err),
			)
			continue
		}

		normalizedExportValues = append(normalizedExportValues, map[string]string{
			"parent": parent,
			"child":  child,
		})

		childValMap, ok := valueMapFromExport(parentName, r.Name, pvals, parent, child)
		if !ok {
			continue
		}

		if merge {
			b = util.MergeTables(deepCopyMap(childValMap), b)
		} else {
			b = util.CoalesceTables(deepCopyMap(childValMap), b)
		}
	}

	setDependencyExportValues(r, normalizedExportValues)

	return b, nil
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

	childPath := joinPath(childSlice[:len(childSlice)-1]...)
	childMap := map[string]interface{}{childSlice[len(childSlice)-1]: value}
	if childPath == "" {
		return childMap, true
	}

	return pathToMap(childPath, childMap), true
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
	vals, err := copyUserValues(v)
	if err != nil {
		return nil, nil, err
	}

	deleteUserValuesAtPaths(vals, opts.ReplacedPaths)

	return importUserValues(c, vals, opts.ReplacedPaths)
}

type userValuesExport struct {
	ChildPath        string
	ParentPath       string
	ToDependencyRoot bool
}

// collectUserValuesExports returns the export-values of the chart dependencies, recursively, with
// the paths from the root chart, parents before their dependencies.
func collectUserValuesExports(c *chart.Chart, chartPath string) []userValuesExport {
	var exports []userValuesExport

	for _, existing := range c.Dependencies() {
		if chartMatchesDependency(existing, c.Metadata.Dependencies) {
			continue
		}

		exports = append(exports, collectUserValuesExports(existing, joinNonEmptyPath(chartPath, existing.Name()))...)
	}

	for _, req := range c.Metadata.Dependencies {
		if req == nil {
			continue
		}

		dep := getAliasDependency(c.Dependencies(), req)
		inRange := dep != nil
		// processDependencyEnabled keeps a chart that matches no dependency by name and version range,
		// and processExportValues applies the export-values of the non-alias dependency with its name to
		// it, so they are applied here too. Such a chart is recursed into in the loop above.
		if !inRange && req.Alias == "" {
			dep, _ = lo.Find(c.Dependencies(), func(d *chart.Chart) bool {
				return d.Name() == req.Name && !chartMatchesDependency(d, c.Metadata.Dependencies)
			})
		}

		if dep == nil {
			continue
		}

		depPath := joinNonEmptyPath(chartPath, dep.Name())

		for _, exportValue := range dependencyExportValues(req) {
			parent, child, err := parseExportValues(exportValue)
			if err != nil {
				continue
			}

			exports = append(exports, userValuesExport{
				ChildPath:        joinNonEmptyPath(depPath, child),
				ParentPath:       joinNonEmptyPath(chartPath, parent),
				ToDependencyRoot: child == "",
			})
		}

		if inRange {
			exports = append(exports, collectUserValuesExports(dep, depPath)...)
		}
	}

	return exports
}

func chartMatchesDependency(c *chart.Chart, reqs []*chart.Dependency) bool {
	return lo.ContainsBy(reqs, func(r *chart.Dependency) bool {
		return r != nil && r.Name == c.Name() && IsCompatibleRange(r.Version, c.Metadata.Version)
	})
}

// sortUserValuesExports orders the exports so that each export comes after the exports that write
// to its parent path, e.g. when its parent path is the child path of an export to a sibling
// dependency listed after it, and after the exports listed before it that write to its child path.
// Otherwise the order is kept. A cycle is broken at its first listed export.
func sortUserValuesExports(exports []userValuesExport) []userValuesExport {
	sorted := make([]userValuesExport, 0, len(exports))
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

			if !exportWaitsForOtherExports(exports, done, i) {
				next = i
				break
			}
		}

		done[next] = true
		sorted = append(sorted, exports[next])
	}

	return sorted
}

func exportWaitsForOtherExports(exports []userValuesExport, done []bool, i int) bool {
	for j := range exports {
		if done[j] || j == i {
			continue
		}

		if pathsOverlap(exports[j].ChildPath, exports[i].ParentPath) || (j < i && pathsOverlap(exports[j].ChildPath, exports[i].ChildPath)) {
			return true
		}
	}

	return false
}

func pathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+".") || strings.HasPrefix(b, a+".")
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

	chartPath := stripFirstPathPart(c.ChartPath())

	imported := map[string]interface{}{}
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

			imported = util.MergeTables(imported, pathToMap(parent, valueCopy.(map[string]interface{})))
		}
	}

	if len(imported) == 0 {
		return vals, skipped, nil
	}

	if chartPath != "" {
		imported = valueToPathMap(parsePath(chartPath), imported)
	}

	vals = util.MergeTables(vals, imported)
	deleteUserValuesAtPaths(vals, replacedPaths)

	return vals, skipped, nil
}

func deleteUserValuesAtPaths(vals map[string]interface{}, paths [][]string) {
	for _, path := range paths {
		table := vals
		for _, key := range path[:len(path)-1] {
			if table, _ = table[key].(map[string]interface{}); table == nil {
				break
			}
		}

		if table != nil {
			delete(table, path[len(path)-1])
		}
	}
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

func copyUserValues(v map[string]interface{}) (map[string]interface{}, error) {
	valsCopy, err := copystructure.Copy(v)
	if err != nil {
		return nil, fmt.Errorf("copy user values: %w", err)
	}

	vals, ok := valsCopy.(map[string]interface{})
	if !ok || vals == nil {
		return map[string]interface{}{}, nil
	}

	return vals, nil
}

func userValueAtPath(vals map[string]interface{}, path string) (interface{}, bool) {
	var value interface{} = vals
	for _, key := range parsePath(path) {
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

func valueToPathMap(path []string, value interface{}) map[string]interface{} {
	result := map[string]interface{}{path[len(path)-1]: value}
	for i := len(path) - 2; i >= 0; i-- {
		result = map[string]interface{}{path[i]: result}
	}

	return result
}

func joinNonEmptyPath(path ...string) string {
	var parts []string
	for _, p := range path {
		if p != "" {
			parts = append(parts, p)
		}
	}

	return joinPath(parts...)
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

func stripFirstPathPart(path string) string {
	pathParts := parsePath(path)
	if len(pathParts) <= 1 {
		return ""
	}

	return joinPath(pathParts[1:]...)
}

func joinPath(path ...string) string {
	return strings.Join(path, ".")
}
