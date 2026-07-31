//go:build ai_tests

package ts_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/common"
	helmchart "github.com/werf/nelm/pkg/helm/pkg/chart"
	v2chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/pkg/ts"
)

func TestAI_GetDenoBinaryCtxEmbeddedData(t *testing.T) {
	payload := []byte("#!/bin/sh\necho fake-deno\n")
	compressed, _ := gzipPayload(t, payload)

	t.Run("explicit binary path wins over ctx data without extraction", func(t *testing.T) {
		cacheHome := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", cacheHome)

		explicit := filepath.Join(t.TempDir(), "deno")
		require.NoError(t, os.WriteFile(explicit, payload, 0o755))

		ctx := ts.NewContextWithTSOptions(context.Background(), common.TypeScriptOptions{
			EmbeddedDenoCompressed: compressed,
		})

		path, err := ts.GetDenoBinary(ctx, explicit)
		require.NoError(t, err)
		assert.Equal(t, explicit, path)
		assertNoEmbeddedCacheDirs(t, cacheHome)
	})

	// An embedder can only hand over the release the lock pins, so bytes that are not it are rejected
	// rather than run.
	t.Run("ctx bytes that are not the pinned release are rejected", func(t *testing.T) {
		cacheHome := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", cacheHome)

		ctx := ts.NewContextWithTSOptions(context.Background(), common.TypeScriptOptions{
			EmbeddedDenoCompressed: compressed,
		})

		path, err := ts.GetDenoBinary(ctx, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "integrity check failed")
		assert.Empty(t, path)
	})

	t.Run("empty ctx options fall through to download cache", func(t *testing.T) {
		if embeddedDenoEnabled {
			t.Skip("embedded deno blob takes priority over the download cache")
		}

		cacheHome := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", cacheHome)

		link, err := ts.GetDownloadLink(runtime.GOOS, runtime.GOARCH)
		require.NoError(t, err)

		cacheDir, err := ts.GetDenoFolder(link)
		require.NoError(t, err)

		cachedBin := filepath.Join(cacheDir, ts.DenoBinaryName(runtime.GOOS))
		require.NoError(t, os.WriteFile(cachedBin, payload, 0o755))

		path, err := ts.GetDenoBinary(ts.NewContextWithTSOptions(context.Background(), common.TypeScriptOptions{}), "")
		require.NoError(t, err)
		assert.Equal(t, cachedBin, path)
		assertNoEmbeddedCacheDirs(t, cacheHome)
	})

	t.Run("chart without TS files never triggers extraction even with invalid ctx data", func(t *testing.T) {
		cacheHome := t.TempDir()
		t.Setenv("XDG_CACHE_HOME", cacheHome)

		ctx := ts.NewContextWithTSOptions(context.Background(), common.TypeScriptOptions{
			EmbeddedDenoCompressed: []byte("not gzip at all"),
		})

		chrt := &v2chart.Chart{Metadata: &v2chart.Metadata{Name: "no-ts", Version: "0.1.0", APIVersion: "v2"}}
		acc, err := helmchart.NewAccessor(chrt)
		require.NoError(t, err)

		require.NoError(t, ts.BundleChartsRecursive(ctx, acc, t.TempDir(), false, ""))
		assertNoEmbeddedCacheDirs(t, cacheHome)
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
