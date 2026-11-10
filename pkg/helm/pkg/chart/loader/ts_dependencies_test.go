package loader

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	nelmcommon "github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
)

// tsDependencyFiles are files that npm, pnpm and yarn berry with Plug'n'Play install into ts/.
var tsDependencyFiles = map[string]string{
	"ts/node_modules/dep/index.js": "module.exports = {};\n",
	"ts/node_modules/dep/dep.prov": "provenance\n",
	"ts/.yarn/install-state.gz":    "state\n",
	"ts/.yarn/cache/dep.zip":       "zip\n",
	"ts/.pnp.cjs":                  "pnp\n",
	"ts/.pnp.loader.mjs":           "pnp loader\n",
}

// tsDependencyBigFiles are larger than a chart file may be, as native binaries of dependencies and
// Plug'n'Play data of large projects are.
var tsDependencyBigFiles = []string{
	"ts/node_modules/esbuild/bin/esbuild",
	"ts/.pnp.data.json",
	"ts/.yarn/unplugged/@esbuild-darwin-arm64-npm-0.28.2/node_modules/@esbuild/darwin-arm64/bin/esbuild",
}

func TestLoad_TSDependencies(t *testing.T) {
	contexts := map[string]context.Context{
		"vanilla": context.Background(),
		"werf": nelmcommon.ContextWithHelmOptions(context.Background(), nelmcommon.HelmOptions{
			ChartLoadOpts: nelmcommon.ChartLoadOptions{ChartType: nelmcommon.LegacyChartTypeChart, NoSecrets: true},
		}),
	}

	for _, apiVersion := range []string{"v2", "v3"} {
		for ctxName, ctx := range contexts {
			t.Run(apiVersion+"/"+ctxName+"/directory", func(t *testing.T) {
				dir := filepath.Join(t.TempDir(), "mychart")

				for path, content := range testChartWithTSDependencies(apiVersion) {
					writeTestFile(t, filepath.Join(dir, filepath.FromSlash(path)), content)
				}

				for _, chartDir := range []string{dir, filepath.Join(dir, "charts", "sub")} {
					for _, path := range tsDependencyBigFiles {
						bigFile := filepath.Join(chartDir, filepath.FromSlash(path))
						writeTestFile(t, bigFile, "")
						require.NoError(t, os.Truncate(bigFile, archive.MaxDecompressedFileSize+1))
					}
				}

				loaded, err := LoadDir(ctx, dir)
				require.NoError(t, err)
				assertNoTSDependencies(t, loaded)
			})

			t.Run(apiVersion+"/"+ctxName+"/archive", func(t *testing.T) {
				files := map[string]string{}
				for path, content := range testChartWithTSDependencies(apiVersion) {
					files["mychart/"+path] = content
				}

				for _, path := range tsDependencyBigFiles {
					files["mychart/"+path] = strings.Repeat("\x00", int(archive.MaxDecompressedFileSize)+1)
				}

				loaded, err := LoadArchive(ctx, bytes.NewReader(tarGzTestChart(t, files)))
				require.NoError(t, err)
				assertNoTSDependencies(t, loaded)
			})
		}
	}
}

func testChartWithTSDependencies(apiVersion string) map[string]string {
	files := map[string]string{
		"Chart.yaml":                 "apiVersion: " + apiVersion + "\nname: mychart\nversion: 1.0.0\n",
		"ts/src/index.ts":            "export {};\n",
		"charts/sub/Chart.yaml":      "apiVersion: v2\nname: sub\nversion: 1.0.0\n",
		"charts/sub/ts/src/index.ts": "export {};\n",
	}

	for path, content := range tsDependencyFiles {
		files[path] = content
		files["charts/sub/"+path] = content
	}

	return files
}
