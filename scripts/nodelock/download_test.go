package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseChecksums(t *testing.T) {
	const digestA = "b7154ae42839d7b1453422e2f33c907e5c68fde8fe9f145cd43b8dd083671a6f"
	const digestB = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	t.Run("parses sha256sum output", func(t *testing.T) {
		checksums, err := parseChecksums(digestA + "  node-v26.13.0-linux-x64.tar.gz\n" + strings.ToUpper(digestB) + "  node-v26.13.0-win-x64.zip\n\n")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{
			"node-v26.13.0-linux-x64.tar.gz": digestA,
			"node-v26.13.0-win-x64.zip":      digestB,
		}, checksums)
	})

	t.Run("rejects a malformed line", func(t *testing.T) {
		_, err := parseChecksums(digestA + "  a.tar.gz extra\n")
		assert.Error(t, err)
	})

	t.Run("rejects a malformed digest", func(t *testing.T) {
		_, err := parseChecksums(strings.Repeat("z", 64) + "  a.tar.gz\n")
		assert.Error(t, err)
	})

	t.Run("rejects an empty file", func(t *testing.T) {
		_, err := parseChecksums("\n")
		assert.Error(t, err)
	})
}
