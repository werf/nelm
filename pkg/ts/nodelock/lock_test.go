package nodelock_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/ts/nodelock"
)

func TestCommittedLock(t *testing.T) {
	lock, err := nodelock.Read()
	require.NoError(t, err)

	t.Run("pins a version without the leading v", func(t *testing.T) {
		assert.NotEmpty(t, lock.Version)
		assert.False(t, strings.HasPrefix(lock.Version, "v"))
		assert.NotEmpty(t, lock.NPMVersion)
	})

	t.Run("pins a digest of the archive, the binary and npm for every platform", func(t *testing.T) {
		require.NotEmpty(t, lock.Platforms)

		for platform, entry := range lock.Platforms {
			assert.NotEmpty(t, entry.Target, platform)

			for name, digest := range map[string]string{"archive": entry.ArchiveSHA256, "binary": entry.BinarySHA256, "npm": entry.NPMSHA256} {
				assert.Len(t, digest, 64, "%s %s digest", platform, name)
				assert.Equal(t, strings.ToLower(digest), digest, "%s %s digest must be lowercase", platform, name)

				_, err := hex.DecodeString(digest)
				assert.NoError(t, err, "%s %s digest must be hex", platform, name)
			}
		}
	})

	t.Run("builds the release URL of the pinned version", func(t *testing.T) {
		url, err := nodelock.ArchiveURL("linux", "amd64")
		require.NoError(t, err)
		assert.Equal(t, "https://nodejs.org/dist/v"+lock.Version+"/node-v"+lock.Version+"-linux-x64.tar.gz", url)

		url, err = nodelock.ArchiveURL("windows", "amd64")
		require.NoError(t, err)
		assert.Equal(t, "https://nodejs.org/dist/v"+lock.Version+"/node-v"+lock.Version+"-win-x64.zip", url)
	})

	t.Run("rejects a platform it does not pin", func(t *testing.T) {
		_, err := nodelock.Get("plan9", "mips")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported platform: plan9/mips")
	})
}

func TestLockValidate(t *testing.T) {
	const digest = "b7154ae42839d7b1453422e2f33c907e5c68fde8fe9f145cd43b8dd083671a6f"

	valid := func() *nodelock.Lock {
		return &nodelock.Lock{
			NPMVersion: "11.20.0",
			Platforms: map[string]*nodelock.Platform{
				"linux/amd64": {ArchiveSHA256: digest, BinarySHA256: digest, NPMSHA256: digest, Target: "linux-x64"},
			},
			Version: "26.13.0",
		}
	}

	require.NoError(t, valid().Validate())

	for name, mutate := range map[string]func(*nodelock.Lock){
		"no version":             func(l *nodelock.Lock) { l.Version = "" },
		"version with leading v": func(l *nodelock.Lock) { l.Version = "v26.13.0" },
		"no npm version":         func(l *nodelock.Lock) { l.NPMVersion = "" },
		"no platforms":           func(l *nodelock.Lock) { l.Platforms = nil },
		"platform key without os": func(l *nodelock.Lock) {
			l.Platforms = map[string]*nodelock.Platform{"amd64": l.Platforms["linux/amd64"]}
		},
		"nil entry": func(l *nodelock.Lock) { l.Platforms["linux/amd64"] = nil },
		"no target": func(l *nodelock.Lock) { l.Platforms["linux/amd64"].Target = "" },
		"truncated archive digest": func(l *nodelock.Lock) {
			l.Platforms["linux/amd64"].ArchiveSHA256 = digest[:32]
		},
		"uppercase binary digest": func(l *nodelock.Lock) {
			l.Platforms["linux/amd64"].BinarySHA256 = strings.ToUpper(digest)
		},
		"non hex npm digest": func(l *nodelock.Lock) {
			l.Platforms["linux/amd64"].NPMSHA256 = strings.Repeat("z", 64)
		},
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			lock := valid()
			mutate(lock)
			assert.Error(t, lock.Validate())
		})
	}
}

func TestReadArchive(t *testing.T) {
	type archiveFile struct {
		content string
		name    string
		symlink bool
	}

	unixFiles := []archiveFile{
		{content: "node", name: "node-v26.13.0-linux-x64/bin/node"},
		{name: "node-v26.13.0-linux-x64/bin/npm", symlink: true},
		{content: "header", name: "node-v26.13.0-linux-x64/include/node/node.h"},
		{content: "cli", name: "node-v26.13.0-linux-x64/lib/node_modules/npm/bin/npm-cli.js"},
		{content: "{}", name: "node-v26.13.0-linux-x64/lib/node_modules/npm/package.json"},
		{content: "{}", name: "node-v26.13.0-linux-x64/lib/node_modules/corepack/package.json"},
	}

	tarGz := func(t *testing.T, files []archiveFile) []byte {
		t.Helper()

		var buf bytes.Buffer

		gzWriter := gzip.NewWriter(&buf)
		tarWriter := tar.NewWriter(gzWriter)

		for _, file := range files {
			if file.symlink {
				require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: file.name, Typeflag: tar.TypeSymlink, Linkname: "../x", Mode: 0o777}))

				continue
			}

			require.NoError(t, tarWriter.WriteHeader(&tar.Header{Name: file.name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(file.content))}))
			_, err := tarWriter.Write([]byte(file.content))
			require.NoError(t, err)
		}

		require.NoError(t, tarWriter.Close())
		require.NoError(t, gzWriter.Close())

		return buf.Bytes()
	}

	read := func(t *testing.T, archive []byte, goos string) (string, map[string]string, error) {
		t.Helper()

		var binary string

		npmFiles := map[string]string{}

		err := nodelock.ReadArchive(bytes.NewReader(archive), int64(len(archive)), goos, nodelock.ReadArchiveOptions{
			OnBinary: func(content io.Reader) error {
				data, err := io.ReadAll(content)
				require.NoError(t, err)

				binary = string(data)

				return nil
			},
			OnNPMFile: func(path string, _ fs.FileMode, content io.Reader) error {
				data, err := io.ReadAll(content)
				require.NoError(t, err)

				npmFiles[path] = string(data)

				return nil
			},
		})
		if err != nil {
			return "", nil, fmt.Errorf("read archive: %w", err)
		}

		return binary, npmFiles, nil
	}

	t.Run("reads the binary and the npm package from a tar.gz", func(t *testing.T) {
		binary, npmFiles, err := read(t, tarGz(t, unixFiles), "linux")
		require.NoError(t, err)
		assert.Equal(t, "node", binary)
		assert.Equal(t, map[string]string{"bin/npm-cli.js": "cli", "package.json": "{}"}, npmFiles)
	})

	t.Run("reads the binary and the npm package from a windows zip", func(t *testing.T) {
		var buf bytes.Buffer

		zipWriter := zip.NewWriter(&buf)
		for _, file := range []archiveFile{
			{content: "node", name: "node-v26.13.0-win-x64/node.exe"},
			{content: "cmd", name: "node-v26.13.0-win-x64/npm.cmd"},
			{content: "cli", name: "node-v26.13.0-win-x64/node_modules/npm/bin/npm-cli.js"},
		} {
			writer, err := zipWriter.Create(file.name)
			require.NoError(t, err)
			_, err = writer.Write([]byte(file.content))
			require.NoError(t, err)
		}

		require.NoError(t, zipWriter.Close())

		binary, npmFiles, err := read(t, buf.Bytes(), "windows")
		require.NoError(t, err)
		assert.Equal(t, "node", binary)
		assert.Equal(t, map[string]string{"bin/npm-cli.js": "cli"}, npmFiles)
	})

	t.Run("fails without the binary", func(t *testing.T) {
		_, _, err := read(t, tarGz(t, unixFiles[1:]), "linux")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no bin/node in archive")
	})

	t.Run("fails without npm", func(t *testing.T) {
		_, _, err := read(t, tarGz(t, unixFiles[:3]), "linux")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no lib/node_modules/npm/ in archive")
	})

	t.Run("rejects a symlink in the npm package", func(t *testing.T) {
		files := slices.Concat(unixFiles, []archiveFile{{name: "node-v26.13.0-linux-x64/lib/node_modules/npm/bin/npx", symlink: true}})

		_, _, err := read(t, tarGz(t, files), "linux")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected entry")
	})

	t.Run("rejects a path escaping the npm package", func(t *testing.T) {
		files := slices.Concat(unixFiles, []archiveFile{{content: "x", name: "node-v26.13.0-linux-x64/lib/node_modules/npm/../../../../etc/passwd"}})

		_, _, err := read(t, tarGz(t, files), "linux")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsafe path")
	})
}

func TestTreeSHA256(t *testing.T) {
	const (
		digestA = "b7154ae42839d7b1453422e2f33c907e5c68fde8fe9f145cd43b8dd083671a6f"
		digestB = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	)

	tree := map[string]string{"bin/npm-cli.js": digestA, "package.json": digestB}

	assert.Equal(t, nodelock.TreeSHA256(tree), nodelock.TreeSHA256(map[string]string{"package.json": digestB, "bin/npm-cli.js": digestA}))
	assert.NotEqual(t, nodelock.TreeSHA256(tree), nodelock.TreeSHA256(map[string]string{"bin/npm-cli.js": digestB, "package.json": digestA}),
		"must depend on which file has which content")
	assert.NotEqual(t, nodelock.TreeSHA256(tree), nodelock.TreeSHA256(map[string]string{"bin/npm-cli.js": digestA}),
		"must depend on the set of files")
	assert.NotEqual(t, nodelock.TreeSHA256(tree), nodelock.TreeSHA256(map[string]string{"bin/npm.js": digestA, "package.json": digestB}),
		"must depend on paths")
}
