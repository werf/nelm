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

var allPackageManagers = []string{"npm", "pnpm", "yarn", "bun"}

func TestEnsureDependencies(t *testing.T) {
	const packageJSON = `{"type": "module", "private": true}`

	npmCI := []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}
	pnpmFrozen := []string{"install", "--frozen-lockfile", "--ignore-scripts"}
	yarnClassicFrozen := []string{"install", "--frozen-lockfile", "--ignore-scripts"}
	yarnBerryImmutable := []string{"install", "--immutable", "--mode=skip-build"}
	bunFrozen := []string{"install", "--frozen-lockfile", "--ignore-scripts"}

	for name, test := range map[string]struct {
		files       map[string]string
		tsDir       string
		yarnVersion string
		yarnStderr  string
		wantManager string
		wantArgs    []string
		wantDir     string
	}{
		"runs npm ci in ts with package-lock.json there, isolated from workspaces above": {
			files:       map[string]string{"ts/package-lock.json": "{}"},
			wantManager: "npm",
			wantArgs:    append(npmCI, "--workspaces=false"),
		},
		"runs npm ci in ts with npm-shrinkwrap.json there": {
			files:       map[string]string{"ts/npm-shrinkwrap.json": "{}"},
			wantManager: "npm",
			wantArgs:    append(npmCI, "--workspaces=false"),
		},
		"runs a frozen pnpm install in ts with pnpm-lock.yaml there, isolated from workspaces above": {
			files:       map[string]string{"ts/pnpm-lock.yaml": ""},
			wantManager: "pnpm",
			wantArgs:    append(pnpmFrozen, "--ignore-workspace"),
		},
		"runs a frozen yarn classic install in ts with yarn.lock there": {
			files:       map[string]string{"ts/yarn.lock": "# yarn lockfile v1\n"},
			wantManager: "yarn",
			wantArgs:    yarnClassicFrozen,
		},
		"runs an immutable yarn berry install with __metadata in yarn.lock": {
			files:       map[string]string{"ts/yarn.lock": "__metadata:\n  version: 8\n  cacheKey: 10c0\n"},
			yarnVersion: "4.18.1",
			wantManager: "yarn",
			wantArgs:    yarnBerryImmutable,
		},
		"runs a frozen bun install in ts with bun.lock there": {
			files:       map[string]string{"ts/bun.lock": ""},
			wantManager: "bun",
			wantArgs:    bunFrozen,
		},
		"runs a frozen bun install in ts with bun.lockb there": {
			files:       map[string]string{"ts/bun.lockb": ""},
			wantManager: "bun",
			wantArgs:    bunFrozen,
		},
		"takes package-lock.json and npm-shrinkwrap.json together as npm": {
			files:       map[string]string{"ts/package-lock.json": "{}", "ts/npm-shrinkwrap.json": "{}"},
			wantManager: "npm",
			wantArgs:    append(npmCI, "--workspaces=false"),
		},
		"runs npm ci in the ancestor with package-lock.json, not isolated": {
			files:       map[string]string{"package-lock.json": "{}"},
			tsDir:       "charts/app/ts",
			wantManager: "npm",
			wantArgs:    npmCI,
			wantDir:     ".",
		},
		"runs a frozen pnpm install in the ancestor with pnpm-lock.yaml, not isolated": {
			files:       map[string]string{"pnpm-lock.yaml": ""},
			tsDir:       "charts/app/ts",
			wantManager: "pnpm",
			wantArgs:    pnpmFrozen,
			wantDir:     ".",
		},
		"runs a yarn berry install with corepack downloading yarn": {
			files:       map[string]string{"ts/yarn.lock": "__metadata:\n  version: 8\n"},
			yarnVersion: "4.18.1",
			yarnStderr:  "! Corepack is about to download https://repo.yarnpkg.com/4.18.1/packages/yarnpkg-cli/bin/yarn.js\n\n",
			wantManager: "yarn",
			wantArgs:    yarnBerryImmutable,
		},
		"runs a yarn berry install in the ancestor with yarn.lock": {
			files:       map[string]string{"yarn.lock": "__metadata:\n  version: 8\n"},
			tsDir:       "charts/app/ts",
			yarnVersion: "2.4.3",
			wantManager: "yarn",
			wantArgs:    yarnBerryImmutable,
			wantDir:     ".",
		},
		"takes the closest lock file": {
			files:       map[string]string{"package-lock.json": "{}", "charts/app/pnpm-lock.yaml": ""},
			tsDir:       "charts/app/ts",
			wantManager: "pnpm",
			wantArgs:    pnpmFrozen,
			wantDir:     "charts/app",
		},
		"takes the lock file in ts over one in an ancestor": {
			files:       map[string]string{"pnpm-lock.yaml": "", "ts/package-lock.json": "{}"},
			wantManager: "npm",
			wantArgs:    append(npmCI, "--workspaces=false"),
		},
		"looks for a lock file in the directory with .git": {
			files:       map[string]string{".git/": "", "package-lock.json": "{}"},
			tsDir:       "charts/app/ts",
			wantManager: "npm",
			wantArgs:    npmCI,
			wantDir:     ".",
		},
		"takes a .git file of a worktree as the boundary too": {
			files:       map[string]string{"repo/.git": "gitdir: /elsewhere\n", "repo/yarn.lock": ""},
			tsDir:       "repo/ts",
			wantManager: "yarn",
			wantArgs:    yarnClassicFrozen,
			wantDir:     "repo",
		},
		"ignores package.json of ancestors": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["ts"], "packageManager": "pnpm@10.0.0",}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    append(npmCI, "--workspaces=false"),
		},
		"ignores packageManager and devEngines of ts": {
			files: map[string]string{
				"ts/package.json":      `{"packageManager": "pnpm@10.0.0", "devEngines": {"packageManager": {"name": "yarn"}}}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    append(npmCI, "--workspaces=false"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			binDir := stubPath(t)
			for _, manager := range allPackageManagers {
				writeStub(t, binDir, manager, "")
			}

			yarnVersion := test.yarnVersion
			if yarnVersion == "" {
				yarnVersion = "1.22.22"
			}

			writeYarnStub(t, binDir, yarnVersion, test.yarnStderr)

			// Corepack would otherwise add packageManager to package.json of the user.
			t.Setenv("COREPACK_ENABLE_AUTO_PIN", "1")
			t.Setenv("NELM_TEST_CANARY", "inherited")

			tsDir := test.tsDir
			if tsDir == "" {
				tsDir = "ts"
			}

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, map[string]string{tsDir + "/package.json": packageJSON})
			writeFiles(t, projectDir, test.files)

			require.NoError(t, ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, filepath.FromSlash(tsDir)), false))

			wantDir := test.wantDir
			if wantDir == "" {
				wantDir = tsDir
			}

			run := readStubRun(t, binDir, test.wantManager)
			assert.Equal(t, test.wantArgs, run.args)
			assert.Equal(t, "0", run.autoPin)
			assert.Equal(t, "inherited", run.canary, "the package manager must run with the environment of nelm")

			if test.wantManager == "yarn" {
				versionAutoPin, err := os.ReadFile(filepath.Join(binDir, "yarn.version-autopin"))
				require.NoError(t, err, "yarn --version was not run")
				assert.Equal(t, "0", string(versionAutoPin))
			}

			assert.Equal(t, resolvePath(t, filepath.Join(projectDir, filepath.FromSlash(wantDir))), run.dir)

			for _, manager := range allPackageManagers {
				if manager != test.wantManager {
					requireStubNotRun(t, binDir, manager)
				}
			}
		})
	}

	for name, test := range map[string]struct {
		files       map[string]string
		tsDir       string
		yarnVersion string
		wantErr     []string
	}{
		"fails with lock files of several package managers in one directory": {
			files:   map[string]string{"ts/package-lock.json": "{}", "ts/yarn.lock": ""},
			wantErr: []string{"lock files of several package managers", "package-lock.json", "yarn.lock"},
		},
		"fails with lock files of several package managers in an ancestor": {
			files:   map[string]string{"pnpm-lock.yaml": "", "bun.lock": ""},
			tsDir:   "charts/app/ts",
			wantErr: []string{"lock files of several package managers", "bun.lock", "pnpm-lock.yaml"},
		},
		"fails without a lock file": {
			wantErr: []string{"no lock file", `run "npm install" in`, "commit the lock file"},
		},
		"fails with a yarn.lock of yarn berry and yarn classic in PATH": {
			files:       map[string]string{"ts/yarn.lock": "__metadata:\n  version: 8\n"},
			yarnVersion: "1.22.22",
			wantErr:     []string{"yarn.lock", "is of Yarn 2 or later", "yarn in PATH is 1.22.22"},
		},
		"fails with a yarn.lock of yarn classic and yarn berry in PATH": {
			files:       map[string]string{"yarn.lock": "# yarn lockfile v1\n"},
			tsDir:       "charts/app/ts",
			yarnVersion: "4.18.1",
			wantErr:     []string{"yarn.lock", "is of Yarn 1", "yarn in PATH is 4.18.1"},
		},
		"fails when yarn cannot tell its version": {
			files:       map[string]string{"ts/yarn.lock": ""},
			yarnVersion: "unknown",
			wantErr:     []string{"yarn --version", `"unknown"`},
		},
		"fails without a lock file below .git with one above it": {
			files:   map[string]string{"package-lock.json": "{}", "repo/.git/": ""},
			tsDir:   "repo/charts/app/ts",
			wantErr: []string{"no lock file"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			binDir := stubPath(t)
			for _, manager := range allPackageManagers {
				writeStub(t, binDir, manager, "")
			}

			if test.yarnVersion != "" {
				writeYarnStub(t, binDir, test.yarnVersion, "")
			}

			tsDir := test.tsDir
			if tsDir == "" {
				tsDir = "ts"
			}

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, map[string]string{tsDir + "/package.json": packageJSON})
			writeFiles(t, projectDir, test.files)

			err := ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, filepath.FromSlash(tsDir)), false)
			require.Error(t, err)

			for _, want := range test.wantErr {
				assert.Contains(t, err.Error(), want)
			}

			for _, manager := range allPackageManagers {
				requireStubNotRun(t, binDir, manager)
			}
		})
	}

	for name, test := range map[string]struct {
		files map[string]string
		tsDir string
	}{
		"there is no package.json": {
			files: map[string]string{"ts/package-lock.json": "{}", "ts/src/index.js": ""},
		},
		"node_modules exists in ts with the lock file": {
			files: map[string]string{"ts/package.json": packageJSON, "ts/package-lock.json": "{}", "ts/node_modules/": ""},
		},
		".pnp.cjs exists in ts with the lock file": {
			files: map[string]string{"ts/package.json": packageJSON, "ts/yarn.lock": "", "ts/.pnp.cjs": ""},
		},
		"node_modules exists in the ancestor with the lock file": {
			files: map[string]string{"package-lock.json": "{}", "node_modules/": "", "charts/app/ts/package.json": packageJSON},
			tsDir: "charts/app/ts",
		},
		".pnp.cjs exists in the ancestor with the lock file": {
			files: map[string]string{"yarn.lock": "", ".pnp.cjs": "", "charts/app/ts/package.json": packageJSON},
			tsDir: "charts/app/ts",
		},
	} {
		t.Run("installs nothing when "+name, func(t *testing.T) {
			binDir := stubPath(t)
			for _, manager := range allPackageManagers {
				writeStub(t, binDir, manager, "")
			}

			tsDir := test.tsDir
			if tsDir == "" {
				tsDir = "ts"
			}

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, test.files)

			require.NoError(t, ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, filepath.FromSlash(tsDir)), false))

			for _, manager := range allPackageManagers {
				requireStubNotRun(t, binDir, manager)
			}
		})
	}

	for _, marker := range []string{"node_modules/", ".pnp.cjs"} {
		t.Run("warns without a lock file anywhere and "+marker+" in ts", func(t *testing.T) {
			logger := recordLogs(t)

			binDir := stubPath(t)
			writeStub(t, binDir, "npm", "")

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, map[string]string{"ts/package.json": packageJSON, "ts/" + marker: ""})

			require.NoError(t, ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "ts"), false))
			requireStubNotRun(t, binDir, "npm")

			tsDir := resolvePath(t, filepath.Join(projectDir, "ts"))
			assert.Equal(t, []string{`No lock file in ` + tsDir + ` or above, so the dependencies installed there may differ elsewhere: run "npm install" in ` + tsDir + ` and commit the lock file`}, logMessages(logger.entries(), "warn"))
		})
	}

	t.Run("tells the exact command without a lock file", func(t *testing.T) {
		stubPath(t)

		projectDir := filepath.Join(t.TempDir(), "project")
		writeFiles(t, projectDir, map[string]string{"ts/package.json": packageJSON})

		err := ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "ts"), false)
		require.Error(t, err)

		tsDir := resolvePath(t, filepath.Join(projectDir, "ts"))
		assert.Equal(t, `no lock file in `+tsDir+` or above: run "npm install" in `+tsDir+` and commit the lock file`, err.Error())
	})

	t.Run("installs with node_modules in ts when the lock file is in the ancestor", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "npm", "")

		projectDir := filepath.Join(t.TempDir(), "project")
		writeFiles(t, projectDir, map[string]string{
			"package-lock.json":           "{}",
			"charts/app/ts/package.json":  packageJSON,
			"charts/app/ts/node_modules/": "",
		})

		require.NoError(t, ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "charts", "app", "ts"), false))

		run := readStubRun(t, binDir, "npm")
		assert.Equal(t, resolvePath(t, projectDir), run.dir)
	})

	t.Run("fails for a chart set up for Deno", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "npm", "")

		projectDir := filepath.Join(t.TempDir(), "project")
		writeFiles(t, projectDir, map[string]string{"ts/deno.json": "{}", "ts/src/index.ts": ""})

		err := ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "ts"), false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "deno.json")
		assert.Contains(t, err.Error(), "TypeScript charts now run on Node.js")
		assert.Contains(t, err.Error(), "nelm chart ts init")
		requireStubNotRun(t, binDir, "npm")
	})

	t.Run("fails with the stderr of yarn when it cannot tell its version", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "yarn", "printf 'Usage Error: corepack cannot download yarn\\n' >&2\nexit 1\n")

		projectDir := filepath.Join(t.TempDir(), "project")
		writeFiles(t, projectDir, map[string]string{"ts/package.json": packageJSON, "ts/yarn.lock": ""})

		err := ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "ts"), false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `run "yarn --version" in `)
		assert.Contains(t, err.Error(), "exit status 1: Usage Error: corepack cannot download yarn")
	})

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
		assert.Contains(t, err.Error(), `run "npm ci --ignore-scripts --no-audit --no-fund --workspaces=false"`)
		assert.Contains(t, err.Error(), "exit status 1")
	})
}

func TestEnsureDependenciesThroughSymlink(t *testing.T) {
	binDir := stubPath(t)
	writeStub(t, binDir, "npm", "")

	repoDir := filepath.Join(t.TempDir(), "repo")
	writeFiles(t, repoDir, map[string]string{
		"package-lock.json":          "{}",
		"node_modules/":              "",
		"charts/app/ts/package.json": "{}",
	})

	linkDir := filepath.Join(t.TempDir(), "app")
	require.NoError(t, os.Symlink(filepath.Join(repoDir, "charts", "app"), linkDir))

	require.NoError(t, ts.EnsureDependencies(context.Background(), filepath.Join(linkDir, "ts"), false),
		"the lock file must be found among the ancestors of the symlink target")
	requireStubNotRun(t, binDir, "npm")
}

func TestInstallDependencies(t *testing.T) {
	t.Run("creates the lock file with npm in ts without one anywhere", func(t *testing.T) {
		binDir := stubPath(t)
		for _, manager := range allPackageManagers {
			writeStub(t, binDir, manager, "")
		}

		projectDir := filepath.Join(t.TempDir(), "project")
		writeFiles(t, projectDir, map[string]string{"pnpm-workspace.yaml": "packages:\n  - '*'\n", "chart/ts/package.json": "{}"})

		require.NoError(t, ts.InstallDependencies(context.Background(), filepath.Join(projectDir, "chart")))

		run := readStubRun(t, binDir, "npm")
		assert.Equal(t, []string{"install", "--ignore-scripts", "--no-audit", "--no-fund", "--workspaces=false"}, run.args)
		assert.Equal(t, resolvePath(t, filepath.Join(projectDir, "chart", "ts")), run.dir)
	})

	for name, files := range map[string]map[string]string{
		"npm workspace that is not installed": {"package.json": `{"workspaces": ["charts/*/ts"]}`, "package-lock.json": "{}"},
		"npm workspace that is installed without the dependencies of the chart": {
			"package.json":      `{"workspaces": ["charts/*/ts"]}`,
			"package-lock.json": "{}",
			"node_modules/":     "",
		},
		"pnpm workspace": {"pnpm-workspace.yaml": "packages:\n  - 'charts/*/ts'\n", "pnpm-lock.yaml": ""},
	} {
		t.Run("tells how to install without installing under the lock file of an ancestor in an "+name, func(t *testing.T) {
			logger := recordLogs(t)

			binDir := stubPath(t)
			for _, manager := range allPackageManagers {
				writeStub(t, binDir, manager, "")
			}

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, files)
			writeFiles(t, projectDir, map[string]string{"charts/app/ts/package.json": "{}"})

			require.NoError(t, ts.InstallDependencies(context.Background(), filepath.Join(projectDir, "charts", "app")))

			for _, manager := range allPackageManagers {
				requireStubNotRun(t, binDir, manager)
			}

			tsDir := resolvePath(t, filepath.Join(projectDir, "charts", "app", "ts"))
			assert.Equal(t, []string{"Dependencies of " + tsDir + " are not installed: the closest lock file is in " + resolvePath(t, projectDir) + ". Set up the workspace and/or install the dependencies manually."}, logMessages(logger.entries(), "warn"))
		})
	}

	t.Run("installs as the build does with the lock file in ts", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "npm", "")
		writeStub(t, binDir, "pnpm", "")

		projectDir := filepath.Join(t.TempDir(), "project")
		writeFiles(t, projectDir, map[string]string{"pnpm-lock.yaml": "", "charts/app/ts/package.json": "{}", "charts/app/ts/package-lock.json": "{}"})

		require.NoError(t, ts.InstallDependencies(context.Background(), filepath.Join(projectDir, "charts", "app")))

		run := readStubRun(t, binDir, "npm")
		assert.Equal(t, []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund", "--workspaces=false"}, run.args)
		assert.Equal(t, resolvePath(t, filepath.Join(projectDir, "charts", "app", "ts")), run.dir)
		requireStubNotRun(t, binDir, "pnpm")
	})

	t.Run("tells how to finish when installing fails", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "npm", "exit 1\n")

		chartDir := filepath.Join(t.TempDir(), "chart")
		writeFiles(t, chartDir, map[string]string{"ts/package.json": "{}"})

		err := ts.InstallDependencies(context.Background(), chartDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "The chart files are created")
		assert.Contains(t, err.Error(), `run "npm install --ignore-scripts --no-audit --no-fund --workspaces=false" in `+resolvePath(t, filepath.Join(chartDir, "ts")))
	})

	t.Run("tells how to finish with the lock file in ts without its package manager in PATH", func(t *testing.T) {
		stubPath(t)

		chartDir := filepath.Join(t.TempDir(), "chart")
		writeFiles(t, chartDir, map[string]string{"ts/package.json": "{}", "ts/pnpm-lock.yaml": ""})

		err := ts.InstallDependencies(context.Background(), chartDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "package manager pnpm is not found in PATH")
		assert.Contains(t, err.Error(), `The chart files are created: once the problem is fixed, run "pnpm install --frozen-lockfile --ignore-scripts --ignore-workspace" in `+resolvePath(t, filepath.Join(chartDir, "ts")))
	})

	t.Run("tells how to finish without the package manager in PATH", func(t *testing.T) {
		stubPath(t)

		chartDir := filepath.Join(t.TempDir(), "chart")
		writeFiles(t, chartDir, map[string]string{"ts/package.json": "{}"})

		err := ts.InstallDependencies(context.Background(), chartDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "package manager npm is not found in PATH")
		assert.Contains(t, err.Error(), "The chart files are created")
	})
}
