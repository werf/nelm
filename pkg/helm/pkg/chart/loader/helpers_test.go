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
// by a package manager into ts/, while their TypeScript sources are kept.
func assertNoTSDependencies(t *testing.T, loaded helmchart.Charter) {
	t.Helper()

	accessor, err := helmchart.NewAccessor(loaded)
	require.NoError(t, err)

	names := append(testFileNames(accessor.Files()), testFileNames(accessor.RuntimeFiles())...)
	assert.Contains(t, names, "ts/src/index.ts")
	assertNoTSDependencyNames(t, names)

	require.Len(t, accessor.Dependencies(), 1)

	dep, err := helmchart.NewAccessor(accessor.Dependencies()[0])
	require.NoError(t, err)

	depNames := append(testFileNames(dep.Files()), testFileNames(dep.RuntimeFiles())...)
	assert.Contains(t, depNames, "ts/src/index.ts")
	assertNoTSDependencyNames(t, depNames)
}

func assertNoTSDependencyNames(t *testing.T, names []string) {
	t.Helper()

	for _, name := range names {
		for _, part := range []string{"node_modules", ".yarn", ".pnp."} {
			assert.NotContains(t, name, part)
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
