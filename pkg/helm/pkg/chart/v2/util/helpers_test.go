package util

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
)

func userValueAtPath(vals map[string]interface{}, path string) (interface{}, bool) {
	return valueAtPath(vals, parsePath(path))
}

func copyUserValues(v map[string]interface{}) (map[string]interface{}, error) {
	return util.DeepCopyValues(v)
}

// chainOfExportsChart returns a chart with n dependencies, each exporting the value exported to the
// dependency listed after it, and the last one exporting "src".
func chainOfExportsChart(n int) *chart.Chart {
	c := &chart.Chart{Metadata: &chart.Metadata{Name: "parent", Version: "0.1.0", APIVersion: chart.APIVersionV2}}
	for i := range n {
		parent := lo.Ternary(i == n-1, "src", fmt.Sprintf("d%03d.out", i+1))
		c.Metadata.Dependencies = append(c.Metadata.Dependencies, &chart.Dependency{
			Name:         fmt.Sprintf("d%03d", i),
			Version:      "0.1.0",
			ExportValues: []interface{}{map[string]interface{}{"parent": parent, "child": "out"}},
		})
		c.AddDependency(&chart.Chart{Metadata: &chart.Metadata{Name: fmt.Sprintf("d%03d", i), Version: "0.1.0", APIVersion: chart.APIVersionV2}})
	}

	return c
}
