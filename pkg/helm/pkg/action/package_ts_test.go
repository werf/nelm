package action

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/featgate"
)

func TestRun_TypeScriptChartIsPackagedAsIs(t *testing.T) {
	t.Setenv(featgate.FeatGateTypescript.EnvVarName(), "true")
	t.Setenv("PATH", t.TempDir())

	const bundle = "export const prebuilt = true;\n"

	chartDir := filepath.Join(t.TempDir(), "mychart")
	for name, content := range map[string]string{
		"Chart.yaml":                     "apiVersion: v2\nname: mychart\nversion: 1.0.0\n",
		"ts/package.json":                `{"type": "module", "private": true}`,
		"ts/src/index.ts":                "export {};\n",
		"ts/dist/bundle.js":              bundle,
		"charts/sub/Chart.yaml":          "apiVersion: v2\nname: sub\nversion: 1.0.0\n",
		"charts/sub/ts/package.json":     `{"type": "module", "private": true}`,
		"charts/sub/ts/src/index.ts":     "export {};\n",
		"charts/sub/ts/dist/bundle.js":   bundle,
	} {
		path := filepath.Join(chartDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	client := NewPackage()
	client.Destination = t.TempDir()

	archivePath, err := client.Run(chartDir, nil)
	require.NoError(t, err, "helm package must need neither Node.js nor a package manager")

	archiveFile, err := os.Open(archivePath)
	require.NoError(t, err)
	defer archiveFile.Close()

	gzipReader, err := gzip.NewReader(archiveFile)
	require.NoError(t, err)

	bundles := map[string]string{}
	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}

		require.NoError(t, err)

		if filepath.Base(header.Name) == "bundle.js" {
			content, err := io.ReadAll(tarReader)
			require.NoError(t, err)

			bundles[header.Name] = string(content)
		}
	}

	assert.Equal(t, map[string]string{
		"mychart/ts/dist/bundle.js":            bundle,
		"mychart/charts/sub/ts/dist/bundle.js": bundle,
	}, bundles)
}
