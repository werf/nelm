package loader

import (
	"context"

	nelmcommon "github.com/werf/nelm/v2/pkg/common"
)

// defaultAPIVersion returns the apiVersion the chart loaders give the top-level chart when its
// Chart.yaml has none, so that the chart goes to the loader of that version.
func defaultAPIVersion(ctx context.Context, apiVersion string) string {
	helmOpts := nelmcommon.HelmOptionsFromContext(ctx)
	if apiVersion != "" || helmOpts.ChartLoadOpts.ChartType != nelmcommon.LegacyChartTypeChart {
		return apiVersion
	}

	return helmOpts.ChartLoadOpts.DefaultChartAPIVersion
}
