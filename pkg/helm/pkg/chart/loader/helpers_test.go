package loader

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
)

// assertNoTSDependencies checks that neither the chart nor its only subchart has files installed
// by a package manager into ts/, while their TypeScript sources and yarn project files are kept.
func assertNoTSDependencies(t *testing.T, loaded helmchart.Charter) {
	t.Helper()

	accessor, err := helmchart.NewAccessor(loaded)
	require.NoError(t, err)

	assertTSDependencyNames(t, append(testFileNames(accessor.Files()), testFileNames(accessor.RuntimeFiles())...))

	require.Len(t, accessor.Dependencies(), 1)

	dep, err := helmchart.NewAccessor(accessor.Dependencies()[0])
	require.NoError(t, err)

	assertTSDependencyNames(t, append(testFileNames(dep.Files()), testFileNames(dep.RuntimeFiles())...))
}

func assertTSDependencyNames(t *testing.T, names []string) {
	t.Helper()

	assert.Contains(t, names, "ts/src/index.ts")

	for path := range tsProjectYarnFiles {
		assert.Contains(t, names, path)
	}

	for _, name := range names {
		assert.NotContains(t, name, "node_modules")
		assert.NotContains(t, name, ".pnp.")

		for _, skipped := range []string{"cache", "unplugged", "__virtual__", "install-state.gz", "build-state.yml"} {
			assert.NotContains(t, name, ".yarn/"+skipped)
		}
	}
}

func tarGzTestChart(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer

	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for name, content := range files {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg}))

		_, err := tarWriter.Write([]byte(content))
		require.NoError(t, err)
	}

	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzWriter.Close())

	return buf.Bytes()
}
