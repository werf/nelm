package chart

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/v2/pkg/common"
	v3chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	chartcommonutil "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	chartv2util "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2/util"
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

// chartValuesPath is a path of the chart values in the root chart values, and the values of the
// parent charts that override the chart values at this path.
type chartValuesPath struct {
	// ancestorValues are the values of the chart and of its ancestor charts in the root chart values.
	ancestorValues map[string]interface{}
	overrides      map[string]interface{}
	path           []string
}

// foldWerfDataDependencies replaces the values of every dependency with .werf-data, at any depth,
// with the values the dependency would be deployed with alone: its values.yaml < its
// secret-values.yaml < its publish-time values groups. Dependency conditions, tags, export-values
// and import-values are not processed.
//
// It also returns the publish-time values layers of these dependencies at their paths in the root
// chart values, without the values that their parent charts override. Placed below the user values
// layers, they keep their precedence over the values exported and imported from the lower layers,
// which the folded values lose.
func foldWerfDataDependencies(ctx context.Context, chrt helmchart.Charter, withoutServiceValues bool) ([]valuesLayer, error) {
	return foldDependenciesWerfData(ctx, chrt, withoutServiceValues, []chartValuesPath{{}})
}

func foldDependenciesWerfData(ctx context.Context, chrt helmchart.Charter, withoutServiceValues bool, chartPaths []chartValuesPath) ([]valuesLayer, error) {
	acc, err := helmchart.NewAccessor(chrt)
	if err != nil {
		return nil, fmt.Errorf("create chart accessor: %w", err)
	}

	var (
		layers         []valuesLayer
		chartOverrides map[string]interface{}
	)

	for _, dep := range acc.Dependencies() {
		depAcc, err := helmchart.NewAccessor(dep)
		if err != nil {
			return nil, fmt.Errorf("create dependency accessor: %w", err)
		}

		var publishedLayers []valuesLayer
		if len(werfDataFiles(dep)) > 0 {
			publishedLayers, err = foldWerfData(ctx, dep, withoutServiceValues)
			if err != nil {
				return nil, fmt.Errorf("fold %s of dependency %q: %w", werfdata.DirName, depAcc.Name(), err)
			}

			if !withoutServiceValues {
				warnIgnoredDependencyMetadata(ctx, dep, depAcc.Name())
			}

			log.Default.Debug(ctx, "Folded %s of dependency %q into its values", werfdata.DirName, depAcc.Name())
		}

		if chartOverrides == nil {
			if chartOverrides, err = chartValuesOverridingDependencies(chrt); err != nil {
				return nil, err
			}
		}

		var depPaths []chartValuesPath
		for _, chartPath := range chartPaths {
			ancestorValues := chartcommon.MergeMaps(chartPath.ancestorValues, nestValues(chartOverrides, chartPath.path))

			for _, key := range dependencyValuesKeys(acc, depAcc) {
				depPaths = append(depPaths, chartValuesPath{
					ancestorValues: ancestorValues,
					overrides:      chartcommon.MergeMaps(tableValue(chartOverrides, key), tableValue(chartPath.overrides, key)),
					path:           append(slices.Clone(chartPath.path), key),
				})
			}
		}

		depLayers, err := foldDependenciesWerfData(ctx, dep, withoutServiceValues, depPaths)
		if err != nil {
			return nil, err
		}

		layers = append(layers, depLayers...)

		for _, depPath := range depPaths {
			for _, layer := range publishedLayers {
				dependencyLayer, err := dependencyValuesLayer(layer, depPath)
				if err != nil {
					return nil, err
				}

				layers = append(layers, dependencyLayer)
			}
		}
	}

	return layers, nil
}

// chartValuesOverridingDependencies returns the values of the chart, with its secret values and with
// its export-values applied, that override the values of its dependencies.
func chartValuesOverridingDependencies(chrt helmchart.Charter) (map[string]interface{}, error) {
	switch c := chrt.(type) {
	case *v2chart.Chart:
		vals, _, err := chartv2util.ExportUserValues(c, chartcommon.MergeMaps(c.Values, defaultSecretValuesOf(c.SecretsRuntimeData)), chartv2util.ExportUserValuesOptions{})
		if err != nil {
			return nil, fmt.Errorf("export chart values: %w", err)
		}

		return vals, nil
	case *v3chart.Chart:
		return chartcommon.MergeMaps(c.Values, defaultSecretValuesOf(c.SecretsRuntimeData)), nil
	default:
		return nil, fmt.Errorf("unexpected chart type %T", chrt)
	}
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

// dependencyValuesLayer returns the publish-time values layer of a dependency at its path in the
// root chart values, without the values that its parent charts override.
func dependencyValuesLayer(layer valuesLayer, depPath chartValuesPath) (valuesLayer, error) {
	vals, err := deepCopyValues(layer.Values)
	if err != nil {
		return valuesLayer{}, fmt.Errorf("copy dependency values layer: %w", err)
	}

	deleteOverriddenValues(vals, depPath.overrides)

	replacedPaths := make([][]string, 0, len(layer.ReplacedPaths))
	for _, path := range layer.ReplacedPaths {
		replacedPaths = append(replacedPaths, append(slices.Clone(depPath.path), path...))
	}

	ancestorValues, err := deepCopyValues(depPath.ancestorValues)
	if err != nil {
		return valuesLayer{}, fmt.Errorf("copy ancestor values: %w", err)
	}

	// The values in the dependency values are already deleted above. The values that the dependency
	// exports into its own dependencies beat them, like its default values do.
	deleteValueAtPath(ancestorValues, depPath.path)

	return valuesLayer{
		AncestorValues: ancestorValues,
		DependencyPath: depPath.path,
		ReplacedPaths:  replacedPaths,
		Values:         nestValues(vals, depPath.path),
	}, nil
}

// foldWerfData folds the .werf-data of the chart into its values and returns its publish-time
// values layers.
func foldWerfData(ctx context.Context, chrt helmchart.Charter, withoutServiceValues bool) ([]valuesLayer, error) {
	data, err := werfdata.Parse(werfDataFiles(chrt))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", werfdata.DirName, err)
	}

	secretValues, err := decryptPublishedSecretValues(ctx, data)
	if err != nil {
		return nil, err
	}

	applySet, err := publishedSetValuesApplier(ctx, data)
	if err != nil {
		return nil, err
	}

	var serviceValues map[string]interface{}
	if !withoutServiceValues {
		serviceValues = data.ExtraValues
	}

	group, err := buildValuesGroup(ctx, chrt, data.Values, secretValues, serviceValues, nil, applySet)
	if err != nil {
		return nil, fmt.Errorf("build values: %w", err)
	}

	groupLayers := valuesGroupLayers(group)

	groupValues, err := mergeValuesLayers(groupLayers, nil)
	if err != nil {
		return nil, fmt.Errorf("merge values layers: %w", err)
	}

	meta := data.Metadata.Values

	switch c := chrt.(type) {
	case *v2chart.Chart:
		chartCopy := *c
		chartCopy.SetDependencies()

		if meta.DefaultValuesDisable {
			chartCopy.Values = nil
		}

		folded, err := chartcommonutil.CoalesceValues(&chartCopy, groupValues)
		if err != nil {
			return nil, fmt.Errorf("coalesce values: %w", err)
		}

		c.Values = folded.AsMap()
		c.SecretsRuntimeData = &werfDataSecretsRuntimeData{base: c.SecretsRuntimeData, customSecretValues: customSecretValuesOf(c.SecretsRuntimeData)}
	case *v3chart.Chart:
		chartCopy := *c
		chartCopy.SetDependencies()

		if meta.DefaultValuesDisable {
			chartCopy.Values = nil
		}

		folded, err := chartcommonutil.CoalesceValues(&chartCopy, groupValues)
		if err != nil {
			return nil, fmt.Errorf("coalesce values: %w", err)
		}

		c.Values = folded.AsMap()
		c.SecretsRuntimeData = &werfDataSecretsRuntimeData{base: c.SecretsRuntimeData, customSecretValues: customSecretValuesOf(c.SecretsRuntimeData)}
	default:
		return nil, fmt.Errorf("unexpected chart type %T", chrt)
	}

	return groupLayers, nil
}

// loadChartWithWerfData loads the chart with the default values switches of the .werf-data of the
// chart and of its dependencies applied on top of the caller's by the loader, so that the secrets
// they disable are not decrypted. The returned context has the effective options of the chart.
func loadChartWithWerfData(ctx context.Context, chartPath string, helmOpts *common.HelmOptions) (context.Context, helmchart.Charter, *werfdata.Data, error) {
	loadOpts := *helmOpts
	loadOpts.ChartLoadOpts.WerfDataValuesSwitches = true

	loadedChart, err := loader.Load(common.ContextWithHelmOptions(ctx, loadOpts), chartPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load chart: %w", err)
	}

	var data *werfdata.Data
	if files := werfDataFiles(loadedChart); len(files) > 0 {
		data, err = werfdata.Parse(files)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("parse %s: %w", werfdata.DirName, err)
		}

		logWerfData(ctx, data)

		chartLoadOpts := &loadOpts.ChartLoadOpts
		chartLoadOpts.DefaultValuesDisable = chartLoadOpts.DefaultValuesDisable || data.Metadata.Values.DefaultValuesDisable
		chartLoadOpts.DefaultSecretValuesDisable = chartLoadOpts.DefaultSecretValuesDisable || data.Metadata.Values.DefaultSecretValuesDisable
	}

	*helmOpts = loadOpts

	return common.ContextWithHelmOptions(ctx, loadOpts), loadedChart, data, nil
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

// deleteOverriddenValues deletes the values that the overrides replace when coalesced over them.
func deleteOverriddenValues(vals, overrides map[string]interface{}) {
	for key, override := range overrides {
		value, found := vals[key]
		if !found {
			continue
		}

		table, isTable := value.(map[string]interface{})
		overrideTable, overrideIsTable := override.(map[string]interface{})

		if isTable && overrideIsTable {
			deleteOverriddenValues(table, overrideTable)
			continue
		}

		delete(vals, key)
	}
}

// dependencyValuesKeys returns the keys of the dependency values in the values of the chart: its
// aliases, or its name.
func dependencyValuesKeys(acc, depAcc helmchart.Accessor) []string {
	var keys []string
	for _, metaDep := range acc.MetaDependencies() {
		metaDepAcc, err := helmchart.NewDependencyAccessor(metaDep)
		if err != nil || metaDepAcc.Name() != depAcc.Name() {
			continue
		}

		keys = append(keys, lo.CoalesceOrEmpty(metaDepAcc.Alias(), metaDepAcc.Name()))
	}

	if len(keys) == 0 {
		return []string{depAcc.Name()}
	}

	return lo.Uniq(keys)
}

// hasDependencyAtPath reports whether the processed chart has the dependency at the path of its
// values in the chart values.
func hasDependencyAtPath(chrt helmchart.Charter, path []string) bool {
	if len(path) == 0 {
		return true
	}

	acc, err := helmchart.NewAccessor(chrt)
	if err != nil {
		return false
	}

	for _, dep := range acc.Dependencies() {
		depAcc, err := helmchart.NewAccessor(dep)
		if err == nil && depAcc.Name() == path[0] {
			return hasDependencyAtPath(dep, path[1:])
		}
	}

	return false
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

// nestValues returns the values at the path.
func nestValues(vals map[string]interface{}, path []string) map[string]interface{} {
	for i := len(path) - 1; i >= 0; i-- {
		vals = map[string]interface{}{path[i]: vals}
	}

	return vals
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

func tableValue(vals map[string]interface{}, key string) map[string]interface{} {
	table, _ := vals[key].(map[string]interface{})

	return table
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
