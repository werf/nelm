package action

import (
	"fmt"

	"github.com/samber/lo"

	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

func mergeWerfDataRuntimeOptions(chart helmchart.Accessor, opts common.ReleaseInstallRuntimeOptions) (common.ReleaseInstallRuntimeOptions, error) {
	werfData, err := chartWerfData(chart)
	if err != nil {
		return common.ReleaseInstallRuntimeOptions{}, err
	}

	opts.ExtraAnnotations = mergeChartMetadataMap(werfData.Metadata.ExtraAnnotations, opts.ExtraAnnotations)
	opts.ExtraLabels = mergeChartMetadataMap(werfData.Metadata.ExtraLabels, opts.ExtraLabels)
	opts.ExtraRuntimeAnnotations = mergeChartMetadataMap(werfData.Metadata.ExtraRuntimeAnnotations, opts.ExtraRuntimeAnnotations)
	opts.ReleaseInfoAnnotations = mergeChartMetadataMap(werfData.Metadata.ReleaseInfoAnnotations, opts.ReleaseInfoAnnotations)

	return opts, nil
}

// chartWerfData returns the parsed .werf-data of the chart, empty if it has none. Stored releases
// never have it, so rollback and uninstall get an empty one.
func chartWerfData(chart helmchart.Accessor) (*werfdata.Data, error) {
	if chart == nil || len(chart.WerfDataFiles()) == 0 {
		return &werfdata.Data{}, nil
	}

	data, err := werfdata.Parse(chart.WerfDataFiles())
	if err != nil {
		return nil, fmt.Errorf("parse %s of chart %q: %w", werfdata.DirName, chart.Name(), err)
	}

	return data, nil
}

// mergeChartMetadataMap returns the chart's entries overridden per key by the caller's.
func mergeChartMetadataMap(chartMap, callerMap map[string]string) map[string]string {
	if len(chartMap) == 0 {
		return callerMap
	}

	return lo.Assign(chartMap, callerMap)
}
