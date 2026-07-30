//go:build ai_tests

package ts_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/ts"
)

func TestAI_ExtractEmbeddedDeno(t *testing.T) {
	payload := []byte("#!/bin/sh\necho fake-deno\n")
	compressed, expectedSHA256 := gzipPayload(t, payload)

	expectedName := "deno"
	if runtime.GOOS == "windows" {
		expectedName = "deno.exe"
	}

	t.Run("extracts, caches on second call, and is content-addressed", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		path, err := ts.ExtractEmbeddedDeno(context.Background(), compressed, expectedSHA256)
		require.NoError(t, err)

		assert.Equal(t, expectedName, filepath.Base(path))
		assert.Contains(t, path, "embedded-"+expectedSHA256[:16])

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, payload, content)

		info, err := os.Stat(path)
		require.NoError(t, err)

		if runtime.GOOS != "windows" {
			assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		}

		cachedPath, err := ts.ExtractEmbeddedDeno(context.Background(), compressed, expectedSHA256)
		require.NoError(t, err)
		assert.Equal(t, path, cachedPath)

		cachedInfo, err := os.Stat(cachedPath)
		require.NoError(t, err)
		assert.Equal(t, info.ModTime(), cachedInfo.ModTime(), "cache hit must not rewrite the binary")

		leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), "*.tmp"))
		require.NoError(t, err)
		assert.Empty(t, leftovers, "no temp files must be left behind")
	})

	t.Run("fails on sha256 mismatch and leaves no binary", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		wrongSHA256 := strings.Repeat("0", 64)

		path, err := ts.ExtractEmbeddedDeno(context.Background(), compressed, wrongSHA256)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integrity check failed")
		assert.Empty(t, path)
	})

	t.Run("fails on corrupted gzip data", func(t *testing.T) {
		t.Setenv("XDG_CACHE_HOME", t.TempDir())

		path, err := ts.ExtractEmbeddedDeno(context.Background(), []byte("not gzip at all"), expectedSHA256)
		require.Error(t, err)
		assert.Empty(t, path)
	})
}

func TestAI_FindChecksum(t *testing.T) {
	const digest = "b7154ae42839d7b1453422e2f33c907e5c68fde8fe9f145cd43b8dd083671a6f"

	t.Run("parses plain sha256sum output", func(t *testing.T) {
		hash, found := ts.FindChecksum(digest + "  deno-x86_64-unknown-linux-gnu.zip\n")
		require.True(t, found)
		assert.Equal(t, digest, hash)
	})

	t.Run("parses PowerShell Get-FileHash output used for windows", func(t *testing.T) {
		body := "\r\nAlgorithm : SHA256\r\nHash      : " + strings.ToUpper(digest) +
			"\r\nPath      : C:\\a\\deno\\deno\\target\\release\\deno-x86_64-pc-windows-msvc.zip\r\n\r\n"

		hash, found := ts.FindChecksum(body)
		require.True(t, found)
		assert.Equal(t, digest, hash, "must be normalized to lowercase")
	})

	t.Run("reports not found when no digest is present", func(t *testing.T) {
		_, found := ts.FindChecksum("Algorithm : SHA256\r\nPath : nowhere\r\n")
		assert.False(t, found)
	})

	t.Run("ignores 64-char non-hex tokens", func(t *testing.T) {
		_, found := ts.FindChecksum(strings.Repeat("z", 64))
		assert.False(t, found)
	})
}

func gzipPayload(t *testing.T, payload []byte) ([]byte, string) {
	t.Helper()

	var buf bytes.Buffer

	writer := gzip.NewWriter(&buf)
	_, err := writer.Write(payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	sum := sha256.Sum256(payload)

	return buf.Bytes(), hex.EncodeToString(sum[:])
}
