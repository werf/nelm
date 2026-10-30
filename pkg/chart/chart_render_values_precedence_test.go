package chart

import (
	"bytes"
	stdlog "log"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/log"
)

func TestRenderChart_ExportAndImportValuesDependencyVersionMismatch(t *testing.T) {
	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want map[string]interface{}
	}{
		{
			name: "value from values.yaml is exported to dependency out of version range",
			want: map[string]interface{}{"m.image.repository": "parent-values-yaml"},
		},
		{
			name: "value from set is exported to dependency out of version range",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=set"}},
			want: map[string]interface{}{"m.image.repository": "set"},
		},
		{
			name: "value from service values is exported to dependency out of version range",
			opts: layeredValuesRenderOptions{ServiceValues: map[string]interface{}{"werf": map[string]interface{}{"repo": "service"}}},
			want: map[string]interface{}{"m.svc": "service"},
		},
		{
			name: "value is imported from dependency out of version range",
			want: map[string]interface{}{"imp.k": "m-values"},
		},
		{
			name: "value from set is imported from dependency out of version range",
			opts: layeredValuesRenderOptions{Set: []string{"m.out.k=set"}},
			want: map[string]interface{}{"imp.k": "set"},
		},
		{
			name: "value is not exported to dependency out of version range disabled by condition",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=set", "m.enabled=false"}},
			want: map[string]interface{}{"m.image": absentValue{}, "imp": absentValue{}},
		},
		{
			name: "value is not exported to alias dependency out of version range",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=set"}},
			want: map[string]interface{}{"s.image.repository": "s-default", "al": absentValue{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ChartDir = "version-mismatch-chart"
			vals := renderedChartValues(t, renderLayeredValuesChart(t, tt.opts))

			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}
		})
	}
}

func TestRenderChart_ExportAndImportValuesEdgeCases(t *testing.T) {
	edgeValuesFile := filepath.Join("testdata", "layered-values", "edge-values-file.yaml")

	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want map[string]interface{}
	}{
		{
			name: "exported value from service values overrides child value from values file",
			opts: layeredValuesRenderOptions{
				ServiceValues: map[string]interface{}{"global": map[string]interface{}{"werf": map[string]interface{}{"images": map[string]interface{}{"app": map[string]interface{}{"repository": "service"}}}}},
				ValuesFiles:   []string{edgeValuesFile},
			},
			want: map[string]interface{}{"a.image.repository": "service"},
		},
		{
			name: "dependency listed before exporting dependency gets exported value",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=set"}},
			want: map[string]interface{}{"b.repo": "set"},
		},
		{
			name: "set patches list exported from values file",
			opts: layeredValuesRenderOptions{Set: []string{"a.list[1]=set"}, ValuesFiles: []string{edgeValuesFile}},
			want: map[string]interface{}{"a.list": []interface{}{"values-file-0", "set"}},
		},
		{
			name: "null exported to dependency path resets dependency values",
			opts: layeredValuesRenderOptions{Set: []string{"g.dnull=null"}},
			want: map[string]interface{}{"a.d.k": "d"},
		},
		{
			name: "null imported to dependency path resets dependency values",
			opts: layeredValuesRenderOptions{Set: []string{"a.out=null"}},
			want: map[string]interface{}{"b.repo": "parent-values-yaml"},
		},
		{
			name: "set-json replaces import child map exported from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`a.out={"new":"set"}`}, ValuesFiles: []string{edgeValuesFile}},
			want: map[string]interface{}{"b.new": "set", "b.old": absentValue{}, "b.out.new": "set", "b.out.old": absentValue{}},
		},
		{
			name: "null in secret-values.yaml overrides value imported from secret-values.yaml",
			want: map[string]interface{}{"imp": absentValue{}},
		},
		{
			name: "null imported from secret-values.yaml deletes value from values.yaml",
			want: map[string]interface{}{"nullImp": map[string]interface{}{}},
		},
		{
			name: "null exported from secret-values.yaml overrides child value from secret-values.yaml",
			want: map[string]interface{}{"a.nullTarget": absentValue{}},
		},
		{
			name: "empty map exported from secret-values.yaml overrides child value from secret-values.yaml",
			want: map[string]interface{}{"a.empty": map[string]interface{}{}},
		},
		{
			name: "null in map exported from secret-values.yaml deletes child value from values.yaml",
			want: map[string]interface{}{"b.cfg": map[string]interface{}{"other": "secret-values-yaml", "z": "b-values"}},
		},
		{
			name: "null in secret-values.yaml deletes value from values.yaml",
			want: map[string]interface{}{"nested": map[string]interface{}{"bar": "secret-values-yaml"}},
		},
		{
			name: "value is not exported to dependency disabled by condition",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=set"}},
			want: map[string]interface{}{"t": absentValue{}},
		},
		{
			name: "value is exported to dependency enabled by condition",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=set", "t.enabled=true"}},
			want: map[string]interface{}{"t.x": "set"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ChartDir = "export-import-edge-chart"
			vals := renderedChartValues(t, renderLayeredValuesChart(t, tt.opts))

			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}
		})
	}
}

func TestRenderChart_ExportAndImportValuesNotFoundWarnings(t *testing.T) {
	const (
		exportWarning     = `Skipping export-values of "img.repo" from chart "parent" to dependency "sub": the path is not set in any values`
		importMapWarning  = `Skipping import-values of "out" from dependency "sub" to chart "parent": no map is set at the path in any values`
		importNameWarning = `Skipping import-values of "exports.data" from dependency "sub" to chart "parent": no map is set at the path in any values`
	)

	allPathsSet := []string{"img.repo=set", "sub.out.k=set", "sub.exports.data.k=set"}

	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want []string
	}{
		{
			name: "paths set nowhere",
			want: []string{exportWarning, importMapWarning, importNameWarning},
		},
		{
			name: "export parent path set nowhere",
			opts: layeredValuesRenderOptions{Set: []string{"img.repoo=typo", "sub.out.k=set", "sub.exports.data.k=set"}},
			want: []string{exportWarning},
		},
		{
			name: "import child tables set nowhere",
			opts: layeredValuesRenderOptions{Set: []string{"img.repo=set", "sub.outt.k=typo", "sub.exportz.data.k=typo"}},
			want: []string{importMapWarning, importNameWarning},
		},
		{
			name: "paths set only in set",
			opts: layeredValuesRenderOptions{Set: allPathsSet},
		},
		{
			name: "paths set only in values file",
			opts: layeredValuesRenderOptions{ValuesFiles: []string{filepath.Join("testdata", "layered-values", "not-found-paths-values-file.yaml")}},
		},
		{
			name: "paths set only in service values",
			opts: layeredValuesRenderOptions{ServiceValues: readValuesFile(t, filepath.Join("testdata", "layered-values", "not-found-paths-values-file.yaml"))},
		},
		{
			name: "paths of enabled dependency set nowhere",
			opts: layeredValuesRenderOptions{Set: append([]string{"disabled.enabled=true"}, allPathsSet...)},
			want: []string{
				`Skipping export-values of "disabledExport" from chart "parent" to dependency "disabled": the path is not set in any values`,
				`Skipping import-values of "disabledImport" from dependency "disabled" to chart "parent": no map is set at the path in any values`,
			},
		},
		{
			name: "paths set in nested chart values overridden by parent chart values",
			opts: layeredValuesRenderOptions{ChartDir: "not-found-paths-nested-chart"},
		},
		{
			name: "paths read before exports overwrite them",
			opts: layeredValuesRenderOptions{ChartDir: "not-found-paths-overwrite-chart"},
			want: []string{`Skipping export-values of "sub.typoCycle.typo" from chart "parent" to dependency "sub": the path is not set in any values`},
		},
		{
			name: "paths read from set before exports overwrite them",
			opts: layeredValuesRenderOptions{ChartDir: "not-found-paths-overwrite-chart", Set: []string{"sub.cycle.k=set", "sub.self.k=set"}},
			want: []string{`Skipping export-values of "sub.typoCycle.typo" from chart "parent" to dependency "sub": the path is not set in any values`},
		},
		{
			name: "import child paths set to scalars in chart values",
			opts: layeredValuesRenderOptions{ChartDir: "not-found-paths-scalar-import-chart"},
			want: []string{importMapWarning, importNameWarning},
		},
		{
			name: "dependency not in Chart.yaml with values invalid without set",
			opts: layeredValuesRenderOptions{ChartDir: "scalar-sub-no-dependencies-chart", Set: []string{"sub.enabled=true"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &warningsLogger{Logger: log.Default}
			log.Default = logger
			t.Cleanup(func() { log.Default = logger.Logger })

			defaultLogger, stdLogWriter, stdLogFlags := slog.Default(), stdlog.Writer(), stdlog.Flags()
			t.Cleanup(func() {
				slog.SetDefault(defaultLogger)
				stdlog.SetOutput(stdLogWriter)
				stdlog.SetFlags(stdLogFlags)
			})

			var slogWarnings bytes.Buffer
			slog.SetDefault(slog.New(slog.NewTextHandler(&slogWarnings, &slog.HandlerOptions{Level: slog.LevelWarn})))

			if tt.opts.ChartDir == "" {
				tt.opts.ChartDir = "not-found-paths-chart"
			}

			renderLayeredValuesChart(t, tt.opts)

			skipWarnings := lo.Filter(logger.warnings, func(warning string, _ int) bool {
				return strings.HasPrefix(warning, "Skipping")
			})
			assert.ElementsMatch(t, tt.want, skipWarnings)
			assert.Empty(t, slogWarnings.String())
		})
	}
}

// TestRenderChart_ExportAndImportValuesPrecedenceMatrix checks every pair of value layers. Within a
// layer, an exported value beats the child value and the parent value beats an imported value.
// Across layers, the higher layer wins.
func TestRenderChart_ExportAndImportValuesPrecedenceMatrix(t *testing.T) {
	layers := []string{"cv", "cs", "pv", "ps", "file", "secretfile", "set", "service"}

	result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
		ChartDir:          "precedence-matrix-chart",
		SecretValuesFiles: []string{filepath.Join("testdata", "layered-values", "pm-secret-values-file.yaml")},
		ServiceValues:     readValuesFile(t, filepath.Join("testdata", "layered-values", "pm-service-values.yaml")),
		Set:               flattenToSetValues(readValuesFile(t, filepath.Join("testdata", "layered-values", "pm-set-values.yaml")), ""),
		ValuesFiles:       []string{filepath.Join("testdata", "layered-values", "pm-values-file.yaml")},
	})
	vals := renderedChartValues(t, result)

	for sourceRank, source := range layers {
		for targetRank, target := range layers {
			if sourceRank >= 2 {
				t.Run("export from "+source+" to "+target, func(t *testing.T) {
					assert.Equal(t, lo.Ternary(sourceRank >= targetRank, "src-"+source, "dst-"+target), valueAtPath(t, vals, "sub.exp."+source+"."+target))
				})
			}

			if targetRank >= 2 {
				t.Run("import from "+source+" to "+target, func(t *testing.T) {
					assert.Equal(t, lo.Ternary(sourceRank > targetRank, "src-"+source, "dst-"+target), valueAtPath(t, vals, "imp."+source+"."+target))
				})
			}
		}
	}
}

func TestRenderChart_ExportAndImportValuesSkippedWarnings(t *testing.T) {
	tests := []struct {
		name string
		set  string
		want string
	}{
		{name: "scalar exported to dependency root", set: "g.prim=set", want: `Skipping export-values of "g.prim" to dependency "a": only a map can be exported to the dependency root`},
		{name: "scalar imported", set: "a.out=set", want: `Skipping import-values of "a.out": only a map can be imported`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := &warningsLogger{Logger: log.Default}
			log.Default = logger
			t.Cleanup(func() { log.Default = logger.Logger })

			renderLayeredValuesChart(t, layeredValuesRenderOptions{ChartDir: "export-import-edge-chart", Set: []string{tt.set}})

			skipWarnings := lo.Filter(logger.warnings, func(warning string, _ int) bool {
				return strings.HasPrefix(warning, "Skipping") && strings.Contains(warning, "only a map")
			})
			assert.Equal(t, []string{tt.want}, skipWarnings)
		})
	}
}

func TestRenderChart_ExportGlobalValuesFromNestedChart(t *testing.T) {
	serviceValues := map[string]interface{}{"global": map[string]interface{}{
		"img":  map[string]interface{}{"repo": "service"},
		"werf": map[string]interface{}{"images": map[string]interface{}{"leaf": map[string]interface{}{"repository": "service-repo", "tag": "service-tag"}}},
	}}

	tests := []struct {
		name              string
		opts              layeredValuesRenderOptions
		want              map[string]interface{}
		wantReleaseConfig map[string]interface{}
	}{
		{
			name: "global values from root and nested chart values.yaml, root wins",
			want: map[string]interface{}{
				"mid.leaf.image.repository": "leaf-default",
				"mid.leaf.fromRoot":         "root-values-yaml",
				"mid.leaf.fromMid":          "mid-values-yaml",
				"mid.leaf.fromBoth":         "root-values-yaml",
			},
			wantReleaseConfig: map[string]interface{}{},
		},
		{
			name: "global value from set",
			opts: layeredValuesRenderOptions{Set: []string{"global.img.repo=set"}},
			want: map[string]interface{}{"mid.leaf.image.repository": "set"},
			wantReleaseConfig: map[string]interface{}{
				"global": map[string]interface{}{"img": map[string]interface{}{"repo": "set"}},
				"mid":    map[string]interface{}{"leaf": map[string]interface{}{"image": map[string]interface{}{"repository": "set"}}},
			},
		},
		{
			name: "global value from values file",
			opts: layeredValuesRenderOptions{ValuesFiles: []string{filepath.Join("testdata", "layered-values", "nested-global-values-file.yaml")}},
			want: map[string]interface{}{"mid.leaf.image.repository": "values-file"},
		},
		{
			name: "global value from set overrides global value from nested chart values.yaml",
			opts: layeredValuesRenderOptions{Set: []string{"global.mid=set"}},
			want: map[string]interface{}{"mid.leaf.fromMid": "set"},
		},
		{
			name: "nested chart global value from set",
			opts: layeredValuesRenderOptions{Set: []string{"mid.global.img.repo=mid-set"}},
			want: map[string]interface{}{"mid.leaf.image.repository": "mid-set"},
		},
		{
			name: "root global value from set overrides nested chart global value from set",
			opts: layeredValuesRenderOptions{Set: []string{"global.img.repo=set", "mid.global.img.repo=mid-set"}},
			want: map[string]interface{}{"mid.leaf.image.repository": "set"},
		},
		{
			name:              "global values from service values",
			opts:              layeredValuesRenderOptions{ServiceValues: serviceValues, Set: []string{"global.img.repo=set"}},
			want:              map[string]interface{}{"mid.leaf.image.repository": "service", "mid.leaf.werfImage": map[string]interface{}{"repository": "service-repo", "tag": "service-tag"}},
			wantReleaseConfig: map[string]interface{}{"global": map[string]interface{}{"img": map[string]interface{}{"repo": "set"}}, "mid": map[string]interface{}{"leaf": map[string]interface{}{"image": map[string]interface{}{"repository": "set"}}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ChartDir = "nested-global-export-chart"
			result := renderLayeredValuesChart(t, tt.opts)
			vals := renderedChartValues(t, result)

			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, vals, path), path)
			}

			if tt.wantReleaseConfig != nil {
				assert.Equal(t, tt.wantReleaseConfig, result.ReleaseConfig)
			}
		})
	}
}

func TestRenderChart_ExportValuesIndependentOfDependencyOrder(t *testing.T) {
	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want map[string]interface{}
	}{
		{
			name: "value from values.yaml is exported through exports to sibling dependencies",
			want: map[string]interface{}{
				"a.fromB":               "parent-values-yaml",
				"b.foo":                 "parent-values-yaml",
				"c.bar":                 "parent-values-yaml",
				"mid.reader.fromWriter": "parent-values-yaml",
				"mid.writer.v":          "parent-values-yaml",
			},
		},
		{
			name: "value from set is exported through exports to sibling dependencies",
			opts: layeredValuesRenderOptions{Set: []string{"g.bar=set", "g.src=set"}},
			want: map[string]interface{}{"a.fromB": "set", "mid.reader.fromWriter": "set"},
		},
		{
			name: "later export to same child path wins",
			want: map[string]interface{}{"c.same": "second"},
		},
	}

	chartDirs := map[string]string{
		"listed order":   "export-order-chart",
		"reversed order": reversedDependenciesChartDir(t, "export-order-chart"),
	}

	for _, tt := range tests {
		for order, chartDir := range chartDirs {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				tt.opts.ChartDir = chartDir
				vals := renderedChartValues(t, renderLayeredValuesChart(t, tt.opts))

				for path, want := range tt.want {
					assert.Equal(t, want, valueAtPath(t, vals, path), path)
				}
			})
		}
	}
}

func TestRenderChart_ExportedValueOverridesValuesFileForDependencyWithSameNameAsParent(t *testing.T) {
	serviceValues := readValuesFile(t, filepath.Join("testdata", "layered-values", "same-name-service-values.yaml"))

	tests := []struct {
		name              string
		chartDir          string
		opts              layeredValuesRenderOptions
		wantImage         string
		wantReleaseConfig map[string]interface{}
	}{
		{
			name:              "global.werf.images from service values",
			chartDir:          "same-name-global-export-chart",
			opts:              layeredValuesRenderOptions{ServiceValues: serviceValues},
			wantImage:         "service-repo:service-tag",
			wantReleaseConfig: map[string]interface{}{"litellm-helm": map[string]interface{}{"image": map[string]interface{}{"pullPolicy": "Always", "repository": "values-file-repo", "tag": "values-file-tag"}}},
		},
		{
			name:              "werf.repo and werf.tag from service values",
			chartDir:          "same-name-werf-export-chart",
			opts:              layeredValuesRenderOptions{ServiceValues: serviceValues},
			wantImage:         "service-repo:service-tag",
			wantReleaseConfig: map[string]interface{}{"litellm-helm": map[string]interface{}{"image": map[string]interface{}{"pullPolicy": "Always", "repository": "values-file-repo", "tag": "values-file-tag"}}},
		},
		{
			name:      "global.werf.images from set",
			chartDir:  "same-name-global-export-chart",
			opts:      layeredValuesRenderOptions{Set: []string{"global.werf.images.litellm.repository=set-repo", "global.werf.images.litellm.tag=set-tag"}},
			wantImage: "set-repo:set-tag",
			wantReleaseConfig: map[string]interface{}{
				"global":       map[string]interface{}{"werf": map[string]interface{}{"images": map[string]interface{}{"litellm": map[string]interface{}{"repository": "set-repo", "tag": "set-tag"}}}},
				"litellm-helm": map[string]interface{}{"image": map[string]interface{}{"pullPolicy": "Always", "repository": "set-repo", "tag": "set-tag"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ChartDir = tt.chartDir
			tt.opts.ValuesFiles = []string{filepath.Join("testdata", "layered-values", "same-name-values-file.yaml")}

			defaultLogger, stdLogWriter, stdLogFlags := slog.Default(), stdlog.Writer(), stdlog.Flags()
			t.Cleanup(func() {
				slog.SetDefault(defaultLogger)
				stdlog.SetOutput(stdLogWriter)
				stdlog.SetFlags(stdLogFlags)
			})

			var warnings bytes.Buffer
			slog.SetDefault(slog.New(slog.NewTextHandler(&warnings, &slog.HandlerOptions{Level: slog.LevelWarn})))

			logger := &warningsLogger{Logger: log.Default}
			log.Default = logger
			t.Cleanup(func() { log.Default = logger.Logger })

			result := renderLayeredValuesChart(t, tt.opts)

			require.Len(t, result.ResourceSpecs, 1)
			assert.Equal(t, map[string]interface{}{"image": tt.wantImage, "pullPolicy": "Always"}, result.ResourceSpecs[0].Unstruct.Object["data"])
			assert.Equal(t, tt.wantReleaseConfig, result.ReleaseConfig)
			assert.Empty(t, warnings.String())
			assert.Empty(t, lo.Filter(logger.warnings, func(warning string, _ int) bool {
				return strings.HasPrefix(warning, "Skipping")
			}))
		})
	}
}

// TestRenderChart_ImportValuesNotChainedBetweenSiblings pins that, like in Helm, a value imported into
// the values of a sibling dependency is not imported again by the import-values of that sibling.
func TestRenderChart_ImportValuesNotChainedBetweenSiblings(t *testing.T) {
	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want map[string]interface{}
	}{
		{
			name: "value from values.yaml",
			want: map[string]interface{}{"b.tbl.k": "a-values", "z": absentValue{}},
		},
		{
			name: "value from set",
			opts: layeredValuesRenderOptions{Set: []string{"a.out.k=set"}},
			want: map[string]interface{}{"b.tbl.k": "set", "z": absentValue{}},
		},
	}

	chartDirs := map[string]string{
		"listed order":   "sibling-import-chain-chart",
		"reversed order": reversedDependenciesChartDir(t, "sibling-import-chain-chart"),
	}

	for _, tt := range tests {
		for order, chartDir := range chartDirs {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				tt.opts.ChartDir = chartDir
				vals := renderedChartValues(t, renderLayeredValuesChart(t, tt.opts))

				for path, want := range tt.want {
					assert.Equal(t, want, valueAtPath(t, vals, path), path)
				}
			})
		}
	}
}

func TestRenderChart_NullInSecretValuesYAMLWhenChartValuesAreInvalidAlone(t *testing.T) {
	tests := []struct {
		chartDir         string
		wantDependencies []string
	}{
		{chartDir: "scalar-sub-secret-null-v2", wantDependencies: []string{"sub", "t"}},
		{chartDir: "scalar-sub-secret-null-v3", wantDependencies: []string{"sub"}},
	}

	for _, tt := range tests {
		t.Run(tt.chartDir, func(t *testing.T) {
			result := renderLayeredValuesChart(t, layeredValuesRenderOptions{
				ChartDir:          tt.chartDir,
				SecretValuesFiles: []string{layeredSecretValuesSubFile},
			})

			assert.Equal(t, map[string]interface{}{"bar": "values-yaml"}, renderedChartValues(t, result)["nested"])
			assert.Equal(t, tt.wantDependencies, dependencyNames(t, result), "null exported from secret-values.yaml deletes condition value from values.yaml")
		})
	}
}

func TestRenderChart_ReleaseConfigFollowsExportAndImportValuesPrecedence(t *testing.T) {
	edgeValuesFile := filepath.Join("testdata", "layered-values", "edge-values-file.yaml")

	tests := []struct {
		name string
		opts layeredValuesRenderOptions
		want map[string]interface{}
	}{
		{
			name: "without service values, chart secret-values.yaml and imported values",
			opts: layeredValuesRenderOptions{
				ServiceValues: map[string]interface{}{"global": map[string]interface{}{"werf": map[string]interface{}{"images": map[string]interface{}{"app": map[string]interface{}{"repository": "service"}}}}},
				ValuesFiles:   []string{edgeValuesFile},
			},
			want: map[string]interface{}{
				"a.image.repository": "values-file",
				"b.repo":             "values-file",
				"global":             absentValue{},
				"nested":             absentValue{},
				"imp":                absentValue{},
				"a.nullTarget":       absentValue{},
				"b.old":              absentValue{},
			},
		},
		{
			name: "set-json replaces export source map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`a.out={"new":"set"}`}, ValuesFiles: []string{edgeValuesFile}},
			want: map[string]interface{}{
				"a.out":   map[string]interface{}{"new": "set"},
				"b.out":   map[string]interface{}{"new": "set"},
				"g.out":   map[string]interface{}{"old": "values-file"},
				"b.new":   absentValue{},
				"b.old":   absentValue{},
				"a.image": map[string]interface{}{"repository": "values-file"},
			},
		},
		{
			name: "set-json replaces export parent map from values file",
			opts: layeredValuesRenderOptions{SetJSON: []string{`g.out={"new":"set"}`}, ValuesFiles: []string{edgeValuesFile}},
			want: map[string]interface{}{
				"a.out": map[string]interface{}{"new": "set"},
				"b.out": map[string]interface{}{"new": "set"},
			},
		},
		{
			name: "set patches list exported from values file",
			opts: layeredValuesRenderOptions{Set: []string{"a.list[1]=set"}, ValuesFiles: []string{edgeValuesFile}},
			want: map[string]interface{}{
				"a.list": []interface{}{"values-file-0", "set"},
				"g.list": []interface{}{"values-file-0", "values-file-1"},
			},
		},
		{
			name: "exported value from set replaces null from set for dependency",
			opts: layeredValuesRenderOptions{Set: []string{"a=null", "g.repo=set"}},
			want: map[string]interface{}{"a.image.repository": "set"},
		},
		{
			name: "value is not exported to dependency disabled by condition",
			opts: layeredValuesRenderOptions{Set: []string{"g.repo=set"}},
			want: map[string]interface{}{"a.image.repository": "set", "b.repo": "set", "t": absentValue{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.ChartDir = "export-import-edge-chart"
			result := renderLayeredValuesChart(t, tt.opts)

			for path, want := range tt.want {
				assert.Equal(t, want, valueAtPath(t, result.ReleaseConfig, path), path)
			}
		})
	}
}
