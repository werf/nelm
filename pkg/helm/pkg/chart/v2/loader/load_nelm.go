package loader

import (
	"fmt"

	nelmcommon "github.com/werf/nelm/v2/pkg/common"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

// applyWerfDataValuesSwitches turns on the default values switches stored in .werf-data of the
// chart. It runs before the secrets are decoded, so that the default secret values of the chart
// and of its dependencies, which inherit these options, are not decrypted.
func applyWerfDataValuesSwitches(helmOpts *nelmcommon.HelmOptions, werfDataFiles []*chartcommon.File) error {
	if !helmOpts.ChartLoadOpts.WerfDataValuesSwitches || len(werfDataFiles) == 0 {
		return nil
	}

	data, err := werfdata.Parse(werfDataFiles)
	if err != nil {
		return fmt.Errorf("cannot load %s: %w", werfdata.DirName, err)
	}

	helmOpts.ChartLoadOpts.DefaultSecretValuesDisable = helmOpts.ChartLoadOpts.DefaultSecretValuesDisable || data.Metadata.Values.DefaultSecretValuesDisable
	helmOpts.ChartLoadOpts.DefaultValuesDisable = helmOpts.ChartLoadOpts.DefaultValuesDisable || data.Metadata.Values.DefaultValuesDisable

	return nil
}

// defaultChartAPIVersion returns the apiVersion to give the top-level chart whose Chart.yaml has
// none. It is applied as soon as Chart.yaml is read, since a v1 chart keeps requirements.yaml among
// its files and loses Chart.lock on save.
func defaultChartAPIVersion(applyWerfExtensions bool, helmOpts nelmcommon.HelmOptions) string {
	if !applyWerfExtensions || helmOpts.ChartLoadOpts.ChartType != nelmcommon.LegacyChartTypeChart {
		return ""
	}

	return helmOpts.ChartLoadOpts.DefaultChartAPIVersion
}
