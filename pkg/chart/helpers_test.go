package chart

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	v3chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	chartcommonutil "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

const layeredValuesSecretKey = "eb1988e0ffd7272892a851f4c20f6512" //nolint:gosec // Test-only key that encrypts testdata/layered-values.

var (
	layeredImportExportSecretValuesFile = filepath.Join("testdata", "layered-values", "ie-secret-values-file.yaml")
	layeredImportExportValuesFile       = filepath.Join("testdata", "layered-values", "ie-values-file.yaml")
	layeredValuesFile                   = filepath.Join("testdata", "layered-values", "values-file.yaml")
	layeredValuesExportFile             = filepath.Join("testdata", "layered-values", "values-file-export.yaml")
	layeredSecretValuesExportFile       = filepath.Join("testdata", "layered-values", "secret-values-file-export.yaml")
	layeredSecretValuesFile             = filepath.Join("testdata", "layered-values", "secret-values-file.yaml")
	layeredSecretValuesNullFile         = filepath.Join("testdata", "layered-values", "secret-values-file-null.yaml")
	layeredSecretValuesShapeFile        = filepath.Join("testdata", "layered-values", "secret-values-file-shape.yaml")
	layeredSecretValuesShapeScalarFile  = filepath.Join("testdata", "layered-values", "secret-values-file-shape-scalar.yaml")
	layeredSecretValuesSubFile          = filepath.Join("testdata", "layered-values", "secret-values-file-sub.yaml")
	layeredValuesShapeFile              = filepath.Join("testdata", "layered-values", "values-file-shape.yaml")
	layeredValuesShapeMapFile           = filepath.Join("testdata", "layered-values", "values-file-shape-map.yaml")
)

type absentValue struct{}

// werfDataFlags are value flags that are passed either as the caller's options or stored in
// .werf-data.
type werfDataFlags struct {
	DefaultSecretValuesDisable bool
	DefaultValuesDisable       bool
	ExtraValues                string
	SecretValues               string
	Set                        []string
	SetFile                    []werfDataSetFile
	SetJSON                    []string
	SetLiteral                 []string
	SetRootJSON                []string
	SetString                  []string
	Values                     string
}

type werfDataSetFile struct {
	Contents string
	Key      string
	// Path is the path in the set file value, a temporary file by default.
	Path string
}

type warningsLogger struct {
	log.Logger

	infos    []string
	warnings []string
}

func (l *warningsLogger) Info(_ context.Context, format string, a ...interface{}) {
	l.infos = append(l.infos, fmt.Sprintf(format, a...))
}

func (l *warningsLogger) Warn(_ context.Context, format string, a ...interface{}) {
	l.warnings = append(l.warnings, fmt.Sprintf(format, a...))
}

type layeredValuesRenderOptions struct {
	ChartDir                   string
	DefaultSecretValuesDisable bool
	SecretValuesFiles          []string
	ServiceValues              map[string]interface{}
	Set                        []string
	SetJSON                    []string
	SetString                  []string
	ValuesFiles                []string
}

// werfDataFlagsAsFiles returns the .werf-data files that store the inputs. The default paths of
// the set file values point to files that don't exist.
func werfDataFlagsAsFiles(t *testing.T, flags werfDataFlags) map[string]string {
	t.Helper()

	data := &werfdata.Data{
		Files: map[string][]byte{},
		Metadata: werfdata.Metadata{
			Values: werfdata.ValuesMetadata{
				DefaultSecretValuesDisable: flags.DefaultSecretValuesDisable,
				DefaultValuesDisable:       flags.DefaultValuesDisable,
				ValuesSet:                  flags.Set,
				ValuesSetJSON:              flags.SetJSON,
				ValuesSetLiteral:           flags.SetLiteral,
				ValuesSetRootJSON:          flags.SetRootJSON,
				ValuesSetString:            flags.SetString,
			},
		},
	}

	if flags.Values != "" {
		data.Values = parseTestValues(t, flags.Values)
	}

	if flags.SecretValues != "" {
		data.SecretValues = []byte(encryptTestValues(t, flags.SecretValues))
	}

	if flags.ExtraValues != "" {
		data.ExtraValues = parseTestValues(t, flags.ExtraValues)
	}

	for i, setFile := range flags.SetFile {
		filePath := setFile.Path
		if filePath == "" {
			filePath = filepath.Join(t.TempDir(), fmt.Sprintf("set-file-%d", i))
		}

		storedName := fmt.Sprintf("%s/%d", werfdata.SetFilesDirName, i)
		data.Files[storedName] = []byte(setFile.Contents)
		data.Metadata.Values.ValuesSetFile = append(data.Metadata.Values.ValuesSetFile, werfdata.SetFileRecord{
			Contents: map[string]string{filePath: storedName},
			Value:    setFile.Key + "=" + filePath,
		})
	}

	chartFiles, err := data.ChartFiles()
	require.NoError(t, err)

	files := map[string]string{}
	for _, f := range chartFiles {
		files[f.Name] = string(f.Data)
	}

	return files
}

// werfDataFlagsAsOptions writes the files of the flags into a temporary directory and adds the flags
// to the caller's options.
func werfDataFlagsAsOptions(t *testing.T, flags werfDataFlags, opts RenderChartOptions) RenderChartOptions {
	t.Helper()

	dir := t.TempDir()

	if flags.Values != "" {
		opts.ValuesFiles = append(opts.ValuesFiles, writeTestFile(t, filepath.Join(dir, "values.yaml"), flags.Values))
	}

	if flags.SecretValues != "" {
		opts.HelmOptions.ChartLoadOpts.SecretValuesFiles = append(opts.HelmOptions.ChartLoadOpts.SecretValuesFiles, writeTestFile(t, filepath.Join(dir, "secret-values.yaml"), encryptTestValues(t, flags.SecretValues)))
	}

	for i, setFile := range flags.SetFile {
		filePath := setFile.Path
		if filePath == "" {
			filePath = filepath.Join(dir, fmt.Sprintf("set-file-%d", i))
		}

		writeTestFile(t, filePath, setFile.Contents)
		opts.ValuesSetFile = append(opts.ValuesSetFile, setFile.Key+"="+filePath)
	}

	if flags.ExtraValues != "" {
		opts.HelmOptions.ChartLoadOpts.ExtraValues = chartcommon.MergeMaps(parseTestValues(t, flags.ExtraValues), opts.HelmOptions.ChartLoadOpts.ExtraValues)
	}

	opts.ValuesSet = append(opts.ValuesSet, flags.Set...)
	opts.ValuesSetJSON = append(opts.ValuesSetJSON, flags.SetJSON...)
	opts.ValuesSetLiteral = append(opts.ValuesSetLiteral, flags.SetLiteral...)
	opts.ValuesSetString = append(opts.ValuesSetString, flags.SetString...)
	opts.RootSetJSON = append(opts.RootSetJSON, flags.SetRootJSON...)
	opts.HelmOptions.ChartLoadOpts.DefaultValuesDisable = opts.HelmOptions.ChartLoadOpts.DefaultValuesDisable || flags.DefaultValuesDisable
	opts.HelmOptions.ChartLoadOpts.DefaultSecretValuesDisable = opts.HelmOptions.ChartLoadOpts.DefaultSecretValuesDisable || flags.DefaultSecretValuesDisable

	return opts
}

// writeWerfDataTestChart writes the chart files into a temporary directory and returns its path.
// The files named secret-values.yaml are encrypted.
func writeWerfDataTestChart(t *testing.T, files ...map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	for _, chartFiles := range files {
		for name, data := range chartFiles {
			if filepath.Base(name) == "secret-values.yaml" && !werfdata.IsWerfDataFile(name) && !strings.Contains(name, "/"+werfdata.DirName+"/") {
				data = encryptTestValues(t, data)
			}

			writeTestFile(t, filepath.Join(dir, filepath.FromSlash(name)), data)
		}
	}

	return dir
}

func encryptTestValues(t *testing.T, plain string) string {
	t.Helper()

	return encryptTestValuesWithKey(t, layeredValuesSecretKey, plain)
}

func renderWerfDataTestChart(t *testing.T, chartPath string, opts RenderChartOptions) (*RenderChartResult, error) {
	t.Helper()

	return renderWerfDataTestChartWithSecretKey(t, chartPath, layeredValuesSecretKey, opts)
}

func werfDataCardChartWithSubchartSecrets(values, secretValues, subchartValues, subchartSecretValues string) map[string]string {
	files := werfDataCardChart("- name: sub\n  version: 1.0.0", values, secretValues, subchartValues)
	files["charts/sub/secret-values.yaml"] = subchartSecretValues

	return files
}

func deleteNulls(vals map[string]interface{}) {
	for key, value := range vals {
		switch table, isTable := value.(map[string]interface{}); {
		case value == nil:
			delete(vals, key)
		case isTable:
			deleteNulls(table)
		}
	}
}

func dependencyNames(t *testing.T, result *RenderChartResult) []string {
	t.Helper()

	var names []string
	for _, dep := range result.Chart.Dependencies() {
		depAccessor, err := helmchart.NewAccessor(dep)
		require.NoError(t, err)

		names = append(names, depAccessor.Name())
	}

	sort.Strings(names)

	return names
}

func encryptTestValuesWithKey(t *testing.T, secretKey, plain string) string {
	t.Helper()

	t.Setenv("WERF_SECRET_KEY", secretKey)

	encoder, err := secrets_manager.Manager.GetYamlEncoder(context.Background(), "", false)
	require.NoError(t, err)

	encrypted, err := encoder.EncryptYamlData([]byte(plain))
	require.NoError(t, err)

	return string(encrypted)
}

func flattenToSetValues(vals map[string]interface{}, prefix string) []string {
	var result []string
	for key, value := range vals {
		if table, ok := value.(map[string]interface{}); ok {
			result = append(result, flattenToSetValues(table, prefix+key+".")...)
			continue
		}

		result = append(result, prefix+key+"="+fmt.Sprint(value))
	}

	return result
}

func mergeTestFiles(files ...map[string]string) map[string]string {
	result := map[string]string{}
	for _, f := range files {
		maps.Copy(result, f)
	}

	return result
}

func parseTestValues(t *testing.T, data string) map[string]interface{} {
	t.Helper()

	vals := map[string]interface{}{}
	require.NoError(t, yaml.Unmarshal([]byte(data), &vals))

	return vals
}

func prefixTestFiles(prefix string, files map[string]string) map[string]string {
	result := map[string]string{}
	for name, data := range files {
		result[prefix+name] = data
	}

	return result
}

func readTestFile(t *testing.T, filePath string) string {
	t.Helper()

	data, err := os.ReadFile(filePath)
	require.NoError(t, err)

	return string(data)
}

func readValuesFile(t *testing.T, path string) map[string]interface{} {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var vals map[string]interface{}
	require.NoError(t, yaml.Unmarshal(data, &vals))

	return vals
}

func renderLayeredValuesChart(t *testing.T, opts layeredValuesRenderOptions) *RenderChartResult {
	t.Helper()

	t.Setenv("WERF_SECRET_KEY", layeredValuesSecretKey)

	chartDir := opts.ChartDir
	if chartDir == "" {
		chartDir = "chart"
	}

	if !filepath.IsAbs(chartDir) {
		chartDir = filepath.Join("testdata", "layered-values", chartDir)
	}

	chartPath, err := filepath.Abs(chartDir)
	require.NoError(t, err)

	renderOpts := RenderChartOptions{
		HelmOptions: common.HelmOptions{
			ChartLoadOpts: common.ChartLoadOptions{
				ExtraValues: opts.ServiceValues,
			},
		},
		TempDirPath: t.TempDir(),
	}
	renderOpts.ValuesSet = opts.Set
	renderOpts.ValuesSetJSON = opts.SetJSON
	renderOpts.ValuesSetString = opts.SetString
	renderOpts.ValuesFiles = opts.ValuesFiles
	renderOpts.HelmOptions.ChartLoadOpts.SecretValuesFiles = opts.SecretValuesFiles
	renderOpts.HelmOptions.ChartLoadOpts.DefaultSecretValuesDisable = opts.DefaultSecretValuesDisable

	ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())

	result, err := RenderChart(ctx, chartPath, "rel", "ns", 1, common.DeployTypeInitial, nil, nil, renderOpts)
	require.NoError(t, err)

	return result
}

// renderWerfDataTestChartWithSecretKey renders the chart with the secret key, none if empty.
func renderWerfDataTestChartWithSecretKey(t *testing.T, chartPath, secretKey string, opts RenderChartOptions) (*RenderChartResult, error) {
	t.Helper()

	t.Setenv("WERF_SECRET_KEY", secretKey)
	secrets_manager.SetWerfHomeDir(t.TempDir())
	t.Cleanup(func() { secrets_manager.SetWerfHomeDir("") })

	opts.LocalKubeVersion = "1.30.0"
	opts.TempDirPath = t.TempDir()
	opts.HelmOptions.ChartLoadOpts.SecretWorkDir = chartPath

	ctx := logboek.NewContext(context.Background(), logboek.DefaultLogger())

	return RenderChart(ctx, chartPath, "rel", "ns", 1, common.DeployTypeInitial, nil, nil, opts)
}

func renderedChartValues(t *testing.T, result *RenderChartResult) map[string]interface{} {
	t.Helper()

	vals, ok := result.Values["Values"].(chartcommon.Values)
	require.True(t, ok, "unexpected rendered values type %T", result.Values["Values"])

	return vals.AsMap()
}

func renderedConfigMapNames(result *RenderChartResult) []string {
	var names []string
	for _, res := range result.ResourceSpecs {
		if res.GroupVersionKind.Kind == "ConfigMap" {
			names = append(names, res.Name)
		}
	}

	sort.Strings(names)

	return names
}

func requireTestNode(t *testing.T) string {
	t.Helper()

	nodePath := os.Getenv("NELM_TEST_NODE_BINARY_PATH")
	if nodePath == "" {
		t.Skip("NELM_TEST_NODE_BINARY_PATH is not set")
	}

	return nodePath
}

// reversedDependenciesChartDir copies the chart from testdata/layered-values and reverses the
// dependencies in each Chart.yaml of the copy.
func reversedDependenciesChartDir(t *testing.T, chartDir string) string {
	t.Helper()

	dir := t.TempDir()
	require.NoError(t, os.CopyFS(dir, os.DirFS(filepath.Join("testdata", "layered-values", chartDir))))

	root, err := os.OpenRoot(dir)
	require.NoError(t, err)

	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.Name() != "Chart.yaml" {
			return err
		}

		data, err := root.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", path, err)
		}

		var metadata map[string]interface{}
		if err := yaml.Unmarshal(data, &metadata); err != nil {
			return fmt.Errorf("unmarshal %q: %w", path, err)
		}

		if deps, ok := metadata["dependencies"].([]interface{}); ok {
			slices.Reverse(deps)
		}

		if data, err = yaml.Marshal(metadata); err != nil {
			return fmt.Errorf("marshal %q: %w", path, err)
		}

		return root.WriteFile(path, data, 0o600)
	})
	require.NoError(t, err)
	require.NoError(t, root.Close())

	return dir
}

// storedReleaseValues returns the values that release get prints for the release stored from the
// result: the chart as stored in the release coalesced with the release config.
func storedReleaseValues(t *testing.T, result *RenderChartResult) map[string]interface{} {
	t.Helper()

	data, err := json.Marshal(result.Chart.Charter())
	require.NoError(t, err)

	var stored helmchart.Charter
	switch result.Chart.Charter().(type) {
	case *v2chart.Chart:
		storedChart := &v2chart.Chart{}
		require.NoError(t, json.Unmarshal(data, storedChart))
		stored = storedChart
	case *v3chart.Chart:
		storedChart := &v3chart.Chart{}
		require.NoError(t, json.Unmarshal(data, storedChart))
		stored = storedChart
	default:
		t.Fatalf("unexpected chart type %T", result.Chart.Charter())
	}

	vals, err := chartcommonutil.CoalesceValues(stored, result.ReleaseConfig)
	require.NoError(t, err)

	return vals.AsMap()
}

// valueAtPath returns the value at the dot-separated path, or absentValue if there is none.
func valueAtPath(t *testing.T, vals map[string]interface{}, path string) interface{} {
	t.Helper()

	var value interface{} = vals
	for key := range strings.SplitSeq(path, ".") {
		table, ok := value.(map[string]interface{})
		require.True(t, ok, "value at %q is not a map", path)

		if value, ok = table[key]; !ok {
			return absentValue{}
		}
	}

	return value
}

// werfDataCardChart returns the files of a chart with an optional dependency "sub".
func werfDataCardChart(dependencies, values, secretValues, subchartValues string) map[string]string {
	files := map[string]string{
		"Chart.yaml":  "apiVersion: v2\nname: root\nversion: 1.0.0\n",
		"values.yaml": values,
	}

	if dependencies != "" {
		files["Chart.yaml"] += "dependencies:\n" + dependencies + "\n"
		files["charts/sub/Chart.yaml"] = "apiVersion: v2\nname: sub\nversion: 1.0.0\n"
		files["charts/sub/values.yaml"] = subchartValues
	}

	if secretValues != "" {
		files["secret-values.yaml"] = secretValues
	}

	return files
}

// werfDataNestedSubchartsChart returns the files of a chart with a dependency "sub" with a dependency
// "subsub", each exporting werf to its dependency, and rendering a ConfigMap with an image and the
// repo from the extra values.
func werfDataNestedSubchartsChart() map[string]string {
	const template = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Chart.Name }}\ndata:\n  image: {{ .Values.werf.image.x }}\n  repo: {{ .Values.global.werf.repo }}\n"

	return map[string]string{
		"Chart.yaml":                                 "apiVersion: v2\nname: root\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: werf\n    child: werf\n",
		"templates/cm.yaml":                          template,
		"charts/sub/Chart.yaml":                      "apiVersion: v2\nname: sub\nversion: 1.0.0\ndependencies:\n- name: subsub\n  version: 1.0.0\n  export-values:\n  - parent: werf\n    child: werf\n",
		"charts/sub/templates/cm.yaml":               template,
		"charts/sub/charts/subsub/Chart.yaml":        "apiVersion: v2\nname: subsub\nversion: 1.0.0\n",
		"charts/sub/charts/subsub/templates/cm.yaml": template,
	}
}

// werfDataVersionedChart returns the files of a chart of the Chart.yaml apiVersion with a dependency
// "sub".
func werfDataVersionedChart(apiVersion string) map[string]string {
	files := map[string]string{
		"Chart.yaml":             "apiVersion: " + apiVersion + "\nname: root\nversion: 1.0.0\n",
		"values.yaml":            "a: chart",
		"secret-values.yaml":     "b: chart-secret",
		"charts/sub/Chart.yaml":  "apiVersion: " + apiVersion + "\nname: sub\nversion: 1.0.0\n",
		"charts/sub/values.yaml": "x: sub",
	}

	dependencies := "dependencies:\n- name: sub\n  version: 1.0.0\n"
	if apiVersion == "v1" {
		files["requirements.yaml"] = dependencies
	} else {
		files["Chart.yaml"] += dependencies
	}

	return files
}

// werfServiceValuesTestChart returns the files of a chart "root" with a dependency "sub" with a
// dependency "subsub", each rendering a ConfigMap named after the chart. The dependency fields are
// appended to the dependency entries of root and sub.
func werfServiceValuesTestChart(rootDependencyFields, subDependencyFields, subValues string) map[string]string {
	const template = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Chart.Name }}\n"

	return map[string]string{
		"Chart.yaml":                                 "apiVersion: v2\nname: root\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n" + rootDependencyFields,
		"templates/cm.yaml":                          template,
		"charts/sub/Chart.yaml":                      "apiVersion: v2\nname: sub\nversion: 1.0.0\ndependencies:\n- name: subsub\n  version: 1.0.0\n" + subDependencyFields,
		"charts/sub/values.yaml":                     subValues,
		"charts/sub/templates/cm.yaml":               template,
		"charts/sub/charts/subsub/Chart.yaml":        "apiVersion: v2\nname: subsub\nversion: 1.0.0\n",
		"charts/sub/charts/subsub/templates/cm.yaml": template,
	}
}

// werfStubServiceValues returns the service values werf passes to werf render without images built,
// for an image "app" and the environment, none if empty.
func werfStubServiceValues(env string) string {
	var werfEnv, globalWerfEnv string
	if env != "" {
		werfEnv = "\n  env: " + env
		globalWerfEnv = "\n    env: " + env
	}

	return fmt.Sprintf(`werf:
  name: proj
  repo: REPO
  is_stub: true
  stub_image: REPO:TAG%s
  image:
    app: REPO:TAG
  tag:
    app: TAG
global:
  werf:
    name: proj
    repo: REPO
    is_stub: true
    stub_image: REPO:TAG%s
    images:
      app:
        registry: REGISTRY
        repository: NAMESPACE/NAME
        tag: TAG
        image: REPO
`, werfEnv, globalWerfEnv)
}

func writeTestFile(t *testing.T, filePath, data string) string {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, []byte(data), 0o644))

	return filePath
}
