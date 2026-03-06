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

package chartutil

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/mitchellh/copystructure"
	"github.com/werf/3p-helm/pkg/chart"
)

// ProcessDependencies checks through this chart's dependencies, processing accordingly.
//
// TODO: For Helm v4 this can be combined with or turned into ProcessDependenciesWithMerge
func ProcessDependencies(c *chart.Chart, v *map[string]interface{}) error {
	if err := processDependencyExportExtraValues(c, v, false); err != nil {
		return err
	}

	if err := processDependencyEnabled(c, *v, ""); err != nil {
		return err
	}

	return processDependencyImportExportValues(c, false)
}

// ProcessDependenciesWithMerge checks through this chart's dependencies, processing accordingly.
// It is similar to ProcessDependencies but it does not remove nil values during
// the import/export handling process.
func ProcessDependenciesWithMerge(c *chart.Chart, v *map[string]interface{}) error {
	if err := processDependencyExportExtraValues(c, v, true); err != nil {
		return err
	}

	if err := processDependencyEnabled(c, *v, ""); err != nil {
		return err
	}

	return processDependencyImportExportValues(c, true)
}

// processDependencyConditions disables charts based on condition path value in values
func processDependencyConditions(reqs []*chart.Dependency, cvals Values, cpath string) {
	if reqs == nil {
		return
	}
	for _, r := range reqs {
		for _, c := range strings.Split(strings.TrimSpace(r.Condition), ",") {
			if len(c) > 0 {
				// retrieve value
				vv, err := cvals.PathValue(cpath + c)
				if err == nil {
					// if not bool, warn
					if bv, ok := vv.(bool); ok {
						r.Enabled = bv
						break
					}
					log.Printf("Warning: Condition path '%s' for chart %s returned non-bool value", c, r.Name)
				} else if _, ok := err.(ErrNoValue); !ok {
					// this is a real error
					log.Printf("Warning: PathValue returned error %v", err)
				}
			}
		}
	}
}

// processDependencyTags disables charts based on tags in values
func processDependencyTags(reqs []*chart.Dependency, cvals Values) {
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
					log.Printf("Warning: Tag '%s' for chart %s returned non-bool value", k, r.Name)
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
		md := *c.Metadata
		out.Metadata = &md

		if dep.Alias != "" {
			md.Name = dep.Alias
		}
		return &out
	}
	return nil
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
	// we should not add it, as it would be anyways processed from Chart.yaml

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
	cvals, err := CoalesceValues(c, v)
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
	var cvals Values
	var err error
	if merge {
		cvals, err = MergeValues(c, nil)
	} else {
		cvals, err = CoalesceValues(c, nil)
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
				child := iv["child"].(string)
				parent := iv["parent"].(string)

				outiv = append(outiv, map[string]string{
					"child":  child,
					"parent": parent,
				})

				// get child table
				vv, err := cvals.Table(r.Name + "." + child)
				if err != nil {
					log.Printf("Warning: ImportValues missing table from chart %s: %v", r.Name, err)
					continue
				}
				// create value map from child to be merged into parent
				if merge {
					b = MergeTables(b, pathToMap(parent, vv.AsMap()))
				} else {
					b = CoalesceTables(b, pathToMap(parent, vv.AsMap()))
				}
			case string:
				child := "exports." + iv
				outiv = append(outiv, map[string]string{
					"child":  child,
					"parent": ".",
				})
				vm, err := cvals.Table(r.Name + "." + child)
				if err != nil {
					log.Printf("Warning: ImportValues missing table: %v", err)
					continue
				}
				if merge {
					b = MergeTables(b, vm.AsMap())
				} else {
					b = CoalesceTables(b, vm.AsMap())
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
		c.Values = MergeTables(cvals, b)
	} else {
		// Trimming the nil values from cvals is needed for backwards compatibility.
		// Previously, the b value had been populated with cvals along with some
		// overrides. This caused the coalescing functionality to remove the
		// nil/null values. This trimming is for backwards compat.
		cvals = trimNilValues(cvals)
		c.Values = CoalesceTables(cvals, b)
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

// Extend Chart Values according to export-values directive of its parent Chart.
func processExportValues(c *chart.Chart, merge bool) error {
	if c.Parent() == nil || c.Parent().Metadata.Dependencies == nil {
		return nil
	}

	// Get current chart as chart.Dependency object.
	var cr *chart.Dependency
	for _, r := range c.Parent().Metadata.Dependencies {
		if r.Name == c.Name() {
			cr = r
			break
		}
	}

	if cr == nil {
		return nil
	}

	// Get parent chart values.
	var pvals Values
	var err error
	if merge {
		pvals, err = MergeValues(c.Parent(), nil)
		if err != nil {
			return err
		}
	} else {
		pvals, err = CoalesceValues(c.Parent(), nil)
		if err != nil {
			return err
		}
	}

	// Get current chart values.
	var cvals Values
	if merge {
		cvals, err = MergeValues(c, nil)
		if err != nil {
			return err
		}
	} else {
		cvals, err = CoalesceValues(c, nil)
		if err != nil {
			return err
		}
	}

	// Generate Values map to be merged into current chart, according to export-values directive.
	exportedValues, err := getExportedValues(c.Parent().Name(), cr, pvals, merge)
	if err != nil {
		return err
	}

	cv, err := copystructure.Copy(cvals)
	if err != nil {
		return err
	}

	ev, err := copystructure.Copy(exportedValues)
	if err != nil {
		return err
	}

	// Merge newly generated extra Values map into current chart Values.
	if merge {
		c.Values = MergeTables(ev.(map[string]interface{}), cv.(Values))
	} else {
		c.Values = CoalesceTables(ev.(map[string]interface{}), cv.(Values))
	}

	evForSync, err := copystructure.Copy(exportedValues)
	if err != nil {
		return err
	}

	// Make sure no parent chart will override our new extra Values in this chart.
	if err := syncChartOverridesToParentsValues(c, evForSync.(map[string]interface{}), merge); err != nil {
		return err
	}

	return nil
}

// Get Values map with overrides destined for current chart and merge these overrides into all its parent charts
// Values, while prefixing the to be applied parent overrides with the relative path to the current chart. This is
// to avoid values from parent charts having precedence to the overrides passed to the current chart.
func syncChartOverridesToParentsValues(c *chart.Chart, overrides map[string]interface{}, merge bool) error {
	if c.Parent() == nil {
		return nil
	}

	// Get parent chart values.
	var pvals Values
	var err error
	if merge {
		pvals, err = MergeValues(c.Parent(), nil)
		if err != nil {
			return err
		}
	} else {
		pvals, err = CoalesceValues(c.Parent(), nil)
		if err != nil {
			return err
		}
	}

	pv, err := copystructure.Copy(pvals)
	if err != nil {
		return err
	}

	o, err := copystructure.Copy(overrides)
	if err != nil {
		return err
	}

	parentOverrides := pathToMap(c.Name(), o.(map[string]interface{}))

	po, err := copystructure.Copy(parentOverrides)
	if err != nil {
		return err
	}

	if merge {
		c.Parent().Values = MergeTables(po.(map[string]interface{}), pv.(Values))
	} else {
		c.Parent().Values = CoalesceTables(po.(map[string]interface{}), pv.(Values))
	}

	return syncChartOverridesToParentsValues(c.Parent(), parentOverrides, merge)
}

// Extend extra Values overrides according to export-values directive, if needed.
func processExportExtraValues(c *chart.Chart, extraVals *map[string]interface{}, merge bool) error {
	if c.Parent() == nil || c.Parent().Metadata.Dependencies == nil {
		return nil
	}

	// Get current Chart as chart.Dependency.
	var cr *chart.Dependency
	for _, r := range c.Parent().Metadata.Dependencies {
		if r.Name == c.Name() {
			cr = r
			break
		}
	}

	if cr == nil {
		return nil
	}

	for _, exportValue := range cr.ExportValues {
		parent, child, err := parseExportValues(exportValue)
		if err != nil {
			log.Printf("Warning: invalid ExportValues defined in chart %q for its dependency %q: %s", c.Parent().Name(), cr.Name, err)
			continue
		}

		headlessParentChartPath := stripFirstPathPart(c.Parent().ChartPath())
		var exportParentTablePath string
		if headlessParentChartPath != "" {
			exportParentTablePath = joinPath(headlessParentChartPath, parent)
		} else {
			exportParentTablePath = parent
		}

		// If present, get extra Values overrides table from parent path, as defined in export-values.
		extraParentVals, err := Values(*extraVals).Table(exportParentTablePath)
		if err != nil {
			var errNoTable ErrNoTable
			if errors.As(err, &errNoTable) {
				continue
			} else {
				return err
			}
		}

		var extraChildValsPath string
		if child != "" {
			extraChildValsPath = joinPath(stripFirstPathPart(c.ChartPath()), child)
		} else {
			extraChildValsPath = stripFirstPathPart(c.ChartPath())
		}

		// Do not overwrite anything — skip if something present in destination.
		var errNoTable ErrNoTable
		var errNoValue ErrNoValue
		_, errTable := Values(*extraVals).Table(extraChildValsPath)
		_, errValue := Values(*extraVals).PathValue(extraChildValsPath)
		if !(errors.As(errTable, &errNoTable) && errors.As(errValue, &errNoValue)) {
			continue
		}

		// Create new Values map structure to be merged into extra Values overrides map.
		extraChildVals, err := copystructure.Copy(pathToMap(extraChildValsPath, extraParentVals.AsMap()))
		if err != nil {
			return err
		}

		// Merge new Values into existing extra Values overrides.
		if merge {
			*extraVals = MergeTables(extraChildVals.(map[string]interface{}), *extraVals)
		} else {
			*extraVals = CoalesceTables(extraChildVals.(map[string]interface{}), *extraVals)
		}
	}

	return nil
}

// Generate Values map to be merged into child chart, according to export-values directive of parent chart.
func getExportedValues(parentName string, r *chart.Dependency, pvals Values, merge bool) (map[string]interface{}, error) {
	b := make(map[string]interface{})
	var exportValues []interface{}
	for _, rev := range r.ExportValues {
		parent, child, err := parseExportValues(rev)
		if err != nil {
			log.Printf("Warning: invalid ExportValues defined in chart %q for its dependency %q: %s", parentName, r.Name, err)
			continue
		}

		exportValues = append(exportValues, map[string]string{
			"parent": parent,
			"child":  child,
		})

		var childValMap map[string]interface{}
		// Try to get parent table for parent path specified in export-values.
		vm, err := pvals.Table(parent)
		if err == nil {
			// It IS a valid table.
			if child == "" {
				childValMap = vm.AsMap()
			} else {
				childValMap = pathToMap(child, vm.AsMap())
			}
		} else {
			// If it's not a table, it might be a simple value.
			value, e := pvals.PathValue(parent)
			if e != nil {
				log.Printf("Warning: ExportValues defined in chart %q for its dependency %q can't get the parent path: %s", parentName, r.Name, err.Error())
				continue
			}

			childSlice := parsePath(child)
			if len(childSlice) == 1 && childSlice[0] == "" {
				log.Printf("Warning: in ExportValues defined in chart %q for its dependency %q you are trying to assign a primitive data type (string, int, etc) to the root of your dependent chart values. We will ignore this ExportValues, because this is most likely not what you want. Fix the ExportValues to hide this warning.", parentName, r.Name)
				continue
			}

			childPath := joinPath(childSlice[:len(childSlice)-1]...)
			childMap := map[string]interface{}{
				childSlice[len(childSlice)-1]: value,
			}

			if childPath != "" {
				childValMap = pathToMap(childPath, childMap)
			} else {
				childValMap = childMap
			}
		}

		chValMap, err := copystructure.Copy(childValMap)
		if err != nil {
			return b, err
		}

		// Merge new Values map for current export-values directive into other new Values maps for other export-values directives.
		if merge {
			b = MergeTables(chValMap.(map[string]interface{}), b)
		} else {
			b = CoalesceTables(chValMap.(map[string]interface{}), b)
		}
	}

	// Set formatted export values.
	r.ExportValues = exportValues

	return b, nil
}

// Parse and validate export-values.
func parseExportValues(rev interface{}) (string, string, error) {
	var parent, child string

	switch ev := rev.(type) {
	case map[string]interface{}:
		var ok bool
		parent, ok = ev["parent"].(string)
		if !ok {
			return "", "", fmt.Errorf("parent must be a string")
		}

		child, ok = ev["child"].(string)
		if !ok {
			return "", "", fmt.Errorf("child must be a string")
		}

		if strings.TrimSpace(parent) == "" || strings.TrimSpace(parent) == "." {
			return "", "", fmt.Errorf("parent %q is not allowed", parent)
		}

		parent = strings.TrimSpace(parent)
		child = strings.TrimSpace(child)

		if child == "." {
			child = ""
		}
	case string:
		switch parent = strings.TrimSpace(ev); parent {
		case "", ".":
			parent = "exports"
		default:
			parent = "exports." + parent
		}
		child = ""
	default:
		return "", "", fmt.Errorf("invalid format of ExportValues")
	}

	return parent, child, nil
}

func processDependencyImportExportValues(c *chart.Chart, merge bool) error {
	if err := processDependencyExportValues(c, merge); err != nil {
		return err
	}

	return processDependencyImportValues(c, merge)
}

// Update Values of Chart and its Dependencies according to export-values directive.
func processDependencyExportValues(c *chart.Chart, merge bool) error {
	if err := processExportValues(c, merge); err != nil {
		return err
	}

	for _, d := range c.Dependencies() {
		// recurse
		if err := processDependencyExportValues(d, merge); err != nil {
			return err
		}
	}

	return nil
}

// Update extra Values overrides according to export-values directive, if needed.
func processDependencyExportExtraValues(c *chart.Chart, extraVals *map[string]interface{}, merge bool) error {
	if err := processExportExtraValues(c, extraVals, merge); err != nil {
		return err
	}

	for _, d := range c.Dependencies() {
		// recurse
		if err := processDependencyExportExtraValues(d, extraVals, merge); err != nil {
			return err
		}
	}

	return nil
}

func stripFirstPathPart(path string) string {
	pathParts := parsePath(path)[1:]
	return joinPath(pathParts...)
}
