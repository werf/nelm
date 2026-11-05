package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/ts"
)

func TestEnsureDependencies(t *testing.T) {
	missingNode := filepath.Join(t.TempDir(), "missing-node")

	newTSDir := func(t *testing.T, files ...string) string {
		t.Helper()

		tsDir := t.TempDir()
		for _, file := range files {
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(tsDir, file)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(tsDir, file), []byte("{}"), 0o644))
		}

		return tsDir
	}

	for name, files := range map[string][]string{
		"node_modules exists":      {"package.json", "package-lock.json", "node_modules/.package-lock.json"},
		".pnp.cjs exists":          {"package.json", "yarn.lock", ".pnp.cjs"},
		"there is no package.json": {"src/index.js"},
	} {
		t.Run("installs nothing when "+name, func(t *testing.T) {
			require.NoError(t, ts.EnsureDependencies(context.Background(), newTSDir(t, files...), missingNode))
		})
	}

	for _, lockFile := range []string{"yarn.lock", "pnpm-lock.yaml", "bun.lock", "bun.lockb"} {
		t.Run("fails when dependencies are locked by another package manager with "+lockFile, func(t *testing.T) {
			err := ts.EnsureDependencies(context.Background(), newTSDir(t, "package.json", lockFile), missingNode)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "dependencies in")
			assert.Contains(t, err.Error(), "are not installed")
			assert.Contains(t, err.Error(), lockFile)
		})
	}

	for name, test := range map[string]struct {
		files    []string
		wantArgs []string
	}{
		"runs npm ci when there is a package-lock.json": {
			files:    []string{"package.json", "package-lock.json"},
			wantArgs: []string{"ci", "--ignore-scripts"},
		},
		"prefers package-lock.json over lock files of other package managers": {
			files:    []string{"package.json", "package-lock.json", "yarn.lock"},
			wantArgs: []string{"ci", "--ignore-scripts"},
		},
		"runs npm install when there is no lock file": {
			files:    []string{"package.json"},
			wantArgs: []string{"install", "--ignore-scripts"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			skipWithoutShell(t)

			cacheDir := populateDownloadCache(t)
			t.Setenv("NELM_TEST_CANARY", "inherited")

			nodePath := writeFakeNode(t, filepath.Join(t.TempDir(), "node"))
			tsDir := newTSDir(t, test.files...)

			require.NoError(t, ts.EnsureDependencies(context.Background(), tsDir, nodePath))

			run := readFakeNodeRun(t, nodePath)
			assert.Equal(t, append([]string{filepath.Join(cacheDir, "npm", "bin", "npm-cli.js")}, test.wantArgs...), run.args)
			assert.Equal(t, resolvePath(t, tsDir), run.dir)
			assert.Contains(t, run.env, "NELM_TEST_CANARY=inherited", "npm must inherit the environment")
		})
	}
}

func TestRunNPMInstall(t *testing.T) {
	skipWithoutShell(t)

	cacheDir := populateDownloadCache(t)

	chartDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(chartDir, "ts"), 0o755))

	require.NoError(t, ts.RunNPMInstall(context.Background(), chartDir, ""))

	run := readFakeNodeRun(t, filepath.Join(cacheDir, "node"))
	assert.Equal(t, []string{filepath.Join(cacheDir, "npm", "bin", "npm-cli.js"), "install", "--ignore-scripts"}, run.args)
	assert.Equal(t, resolvePath(t, filepath.Join(chartDir, "ts")), run.dir)
}

func resolvePath(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)

	return resolved
}
