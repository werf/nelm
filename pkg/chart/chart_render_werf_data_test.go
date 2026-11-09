package chart

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/v2/pkg/common"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	chartcommonutil "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
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
			name:        "deploy set values patch a list from publish values files",
			chart:       werfDataCardChart("", "", "", ""),
			published:   werfDataFlags{Values: "l:\n- values-0\n- values-1"},
			deployFlags: werfDataFlags{Set: []string{"l[1]=set"}},
			want:        map[string]interface{}{"l": []interface{}{"values-0", "set"}},
		},
		{
			name:              "deploy set values patch a list element set by publish set values, like a single deploy with both",
			chart:             werfDataCardChart("", "", "", ""),
			published:         werfDataFlags{Set: []string{"l[0]=a"}},
			deployFlags:       werfDataFlags{Set: []string{"l[1]=b"}},
			want:              map[string]interface{}{"l": []interface{}{"a", "b"}},
			wantReleaseConfig: "l: [a, b]",
		},
		{
			name:              "deploy set values patch a list from deploy values files, not from publish set values below them",
			chart:             werfDataCardChart("", "", "", ""),
			published:         werfDataFlags{Set: []string{"l[0]=published"}},
			deployFlags:       werfDataFlags{Values: "l: [deploy0, deploy1]", Set: []string{"l[1]=patched"}},
			want:              map[string]interface{}{"l": []interface{}{"deploy0", "patched"}},
			wantReleaseConfig: "l: [deploy0, patched]",
		},
		{
			name:              "deploy set values patch a longer list from deploy values files over a list from publish set values",
			chart:             werfDataCardChart("", "", "", ""),
			published:         werfDataFlags{Set: []string{"l[0]=a"}},
			deployFlags:       werfDataFlags{Values: "l: [v0, v1, v2]", Set: []string{"l[1]=b"}},
			want:              map[string]interface{}{"l": []interface{}{"v0", "b", "v2"}},
			wantReleaseConfig: "l: [v0, b, v2]",
		},
		{
			name:              "deploy set values patch a list from deploy secret values files over a list from publish set values",
			chart:             werfDataCardChart("", "", "", ""),
			published:         werfDataFlags{Set: []string{"l[0]=a"}},
			deployFlags:       werfDataFlags{SecretValues: "l: [v0, v1, v2]", Set: []string{"l[1]=b"}}, //nolint:gosec // Test values, not credentials.
			want:              map[string]interface{}{"l": []interface{}{"v0", "b", "v2"}},
			wantReleaseConfig: "l: [v0, b, v2]",
		},
		{
			name:              "deploy values files replace a map set by publish set values with a scalar",
			chart:             werfDataCardChart("", "", "", ""),
			published:         werfDataFlags{Set: []string{"m.a=1"}},
			deployFlags:       werfDataFlags{Values: "m: scalar"},
			want:              map[string]interface{}{"m": "scalar"},
			wantReleaseConfig: "m: scalar",
		},
		{
			name:        "a deploy set value sets a key of a map in a list set by publish set values, like a single deploy with both",
			chart:       werfDataCardChart("", "", "", ""),
			published:   werfDataFlags{Set: []string{"l[0].a=1"}},
			deployFlags: werfDataFlags{Set: []string{"l[0].b=2"}},
			want:        map[string]interface{}{"l": []interface{}{map[string]interface{}{"a": int64(1), "b": int64(2)}}},
		},
		{
			name:        "deploy string set values patch a list set by publish set values, like a single deploy with both",
			chart:       werfDataCardChart("", "", "", ""),
			published:   werfDataFlags{Set: []string{"l[0]=1"}},
			deployFlags: werfDataFlags{SetString: []string{"l[1]=2"}},
			want:        map[string]interface{}{"l": []interface{}{int64(1), "2"}},
		},
		{
			// Unlike a single deploy with both, which applies all JSON set values before all set values,
			// the deploy set values are applied after the publish ones.
			name:        "deploy JSON set values replace a map set by publish set values",
			chart:       werfDataCardChart("", "", "", ""),
			published:   werfDataFlags{Set: []string{"m.a=1"}},
			deployFlags: werfDataFlags{SetJSON: []string{`m={"b":2}`}},
			want:        map[string]interface{}{"m": map[string]interface{}{"b": float64(2)}},
		},
		{
			// Unlike a single deploy with both, in which all set values beat all values files, every
			// deploy input beats every publish input.
			name:        "deploy values files beat publish set values",
			chart:       werfDataCardChart("", "", "", ""),
			published:   werfDataFlags{Set: []string{"k=published-set"}},
			deployFlags: werfDataFlags{Values: "k: deploy-values"},
			want:        map[string]interface{}{"k": "deploy-values"},
		},
		{
			name:        "deploy values files beat publish extra values",
			chart:       werfDataCardChart("", "", "", ""),
			published:   werfDataFlags{ExtraValues: "werf:\n  env: published"},
			deployFlags: werfDataFlags{Values: "werf:\n  env: deploy"},
			want:        map[string]interface{}{"werf.env": "deploy"},
		},
		{
			name:        "deploy secret values files beat publish secret values files beating the chart secret-values.yaml",
			chart:       werfDataCardChart("", "", "a: chart-secret\nb: chart-secret\nc: chart-secret", ""),
			published:   werfDataFlags{SecretValues: "b: published-secret\nc: published-secret"},
			deployFlags: werfDataFlags{SecretValues: "c: deploy-secret"},
			want:        map[string]interface{}{"a": "chart-secret", "b": "published-secret", "c": "deploy-secret"},
		},
		{
			name:       "a publish set file value keeps its stored contents when a deploy set file value reads the same path",
			chart:      werfDataCardChart("", "", "", ""),
			published:  werfDataFlags{SetFile: []werfDataSetFile{{Key: "published", Contents: "published", Path: setFilePath}}},
			deployOpts: RenderChartOptions{ValuesOptions: common.ValuesOptions{ValuesSetFile: []string{"deploy=" + setFilePath}}},
			want:       map[string]interface{}{"published": "published", "deploy": "deploy"},
		},
		{
			name:            "deploy root JSON values are applied after publish root JSON values",
			chart:           werfDataCardChart("", "", "", ""),
			published:       werfDataFlags{SetRootJSON: []string{`root.a="published"`, `root.b="published"`}},
			deployFlags:     werfDataFlags{SetRootJSON: []string{`root.a="deploy"`}},
			wantRootContext: map[string]interface{}{"root": map[string]interface{}{"a": "deploy", "b": "published"}},
		},
		{
			name:        "deploy disabled default values drop only the chart values.yaml",
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
		"charts/pub/Chart.yaml":            "apiVersion: v2\nname: pub\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x\n  import-values:\n  - child: fromchild\n    parent: imported\n",
		"charts/pub/values.yaml":           "own: pub-values",
		"charts/pub/charts/sub/Chart.yaml": "apiVersion: v2\nname: sub\nversion: 1.0.0\n",
	}, prefixTestFiles("charts/pub/", werfDataFlagsAsFiles(t, werfDataFlags{
		Set:    []string{"published=pub-set"},
		Values: "own: pub-published\nexported:\n  k: pub-published\ng: pub-published\nsub:\n  fromchild:\n    k: pub-published",
	})))

	tests := []struct {
		name         string
		dependencies string
		values       string
		secretValues string
		want         map[string]interface{}
		// dependency replaces the files of the published dependency.
		dependency map[string]string
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
			name:         "the parent values at an import-values destination of the dependency",
			dependencies: "- name: pub\n  version: 1.0.0",
			values:       "pub:\n  imported:\n    k: parent",
			want:         map[string]interface{}{"pub.imported.k": "parent", "pub.sub.fromchild.k": "pub-published"},
		},
		{
			name:         "an export-values of the dependency beats the parent values, like its values.yaml does",
			dependencies: "- name: pub\n  version: 1.0.0",
			values:       "pub:\n  sub:\n    x: parent",
			want:         map[string]interface{}{"pub.sub.x": "pub-published"},
		},
		{
			name:         "an export-values of the dependency into a map that its import-values extend beats the parent values",
			dependencies: "- name: pub\n  version: 1.0.0",
			values:       "pub:\n  sub:\n    x: parent",
			want:         map[string]interface{}{"pub.sub.x": map[string]interface{}{"a": "exported"}},
			dependency: mergeTestFiles(map[string]string{
				"charts/pub/Chart.yaml":            "apiVersion: v2\nname: pub\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x\n  import-values:\n  - child: fromchild\n    parent: sub.x\n",
				"charts/pub/charts/sub/Chart.yaml": "apiVersion: v2\nname: sub\nversion: 1.0.0\n",
			}, prefixTestFiles("charts/pub/", werfDataFlagsAsFiles(t, werfDataFlags{Values: "g:\n  a: exported\nsub:\n  fromchild:\n    b: imported"}))),
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
			if tt.dependency != nil {
				dependencyFiles = tt.dependency
			}

			if tt.nested {
				dependencyFiles = prefixTestFiles("charts/mid/", published)
				dependencyFiles["charts/mid/Chart.yaml"] = "apiVersion: v2\nname: mid\nversion: 1.0.0\ndependencies:\n- name: pub\n  version: 1.0.0\n  import-values:\n  - child: exported\n    parent: imported\n"
			}

			result, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, dependencyFiles, files), RenderChartOptions{})
			require.NoError(t, err)

			vals := renderedChartValues(t, result)

			storedVals := storedReleaseValues(t, result)
			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
				assert.Equal(t, want, valueAtPath(t, storedVals, path), "release get view of %s", path)
			}
		})
	}
}

func TestRenderChart_WerfDataDependencyBelowParentValuesV3(t *testing.T) {
	published := mergeTestFiles(map[string]string{
		"charts/pub/Chart.yaml":            "apiVersion: v3\nname: pub\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported\n",
		"charts/pub/charts/sub/Chart.yaml": "apiVersion: v3\nname: sub\nversion: 1.0.0\n",
	}, prefixTestFiles("charts/pub/", werfDataFlagsAsFiles(t, werfDataFlags{
		Values: "exported:\n  k: pub-published\nsub:\n  fromchild:\n    k: pub-published\n    l: pub-published",
	})))

	tests := []struct {
		name         string
		dependencies string
		values       string
		want         map[string]interface{}
		// nested puts the dependency into a dependency "mid" that imports its "exported" values.
		nested bool
	}{
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
			want:         map[string]interface{}{"mid.imported.k": "parent", "mid.pub.exported.k": "pub-published", "mid.pub.imported.l": "pub-published"},
			nested:       true,
		},
		{
			name:         "the parent values at an import-values destination of the dependency",
			dependencies: "- name: pub\n  version: 1.0.0",
			values:       "pub:\n  imported:\n    k: parent",
			want:         map[string]interface{}{"pub.imported.k": "parent", "pub.imported.l": "pub-published", "pub.sub.fromchild.k": "pub-published"},
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
			dependencyFiles := published
			if tt.nested {
				dependencyFiles = prefixTestFiles("charts/mid/", published)
				dependencyFiles["charts/mid/Chart.yaml"] = "apiVersion: v3\nname: mid\nversion: 1.0.0\ndependencies:\n- name: pub\n  version: 1.0.0\n  import-values:\n  - child: exported\n    parent: imported\n"
			}

			result, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, dependencyFiles, map[string]string{
				"Chart.yaml":  "apiVersion: v3\nname: root\nversion: 1.0.0\ndependencies:\n" + tt.dependencies + "\n",
				"values.yaml": tt.values,
			}), RenderChartOptions{})
			require.NoError(t, err)

			vals := renderedChartValues(t, result)
			storedVals := storedReleaseValues(t, result)

			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
				assert.Equal(t, want, valueAtPath(t, storedVals, path), "release get view of %s", path)
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
// and expects the dependency to get the same values. The overrides are deploy values files of the
// chart rendered alone and the parent values of the dependency.
func TestRenderChart_WerfDataDependencyEquivalence(t *testing.T) {
	optionalKeySchema := `{"type":"object","properties":{"key":{"type":"string"}}}`
	exportingChart := werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "key: default\ng: plain", "sub:\n  x: secret\n  s: secret", "")

	tests := []struct {
		name      string
		chart     map[string]string
		flags     werfDataFlags
		overrides string
		// withGlobal keeps the global values in the comparison.
		withGlobal bool
		// storedWithoutNulls is set when release get loses the nulls: the stored chart values have the
		// nulls of the dependency layers, and coalescing the chart values drops them.
		storedWithoutNulls bool
	}{
		{
			name:  "set values beat an export from the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "g: chart", "", ""),
			flags: werfDataFlags{Set: []string{"sub.x=set"}},
		},
		{
			name:  "an export from values files beats the dependency values",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "", "x: sub"),
			flags: werfDataFlags{Values: "g: values"},
		},
		{
			name:  "an import from values files beats the own key of the parent in the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported", "imported:\n  k: chart", "", ""),
			flags: werfDataFlags{Values: "sub:\n  fromchild:\n    k: values"},
		},
		{
			name:  "set values patch a list from secret values files",
			chart: werfDataCardChart("", "", "", ""),
			flags: werfDataFlags{SecretValues: "l:\n- custom-secret-0\n- custom-secret-1", Set: []string{"l[1]=set"}}, //nolint:gosec // Test values, not credentials.
		},
		{
			name:  "a null set value deletes a default",
			chart: werfDataCardChart("", "key: default", "", ""),
			flags: werfDataFlags{Set: []string{"key=null"}},
		},
		{
			name:  "a null set value deletes a default of an optional property of the schema",
			chart: mergeTestFiles(werfDataCardChart("", "key: default", "", ""), map[string]string{"values.schema.json": optionalKeySchema}),
			flags: werfDataFlags{Set: []string{"key=null"}},
		},
		{
			name:               "a null set value of a key without a default stays null",
			chart:              werfDataCardChart("", "other: default", "", ""),
			flags:              werfDataFlags{Set: []string{"key=null", "nested.key=null"}},
			storedWithoutNulls: true,
		},
		{
			name:  "a null set value deletes a value of the chart secret-values.yaml",
			chart: werfDataCardChart("", "key: default\nother: default", "key: secret", ""),
			flags: werfDataFlags{Set: []string{"key=null"}},
		},
		{
			name:  "a null set value deletes a value of values files",
			chart: werfDataCardChart("", "key: default\nother: default", "", ""),
			flags: werfDataFlags{Values: "key: values", Set: []string{"key=null"}},
		},
		{
			name:  "a null set value deletes nested values of the chart secret-values.yaml and values files",
			chart: werfDataCardChart("", "a:\n  key: default\n  other: default\nb:\n  key: default", "a:\n  key: secret\nb:\n  key: secret", ""),
			flags: werfDataFlags{Values: "a:\n  key: values\nb:\n  other: values", Set: []string{"a.key=null", "b=null"}},
		},
		{
			name:      "the overrides beat a null set value deleting values of the chart secret-values.yaml and values files",
			chart:     werfDataCardChart("", "key: default\nother: default", "key: secret\nother: secret", ""),
			flags:     werfDataFlags{Values: "key: values\nother: values", Set: []string{"key=null", "other=null"}},
			overrides: "key: parent",
		},
		{
			name:  "a null set value deletes an exported value of the chart secret-values.yaml",
			chart: exportingChart,
			flags: werfDataFlags{Set: []string{"g=null"}},
		},
		{
			name:  "a null set value deletes a value of a sub-dependency overridden in the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0", "sub:\n  x: 2", "", "x: 1\nz: 1"),
			flags: werfDataFlags{Set: []string{"sub.x=null"}},
		},
		{
			name:  "a null in the chart secret-values.yaml deletes a default of a sub-dependency",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0", "", "sub:\n  x: null", "x: 1\nz: 1"),
			flags: werfDataFlags{Set: []string{"marker=set"}},
		},
		{
			name:  "a null in the chart secret-values.yaml of a key without a default is dropped",
			chart: werfDataCardChart("", "other: default", "key: null", ""),
			flags: werfDataFlags{Set: []string{"marker=set"}},
		},
		{
			name:  "a null in the chart secret-values.yaml of an optional property of the schema without a default is dropped",
			chart: mergeTestFiles(werfDataCardChart("", "other: default", "key: null", ""), map[string]string{"values.schema.json": optionalKeySchema}),
			flags: werfDataFlags{Set: []string{"marker=set"}},
		},
		{
			name:      "the overrides beat an import from values files into the chart values",
			chart:     werfDataCardChart("- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported", "", "", ""),
			flags:     werfDataFlags{Values: "sub:\n  fromchild:\n    k: published"},
			overrides: "imported:\n  k: parent",
		},
		{
			name:  "a null in the chart secret-values.yaml deletes a value of the secret-values.yaml of a sub-dependency",
			chart: werfDataCardChartWithSubchartSecrets("other: default", "sub:\n  key: null", "", "key: sub-secret"),
			flags: werfDataFlags{Set: []string{"marker=set"}},
		},
		{
			name:  "a null in the chart secret-values.yaml deletes a value exported from the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "g: default", "sub:\n  x: null", ""),
			flags: werfDataFlags{Set: []string{"marker=set"}},
		},
		{
			name:  "a null exported from the chart secret-values.yaml deletes a default of a sub-dependency",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "g: null", "x: default"),
			flags: werfDataFlags{Set: []string{"marker=set"}},
		},
		{
			name:  "a null in the chart secret-values.yaml deletes a default",
			chart: werfDataCardChart("", "key: default\nother: default\na:\n  k: default\n  other: default", "key: null\na:\n  k: null", ""),
			flags: werfDataFlags{Set: []string{"marker=set"}},
		},
		{
			name:       "a null set value deletes a global default",
			chart:      werfDataCardChart("- name: sub\n  version: 1.0.0", "global:\n  x: default\n  z: default", "", ""),
			flags:      werfDataFlags{Set: []string{"global.x=null"}},
			withGlobal: true,
		},
		{
			name:  "JSON set values replace a map with the values exported from values files",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "", ""),
			flags: werfDataFlags{Values: "g: published", SetJSON: []string{`sub={"z":1}`}},
		},
		{
			name:  "the chart secret-values.yaml beats an export from the chart values.yaml",
			chart: exportingChart,
			flags: werfDataFlags{Set: []string{"marker=set"}},
		},
		{
			name:      "the overrides beat a null set value deleting a default and the chart secret-values.yaml beating an export",
			chart:     mergeTestFiles(exportingChart, map[string]string{"values.schema.json": optionalKeySchema}),
			flags:     werfDataFlags{Set: []string{"key=null", "marker=set"}},
			overrides: "marker: parent\nsub:\n  s: parent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			published := mergeTestFiles(tt.chart, werfDataFlagsAsFiles(t, tt.flags))
			published["Chart.yaml"] = strings.Replace(published["Chart.yaml"], "name: root", "name: pub", 1)

			alone, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, published), werfDataFlagsAsOptions(t, werfDataFlags{Values: tt.overrides}, RenderChartOptions{}))
			require.NoError(t, err)

			aloneValues := renderedChartValues(t, alone)
			if !tt.withGlobal {
				delete(aloneValues, "global")
			}

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
				parentValues, err := yaml.Marshal(nestValues(parseTestValues(t, tt.overrides), strings.Split(layout.pubPath, ".")))
				require.NoError(t, err)

				layout.files["values.yaml"] = string(parentValues)

				asDependency, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, layout.files), RenderChartOptions{})
				require.NoError(t, err)

				dependencyValues, ok := valueAtPath(t, renderedChartValues(t, asDependency), layout.pubPath).(map[string]interface{})
				require.True(t, ok, layout.pubPath)

				if !tt.withGlobal {
					delete(dependencyValues, "global")
				}

				assert.Equal(t, aloneValues, dependencyValues, layout.pubPath)
				assert.Empty(t, asDependency.ReleaseConfig, layout.pubPath)

				dependencyStoredValues, ok := valueAtPath(t, storedReleaseValues(t, asDependency), layout.pubPath).(map[string]interface{})
				require.True(t, ok, layout.pubPath)

				if !tt.withGlobal {
					delete(dependencyStoredValues, "global")
				}

				wantStoredValues := dependencyValues
				if tt.storedWithoutNulls {
					wantStoredValues, err = chartcommonutil.DeepCopyValues(dependencyValues)
					require.NoError(t, err)

					deleteNulls(wantStoredValues)
				}

				assert.Equal(t, wantStoredValues, dependencyStoredValues, "release get view of %s", layout.pubPath)
			}
		})
	}
}

func TestRenderChart_WerfDataDependencyLayers(t *testing.T) {
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
			name:         "parent values override the published values",
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
			name:         "parent re-enables a sub-dependency disabled by the published values",
			dependencies: "- name: pub\n  version: 1.0.0",
			values:       "pub:\n  subsub:\n    enabled: true",
			want:         map[string]interface{}{"pub.subsub.k": "subsub"},
			wantDeps:     []string{"pub"},
		},
		{
			name:         "two aliases of the published dependency get the published values",
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
			wantWarnings: []string{`Ignoring patches, disabled default patches, root JSON values, release defaults in .werf-data of dependency "pub": only those of the root chart are applied`},
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

func TestRenderChart_WerfDataDependencySecretNullAtImportSourceV3(t *testing.T) {
	result, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, map[string]string{ //nolint:gosec // Test values, not credentials.
		"Chart.yaml":                        "apiVersion: v3\nname: root\nversion: 1.0.0\ndependencies:\n- name: pub\n  version: 1.0.0\n",
		"charts/pub/Chart.yaml":             "apiVersion: v3\nname: pub\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported\n",
		"charts/pub/secret-values.yaml":     "sub:\n  fromchild:\n    k: null",
		"charts/pub/charts/sub/Chart.yaml":  "apiVersion: v3\nname: sub\nversion: 1.0.0\n",
		"charts/pub/charts/sub/values.yaml": "fromchild:\n  k: default\n  l: default",
	}, prefixTestFiles("charts/pub/", werfDataFlagsAsFiles(t, werfDataFlags{Values: "own: published"}))), RenderChartOptions{})
	require.NoError(t, err)

	vals := renderedChartValues(t, result)

	assert.Equal(t, absentValue{}, valueAtPath(t, vals, "pub.imported.k"))
	assert.Equal(t, "default", valueAtPath(t, vals, "pub.imported.l"))
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
			name:  "1: set values beat an export from the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "g: chart", "", ""),
			flags: werfDataFlags{Set: []string{"sub.x=set"}},
			want:  "g: chart\nsub:\n  global: {}\n  x: set",
		},
		{
			name:  "1: an import from values files beats the own key of the parent in the chart values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported", "imported:\n  k: chart", "", ""),
			flags: werfDataFlags{Values: "sub:\n  fromchild:\n    k: values"},
			want:  "imported:\n  k: values\nsub:\n  fromchild:\n    k: values\n  global: {}",
		},
		{
			name:  "2: an import from set values beats the chart secret-values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported", "", "imported:\n  k: chart-secret", ""),
			flags: werfDataFlags{Set: []string{"sub.fromchild.k=set"}},
			want:  "imported:\n  k: set\nsub:\n  fromchild:\n    k: set\n  global: {}",
		},
		{
			name:  "3: secret values files beat an export from the chart secret-values.yaml",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "g: chart-secret", ""),
			flags: werfDataFlags{SecretValues: "sub:\n  x: custom-secret"}, //nolint:gosec // Test values, not credentials.
			want:  "g: chart-secret\nsub:\n  global: {}\n  x: custom-secret",
		},
		{
			name:  "3: values files beat an export from the chart secret-values.yaml",
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
			name:  "5: a null set value deletes the values exported from secret values files",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "sub:\n  a: chart", "", "own: sub"),
			flags: werfDataFlags{SecretValues: "g: custom-secret", Set: []string{"sub=null"}},
			want:  "g: custom-secret\nsub:\n  global: {}\n  own: sub",
		},
		{
			name:  "6: JSON set values replace a map with the values exported from the secret values",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "g: chart-secret", ""),
			flags: werfDataFlags{SecretValues: "g: custom-secret", SetJSON: []string{`sub={"z":1}`}},
			want:  "g: custom-secret\nsub:\n  global: {}\n  x: chart-secret\n  z: 1",
		},
		{
			name:  "7: a null in secret values files replaces a map exported from values files",
			chart: werfDataCardChart("- name: sub\n  version: 1.0.0\n  export-values:\n  - parent: g\n    child: x", "", "", ""),
			flags: werfDataFlags{Values: "g:\n  k: values", SecretValues: "g: null"},
			want:  "g: null\nsub:\n  global: {}\n  x: null",
		},
		{
			name:  "8: set values patch a list from secret values files",
			chart: werfDataCardChart("", "", "", ""),
			flags: werfDataFlags{SecretValues: "l:\n- custom-secret-0\n- custom-secret-1", Set: []string{"l[1]=set"}}, //nolint:gosec // Test values, not credentials.
			want:  "l:\n- custom-secret-0\n- set",
		},
		{
			name:  "9: a null in values files for a key absent in the charts is kept",
			chart: werfDataCardChart("", "", "", ""),
			flags: werfDataFlags{Values: "unknown: null\nknown: values"},
			want:  "known: values\nunknown: null",
		},
		{
			name:  "10: values files replace a null in the chart secret-values.yaml",
			chart: werfDataCardChart("", "a:\n  c: chart", "a: null", ""),
			flags: werfDataFlags{Values: "a:\n  b: values"},
			want:  "a:\n  b: values",
		},
		{
			name:       "11: values files overridden by secret values files are kept without the chart secret-values.yaml",
			chart:      werfDataCardChart("", "", "b: chart-secret", ""),
			flags:      werfDataFlags{Values: "a: values", SecretValues: "a: custom-secret"},
			deployOpts: RenderChartOptions{HelmOptions: common.HelmOptions{ChartLoadOpts: common.ChartLoadOptions{DefaultSecretValuesDisable: true}}},
			want:       "a: custom-secret",
		},
		{
			name:  "12: a null set value deletes a value of the dependency",
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
			name:  "extra values reach nested subcharts",
			chart: werfDataNestedSubchartsChart(),
			flags: werfDataFlags{ExtraValues: "werf:\n  image:\n    x: published-image\nglobal:\n  werf:\n    repo: published-repo"},
			want:  "global:\n  werf:\n    repo: published-repo\nsub:\n  global:\n    werf:\n      repo: published-repo\n  subsub:\n    global:\n      werf:\n        repo: published-repo\n    werf:\n      image:\n        x: published-image\n  werf:\n    image:\n      x: published-image\nwerf:\n  image:\n    x: published-image",
		},
		{
			name:       "deploy extra values beat published extra values in nested subcharts",
			chart:      werfDataNestedSubchartsChart(),
			flags:      werfDataFlags{ExtraValues: "werf:\n  image:\n    x: published-image\n    y: published-image\nglobal:\n  werf:\n    repo: published-repo"},
			deployOpts: RenderChartOptions{HelmOptions: common.HelmOptions{ChartLoadOpts: common.ChartLoadOptions{ExtraValues: map[string]interface{}{"werf": map[string]interface{}{"image": map[string]interface{}{"x": "deploy-image"}}, "global": map[string]interface{}{"werf": map[string]interface{}{"repo": "deploy-repo"}}}}}},
			want:       "global:\n  werf:\n    repo: deploy-repo\nsub:\n  global:\n    werf:\n      repo: deploy-repo\n  subsub:\n    global:\n      werf:\n        repo: deploy-repo\n    werf:\n      image:\n        x: deploy-image\n        y: published-image\n  werf:\n    image:\n      x: deploy-image\n      y: published-image\nwerf:\n  image:\n    x: deploy-image\n    y: published-image",
		},
		{
			name:  "string set values with literal set values and set file values whose file is absent at deploy",
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
			name:  "root JSON values",
			chart: werfDataCardChart("", "", "", ""),
			flags: werfDataFlags{SetRootJSON: []string{`root={"a":"set"}`}},
			want:  "{}",
		},
		{
			name:  "disabled default values",
			chart: werfDataCardChart("", "a: chart", "b: chart-secret", ""),
			flags: werfDataFlags{DefaultValuesDisable: true, Values: "c: values"},
			want:  "b: chart-secret\nc: values",
		},
		{
			name:  "disabled default secret values apply to subcharts",
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
			name: "chart apiVersion v3 imports from secret values files and extra values",
			chart: map[string]string{
				"Chart.yaml":             "apiVersion: v3\nname: root\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported\n",
				"charts/sub/Chart.yaml":  "apiVersion: v3\nname: sub\nversion: 1.0.0\n",
				"charts/sub/values.yaml": "fromchild:\n  a: sub\n  b: sub\n  c: sub",
			},
			flags: werfDataFlags{SecretValues: "sub:\n  fromchild:\n    a: custom-secret", ExtraValues: "sub:\n  fromchild:\n    b: service"}, //nolint:gosec // Test values, not credentials.
			want:  "imported:\n  a: custom-secret\n  b: service\n  c: sub\nsub:\n  fromchild:\n    a: custom-secret\n    b: service\n    c: sub\n  global: {}",
		},
		{
			name: "chart apiVersion v3 imports from values files and set values",
			chart: map[string]string{
				"Chart.yaml":             "apiVersion: v3\nname: root\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n  import-values:\n  - child: fromchild\n    parent: imported\n",
				"charts/sub/Chart.yaml":  "apiVersion: v3\nname: sub\nversion: 1.0.0\n",
				"charts/sub/values.yaml": "fromchild:\n  a: sub\n  b: sub\n  c: sub",
			},
			flags: werfDataFlags{Values: "sub:\n  fromchild:\n    a: values", Set: []string{"sub.fromchild.b=set"}},
			want:  "imported:\n  a: values\n  b: set\n  c: sub\nsub:\n  fromchild:\n    a: values\n    b: set\n    c: sub\n  global: {}",
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
			assert.Equal(t, fromOptions.ResourceSpecs, fromWerfData.ResourceSpecs)

			for key, value := range fromOptions.Values {
				if _, isValues := value.(chartcommon.Values); isValues {
					continue
				}

				assert.Equal(t, value, fromWerfData.Values[key], key)
			}
		})
	}
}

func TestRenderChart_WerfDataExtraValuesTypes(t *testing.T) {
	const template = "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Chart.Name }}\ndata:\n  unix: {{ .Values.werf.commit.date.unix | quote }}\n  float: {{ .Values.werf.float | quote }}\n"

	extraValues := map[string]interface{}{
		"werf": map[string]interface{}{
			"commit": map[string]interface{}{"date": map[string]interface{}{"unix": int64(1700000000)}},
			"float":  1.5,
		},
	}
	wantData := map[string]interface{}{"unix": "1700000000", "float": "1.5"}

	werfDataFiles, err := (&werfdata.Data{ExtraValues: extraValues}).ChartFiles()
	require.NoError(t, err)

	storedFiles := map[string]string{}
	for _, f := range werfDataFiles {
		storedFiles[f.Name] = string(f.Data)
	}

	t.Run("chart", func(t *testing.T) {
		chart := map[string]string{
			"Chart.yaml":        "apiVersion: v2\nname: root\nversion: 1.0.0\n",
			"templates/cm.yaml": template,
		}

		fromOptions, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, chart), RenderChartOptions{HelmOptions: common.HelmOptions{ChartLoadOpts: common.ChartLoadOptions{ExtraValues: extraValues}}})
		require.NoError(t, err)

		fromWerfData, err := renderWerfDataTestChart(t, writeWerfDataTestChart(t, chart, storedFiles), RenderChartOptions{})
		require.NoError(t, err)

		require.Len(t, fromOptions.ResourceSpecs, 1)
		assert.Equal(t, wantData, fromOptions.ResourceSpecs[0].Unstruct.Object["data"])
		assert.Equal(t, fromOptions.ResourceSpecs, fromWerfData.ResourceSpecs)
		assert.Equal(t, renderedChartValues(t, fromOptions), renderedChartValues(t, fromWerfData))
	})

	t.Run("dependency", func(t *testing.T) {
		chartPath := writeWerfDataTestChart(t, map[string]string{
			"Chart.yaml":                   "apiVersion: v2\nname: root\nversion: 1.0.0\ndependencies:\n- name: sub\n  version: 1.0.0\n",
			"charts/sub/Chart.yaml":        "apiVersion: v2\nname: sub\nversion: 1.0.0\n",
			"charts/sub/templates/cm.yaml": template,
		}, prefixTestFiles("charts/sub/", storedFiles))

		result, err := renderWerfDataTestChart(t, chartPath, RenderChartOptions{})
		require.NoError(t, err)

		require.Len(t, result.ResourceSpecs, 1)
		assert.Equal(t, wantData, result.ResourceSpecs[0].Unstruct.Object["data"])
	})
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
			wantLine: "Using publish-time inputs from .werf-data: values, 2 set values, default secret values disabled, 1 patches files, default patches disabled, annotations, labels, runtime annotations, release info annotations, release defaults",
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
			name: "a typo in publish values files fails a strict root schema without naming extra values",
			chart: mergeTestFiles(
				werfDataCardChart("- name: sub\n  version: 1.0.0", "foo: chart", "", ""),
				map[string]string{"values.schema.json": strictRootSchema},
				werfDataFlagsAsFiles(t, werfDataFlags{Values: "fooo: typo", ExtraValues: "werf:\n  repo: service"}),
			),
			wantErr:      "fooo",
			wantNotInErr: "werf",
		},
		{
			name: "extra values of a published dependency are tolerated by its strict schema",
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

func TestWerfDataLayers_SkipsEmptyLayers(t *testing.T) {
	tests := []struct {
		name                string
		defaultSecretValues map[string]interface{}
		flags               werfDataFlags
		wantLayers          int
	}{
		{
			name: "without values",
		},
		{
			name:                "with chart secret values",
			defaultSecretValues: map[string]interface{}{"key": "secret"},
			wantLayers:          1,
		},
		{
			name:       "with values files and set values",
			flags:      werfDataFlags{Values: "key: values", Set: []string{"other=set"}},
			wantLayers: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var werfDataFiles []*chartcommon.File
			for name, data := range werfDataFlagsAsFiles(t, tt.flags) {
				werfDataFiles = append(werfDataFiles, &chartcommon.File{Name: name, Data: []byte(data)})
			}

			chrt := &v2chart.Chart{
				Metadata:           &v2chart.Metadata{APIVersion: "v2", Name: "pub", Version: "1.0.0"},
				SecretsRuntimeData: &werfDataSecretsRuntimeData{defaultSecretValues: tt.defaultSecretValues},
				WerfDataFiles:      werfDataFiles,
			}

			layers, err := werfDataLayers(context.Background(), chrt, true)
			require.NoError(t, err)

			assert.Len(t, layers, tt.wantLayers)
		})
	}
}
