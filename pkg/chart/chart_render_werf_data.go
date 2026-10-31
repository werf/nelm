package chart

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/werf/common-go/pkg/secrets_manager"
	commonutil "github.com/werf/common-go/pkg/util"
	"github.com/werf/nelm/v2/pkg/common"
	v3chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	chartcommonutil "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/v2/pkg/helm/pkg/cli/values"
	"github.com/werf/nelm/v2/pkg/helm/pkg/getter"
	"github.com/werf/nelm/v2/pkg/helm/pkg/strvals"
	legacysecret "github.com/werf/nelm/v2/pkg/legacy/secret"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

var _ chartcommon.RuntimeData = (*werfDataSecretsRuntimeData)(nil)

// werfDataSecretsRuntimeData overrides the decrypted secret values of a chart with the ones that
// account for .werf-data, keeping its decrypted secret files.
type werfDataSecretsRuntimeData struct {
	base                chartcommon.RuntimeData
	customSecretValues  map[string]interface{}
	defaultSecretValues map[string]interface{}
}

func (d *werfDataSecretsRuntimeData) DecodeAndLoadSecrets(ctx context.Context, loadedChartFiles []*common.BufferedFile, secretsManager *secrets_manager.SecretsManager, opts chartcommon.DecodeAndLoadSecretsOptions) error {
	if d.base == nil {
		return nil
	}

	if err := d.base.DecodeAndLoadSecrets(ctx, loadedChartFiles, secretsManager, opts); err != nil {
		return fmt.Errorf("decode and load secrets: %w", err)
	}

	return nil
}

func (d *werfDataSecretsRuntimeData) GetDecryptedCustomSecretValues() map[string]interface{} {
	return d.customSecretValues
}

func (d *werfDataSecretsRuntimeData) GetDecryptedDefaultSecretValues() map[string]interface{} {
	return d.defaultSecretValues
}

func (d *werfDataSecretsRuntimeData) GetDecryptedSecretFilesData() map[string]string {
	if d.base == nil {
		return nil
	}

	return d.base.GetDecryptedSecretFilesData()
}

// foldWerfDataDependencies replaces the values of every dependency with .werf-data, at any depth,
// with the values the dependency would be deployed with alone: its values.yaml < its
// secret-values.yaml < its publish-time values groups. Dependency conditions, tags, export-values
// and import-values are not processed.
func foldWerfDataDependencies(ctx context.Context, chrt helmchart.Charter, withoutServiceValues bool) error {
	acc, err := helmchart.NewAccessor(chrt)
	if err != nil {
		return fmt.Errorf("create chart accessor: %w", err)
	}

	for _, dep := range acc.Dependencies() {
		depAcc, err := helmchart.NewAccessor(dep)
		if err != nil {
			return fmt.Errorf("create dependency accessor: %w", err)
		}

		if len(werfDataFiles(dep)) > 0 {
			if err := foldWerfData(ctx, dep, withoutServiceValues); err != nil {
				return fmt.Errorf("fold %s of dependency %q: %w", werfdata.DirName, depAcc.Name(), err)
			}

			if !withoutServiceValues {
				warnIgnoredDependencyMetadata(ctx, dep, depAcc.Name())
			}

			log.Default.Debug(ctx, "Folded %s of dependency %q into its values", werfdata.DirName, depAcc.Name())
		}

		if err := foldWerfDataDependencies(ctx, dep, withoutServiceValues); err != nil {
			return err
		}
	}

	return nil
}

func foldWerfData(ctx context.Context, chrt helmchart.Charter, withoutServiceValues bool) error {
	data, err := werfdata.Parse(werfDataFiles(chrt))
	if err != nil {
		return fmt.Errorf("parse %s: %w", werfdata.DirName, err)
	}

	secretValues, err := decryptPublishedSecretValues(ctx, data)
	if err != nil {
		return err
	}

	applySet, err := publishedSetValuesApplier(ctx, data)
	if err != nil {
		return err
	}

	var serviceValues map[string]interface{}
	if !withoutServiceValues {
		serviceValues = data.ExtraValues
	}

	group, err := buildValuesGroup(ctx, chrt, data.Values, secretValues, serviceValues, nil, applySet)
	if err != nil {
		return fmt.Errorf("build values: %w", err)
	}

	groupValues, err := mergeValuesLayers(valuesGroupLayers(group), nil)
	if err != nil {
		return fmt.Errorf("merge values layers: %w", err)
	}

	meta := data.Metadata.Values

	switch c := chrt.(type) {
	case *v2chart.Chart:
		chartCopy := *c
		chartCopy.SetDependencies()

		if meta.DefaultValuesDisable {
			chartCopy.Values = nil
		}

		if meta.DefaultSecretValuesDisable {
			chartCopy.SecretsRuntimeData = &werfDataSecretsRuntimeData{base: c.SecretsRuntimeData}
		}

		folded, err := chartcommonutil.CoalesceValues(&chartCopy, groupValues)
		if err != nil {
			return fmt.Errorf("coalesce values: %w", err)
		}

		c.Values = folded.AsMap()
		c.SecretsRuntimeData = &werfDataSecretsRuntimeData{base: c.SecretsRuntimeData, customSecretValues: customSecretValuesOf(c.SecretsRuntimeData)}
	case *v3chart.Chart:
		chartCopy := *c
		chartCopy.SetDependencies()

		if meta.DefaultValuesDisable {
			chartCopy.Values = nil
		}

		if meta.DefaultSecretValuesDisable {
			chartCopy.SecretsRuntimeData = &werfDataSecretsRuntimeData{base: c.SecretsRuntimeData}
		}

		folded, err := chartcommonutil.CoalesceValues(&chartCopy, groupValues)
		if err != nil {
			return fmt.Errorf("coalesce values: %w", err)
		}

		c.Values = folded.AsMap()
		c.SecretsRuntimeData = &werfDataSecretsRuntimeData{base: c.SecretsRuntimeData, customSecretValues: customSecretValuesOf(c.SecretsRuntimeData)}
	default:
		return fmt.Errorf("unexpected chart type %T", chrt)
	}

	if meta.DefaultSecretValuesDisable {
		if err := disableDependenciesDefaultSecretValues(chrt); err != nil {
			return err
		}
	}

	return nil
}

// dependencyHasWerfDataExtraValues reports whether the chart or any of its dependencies has extra
// values in its .werf-data.
func dependencyHasWerfDataExtraValues(chrt helmchart.Charter) bool {
	acc, err := helmchart.NewAccessor(chrt)
	if err != nil {
		return false
	}

	for _, f := range werfDataFiles(chrt) {
		if f.Name == path.Join(werfdata.DirName, werfdata.ExtraValuesFileName) {
			return true
		}
	}

	for _, dep := range acc.Dependencies() {
		if dependencyHasWerfDataExtraValues(dep) {
			return true
		}
	}

	return false
}

// disableDependenciesDefaultSecretValues drops the secret-values.yaml of every dependency of the
// chart, at any depth, as --disable-default-secret-values does for the subcharts of the root chart.
func disableDependenciesDefaultSecretValues(chrt helmchart.Charter) error {
	acc, err := helmchart.NewAccessor(chrt)
	if err != nil {
		return fmt.Errorf("create chart accessor: %w", err)
	}

	for _, dep := range acc.Dependencies() {
		switch c := dep.(type) {
		case *v2chart.Chart:
			c.SecretsRuntimeData = &werfDataSecretsRuntimeData{base: c.SecretsRuntimeData, customSecretValues: customSecretValuesOf(c.SecretsRuntimeData)}
		case *v3chart.Chart:
			c.SecretsRuntimeData = &werfDataSecretsRuntimeData{base: c.SecretsRuntimeData, customSecretValues: customSecretValuesOf(c.SecretsRuntimeData)}
		default:
			return fmt.Errorf("unexpected chart type %T", dep)
		}

		if err := disableDependenciesDefaultSecretValues(dep); err != nil {
			return err
		}
	}

	return nil
}

// loadChartWithWerfData loads the chart with the default values switches of its .werf-data applied
// on top of the caller's. The .werf-data is read from a load without secrets first, so that a chart
// whose secrets are disabled by its .werf-data loads without the secret key, as with the caller's
// switches. The returned context has the effective options.
func loadChartWithWerfData(ctx context.Context, chartPath string, helmOpts *common.HelmOptions) (context.Context, helmchart.Charter, *werfdata.Data, error) {
	probeOpts := *helmOpts
	probeOpts.ChartLoadOpts.NoSecrets = true

	probeChart, err := loader.Load(common.ContextWithHelmOptions(ctx, probeOpts), chartPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load chart without secrets: %w", err)
	}

	var (
		data            *werfdata.Data
		switchesChanged bool
	)

	if files := werfDataFiles(probeChart); len(files) > 0 {
		data, err = werfdata.Parse(files)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse %s: %w", werfdata.DirName, err)
		}

		logWerfData(ctx, data)

		valuesMetadata := data.Metadata.Values
		chartLoadOpts := &helmOpts.ChartLoadOpts
		switchesChanged = (valuesMetadata.DefaultValuesDisable && !chartLoadOpts.DefaultValuesDisable) ||
			(valuesMetadata.DefaultSecretValuesDisable && !chartLoadOpts.DefaultSecretValuesDisable)
		chartLoadOpts.DefaultValuesDisable = chartLoadOpts.DefaultValuesDisable || valuesMetadata.DefaultValuesDisable
		chartLoadOpts.DefaultSecretValuesDisable = chartLoadOpts.DefaultSecretValuesDisable || valuesMetadata.DefaultSecretValuesDisable
	}

	ctx = common.ContextWithHelmOptions(ctx, *helmOpts)

	// Without secrets to decrypt, the load without secrets is the same as a load with them.
	secretsToLoad := !helmOpts.ChartLoadOpts.NoSecrets && (len(helmOpts.ChartLoadOpts.SecretValuesFiles) > 0 || chartTreeHasSecretFiles(probeChart))
	if !switchesChanged && !secretsToLoad {
		return ctx, probeChart, data, nil
	}

	loadedChart, err := loader.Load(ctx, chartPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load chart: %w", err)
	}

	return ctx, loadedChart, data, nil
}

// mergePublishedValuesIntoChart makes the published service values and secret values part of the
// service values and custom secret values of the chart, below the deploy ones, for the dependency
// processing that reads them from the chart.
func mergePublishedValuesIntoChart(chrt helmchart.Charter, userVals userValues) {
	serviceValues := chartcommon.MergeMaps(userVals.Published.Service, userVals.Deploy.Service)
	customSecretValues := chartcommon.MergeMaps(userVals.Published.SecretValuesFiles, userVals.Deploy.SecretValuesFiles)

	switch c := chrt.(type) {
	case *v2chart.Chart:
		c.ExtraValues = serviceValues
		c.SecretsRuntimeData = &werfDataSecretsRuntimeData{
			base:                c.SecretsRuntimeData,
			customSecretValues:  customSecretValues,
			defaultSecretValues: defaultSecretValuesOf(c.SecretsRuntimeData),
		}
	case *v3chart.Chart:
		c.ExtraValues = serviceValues
		c.SecretsRuntimeData = &werfDataSecretsRuntimeData{
			base:                c.SecretsRuntimeData,
			customSecretValues:  customSecretValues,
			defaultSecretValues: defaultSecretValuesOf(c.SecretsRuntimeData),
		}
	}
}

// warnIgnoredDependencyMetadata warns that the release-wide publish-time inputs of a werf-published
// dependency are not applied: only the root chart decides those for the release.
func warnIgnoredDependencyMetadata(ctx context.Context, dep helmchart.Charter, name string) {
	data, err := werfdata.Parse(werfDataFiles(dep))
	if err != nil {
		return
	}

	metadata := data.Metadata

	var ignored []string
	if len(metadata.ExtraAnnotations) > 0 || len(metadata.ExtraLabels) > 0 || len(metadata.ExtraRuntimeAnnotations) > 0 || len(metadata.ReleaseInfoAnnotations) > 0 {
		ignored = append(ignored, "annotations and labels")
	}

	if len(metadata.Patches) > 0 {
		ignored = append(ignored, "patches")
	}

	if metadata.DefaultPatchesDisable {
		ignored = append(ignored, "disabled default patches")
	}

	if len(metadata.Values.ValuesSetRootJSON) > 0 {
		ignored = append(ignored, "--set-root-json values")
	}

	if metadata.Release != nil && *metadata.Release != (werfdata.ReleaseMetadata{}) {
		ignored = append(ignored, "release defaults")
	}

	if len(ignored) == 0 {
		return
	}

	log.Default.Warn(ctx, "Ignoring %s in %s of dependency %q: only those of the root chart are applied", strings.Join(ignored, ", "), werfdata.DirName, name)
}

// chartTreeHasSecretFiles reports whether the chart or any of its dependencies has a
// secret-values.yaml or secret files.
func chartTreeHasSecretFiles(chrt helmchart.Charter) bool {
	var raw []*chartcommon.File

	switch c := chrt.(type) {
	case *v2chart.Chart:
		raw = c.Raw
	case *v3chart.Chart:
		raw = c.Raw
	}

	for _, f := range raw {
		if f.Name == legacysecret.DefaultSecretValuesFileName || commonutil.IsSubpathOfBasePath(legacysecret.SecretDirName, f.Name) {
			return true
		}
	}

	acc, err := helmchart.NewAccessor(chrt)
	if err != nil {
		return false
	}

	for _, dep := range acc.Dependencies() {
		if chartTreeHasSecretFiles(dep) {
			return true
		}
	}

	return false
}

func customSecretValuesOf(secretsRuntimeData chartcommon.RuntimeData) map[string]interface{} {
	if secretsRuntimeData == nil {
		return nil
	}

	return secretsRuntimeData.GetDecryptedCustomSecretValues()
}

// decryptPublishedSecretValues decrypts the publish-time --secret-values like the secret-values.yaml
// of the chart.
func decryptPublishedSecretValues(ctx context.Context, data *werfdata.Data) (map[string]interface{}, error) {
	chartLoadOpts := common.HelmOptionsFromContext(ctx).ChartLoadOpts
	if data.SecretValues == nil || chartLoadOpts.NoSecrets {
		return map[string]interface{}{}, nil
	}

	encoder, err := secrets_manager.Manager.GetYamlEncoder(ctx, chartLoadOpts.SecretWorkDir, chartLoadOpts.SecretKeyIgnore)
	if err != nil {
		return nil, fmt.Errorf("get secrets yaml encoder: %w", err)
	}

	vals, err := legacysecret.LoadChartSecretValueFiles([]*common.BufferedFile{
		{Name: path.Join(werfdata.DirName, werfdata.SecretValuesFileName), Data: data.SecretValues},
	}, encoder)
	if err != nil {
		return nil, fmt.Errorf("load %s/%s: %w", werfdata.DirName, werfdata.SecretValuesFileName, err)
	}

	return vals, nil
}

func defaultSecretValuesOf(secretsRuntimeData chartcommon.RuntimeData) map[string]interface{} {
	if secretsRuntimeData == nil {
		return nil
	}

	return secretsRuntimeData.GetDecryptedDefaultSecretValues()
}

func logWerfData(ctx context.Context, data *werfdata.Data) {
	meta := data.Metadata.Values

	var loaded []string
	if data.Values != nil {
		loaded = append(loaded, "values")
	}

	if data.SecretValues != nil {
		loaded = append(loaded, "secret values")
	}

	for _, set := range []struct {
		flag   string
		values int
	}{
		{flag: "--set", values: len(meta.ValuesSet)},
		{flag: "--set-string", values: len(meta.ValuesSetString)},
		{flag: "--set-json", values: len(meta.ValuesSetJSON)},
		{flag: "--set-literal", values: len(meta.ValuesSetLiteral)},
		{flag: "--set-file", values: len(meta.ValuesSetFile)},
		{flag: "--set-root-json", values: len(meta.ValuesSetRootJSON)},
	} {
		if set.values > 0 {
			loaded = append(loaded, fmt.Sprintf("%d %s", set.values, set.flag))
		}
	}

	if data.ExtraValues != nil {
		loaded = append(loaded, "extra values")
	}

	if meta.DefaultValuesDisable {
		loaded = append(loaded, "default values disabled")
	}

	if meta.DefaultSecretValuesDisable {
		loaded = append(loaded, "default secret values disabled")
	}

	metadata := data.Metadata
	if len(metadata.Patches) > 0 {
		loaded = append(loaded, fmt.Sprintf("%d patches files", len(metadata.Patches)))
	}

	if metadata.DefaultPatchesDisable {
		loaded = append(loaded, "default patches disabled")
	}

	for _, entries := range []struct {
		name    string
		entries map[string]string
	}{
		{name: "annotations", entries: metadata.ExtraAnnotations},
		{name: "labels", entries: metadata.ExtraLabels},
		{name: "runtime annotations", entries: metadata.ExtraRuntimeAnnotations},
		{name: "release info annotations", entries: metadata.ReleaseInfoAnnotations},
	} {
		if len(entries.entries) > 0 {
			loaded = append(loaded, entries.name)
		}
	}

	if metadata.Release != nil && *metadata.Release != (werfdata.ReleaseMetadata{}) {
		loaded = append(loaded, "release defaults")
	}

	if len(loaded) == 0 {
		loaded = append(loaded, "nothing")
	}

	log.Default.Info(ctx, "Using publish-time inputs from %s: %s", werfdata.DirName, strings.Join(loaded, ", "))
}

// publishedSetValuesApplier returns a function that applies the publish-time --set* values in the
// order Helm applies them. Each --set-file record reads only its own stored contents, never the
// local filesystem.
func publishedSetValuesApplier(ctx context.Context, data *werfdata.Data) (func(base map[string]interface{}) (map[string]interface{}, error), error) {
	meta := data.Metadata.Values

	setFilesContents := make([]map[string]string, len(meta.ValuesSetFile))
	for i, record := range meta.ValuesSetFile {
		contents, err := data.SetFileContents(record)
		if err != nil {
			return nil, fmt.Errorf("get stored --set-file contents: %w", err)
		}

		setFilesContents[i] = contents
	}

	setOpts := &values.Options{
		JSONValues:   meta.ValuesSetJSON,
		StringValues: meta.ValuesSetString,
		Values:       meta.ValuesSet,
	}
	literalOpts := &values.Options{LiteralValues: meta.ValuesSetLiteral}

	return func(base map[string]interface{}) (map[string]interface{}, error) {
		base, err := setOpts.MergeSetValues(ctx, getter.Getters(), base, values.MergeSetValuesOptions{})
		if err != nil {
			return nil, fmt.Errorf("merge --set values: %w", err)
		}

		for i, record := range meta.ValuesSetFile {
			reader := func(rs []rune) (interface{}, error) {
				contents, ok := setFilesContents[i][string(rs)]
				if !ok {
					return nil, fmt.Errorf("%s has no stored contents for file %q", werfdata.DirName, string(rs))
				}

				return contents, nil
			}

			if err := strvals.ParseIntoFile(record.Value, base, reader); err != nil {
				return nil, fmt.Errorf("failed parsing --set-file data: %w", err)
			}
		}

		base, err = literalOpts.MergeSetValues(ctx, getter.Getters(), base, values.MergeSetValuesOptions{})
		if err != nil {
			return nil, fmt.Errorf("merge --set-literal values: %w", err)
		}

		return base, nil
	}, nil
}

func werfDataFiles(chrt helmchart.Charter) []*chartcommon.File {
	switch c := chrt.(type) {
	case *v2chart.Chart:
		return c.WerfDataFiles
	case *v3chart.Chart:
		return c.WerfDataFiles
	default:
		return nil
	}
}
