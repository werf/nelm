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
	"strings"

	chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	"github.com/werf/nelm/v2/pkg/helm/intern/copystructure"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
)

// ProcessDependencies checks through this chart's dependencies, processing accordingly. The service
// values and the custom secret values of the chart override v.
func ProcessDependencies(c *chart.Chart, v common.Values) error {
	extraVals, err := util.ServiceAndCustomSecretValues(c)
	if err != nil {
		return err
	}

	return ProcessDependenciesWithMergedValues(c, common.MergeMaps(v, extraVals))
}

// ProcessDependenciesWithMergedValues processes dependencies like ProcessDependencies, except that v
// must already contain the service values and the custom secret values, and import-values are not
// applied to v (see ImportUserValues).
func ProcessDependenciesWithMergedValues(c *chart.Chart, v common.Values) error {
	if err := processDependencyEnabled(c, v, ""); err != nil {
		return err
	}

	return processDependencyImportValues(c, true)
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
				} else if errors.As(err, &errNoValue) {
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
						slog.String("chart", "chart"),
						slog.String("name", r.Name),
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

// valuesForChart returns the chart values. The service values and the custom secret values are user
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
func ImportUserValues(c *chart.Chart, v map[string]any, opts ImportUserValuesOptions) (map[string]any, []string, error) {
	vals, err := copyUserValues(v)
	if err != nil {
		return nil, nil, err
	}

	deleteUserValuesAtPaths(vals, opts.ReplacedPaths)

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
	deleteUserValuesAtPaths(vals, replacedPaths)

	return vals, skipped, nil
}

func deleteUserValuesAtPaths(vals map[string]any, paths [][]string) {
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

func copyUserValues(v map[string]any) (map[string]any, error) {
	valsCopy, err := copystructure.Copy(v)
	if err != nil {
		return nil, fmt.Errorf("copy user values: %w", err)
	}

	vals, ok := valsCopy.(map[string]any)
	if !ok || vals == nil {
		return map[string]any{}, nil
	}

	return vals, nil
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

func joinNonEmptyPath(path ...string) string {
	var parts []string
	for _, p := range path {
		if p != "" {
			parts = append(parts, p)
		}
	}

	return strings.Join(parts, ".")
}

func stripFirstPathPart(path string) string {
	pathParts := parsePath(path)
	if len(pathParts) <= 1 {
		return ""
	}

	return strings.Join(pathParts[1:], ".")
}
