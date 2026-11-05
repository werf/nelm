package ts_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/ts"
	"github.com/werf/nelm/v2/pkg/ts/nodelock"
)

func TestExtractEmbeddedNPM(t *testing.T) {
	files := map[string]string{
		"bin/npm-cli.js":                   "// cli",
		"package.json":                     `{"version":"11.20.0"}`,
		"node_modules/abbrev/package.json": "{}",
	}
	compressed, expectedSHA256 := tarGzNPM(t, files)

	t.Run("extracts, caches on second call, and is content-addressed", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		npmDir, err := ts.ExtractEmbeddedNPM(context.Background(), compressed, expectedSHA256)
		require.NoError(t, err)
		assert.Contains(t, npmDir, "embedded-npm-"+expectedSHA256[:16])

		for name, content := range files {
			data, err := os.ReadFile(filepath.Join(npmDir, filepath.FromSlash(name)))
			require.NoError(t, err)
			assert.Equal(t, content, string(data))
		}

		cachedDir, err := ts.ExtractEmbeddedNPM(context.Background(), compressed, expectedSHA256)
		require.NoError(t, err)
		assert.Equal(t, npmDir, cachedDir)

		assertOnlyEntries(t, filepath.Dir(npmDir), "npm", "lock")
	})

	t.Run("fails on digest mismatch and leaves no package", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		wrongSHA256 := strings.Repeat("0", 64)

		npmDir, err := ts.ExtractEmbeddedNPM(context.Background(), compressed, wrongSHA256)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integrity check failed")
		assert.Empty(t, npmDir)

		matches, err := filepath.Glob(filepath.Join(os.Getenv("XDG_CACHE_HOME"), "*", "nelm", "node", "embedded-npm-*", "*"))
		require.NoError(t, err)

		for _, match := range matches {
			assert.Equal(t, "lock", filepath.Base(match), "nothing but the lock must be left behind")
		}
	})

	t.Run("rejects a path escaping the package", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		malicious, digest := tarGzNPM(t, map[string]string{"../escaped": "x"})

		_, err := ts.ExtractEmbeddedNPM(context.Background(), malicious, digest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsafe path")
	})

	t.Run("rejects a path with a newline", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		ambiguous, digest := tarGzNPM(t, map[string]string{"a\nb": "x"})

		_, err := ts.ExtractEmbeddedNPM(context.Background(), ambiguous, digest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsafe path")
	})

	t.Run("rejects a symlink", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		var buf bytes.Buffer

		gzWriter := gzip.NewWriter(&buf)
		tarWriter := tar.NewWriter(gzWriter)
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: "bin/npm", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}))
		require.NoError(t, tarWriter.Close())
		require.NoError(t, gzWriter.Close())

		_, err := ts.ExtractEmbeddedNPM(context.Background(), buf.Bytes(), nodelock.TreeSHA256(map[string]string{}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected entry")
	})
}

func TestExtractEmbeddedNode(t *testing.T) {
	payload := []byte("#!/bin/sh\necho fake-node\n")
	compressed, expectedSHA256 := gzipPayload(t, payload)

	t.Run("extracts, caches on second call, and is content-addressed", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		path, err := ts.ExtractEmbeddedNode(context.Background(), compressed, expectedSHA256)
		require.NoError(t, err)

		assert.Equal(t, nodelock.BinaryName(runtime.GOOS), filepath.Base(path))
		assert.Contains(t, path, "embedded-"+expectedSHA256[:16])

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, payload, content)

		info, err := os.Stat(path)
		require.NoError(t, err)

		if runtime.GOOS != "windows" {
			assert.NotZero(t, info.Mode().Perm()&0o100, "must be executable")
		}

		cachedPath, err := ts.ExtractEmbeddedNode(context.Background(), compressed, expectedSHA256)
		require.NoError(t, err)
		assert.Equal(t, path, cachedPath)

		cachedInfo, err := os.Stat(cachedPath)
		require.NoError(t, err)
		assert.Equal(t, info.ModTime(), cachedInfo.ModTime(), "cache hit must not rewrite the binary")

		assertOnlyEntries(t, filepath.Dir(path), filepath.Base(path), "lock")
	})

	t.Run("fails on sha256 mismatch and leaves no binary", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		wrongSHA256 := strings.Repeat("0", 64)

		path, err := ts.ExtractEmbeddedNode(context.Background(), compressed, wrongSHA256)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integrity check failed")
		assert.Empty(t, path)
	})

	t.Run("fails on corrupted gzip data", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		path, err := ts.ExtractEmbeddedNode(context.Background(), []byte("not gzip at all"), expectedSHA256)
		require.Error(t, err)
		assert.Empty(t, path)
	})
}

func TestGetNPMCLI(t *testing.T) {
	t.Run("ctx bytes that are not the pinned npm are rejected", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		compressed, _ := tarGzNPM(t, map[string]string{"bin/npm-cli.js": "// cli"})

		ctx := ts.NewContextWithTSOptions(context.Background(), common.TypeScriptOptions{
			EmbeddedNPMCompressed: compressed,
		})

		_, err := ts.GetNPMCLI(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integrity check failed")
	})

	t.Run("empty ctx options fall through to download cache", func(t *testing.T) {
		cacheDir := populateDownloadCache(t)

		path, err := ts.GetNPMCLI(context.Background())
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(cacheDir, "npm", "bin", "npm-cli.js"), path)
	})
}

func TestGetNodeBinary(t *testing.T) {
	payload := []byte("#!/bin/sh\necho fake-node\n")
	compressed, _ := gzipPayload(t, payload)

	t.Run("explicit binary path wins over ctx data without extraction", func(t *testing.T) {
		cacheHome := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", cacheHome)

		explicit := filepath.Join(t.TempDir(), "node")
		require.NoError(t, os.WriteFile(explicit, payload, 0o755))

		ctx := ts.NewContextWithTSOptions(context.Background(), common.TypeScriptOptions{
			EmbeddedNodeCompressed: compressed,
		})

		path, err := ts.GetNodeBinary(ctx, explicit)
		require.NoError(t, err)
		assert.Equal(t, explicit, path)
		assertNoEmbeddedCacheDirs(t, cacheHome)
	})

	t.Run("missing explicit binary path is an error", func(t *testing.T) {
		_, err := ts.GetNodeBinary(context.Background(), filepath.Join(t.TempDir(), "missing"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "node binary not found")
	})

	t.Run("ctx bytes that are not the pinned release are rejected", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		ctx := ts.NewContextWithTSOptions(context.Background(), common.TypeScriptOptions{
			EmbeddedNodeCompressed: compressed,
		})

		path, err := ts.GetNodeBinary(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integrity check failed")
		assert.Empty(t, path)
	})

	t.Run("empty ctx options fall through to download cache", func(t *testing.T) {
		cacheDir := populateDownloadCache(t)

		path, err := ts.GetNodeBinary(ts.NewContextWithTSOptions(context.Background(), common.TypeScriptOptions{}), "")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(cacheDir, nodelock.BinaryName(runtime.GOOS)), path)
	})
}

func assertNoEmbeddedCacheDirs(t *testing.T, cacheHome string) {
	t.Helper()

	var matches []string
	require.NoError(t, filepath.WalkDir(cacheHome, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if strings.HasPrefix(d.Name(), "embedded-") {
			matches = append(matches, path)
		}

		return nil
	}))
	assert.Empty(t, matches)
}

func assertOnlyEntries(t *testing.T, dir string, names ...string) {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(dir, "*"))
	require.NoError(t, err)

	actual := lo.Map(paths, func(path string, _ int) string { return filepath.Base(path) })

	assert.ElementsMatch(t, names, actual, "no temporary files must be left behind")
}
