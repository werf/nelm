package ts_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/ts"
)

func TestEnsureDependencies(t *testing.T) {
	const packageJSON = `{"type": "module", "private": true}`

	for name, test := range map[string]struct {
		files          map[string]string
		createLockFile bool
		wantManager    string
		wantArgs       []string
	}{
		"runs npm ci with package-lock.json": {
			files:       map[string]string{"ts/package-lock.json": "{}"},
			wantManager: "npm",
			wantArgs:    []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"},
		},
		"runs npm ci with npm-shrinkwrap.json": {
			files:       map[string]string{"ts/npm-shrinkwrap.json": "{}"},
			wantManager: "npm",
			wantArgs:    []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"},
		},
		"runs a frozen pnpm install with pnpm-lock.yaml": {
			files:       map[string]string{"ts/pnpm-lock.yaml": ""},
			wantManager: "pnpm",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"runs a frozen yarn classic install with yarn.lock": {
			files:       map[string]string{"ts/yarn.lock": ""},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"runs an immutable yarn berry install with yarn.lock and .yarnrc.yml": {
			files:       map[string]string{"ts/yarn.lock": "", "ts/.yarnrc.yml": ""},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--immutable", "--mode=skip-build"},
		},
		"runs a frozen bun install with bun.lock": {
			files:       map[string]string{"ts/bun.lock": ""},
			wantManager: "bun",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"runs a frozen bun install with bun.lockb": {
			files:       map[string]string{"ts/bun.lockb": ""},
			wantManager: "bun",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"takes the package manager from packageManager over lock files of others": {
			files: map[string]string{
				"ts/package.json":      `{"packageManager": "pnpm@10.20.0+sha512.abc"}`,
				"ts/package-lock.json": "{}",
				"ts/pnpm-lock.yaml":    "",
				"ts/yarn.lock":         "",
			},
			wantManager: "pnpm",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"takes yarn berry from the version in packageManager": {
			files: map[string]string{
				"ts/package.json": `{"packageManager": "yarn@4.10.3"}`,
				"ts/yarn.lock":    "",
			},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--immutable", "--mode=skip-build"},
		},
		"takes yarn classic from the version in packageManager": {
			files: map[string]string{
				"ts/package.json": `{"packageManager": "yarn@1.22.22"}`,
				"ts/yarn.lock":    "",
			},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"takes the package manager from devEngines": {
			files: map[string]string{
				"ts/package.json":   `{"devEngines": {"packageManager": {"name": "bun", "version": "^1.3.0"}}}`,
				"ts/pnpm-lock.yaml": "",
				"ts/bun.lock":       "",
			},
			wantManager: "bun",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"takes the first package manager from a list in devEngines": {
			files: map[string]string{
				"ts/package.json":      `{"devEngines": {"packageManager": [{"name": "pnpm"}, {"name": "npm"}]}}`,
				"ts/package-lock.json": "{}",
				"ts/pnpm-lock.yaml":    "",
			},
			wantManager: "pnpm",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"installs into a workspace that does not include ts": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["packages/*"]}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"},
		},
		"installs into a workspace that excludes ts": {
			files: map[string]string{
				"pnpm-workspace.yaml":  "packages:\n  - '**'\n  - '!ts'\n",
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"},
		},
		"does not look for a workspace above the git repository": {
			files: map[string]string{
				"../package.json":      `{"workspaces": ["**"]}`,
				".git/":                "",
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"},
		},
		"creates the npm lock file in init": {
			createLockFile: true,
			wantManager:    "npm",
			wantArgs:       []string{"install", "--ignore-scripts", "--no-audit", "--no-fund"},
		},
		"creates the pnpm lock file in init": {
			files:          map[string]string{"ts/package.json": `{"packageManager": "pnpm@10.20.0"}`},
			createLockFile: true,
			wantManager:    "pnpm",
			wantArgs:       []string{"install", "--ignore-scripts"},
		},
		"creates the yarn berry lock file in init": {
			files:          map[string]string{"ts/package.json": `{"packageManager": "yarn@4.10.3"}`},
			createLockFile: true,
			wantManager:    "yarn",
			wantArgs:       []string{"install", "--mode=skip-build"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			binDir := stubPath(t)
			for _, manager := range []string{"npm", "pnpm", "yarn", "bun"} {
				writeStub(t, binDir, manager, "")
			}

			t.Setenv("NELM_TEST_CANARY", "inherited")

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, map[string]string{"ts/package.json": packageJSON})
			writeFiles(t, projectDir, test.files)

			tsDir := filepath.Join(projectDir, "ts")
			require.NoError(t, ts.EnsureDependencies(context.Background(), tsDir, test.createLockFile))

			run := readStubRun(t, binDir, test.wantManager)
			assert.Equal(t, test.wantArgs, run.args)
			assert.Equal(t, resolvePath(t, tsDir), run.dir)
			assert.Equal(t, "inherited", run.canary, "the package manager must inherit the environment")
		})
	}

	for name, test := range map[string]struct {
		files   map[string]string
		wantErr []string
	}{
		"fails with lock files of several package managers": {
			files:   map[string]string{"ts/package-lock.json": "{}", "ts/yarn.lock": ""},
			wantErr: []string{"lock files of several package managers: package-lock.json, yarn.lock", "packageManager"},
		},
		"fails with an unsupported package manager": {
			files:   map[string]string{"ts/package.json": `{"packageManager": "deno@2.9.4"}`},
			wantErr: []string{`unsupported package manager "deno" in packageManager of package.json`},
		},
		"fails without a lock file": {
			wantErr: []string{"no lock file of npm in", `run "npm install" there and commit the lock file`},
		},
		"fails without a lock file of the chosen package manager": {
			files:   map[string]string{"ts/package.json": `{"packageManager": "pnpm@10.20.0"}`, "ts/package-lock.json": "{}"},
			wantErr: []string{"no lock file of pnpm in"},
		},
		"fails when ts is in an npm workspace": {
			files: map[string]string{
				"package.json":      `{"workspaces": ["./ts/"]}`,
				"package-lock.json": "{}",
			},
			wantErr: []string{"are not installed", `run "npm install" in workspace root`},
		},
		"fails when ts is in a yarn classic workspace": {
			files: map[string]string{
				"package.json": `{"workspaces": {"packages": ["*"]}}`,
				"yarn.lock":    "",
			},
			wantErr: []string{`run "yarn install" in workspace root`},
		},
		"fails when ts is in a pnpm workspace": {
			files: map[string]string{
				"pnpm-workspace.yaml": "packages:\n  - '**/ts'\n",
				"pnpm-lock.yaml":      "",
			},
			wantErr: []string{`run "pnpm install" in workspace root`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			binDir := stubPath(t)
			for _, manager := range []string{"npm", "pnpm", "yarn", "bun"} {
				writeStub(t, binDir, manager, "")
			}

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, map[string]string{"ts/package.json": packageJSON})
			writeFiles(t, projectDir, test.files)

			err := ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "ts"), false)
			require.Error(t, err)

			for _, want := range test.wantErr {
				assert.Contains(t, err.Error(), want)
			}

			for _, manager := range []string{"npm", "pnpm", "yarn", "bun"} {
				requireStubNotRun(t, binDir, manager)
			}
		})
	}

	for name, files := range map[string]map[string]string{
		"there is no package.json": {"ts/package-lock.json": "{}", "ts/src/index.js": ""},
		"node_modules exists":      {"ts/package.json": packageJSON, "ts/package-lock.json": "{}", "ts/node_modules/": ""},
		".pnp.cjs exists":          {"ts/package.json": packageJSON, "ts/yarn.lock": "", "ts/.pnp.cjs": ""},
		"node_modules exists in the workspace root": {
			"package.json":      `{"workspaces": ["ts"]}`,
			"package-lock.json": "{}",
			"node_modules/":     "",
			"ts/package.json":   packageJSON,
		},
	} {
		t.Run("installs nothing when "+name, func(t *testing.T) {
			binDir := stubPath(t)
			writeStub(t, binDir, "npm", "")
			writeStub(t, binDir, "yarn", "")

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, files)

			require.NoError(t, ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "ts"), true))

			requireStubNotRun(t, binDir, "npm")
			requireStubNotRun(t, binDir, "yarn")
		})
	}

	t.Run("fails without the package manager in PATH and does not fall back to another", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "npm", "")

		projectDir := filepath.Join(t.TempDir(), "project")
		writeFiles(t, projectDir, map[string]string{"ts/package.json": packageJSON, "ts/pnpm-lock.yaml": ""})

		err := ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "ts"), false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "package manager pnpm is not found in PATH")
		requireStubNotRun(t, binDir, "npm")
	})

	t.Run("fails when the package manager fails", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "npm", "printf 'npm error code E404\\n' >&2\nexit 1\n")

		projectDir := filepath.Join(t.TempDir(), "project")
		writeFiles(t, projectDir, map[string]string{"ts/package.json": packageJSON, "ts/package-lock.json": "{}"})

		err := ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "ts"), false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `run "npm ci --ignore-scripts --no-audit --no-fund"`)
		assert.Contains(t, err.Error(), "exit status 1")
	})
}

func TestInstallDependencies(t *testing.T) {
	binDir := stubPath(t)
	writeStub(t, binDir, "npm", "")

	chartDir := filepath.Join(t.TempDir(), "chart")
	writeFiles(t, chartDir, map[string]string{"ts/package.json": "{}"})

	require.NoError(t, ts.InstallDependencies(context.Background(), chartDir))

	run := readStubRun(t, binDir, "npm")
	assert.Equal(t, []string{"install", "--ignore-scripts", "--no-audit", "--no-fund"}, run.args)
	assert.Equal(t, resolvePath(t, filepath.Join(chartDir, "ts")), run.dir)
}
