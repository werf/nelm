//go:build ai_tests

// This is an internal test on purpose: unpackBundle is what stands between a tampered archive and
// the file system, and there is no way to feed it a crafted archive through the exported API.
package schemas

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAI_EnsureExtractedRepairs covers the two ways an extraction directory can be wrong without
// being absent. Both used to be taken for a usable bundle, which makes kubeconform find no schema and
// validation pass silently.
func TestAI_EnsureExtractedRepairs(t *testing.T) {
	t.Run("re-extracts a bundle that lost files", func(t *testing.T) {
		setupCacheHome(t)

		ctx := context.Background()
		bundle := testKubernetesBundle(t)

		dir, err := bundle.ensureExtracted(ctx)
		require.NoError(t, err)

		marker := filepath.Join(dir, "deployment-apps-v1.json")
		require.NoError(t, os.Remove(marker))

		extracted, err := bundle.isExtracted(dir)
		require.NoError(t, err)
		assert.False(t, extracted, "a directory missing schemas must not count as extracted")

		forgetExtraction(dir)

		redone, err := bundle.ensureExtracted(ctx)
		require.NoError(t, err)
		assert.Equal(t, dir, redone)
		assert.FileExists(t, marker)
	})

	t.Run("re-extracts an emptied bundle", func(t *testing.T) {
		setupCacheHome(t)

		ctx := context.Background()
		bundle := testKubernetesBundle(t)

		dir, err := bundle.ensureExtracted(ctx)
		require.NoError(t, err)

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)

		for _, entry := range entries {
			require.NoError(t, os.RemoveAll(filepath.Join(dir, entry.Name())))
		}

		forgetExtraction(dir)

		redone, err := bundle.ensureExtracted(ctx)
		require.NoError(t, err)

		count, err := countFiles(redone)
		require.NoError(t, err)
		assert.Equal(t, bundle.index.FilesCount, count)
	})
}

// TestAI_RemoveStaleExtractionsKeepsEveryLiveBundle guards against the cleanup dropping the extraction
// of a bundle other than the one just extracted, which happens once that other bundle's directory has
// aged past staleExtractionLifetime while still being the current one.
func TestAI_RemoveStaleExtractionsKeepsEveryLiveBundle(t *testing.T) {
	setupCacheHome(t)

	ctx := context.Background()

	index, err := ReadIndex()
	require.NoError(t, err)

	crdsDir, err := newBundle(crdsBundleName, crdsArchiveFileName, crdsSchemaPathTemplate,
		crdsMaxPathDepth, index.CRDs).ensureExtracted(ctx)
	require.NoError(t, err)

	// Age the CRDs extraction past the cleanup threshold, as a cache untouched for a month would be.
	aged := time.Now().Add(-2 * staleExtractionLifetime)
	require.NoError(t, os.Chtimes(crdsDir, aged, aged))

	root := filepath.Dir(crdsDir)

	// A leftover of some other nelm version, which is what the cleanup is actually for.
	staleDir := filepath.Join(root, "kubernetes-000000000000")
	require.NoError(t, os.MkdirAll(staleDir, 0o755))
	require.NoError(t, os.Chtimes(staleDir, aged, aged))

	_, err = newBundle(kubernetesBundleName, kubernetesArchiveFileName, kubernetesSchemaPathTemplate,
		kubernetesMaxPathDepth, index.Kubernetes).ensureExtracted(ctx)
	require.NoError(t, err)

	assert.DirExists(t, crdsDir, "the CRDs extraction is live and must survive extracting another bundle")
	assert.NoDirExists(t, staleDir, "a bundle of another nelm version must be cleaned up")
}

func TestAI_UnpackArchive(t *testing.T) {
	t.Run("unpacks regular entries", func(t *testing.T) {
		destDir := t.TempDir()

		archive := buildTestArchive(t, []tar.Header{
			{Name: "deployment-apps-v1.json", Typeflag: tar.TypeReg},
			{Name: "configmap-v1.json", Typeflag: tar.TypeReg},
		})

		require.NoError(t, unpackArchive(destDir, bytes.NewReader(archive), kubernetesMaxPathDepth))

		for _, name := range []string{"deployment-apps-v1.json", "configmap-v1.json"} {
			content, err := os.ReadFile(filepath.Join(destDir, name))
			require.NoError(t, err)
			assert.Equal(t, "{}", string(content))
		}
	})

	t.Run("rejects entries that escape the destination", func(t *testing.T) {
		for _, name := range []string{
			"../escape.json",
			"nested/deployment.json",
			"/absolute.json",
			"..",
			".",
			`..\windows.json`,
			"a/b/c/deep.json",
		} {
			t.Run(name, func(t *testing.T) {
				destDir := t.TempDir()

				archive := buildTestArchive(t, []tar.Header{{Name: name, Typeflag: tar.TypeReg}})

				err := unpackArchive(destDir, bytes.NewReader(archive), kubernetesMaxPathDepth)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unexpected entry name")

				entries, err := os.ReadDir(destDir)
				require.NoError(t, err)
				assert.Empty(t, entries)
			})
		}
	})

	// Note that this is about the depth allowed in entry names, not about unpacking a part of a
	// bundle: a bundle is always written out whole, in a single streaming pass. The CRDs bundle is laid
	// out as "<group>/<kind>_<version>.json", so it needs one directory level to be accepted, while the
	// Kubernetes bundle is flat and must not have any.
	t.Run("accepts a group directory in an entry name when the bundle allows that depth", func(t *testing.T) {
		destDir := t.TempDir()

		archive := buildTestArchive(t, []tar.Header{
			{Name: "monitoring.coreos.com/prometheus_v1.json", Typeflag: tar.TypeReg},
		})

		require.NoError(t, unpackArchive(destDir, bytes.NewReader(archive), crdsMaxPathDepth))

		assert.FileExists(t, filepath.Join(destDir, "monitoring.coreos.com", "prometheus_v1.json"))
	})

	t.Run("rejects a group directory in a flat bundle", func(t *testing.T) {
		destDir := t.TempDir()

		archive := buildTestArchive(t, []tar.Header{
			{Name: "monitoring.coreos.com/prometheus_v1.json", Typeflag: tar.TypeReg},
		})

		err := unpackArchive(destDir, bytes.NewReader(archive), kubernetesMaxPathDepth)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected entry name")
	})

	t.Run("rejects escaping a group directory", func(t *testing.T) {
		destDir := t.TempDir()

		archive := buildTestArchive(t, []tar.Header{
			{Name: "../escape/prometheus_v1.json", Typeflag: tar.TypeReg},
		})

		err := unpackArchive(destDir, bytes.NewReader(archive), crdsMaxPathDepth)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected entry name")

		entries, err := os.ReadDir(destDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})

	t.Run("rejects non-regular entries", func(t *testing.T) {
		destDir := t.TempDir()

		archive := buildTestArchive(t, []tar.Header{
			{Name: "link.json", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
		})

		err := unpackArchive(destDir, bytes.NewReader(archive), kubernetesMaxPathDepth)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "non-regular entry")
	})

	t.Run("rejects a non-gzip bundle", func(t *testing.T) {
		err := unpackArchive(t.TempDir(), bytes.NewReader([]byte("not an archive")), kubernetesMaxPathDepth)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "gzip")
	})
}

func buildTestArchive(t *testing.T, headers []tar.Header) []byte {
	t.Helper()

	const content = "{}"

	var buf bytes.Buffer

	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, header := range headers {
		header.Mode = 0o644

		if header.Typeflag == tar.TypeReg {
			header.Size = int64(len(content))
		}

		// A crafted archive would not use the strict USTAR format our generator does.
		header.Format = tar.FormatPAX

		require.NoError(t, tarWriter.WriteHeader(&header))

		if header.Typeflag == tar.TypeReg {
			_, err := tarWriter.Write([]byte(content))
			require.NoError(t, err)
		}
	}

	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	return buf.Bytes()
}

// forgetExtraction drops the in-process record that a directory is unpacked, which is what makes the
// next call check the directory on disk again. A bundle is only ever unpacked once per process, so
// this is how a run that finds a damaged extraction left by an earlier one is reproduced.
func forgetExtraction(dir string) {
	extractionMu.Lock()
	defer extractionMu.Unlock()

	delete(extractedDirs, dir)
}

// setupCacheHome points the cache directory at a temporary one. Note that the vendored helmpath only
// honors XDG_CACHE_HOME, not HELM_CACHE_HOME.
func setupCacheHome(t *testing.T) string {
	t.Helper()

	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	return cacheHome
}

func testKubernetesBundle(t *testing.T) *embeddedBundle {
	t.Helper()

	index, err := ReadIndex()
	require.NoError(t, err)

	return newBundle(kubernetesBundleName, kubernetesArchiveFileName, kubernetesSchemaPathTemplate,
		kubernetesMaxPathDepth, index.Kubernetes)
}
