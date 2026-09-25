//go:build ai_tests

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAI_CheckBundlesSize(t *testing.T) {
	for _, tt := range []struct {
		crdsBytes       int
		errContains     string
		kubernetesBytes int
		name            string
		size            string
	}{
		{
			crdsBytes:       500,
			kubernetesBytes: 500, name: "passes under the limit", size: "1KiB",
		},
		{
			crdsBytes:       600,
			kubernetesBytes: 400, name: "passes exactly at the limit", size: "1000",
		},
		{
			crdsBytes:       601,
			errContains:     "over the 1000 B limit",
			kubernetesBytes: 400, name: "fails over the limit", size: "1000",
		},
		{
			crdsBytes:       2048,
			errContains:     "the embedded archives take",
			kubernetesBytes: 1, // Both archives count, so neither can grow unnoticed behind the other one being small.
			name:            "counts both archives", size: "1KiB",
		},
		{
			crdsBytes:       1,
			errContains:     `invalid max bundles size "many"`,
			kubernetesBytes: 1, name: "rejects an unparsable limit", size: "many",
		},
		{
			crdsBytes:       1,
			errContains:     "must be positive",
			kubernetesBytes: 1, name: "rejects a zero limit", size: "0",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			outputDir := t.TempDir()

			writeArchive(t, filepath.Join(outputDir, kubernetesArchiveFileName), tt.kubernetesBytes)
			writeArchive(t, filepath.Join(outputDir, crdsArchiveFileName), tt.crdsBytes)

			err := checkBundlesSize(context.Background(), options{maxBundlesSize: tt.size, outputDir: outputDir})

			if tt.errContains == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}

	t.Run("fails when an archive is missing", func(t *testing.T) {
		outputDir := t.TempDir()

		writeArchive(t, filepath.Join(outputDir, kubernetesArchiveFileName), 1)

		err := checkBundlesSize(context.Background(), options{maxBundlesSize: "1MiB", outputDir: outputDir})
		require.Error(t, err)
		assert.Contains(t, err.Error(), crdsArchiveFileName)
	})
}

func writeArchive(t *testing.T, path string, size int) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, make([]byte, size), 0o644))
}
