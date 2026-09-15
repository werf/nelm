//go:build ai_tests

package denolock_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/ts/denolock"
)

func TestAI_CommittedLock(t *testing.T) {
	lock, err := denolock.Read()
	require.NoError(t, err)

	t.Run("pins a version without the leading v", func(t *testing.T) {
		assert.NotEmpty(t, lock.Version)
		assert.False(t, strings.HasPrefix(lock.Version, "v"))
	})

	t.Run("pins a digest of both the archive and the binary for every platform", func(t *testing.T) {
		require.NotEmpty(t, lock.Platforms)

		for platform, entry := range lock.Platforms {
			assert.NotEmpty(t, entry.Target, platform)

			for name, digest := range map[string]string{"archive": entry.ArchiveSHA256, "binary": entry.BinarySHA256} {
				assert.Len(t, digest, 64, "%s %s digest", platform, name)
				assert.Equal(t, strings.ToLower(digest), digest, "%s %s digest must be lowercase", platform, name)

				_, err := hex.DecodeString(digest)
				assert.NoError(t, err, "%s %s digest must be hex", platform, name)
			}
		}
	})

	t.Run("builds the release URL of the pinned version", func(t *testing.T) {
		url, err := denolock.ArchiveURL("linux", "amd64")
		require.NoError(t, err)

		assert.Equal(t, "https://github.com/denoland/deno/releases/download/v"+lock.Version+
			"/deno-"+lock.Platforms["linux/amd64"].Target+".zip", url)
	})

	t.Run("rejects a platform it does not pin", func(t *testing.T) {
		_, err := denolock.Get("plan9", "mips")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported platform: plan9/mips")
	})
}

func TestAI_LockValidate(t *testing.T) {
	const digest = "b7154ae42839d7b1453422e2f33c907e5c68fde8fe9f145cd43b8dd083671a6f"

	valid := func() *denolock.Lock {
		return &denolock.Lock{
			Platforms: map[string]*denolock.Platform{
				"linux/amd64": {ArchiveSHA256: digest, BinarySHA256: digest, Target: "x86_64-unknown-linux-gnu"},
			},
			Version: "2.7.1",
		}
	}

	require.NoError(t, valid().Validate())

	for name, mutate := range map[string]func(*denolock.Lock){
		"no version":             func(l *denolock.Lock) { l.Version = "" },
		"version with leading v": func(l *denolock.Lock) { l.Version = "v2.7.1" },
		"no platforms":           func(l *denolock.Lock) { l.Platforms = nil },
		"platform key without os": func(l *denolock.Lock) {
			l.Platforms = map[string]*denolock.Platform{"amd64": l.Platforms["linux/amd64"]}
		},
		"nil entry": func(l *denolock.Lock) { l.Platforms["linux/amd64"] = nil },
		"no target": func(l *denolock.Lock) { l.Platforms["linux/amd64"].Target = "" },
		"truncated archive digest": func(l *denolock.Lock) {
			l.Platforms["linux/amd64"].ArchiveSHA256 = digest[:32]
		},
		"uppercase binary digest": func(l *denolock.Lock) {
			l.Platforms["linux/amd64"].BinarySHA256 = strings.ToUpper(digest)
		},
		"non hex binary digest": func(l *denolock.Lock) {
			l.Platforms["linux/amd64"].BinarySHA256 = strings.Repeat("z", 64)
		},
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			lock := valid()
			mutate(lock)
			assert.Error(t, lock.Validate())
		})
	}
}
