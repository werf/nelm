package chart

import (
	"context"
	"fmt"
	"io"

	"sigs.k8s.io/yaml"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

// PackedValuesWarnings returns the warnings about the export-values and import-values that a
// deploy of the packed chart archive at chartPath skips because of its publish-time values in
// .werf-data, e.g. a scalar exported to the root of a dependency. Nothing is decrypted: the
// publish-time secret values of the chart are used as they are stored, which keeps whether a value
// is a map, while the secret-values.yaml files and the publish-time secret values of the
// dependencies are not used. An encrypted dependency condition or tag value is therefore not a
// boolean: it doesn't decide whether the dependency is enabled, but hides the lower values that
// would. Warnings about paths that no values set are not returned, as the deploy can still set
// them.
func PackedValuesWarnings(ctx context.Context, chartPath string) ([]string, error) {
	ctx = common.ContextWithHelmOptions(ctx, common.HelmOptions{
		ChartLoadOpts: common.ChartLoadOptions{
			NoSecrets:              true,
			WerfDataValuesSwitches: true,
		},
	})

	loadedChart, err := loader.LoadFile(ctx, chartPath)
	if err != nil {
		return nil, fmt.Errorf("load chart archive %q: %w", chartPath, err)
	}

	files := werfDataFiles(loadedChart)
	if len(files) == 0 {
		return nil, nil
	}

	data, err := werfdata.Parse(files)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", werfdata.DirName, err)
	}

	// A deploy of the chart warns about the ignored metadata of the dependencies itself.
	quietCtx := logboek.NewContext(ctx, logboek.NewLogger(io.Discard, io.Discard))

	dependencyLayers, err := werfDataDependencyLayers(quietCtx, loadedChart, false)
	if err != nil {
		return nil, fmt.Errorf("get %s values layers of dependencies: %w", werfdata.DirName, err)
	}

	secretValues := map[string]interface{}{}
	if err := yaml.Unmarshal(data.SecretValues, &secretValues); err != nil {
		return nil, fmt.Errorf("parse %s/%s: %w", werfdata.DirName, werfdata.SecretValuesFileName, err)
	}

	applySet, err := publishedSetValuesApplier(ctx, data)
	if err != nil {
		return nil, err
	}

	published, err := buildValuesGroup(ctx, loadedChart, data.Values, secretValues, data.ExtraValues, nil, applySet)
	if err != nil {
		return nil, fmt.Errorf("build %s values: %w", werfdata.DirName, err)
	}

	userVals := userValues{DependencyLayers: dependencyLayers, Published: published}
	mergePublishedValuesIntoChart(loadedChart, userVals)

	_, _, warnings, _, err := processDependencies(loadedChart, userVals, processDependenciesOptions{NoNotFoundWarnings: true})
	if err != nil {
		return nil, fmt.Errorf("process chart dependencies: %w", err)
	}

	return warnings, nil
}
