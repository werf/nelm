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
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/werf/nelm/pkg/helm/intern/copystructure"
	"github.com/werf/nelm/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/pkg/helm/pkg/chart/common/util"
	chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
)

// ProcessDependencies checks through this chart's dependencies, processing accordingly.
func ProcessDependencies(c *chart.Chart, v *map[string]interface{}) error {
	if v == nil {
		return fmt.Errorf("process dependencies: values map pointer is nil")
	}

	if err := processDependencyExportExtraValues(c, v, true); err != nil {
		return fmt.Errorf("process dependency export extra values: %w", err)
	}

	if err := processDependencyEnabled(c, *v, ""); err != nil {
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
				if err == nil {
					// if not bool, warn
					if bv, ok := vv.(bool); ok {
						r.Enabled = bv
						break
					}
					slog.Warn("returned non-bool value", "path", c, "chart", r.Name)
				} else if _, ok := err.(common.ErrNoValue); !ok {
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
func processDependencyEnabled(c *chart.Chart, v map[string]interface{}, path string) error {
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
func pathToMap(path string, data map[string]interface{}) map[string]interface{} {
	if path == "." {
		return data
	}
	return set(parsePath(path), data)
}

func parsePath(key string) []string { return strings.Split(key, ".") }

func set(path []string, data map[string]interface{}) map[string]interface{} {
	if len(path) == 0 {
		return nil
	}
	cur := data
	for i := len(path) - 1; i >= 0; i-- {
		cur = map[string]interface{}{path[i]: cur}
	}
	return cur
}

// processImportValues merges values from child to parent based on the chart's dependencies' ImportValues field.
func processImportValues(c *chart.Chart, merge bool) error {
	if c.Metadata.Dependencies == nil {
		return nil
	}
	// combine chart values and empty config to get Values
	var cvals common.Values
	var err error
	if merge {
		cvals, err = util.MergeValues(c, nil)
	} else {
		cvals, err = util.CoalesceValues(c, nil)
	}
	if err != nil {
		return err
	}
	b := make(map[string]interface{})
	// import values from each dependency if specified in import-values
	for _, r := range c.Metadata.Dependencies {
		var outiv []interface{}
		for _, riv := range r.ImportValues {
			switch iv := riv.(type) {
			case map[string]interface{}:
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

func deepCopyMap(vals map[string]interface{}) map[string]interface{} {
	valsCopy, err := copystructure.Copy(vals)
	if err != nil {
		return vals
	}
	return valsCopy.(map[string]interface{})
}

func trimNilValues(vals map[string]interface{}) map[string]interface{} {
	valsCopy, err := copystructure.Copy(vals)
	if err != nil {
		return vals
	}
	valsCopyMap := valsCopy.(map[string]interface{})
	for key, val := range valsCopyMap {
		if val == nil {
			// Iterate over the values and remove nil keys
			delete(valsCopyMap, key)
		} else if istable(val) {
			// Recursively call into ourselves to remove keys from inner tables
			valsCopyMap[key] = trimNilValues(val.(map[string]interface{}))
		}
	}

	return valsCopyMap
}

// istable is a special-purpose function to see if the present thing matches the definition of a YAML table.
func istable(v interface{}) bool {
	_, ok := v.(map[string]interface{})
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

func processDependencyExportExtraValues(c *chart.Chart, extraVals *map[string]interface{}, merge bool) error {
	if err := processExportExtraValues(c, extraVals, merge); err != nil {
		return fmt.Errorf("process export extra values: %w", err)
	}

	for _, d := range c.Dependencies() {
		if err := processDependencyExportExtraValues(d, extraVals, merge); err != nil {
			return fmt.Errorf("process dependency export extra values recursively: %w", err)
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

func processExportExtraValues(c *chart.Chart, extraVals *map[string]interface{}, merge bool) error {
	if extraVals == nil || c.Parent() == nil || c.Parent().Metadata.Dependencies == nil {
		return nil
	}

	r := dependencyForChart(c)
	if r == nil {
		return nil
	}

	for _, exportValue := range dependencyExportValues(r) {
		parent, child, err := parseExportValues(exportValue)
		if err != nil {
			slog.Warn(
				"invalid ExportValues definition",
				slog.String("chart", c.Parent().Name()),
				slog.String("dependency", r.Name),
				slog.Any("error", err),
			)
			continue
		}

		headlessParentChartPath := stripFirstPathPart(c.Parent().ChartPath())
		exportParentTablePath := parent
		if headlessParentChartPath != "" {
			exportParentTablePath = joinPath(headlessParentChartPath, parent)
		}

		extraParentVals, err := common.Values(*extraVals).Table(exportParentTablePath)
		if err != nil {
			var errNoTable common.ErrNoTable
			if errors.As(err, &errNoTable) {
				continue
			}
			return fmt.Errorf("read extra values table %q: %w", exportParentTablePath, err)
		}

		extraChildValsPath := stripFirstPathPart(c.ChartPath())
		if child != "" {
			extraChildValsPath = joinPath(extraChildValsPath, child)
		}

		var errNoTable common.ErrNoTable
		var errNoValue common.ErrNoValue
		_, errTable := common.Values(*extraVals).Table(extraChildValsPath)
		_, errValue := common.Values(*extraVals).PathValue(extraChildValsPath)
		if !(errors.As(errTable, &errNoTable) && errors.As(errValue, &errNoValue)) {
			continue
		}

		extraChildVals := pathToMap(extraChildValsPath, deepCopyMap(extraParentVals.AsMap()))
		if merge {
			*extraVals = util.MergeTables(extraChildVals, *extraVals)
		} else {
			*extraVals = util.CoalesceTables(extraChildVals, *extraVals)
		}
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
		slog.Warn(
			"ExportValues parent path not found",
			slog.String("chart", parentChartName),
			slog.String("dependency", dependencyName),
			slog.String("path", parent),
			slog.Any("error", err),
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

func valuesForChart(c *chart.Chart, merge bool) (common.Values, error) {
	if merge {
		vals, err := util.MergeValues(c, nil)
		if err != nil {
			return nil, err
		}
		return vals, nil
	}

	vals, err := util.CoalesceValues(c, nil)
	if err != nil {
		return nil, err
	}

	return vals, nil
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
