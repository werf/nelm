//go:build ai_tests

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAI_FindChecksum(t *testing.T) {
	const digest = "b7154ae42839d7b1453422e2f33c907e5c68fde8fe9f145cd43b8dd083671a6f"

	t.Run("parses plain sha256sum output", func(t *testing.T) {
		hash, found := findChecksum(digest + "  deno-x86_64-unknown-linux-gnu.zip\n")
		require.True(t, found)
		assert.Equal(t, digest, hash)
	})

	t.Run("parses PowerShell Get-FileHash output used for windows", func(t *testing.T) {
		body := "\r\nAlgorithm : SHA256\r\nHash      : " + strings.ToUpper(digest) +
			"\r\nPath      : C:\\a\\deno\\deno\\target\\release\\deno-x86_64-pc-windows-msvc.zip\r\n\r\n"

		hash, found := findChecksum(body)
		require.True(t, found)
		assert.Equal(t, digest, hash, "must be normalized to lowercase")
	})

	t.Run("reports not found when no digest is present", func(t *testing.T) {
		_, found := findChecksum("Algorithm : SHA256\r\nPath : nowhere\r\n")
		assert.False(t, found)
	})

	t.Run("ignores 64-char non-hex tokens", func(t *testing.T) {
		_, found := findChecksum(strings.Repeat("z", 64))
		assert.False(t, found)
	})
}
