package chart

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

func TestRenderChart_WerfDataBelowDeployOptions(t *testing.T) {
	setFilePath := filepath.Join(t.TempDir(), "set-file")
	writeTestFile(t, setFilePath, "deploy")

	tests := []struct {
		name              string
		chart             map[string]string
		published         werfDataFlags
		deployFlags       werfDataFlags
		deployOpts        RenderChartOptions
		want              map[string]interface{}
		wantRootContext   map[string]interface{}
		wantReleaseConfig string
	}{
		{
			name:        "deploy --set patches a list from publish --values",
			chart:       werfDataCardChart("", "", "", ""),
			published:   werfDataFlags{Values: "l:\n- values-0\n- values-1"},
			deployFlags: werfDataFlags{Set: []string{"l[1]=set"}},
			want:        map[string]interface{}{"l": []interface{}{"values-0", "set"}},
		},
		{
			name:        "deploy --values beat publish --set",
			chart:       werfDataCardChart("", "", "", ""),
			published:   werfDataFlags{Set: []string{"k=published-set"}},
			deployFlags: werfDataFlags{Values: "k: deploy-values"},
			want:        map[string]interface{}{"k": "deploy-values"},
		},
		{
			name:        "deploy --values beat publish extra values",
			chart:       werfDataCardChart("", "", "", ""),
			published:   werfDataFlags{ExtraValues: "werf:\n  env: published"},
			deployFlags: werfDataFlags{Values: "werf:\n  env: deploy"},
			want:        map[string]interface{}{"werf.env": "deploy"},
		},
		{
			name:        "deploy --secret-values beat publish --secret-values beating the chart secret-values.yaml",
			chart:       werfDataCardChart("", "", "a: chart-secret\nb: chart-secret\nc: chart-secret", ""),
			published:   werfDataFlags{SecretValues: "b: published-secret\nc: published-secret"},
			deployFlags: werfDataFlags{SecretValues: "c: deploy-secret"},
			want:        map[string]interface{}{"a": "chart-secret", "b": "published-secret", "c": "deploy-secret"},
		},
		{
			name:       "publish --set-file keeps its stored contents when deploy --set-file reads the same path",
			chart:      werfDataCardChart("", "", "", ""),
			published:  werfDataFlags{SetFile: []werfDataSetFile{{Key: "published", Contents: "published", Path: setFilePath}}},
			deployOpts: RenderChartOptions{ValuesOptions: common.ValuesOptions{ValuesSetFile: []string{"deploy=" + setFilePath}}},
			want:       map[string]interface{}{"published": "published", "deploy": "deploy"},
		},
		{
			name:            "deploy --set-root-json is applied after publish --set-root-json",
			chart:           werfDataCardChart("", "", "", ""),
			published:       werfDataFlags{SetRootJSON: []string{`root.a="published"`, `root.b="published"`}},
			deployFlags:     werfDataFlags{SetRootJSON: []string{`root.a="deploy"`}},
			wantRootContext: map[string]interface{}{"root": map[string]interface{}{"a": "deploy", "b": "published"}},
		},
		{
			name:        "deploy --disable-default-values drops only the chart values.yaml",
			chart:       werfDataCardChart("", "a: chart", "", ""),
			published:   werfDataFlags{Values: "b: published"},
			deployFlags: werfDataFlags{DefaultValuesDisable: true},
			want:        map[string]interface{}{"a": absentValue{}, "b": "published"},
		},
		{
			name:              "release config has the values of both groups without extra values",
			chart:             werfDataCardChart("", "a: chart", "b: chart-secret", ""),
			published:         werfDataFlags{Values: "c: published", SecretValues: "d: published-secret", Set: []string{"e=published"}, ExtraValues: "werf:\n  repo: published"},
			deployFlags:       werfDataFlags{Values: "f: deploy", SecretValues: "g: deploy-secret", Set: []string{"h=deploy"}, ExtraValues: "werf:\n  env: deploy"},
			want:              map[string]interface{}{"werf.repo": "published", "werf.env": "deploy"},
			wantReleaseConfig: "c: published\nd: published-secret\ne: published\nf: deploy\ng: deploy-secret\nh: deploy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPath := writeWerfDataTestChart(t, tt.chart, werfDataFlagsAsFiles(t, tt.published))

			result, err := renderWerfDataTestChart(t, chartPath, werfDataFlagsAsOptions(t, tt.deployFlags, tt.deployOpts))
			require.NoError(t, err)

			vals := renderedChartValues(t, result)
			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}

			for key, want := range tt.wantRootContext {
				assert.Equal(t, want, result.Values[key], key)
			}

			if tt.wantReleaseConfig != "" {
				assert.Equal(t, parseTestValues(t, tt.wantReleaseConfig), result.ReleaseConfig)
			}
		})
	}
}

func TestRenderChart_WerfDataDefaultSecretValuesDisableWithoutSecretKey(t *testing.T) {
	chart := werfDataCardChart("", "a: chart", "b: chart-secret", "")
	flags := werfDataFlags{DefaultSecretValuesDisable: true, Values: "c: values"}

	tests := []struct {
		name      string
		secretKey string
	}{
		{name: "no secret key"},
		{name: "another secret key", secretKey: "0123456789abcdef0123456789abcdef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceChartPath := writeWerfDataTestChart(t, chart)
			publishedChartPath := writeWerfDataTestChart(t, chart, werfDataFlagsAsFiles(t, flags))
			flagsOpts := werfDataFlagsAsOptions(t, flags, RenderChartOptions{})

			fromOptions, err := renderWerfDataTestChartWithSecretKey(t, sourceChartPath, tt.secretKey, flagsOpts)
			require.NoError(t, err)

			fromWerfData, err := renderWerfDataTestChartWithSecretKey(t, publishedChartPath, tt.secretKey, RenderChartOptions{})
			require.NoError(t, err)

			assert.Equal(t, parseTestValues(t, "a: chart\nc: values"), renderedChartValues(t, fromWerfData))
			assert.Equal(t, renderedChartValues(t, fromOptions), renderedChartValues(t, fromWerfData))
		})
	}
}

func TestRenderChart_WerfDataDependencyBelowParentValues(t *testing.T) {
	published := mergeTestFiles(map[string]string{
		"charts/pub/Chart.yaml":            "apiVersion: v2\nname: pub\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x\n",
		"charts/pub/values.yaml":           "own: pub-values",
		"charts/pub/charts/sub/Chart.yaml": "apiVersion: v2\nname: sub\nversion: 1.0.0\n",
	}, prefixTestFiles("charts/pub/", werfDataFlagsAsFiles(t, werfDataFlags{
		Set:    []string{"published=pub-set"},
		Values: "own: pub-published\nexported:\n  k: pub-published\ng: pub-published",
	})))

	tests := []struct {
		name         string
		dependencies string
		values       string
		secretValues string
		want         map[string]interface{}
		// nested puts the dependency into a dependency "mid" that imports its "exported" values.
		nested bool
	}{
		{
			name:         "an export of the parent values",
			dependencies: "- name: pub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: own",
			values:       "g: parent-export",
			want:         map[string]interface{}{"pub.own": "parent-export", "pub.published": "pub-set"},
		},
		{ //nolint:gosec // Test values, not credentials.
			name:         "the parent secret-values.yaml",
			dependencies: "- name: pub\n  version: 1.0.0",
			secretValues: "pub:\n  own: parent-secret\n  published: parent-secret",
			want:         map[string]interface{}{"pub.own": "parent-secret", "pub.published": "parent-secret"},
		},
		{
			name:         "the parent values at an import-values destination",
			dependencies: "- name: pub\n  version: 1.0.0\n  import-values:\n  - child: exported\n    parent: imported",
			values:       "imported:\n  k: parent",
			want:         map[string]interface{}{"imported.k": "parent", "pub.exported.k": "pub-published"},
		},
		{
			name:         "the parent values at an import-values destination of a nested dependency",
			dependencies: "- name: mid\n  version: 1.0.0",
			values:       "mid:\n  imported:\n    k: parent",
			want:         map[string]interface{}{"mid.imported.k": "parent", "mid.pub.exported.k": "pub-published"},
			nested:       true,
		},
		{
			name:         "an export-values of the dependency beats the parent values, like its values.yaml does",
			dependencies: "- name: pub\n  version: 1.0.0",
			values:       "pub:\n  sub:\n    x: parent",
			want:         map[string]interface{}{"pub.sub.x": "pub-published"},
		},
		{
			name:         "a disabled dependency",
			dependencies: "- name: pub\n  version: 1.0.0\n  condition: pub.enabled",
			values:       "pub:\n  enabled: false",
			want:         map[string]interface{}{"pub": map[string]interface{}{"enabled": false}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"Chart.yaml":  "apiVersion: v2\nname: root\nversion: 1.0.0\ndependencies:\n" + tt.dependencies + "\n",
				"values.yaml": tt.values,
			}
			if tt.secretValues != "" {
				files["secret-values.yaml"] = tt.secretValues
			}

			dependencyFiles := published
			if tt.nested {
				dependencyFiles = prefixTestFiles("charts/mid/", published)
				dependencyFiles["charts/mid/Chart.yaml"] = "apiVersion: v2\nname: mid\nversion: 1.0.0\ndependencies:\n- name: pub\n  version: 1.0.0\n  import-values:\n  - child: exported\n    parent: imported\n"
			}

			result, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, dependencyFiles, files), RenderChartOptions{})
			require.NoError(t, err)

			vals := renderedChartValues(t, result)
			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}
		})
	}
}

func TestRenderChart_WerfDataDependencyDefaultSecretValuesDisable(t *testing.T) {
	chartPath := writeWerfDataTestChart(t, map[string]string{ //nolint:gosec // Test values, not credentials.
		"Chart.yaml":                                 "apiVersion: v2\nname: root\nversion: 1.0.0\ndependencies:\n- name: pub\n  version: 1.0.0\n- name: sibling\n  version: 1.0.0\n",
		"charts/pub/Chart.yaml":                      "apiVersion: v2\nname: pub\nversion: 1.0.0\ndependencies:\n- name: child\n  version: 1.0.0\n",
		"charts/pub/secret-values.yaml":              "own: pub-secret",
		"charts/pub/charts/child/Chart.yaml":         "apiVersion: v2\nname: child\nversion: 1.0.0\n",
		"charts/pub/charts/child/values.yaml":        "k: child",
		"charts/pub/charts/child/secret-values.yaml": "k: child-secret\ns: child-secret",
		"charts/sibling/Chart.yaml":                  "apiVersion: v2\nname: sibling\nversion: 1.0.0\n",
		"charts/sibling/secret-values.yaml":          "s: sibling-secret",
	}, prefixTestFiles("charts/pub/", werfDataFlagsAsFiles(t, werfDataFlags{ //nolint:gosec // Test values, not credentials.
		DefaultSecretValuesDisable: true,
		SecretValues:               "custom: pub-published-secret",
	})))

	result, err := renderWerfDataTestChart(t, chartPath, RenderChartOptions{})
	require.NoError(t, err)

	vals := renderedChartValues(t, result)
	for path, want := range map[string]interface{}{
		"pub.own":     absentValue{},
		"pub.custom":  "pub-published-secret",
		"pub.child.k": "child",
		"pub.child.s": absentValue{},
		"sibling.s":   "sibling-secret",
	} {
		assert.Equal(t, want, valueAtPath(t, vals, path), path)
	}
}

func TestRenderChart_WerfDataDependencyDefaultSecretValuesDisableWithoutSecretKey(t *testing.T) {
	const otherSecretKey = "0123456789abcdef0123456789abcdef"

	tests := []struct {
		name          string
		apiVersion    string
		deploySecrets bool
		secretKey     string
	}{
		{name: "no secret key", apiVersion: "v2"},
		{name: "another secret key", apiVersion: "v2", deploySecrets: true, secretKey: layeredValuesSecretKey},
		{name: "no secret key with chart apiVersion v3", apiVersion: "v3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files := map[string]string{
				"Chart.yaml":                          "apiVersion: " + tt.apiVersion + "\nname: root\nversion: 1.0.0\ndependencies:\n- name: pub\n  version: 1.0.0\n- name: sibling\n  version: 1.0.0\n",
				"charts/pub/Chart.yaml":               "apiVersion: " + tt.apiVersion + "\nname: pub\nversion: 1.0.0\ndependencies:\n- name: child\n  version: 1.0.0\n",
				"charts/pub/charts/child/Chart.yaml":  "apiVersion: " + tt.apiVersion + "\nname: child\nversion: 1.0.0\n",
				"charts/pub/charts/child/values.yaml": "k: child",
				"charts/sibling/Chart.yaml":           "apiVersion: " + tt.apiVersion + "\nname: sibling\nversion: 1.0.0\n",
			}
			flags := werfDataFlags{DefaultSecretValuesDisable: true}
			want := map[string]interface{}{"pub.own": absentValue{}, "pub.child.k": "child", "pub.child.s": absentValue{}}

			if tt.deploySecrets {
				files["charts/sibling/secret-values.yaml"] = "s: sibling-secret"
				flags.SecretValues = "custom: pub-published-secret"
				want["sibling.s"] = "sibling-secret"
				want["pub.custom"] = "pub-published-secret"
			}

			chartPath := writeWerfDataTestChart(t, files, prefixTestFiles("charts/pub/", werfDataFlagsAsFiles(t, flags)))
			writeTestFile(t, filepath.Join(chartPath, "charts/pub/secret-values.yaml"), encryptTestValuesWithKey(t, otherSecretKey, "own: pub-secret"))
			writeTestFile(t, filepath.Join(chartPath, "charts/pub/charts/child/secret-values.yaml"), encryptTestValuesWithKey(t, otherSecretKey, "s: child-secret"))

			result, err := renderWerfDataTestChartWithSecretKey(t, chartPath, tt.secretKey, RenderChartOptions{})
			require.NoError(t, err)

			vals := renderedChartValues(t, result)
			for path, want := range want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}
		})
	}
}

// TestRenderChart_WerfDataDependencyEquivalence renders a published chart alone and as a dependency,
// and expects the dependency to get the same values.
func TestRenderChart_WerfDataDependencyEquivalence(t *testing.T) {
	tests := []struct {
		name  string
		chart map[string]string
		flags werfDataFlags
	}{
		{
			name:  "--set beats an export from the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "g: chart", "", ""),
			flags: werfDataFlags{Set: []string{"sub.x=set"}},
		},
		{
			name:  "an export from --values beats the dependency values",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "", "x: sub"),
			flags: werfDataFlags{Values: "g: values"},
		},
		{
			name:  "an import from --values beats the own key of the parent in the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported", "imported:\n  k: chart", "", ""),
			flags: werfDataFlags{Values: "sub:\n  fromchild:\n    k: values"},
		},
		{
			name:  "--set patches a list from --secret-values",
			chart: werfDataCardChart("", "", "", ""),
			flags: werfDataFlags{SecretValues: "l:\n- custom-secret-0\n- custom-secret-1", Set: []string{"l[1]=set"}}, //nolint:gosec // Test values, not credentials.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			published := mergeTestFiles(tt.chart, werfDataFlagsAsFiles(t, tt.flags))
			published["Chart.yaml"] = strings.Replace(published["Chart.yaml"], "name: root", "name: pub", 1)

			alone, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, published), RenderChartOptions{})
			require.NoError(t, err)

			aloneValues := renderedChartValues(t, alone)
			delete(aloneValues, "global")

			for _, layout := range []struct {
				files   map[string]string
				pubPath string
			}{
				{
					files: mergeTestFiles(prefixTestFiles("charts/pub/", published), map[string]string{
						"Chart.yaml": "apiVersion: v2\nname: parent\nversion: 1.0.0\ndependencies:\n- name: pub\n  version: 1.0.0\n",
					}),
					pubPath: "pub",
				},
				{
					files: mergeTestFiles(prefixTestFiles("charts/mid/charts/pub/", published), map[string]string{
						"Chart.yaml":            "apiVersion: v2\nname: parent\nversion: 1.0.0\ndependencies:\n- name: mid\n  version: 1.0.0\n",
						"charts/mid/Chart.yaml": "apiVersion: v2\nname: mid\nversion: 1.0.0\ndependencies:\n- name: pub\n  version: 1.0.0\n",
					}),
					pubPath: "mid.pub",
				},
			} {
				asDependency, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, layout.files), RenderChartOptions{})
				require.NoError(t, err)

				dependencyValues, ok := valueAtPath(t, renderedChartValues(t, asDependency), layout.pubPath).(map[string]interface{})
				require.True(t, ok, layout.pubPath)
				delete(dependencyValues, "global")

				assert.Equal(t, aloneValues, dependencyValues, layout.pubPath)
				assert.Empty(t, asDependency.ReleaseConfig, layout.pubPath)
			}
		})
	}
}

func TestRenderChart_WerfDataDependencyFolding(t *testing.T) {
	publishedDependency := mergeTestFiles(
		map[string]string{ //nolint:gosec // Test values, not credentials.
			"charts/pub/Chart.yaml":                "apiVersion: v2\nname: pub\nversion: 1.0.0\ndependencies:\n- name: subsub\n  version: 1.0.0\n  condition: subsub.enabled\n",
			"charts/pub/values.yaml":               "own: pub-values\nowned: pub-values\n",
			"charts/pub/secret-values.yaml":        "owned: pub-secret\nshadowed: pub-secret\n",
			"charts/pub/charts/subsub/Chart.yaml":  "apiVersion: v2\nname: subsub\nversion: 1.0.0\n",
			"charts/pub/charts/subsub/values.yaml": "k: subsub\n",
			"charts/pub/templates/cm.yaml":         "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Chart.Name }}-{{ .Values.own }}\n",
		},
		prefixTestFiles("charts/pub/", werfDataFlagsAsFiles(t, werfDataFlags{ //nolint:gosec // Test values, not credentials.
			ExtraValues:  "werf:\n  repo: pub-service\nglobal:\n  werf:\n    repo: pub-service",
			SecretValues: "secret: pub-published-secret",
			Set:          []string{"l[1]=pub-set"},
			Values:       "published: pub-published\nown: pub-published\nshadowed: pub-published\nsubsub:\n  enabled: false\nl:\n- pub-published-0\n- pub-published-1",
		})),
	)

	tests := []struct {
		name         string
		dependencies string
		values       string
		want         map[string]interface{}
		wantDeps     []string
	}{
		{
			name:         "published values become the dependency defaults",
			dependencies: "- name: pub\n  version: 1.0.0",
			want: map[string]interface{}{ //nolint:gosec // Test values, not credentials.
				"pub.own":            "pub-published",
				"pub.owned":          "pub-secret",
				"pub.published":      "pub-published",
				"pub.shadowed":       "pub-published",
				"pub.secret":         "pub-published-secret",
				"pub.l":              []interface{}{"pub-published-0", "pub-set"},
				"pub.werf.repo":      "pub-service",
				"pub.global.werf":    map[string]interface{}{"repo": "pub-service"},
				"pub.subsub.enabled": false,
				"pub.subsub.k":       absentValue{},
			},
			wantDeps: []string{"pub"},
		},
		{
			name:         "parent values override the folded values",
			dependencies: "- name: pub\n  version: 1.0.0",
			values:       "pub:\n  own: parent\n  secret: parent\nglobal:\n  werf:\n    repo: parent",
			want: map[string]interface{}{
				"pub.own":         "parent",
				"pub.secret":      "parent",
				"pub.published":   "pub-published",
				"pub.global.werf": map[string]interface{}{"repo": "parent"},
			},
			wantDeps: []string{"pub"},
		},
		{
			name:         "parent re-enables a sub-dependency disabled by the folded values",
			dependencies: "- name: pub\n  version: 1.0.0",
			values:       "pub:\n  subsub:\n    enabled: true",
			want:         map[string]interface{}{"pub.subsub.k": "subsub"},
			wantDeps:     []string{"pub"},
		},
		{
			name:         "two aliases of the published dependency get the folded values",
			dependencies: "- name: pub\n  alias: one\n  version: 1.0.0\n- name: pub\n  alias: two\n  version: 1.0.0",
			values:       "two:\n  own: parent",
			want: map[string]interface{}{ //nolint:gosec // Test values, not credentials.
				"one.own":       "pub-published",
				"one.secret":    "pub-published-secret",
				"two.own":       "parent",
				"two.published": "pub-published",
				"two.secret":    "pub-published-secret",
			},
			wantDeps: []string{"one", "two"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chartPath := writeWerfDataTestChart(t, publishedDependency, map[string]string{
				"Chart.yaml":  "apiVersion: v2\nname: root\nversion: 1.0.0\ndependencies:\n" + tt.dependencies + "\n",
				"values.yaml": tt.values,
			})

			result, err := renderWerfDataTestChart(t, chartPath, RenderChartOptions{})
			require.NoError(t, err)

			vals := renderedChartValues(t, result)
			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}

			assert.Equal(t, tt.wantDeps, dependencyNames(t, result))
			assert.Empty(t, result.ReleaseConfig)
		})
	}
}

func TestRenderChart_WerfDataDependencyMetadataIgnored(t *testing.T) {
	tests := []struct {
		name         string
		metadata     string
		files        map[string]string
		wantWarnings []string
	}{
		{
			name:         "annotations and labels",
			metadata:     "apiVersion: v2\nextraAnnotations:\n  team: payments\nextraLabels:\n  tier: backend\n",
			wantWarnings: []string{`Ignoring annotations and labels in .werf-data of dependency "pub": only those of the root chart are applied`},
		},
		{
			name:         "patches, root JSON and release defaults",
			metadata:     "apiVersion: v2\npatches: [patches/0.yaml]\ndefaultPatchesDisable: true\nvalues:\n  valuesSetRootJSON: ['a={}']\nrelease:\n  namespace: ns\n",
			files:        map[string]string{"charts/pub/.werf-data/patches/0.yaml": "render: []"},
			wantWarnings: []string{`Ignoring patches, disabled default patches, --set-root-json values, release defaults in .werf-data of dependency "pub": only those of the root chart are applied`},
		},
		{
			name:         "no annotations or labels",
			metadata:     "apiVersion: v2\n",
			wantWarnings: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &warningsLogger{Logger: log.Default}
			log.Default = logger
			t.Cleanup(func() { log.Default = logger.Logger })

			chartPath := writeWerfDataTestChart(t, map[string]string{
				"charts/pub/Chart.yaml":               "apiVersion: v2\nname: pub\nversion: 1.0.0\n",
				"charts/pub/.werf-data/metadata.yaml": tt.metadata,
			}, map[string]string{
				"Chart.yaml": "apiVersion: v2\nname: root\nversion: 1.0.0\ndependencies:\n- name: pub\n  version: 1.0.0\n",
			}, tt.files)

			_, err := renderWerfDataTestChart(t, chartPath, RenderChartOptions{})
			require.NoError(t, err)

			assert.Equal(t, tt.wantWarnings, lo.Filter(logger.warnings, func(w string, _ int) bool {
				return strings.Contains(w, werfdata.DirName)
			}))
		})
	}
}

// TestRenderChart_WerfDataEquivalence renders each chart with the flags passed as the caller's
// options and with the same flags stored in .werf-data, and expects the same values and release
// config.
func TestRenderChart_WerfDataEquivalence(t *testing.T) {
	tests := []struct {
		name  string
		chart map[string]string
		flags werfDataFlags
		// deployOpts are passed to both renders.
		deployOpts RenderChartOptions
		want       string
	}{
		{
			name:  "1: --set beats an export from the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "g: chart", "", ""),
			flags: werfDataFlags{Set: []string{"sub.x=set"}},
			want:  "g: chart\nsub:\n  global: {}\n  x: set",
		},
		{
			name:  "1: an import from --values beats the own key of the parent in the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported", "imported:\n  k: chart", "", ""),
			flags: werfDataFlags{Values: "sub:\n  fromchild:\n    k: values"},
			want:  "imported:\n  k: values\nsub:\n  fromchild:\n    k: values\n  global: {}",
		},
		{
			name:  "2: an import from --set beats the chart secret-values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported", "", "imported:\n  k: chart-secret", ""),
			flags: werfDataFlags{Set: []string{"sub.fromchild.k=set"}},
			want:  "imported:\n  k: set\nsub:\n  fromchild:\n    k: set\n  global: {}",
		},
		{
			name:  "3: --secret-values beat an export from the chart secret-values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "g: chart-secret", ""),
			flags: werfDataFlags{SecretValues: "sub:\n  x: custom-secret"}, //nolint:gosec // Test values, not credentials.
			want:  "g: chart-secret\nsub:\n  global: {}\n  x: custom-secret",
		},
		{
			name:  "3: --values beat an export from the chart secret-values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "g: chart-secret", ""),
			flags: werfDataFlags{Values: "sub:\n  x: values"},
			want:  "g: chart-secret\nsub:\n  global: {}\n  x: values",
		},
		{
			name:  "4: an export to a disabled dependency keeps the secret values",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  condition: sub.enabled\n  export-values:\n  - parent: g\n    child: x", "sub:\n  enabled: false", "sub:\n  x: chart-secret", ""),
			flags: werfDataFlags{Set: []string{"g=set"}},
			want:  "g: set\nsub:\n  enabled: false\n  x: chart-secret",
		},
		{
			name:  "5: --set null deletes the values exported from --secret-values",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "sub:\n  a: chart", "", "own: sub"),
			flags: werfDataFlags{SecretValues: "g: custom-secret", Set: []string{"sub=null"}},
			want:  "g: custom-secret\nsub:\n  global: {}\n  own: sub",
		},
		{
			name:  "6: --set-json replaces a map with the values exported from the secret values",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "g: chart-secret", ""),
			flags: werfDataFlags{SecretValues: "g: custom-secret", SetJSON: []string{`sub={"z":1}`}},
			want:  "g: custom-secret\nsub:\n  global: {}\n  x: chart-secret\n  z: 1",
		},
		{
			name:  "7: a --secret-values null replaces a map exported from --values",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "", ""),
			flags: werfDataFlags{Values: "g:\n  k: values", SecretValues: "g: null"},
			want:  "g: null\nsub:\n  global: {}\n  x: null",
		},
		{
			name:  "8: --set patches a list from --secret-values",
			chart: werfDataCardChart("", "", "", ""),
			flags: werfDataFlags{SecretValues: "l:\n- custom-secret-0\n- custom-secret-1", Set: []string{"l[1]=set"}}, //nolint:gosec // Test values, not credentials.
			want:  "l:\n- custom-secret-0\n- set",
		},
		{
			name:  "9: a null in --values for a key absent in the charts is kept",
			chart: werfDataCardChart("", "", "", ""),
			flags: werfDataFlags{Values: "unknown: null\nknown: values"},
			want:  "known: values\nunknown: null",
		},
		{
			name:  "10: --values replace a null in the chart secret-values.yaml",
			chart: werfDataCardChart("", "a:\n  c: chart", "a: null", ""),
			flags: werfDataFlags{Values: "a:\n  b: values"},
			want:  "a:\n  b: values",
		},
		{
			name:       "11: --values overridden by --secret-values are kept without the chart secret-values.yaml",
			chart:      werfDataCardChart("", "", "b: chart-secret", ""),
			flags:      werfDataFlags{Values: "a: values", SecretValues: "a: custom-secret"},
			deployOpts: RenderChartOptions{HelmOptions: common.HelmOptions{ChartLoadOpts: common.ChartLoadOptions{DefaultSecretValuesDisable: true}}},
			want:       "a: custom-secret",
		},
		{
			name:  "12: --set null deletes a value of the dependency",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0", "", "", "key: sub\nother: sub"),
			flags: werfDataFlags{Set: []string{"sub.key=null"}},
			want:  "sub:\n  global: {}\n  other: sub",
		},
		{
			name:  "service values",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "g: chart", "", ""),
			flags: werfDataFlags{Set: []string{"sub.x=set"}, ExtraValues: "werf:\n  repo: service\nsub:\n  x: service"},
			want:  "g: chart\nsub:\n  global: {}\n  x: service\nwerf:\n  repo: service",
		},
		{
			name:  "--set-string with --set-literal and --set-file whose file is absent at deploy",
			chart: werfDataCardChart("", "", "", ""),
			flags: werfDataFlags{
				SetFile:    []werfDataSetFile{{Key: "file", Contents: "from-file"}},
				SetLiteral: []string{"literal={a,b}"},
				SetString:  []string{"str=1"},
				Values:     "file: values",
			},
			want: "file: from-file\nliteral: '{a,b}'\nstr: \"1\"",
		},
		{
			name:  "--set-root-json",
			chart: werfDataCardChart("", "", "", ""),
			flags: werfDataFlags{SetRootJSON: []string{`root={"a":"set"}`}},
			want:  "{}",
		},
		{
			name:  "--disable-default-values",
			chart: werfDataCardChart("", "a: chart", "b: chart-secret", ""),
			flags: werfDataFlags{DefaultValuesDisable: true, Values: "c: values"},
			want:  "b: chart-secret\nc: values",
		},
		{
			name:  "--disable-default-secret-values applies to subcharts",
			chart: werfDataCardChartWithSubchartSecrets("a: chart", "b: chart-secret", "s: sub", "t: sub-secret"),
			flags: werfDataFlags{DefaultSecretValuesDisable: true, Set: []string{"c=set"}},
			want:  "a: chart\nc: set\nsub:\n  global: {}\n  s: sub",
		},
		{
			name:  "chart apiVersion v3",
			chart: werfDataVersionedChart("v3"),
			flags: werfDataFlags{Values: "l:\n- values-0\n- values-1", SecretValues: "s: custom-secret", Set: []string{"l[1]=set"}, ExtraValues: "werf:\n  repo: service"},
			want:  "a: chart\nb: chart-secret\nl:\n- values-0\n- set\ns: custom-secret\nsub:\n  global: {}\n  x: sub\nwerf:\n  repo: service",
		},
		{
			name: "chart apiVersion v3 imports from --secret-values and extra values",
			chart: map[string]string{
				"Chart.yaml":             "apiVersion: v3\nname: root\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported\n",
				"charts/sub/Chart.yaml":  "apiVersion: v3\nname: sub\nversion: 1.0.0\n",
				"charts/sub/values.yaml": "fromchild:\n  a: sub\n  b: sub\n  c: sub",
			},
			flags: werfDataFlags{SecretValues: "sub:\n  fromchild:\n    a: custom-secret", ExtraValues: "sub:\n  fromchild:\n    b: service"}, //nolint:gosec // Test values, not credentials.
			want:  "imported:\n  a: custom-secret\n  b: service\n  c: sub\nsub:\n  fromchild:\n    a: custom-secret\n    b: service\n    c: sub\n  global: {}",
		},
		{
			name:  "chart apiVersion v1",
			chart: werfDataVersionedChart("v1"),
			flags: werfDataFlags{Values: "l:\n- values-0\n- values-1", SecretValues: "s: custom-secret", Set: []string{"l[1]=set"}, ExtraValues: "werf:\n  repo: service"},
			want:  "a: chart\nb: chart-secret\nl:\n- values-0\n- set\ns: custom-secret\nsub:\n  global: {}\n  x: sub\nwerf:\n  repo: service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceChartPath := writeWerfDataTestChart(t, tt.chart)
			fromOptions, err := renderWerfDataTestChart(t, sourceChartPath, werfDataFlagsAsOptions(t, tt.flags, tt.deployOpts))
			require.NoError(t, err)

			publishedChartPath := writeWerfDataTestChart(t, tt.chart, werfDataFlagsAsFiles(t, tt.flags))
			fromWerfData, err := renderWerfDataTestChart(t, publishedChartPath, tt.deployOpts)
			require.NoError(t, err)

			assert.Equal(t, parseTestValues(t, tt.want), renderedChartValues(t, fromOptions))
			assert.Equal(t, renderedChartValues(t, fromOptions), renderedChartValues(t, fromWerfData))
			assert.Equal(t, fromOptions.ReleaseConfig, fromWerfData.ReleaseConfig)

			for key, value := range fromOptions.Values {
				if _, isValues := value.(chartcommon.Values); isValues {
					continue
				}

				assert.Equal(t, value, fromWerfData.Values[key], key)
			}
		})
	}
}

func TestRenderChart_WerfDataIgnoreSecretKey(t *testing.T) {
	chart := werfDataCardChart("", "", "", "")
	flags := werfDataFlags{SecretValues: "s: custom-secret"}
	ignoreSecretKeyOpts := RenderChartOptions{HelmOptions: common.HelmOptions{ChartLoadOpts: common.ChartLoadOptions{SecretKeyIgnore: true}}}

	publishedChartPath := writeWerfDataTestChart(t, chart, werfDataFlagsAsFiles(t, flags))
	result, err := renderWerfDataTestChart(t, publishedChartPath, ignoreSecretKeyOpts)
	require.NoError(t, err)

	ciphertext := parseTestValues(t, readTestFile(t, filepath.Join(publishedChartPath, werfdata.DirName, werfdata.SecretValuesFileName)))["s"]
	assert.NotEqual(t, "custom-secret", ciphertext)
	assert.Equal(t, ciphertext, renderedChartValues(t, result)["s"])
}

func TestRenderChart_WerfDataInfoLine(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantLine string
	}{
		{
			name: "values and metadata",
			files: map[string]string{
				".werf-data/metadata.yaml": `apiVersion: v2
values:
  values: true
  valuesSet: ["a=1", "b=2"]
  defaultSecretValuesDisable: true
patches: ["patches/0.yaml"]
defaultPatchesDisable: true
release:
  name: rel
extraAnnotations:
  team: payments
extraLabels:
  tier: backend
extraRuntimeAnnotations:
  owner: secret-owner
releaseInfoAnnotations:
  info: secret-info
`,
				".werf-data/values.yaml":    "a: values",
				".werf-data/patches/0.yaml": "render: []",
			},
			wantLine: "Using publish-time inputs from .werf-data: values, 2 --set, default secret values disabled, 1 patches files, default patches disabled, annotations, labels, runtime annotations, release info annotations, release defaults",
		},
		{
			name:     "nothing",
			files:    map[string]string{".werf-data/metadata.yaml": "apiVersion: v2\n"},
			wantLine: "Using publish-time inputs from .werf-data: nothing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &warningsLogger{Logger: log.Default}
			log.Default = logger
			t.Cleanup(func() { log.Default = logger.Logger })

			chartPath := writeWerfDataTestChart(t, werfDataCardChart("", "", "", ""), tt.files)

			_, err := renderWerfDataTestChart(t, chartPath, RenderChartOptions{})
			require.NoError(t, err)

			assert.Equal(t, []string{tt.wantLine}, lo.Filter(logger.infos, func(line string, _ int) bool {
				return strings.Contains(line, werfdata.DirName)
			}))
		})
	}
}

func TestRenderChart_WerfDataLoadsChartOnce(t *testing.T) {
	const loadWarning = "chart loaded"

	tests := []struct {
		name  string
		files map[string]string
	}{
		{name: "without .werf-data"},
		{name: "with .werf-data", files: werfDataFlagsAsFiles(t, werfDataFlags{DefaultSecretValuesDisable: true, DefaultValuesDisable: true})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &warningsLogger{Logger: log.Default}
			log.Default = logger
			t.Cleanup(func() { log.Default = logger.Logger })

			prevWarning := loader.NoChartLockWarning
			loader.NoChartLockWarning = loadWarning
			t.Cleanup(func() { loader.NoChartLockWarning = prevWarning })

			chartPath := writeWerfDataTestChart(t, werfDataCardChart("- name: sub\n  version: 1.0.0", "a: chart", "b: chart-secret", ""), tt.files)

			_, err := renderWerfDataTestChart(t, chartPath, RenderChartOptions{})
			require.NoError(t, err)

			assert.Equal(t, 1, lo.Count(logger.warnings, loadWarning))
		})
	}
}

func TestRenderChart_WerfDataSchemaValidation(t *testing.T) {
	strictRootSchema := `{"type":"object","additionalProperties":false,"properties":{"foo":{"type":"string"},"sub":{"type":"object"}}}`
	strictSubSchema := `{"type":"object","additionalProperties":false,"properties":{"a":{"type":"string"},"global":{"type":"object"}}}`

	tests := []struct {
		name          string
		chart         map[string]string
		wantErr       string
		wantNotInErr  string
		wantValuePath string
	}{
		{
			name: "publish extra values are tolerated by a strict root schema",
			chart: mergeTestFiles(
				werfDataCardChart("- name: sub\n  version: 1.0.0", "foo: chart", "", ""),
				map[string]string{"values.schema.json": strictRootSchema},
				werfDataFlagsAsFiles(t, werfDataFlags{Values: "foo: published", ExtraValues: "werf:\n  repo: service\nglobal:\n  werf:\n    repo: service"}),
			),
			wantValuePath: "werf.repo",
		},
		{
			name: "a publish --values typo fails a strict root schema without naming extra values",
			chart: mergeTestFiles(
				werfDataCardChart("- name: sub\n  version: 1.0.0", "foo: chart", "", ""),
				map[string]string{"values.schema.json": strictRootSchema},
				werfDataFlagsAsFiles(t, werfDataFlags{Values: "fooo: typo", ExtraValues: "werf:\n  repo: service"}),
			),
			wantErr:      "fooo",
			wantNotInErr: "werf",
		},
		{
			name: "extra values folded into a published dependency are tolerated by its strict schema",
			chart: mergeTestFiles(
				werfDataCardChart("- name: sub\n  version: 1.0.0", "", "", "a: sub"),
				map[string]string{"charts/sub/values.schema.json": strictSubSchema},
				prefixTestFiles("charts/sub/", werfDataFlagsAsFiles(t, werfDataFlags{Values: "a: published", ExtraValues: "werf:\n  repo: service"})),
			),
			wantValuePath: "sub.werf.repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, tt.chart), RenderChartOptions{})
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.NotContains(t, err.Error(), tt.wantNotInErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, "service", valueAtPath(t, renderedChartValues(t, result), tt.wantValuePath))
		})
	}
}
