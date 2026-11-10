package ts_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/ts"
)

var allPackageManagers = []string{"npm", "pnpm", "yarn", "bun"}

func TestEnsureDependencies(t *testing.T) {
	const packageJSON = `{"type": "module", "private": true}`

	npmCI := []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}

	// Keeps npm from installing the workspace of an ancestor that does not include ts.
	npmCIOutsideWorkspace := append(slices.Clone(npmCI), "--workspaces=false")
	pnpmFrozen := []string{"install", "--frozen-lockfile", "--ignore-scripts", "--ignore-workspace"}

	for name, test := range map[string]struct {
		files          map[string]string
		tsDir          string
		managersInPath []string
		createLockFile bool
		wantManager    string
		wantArgs       []string
	}{
		"runs npm ci with package-lock.json": {
			files:       map[string]string{"ts/package-lock.json": "{}"},
			wantManager: "npm",
			wantArgs:    npmCI,
		},
		"runs npm ci with npm-shrinkwrap.json": {
			files:       map[string]string{"ts/npm-shrinkwrap.json": "{}"},
			wantManager: "npm",
			wantArgs:    npmCI,
		},
		"runs a frozen pnpm install ignoring workspaces above with pnpm-lock.yaml": {
			files:       map[string]string{"ts/pnpm-lock.yaml": ""},
			wantManager: "pnpm",
			wantArgs:    pnpmFrozen,
		},
		"runs a frozen pnpm install in ts that is a pnpm workspace itself": {
			files:       map[string]string{"ts/pnpm-lock.yaml": "", "ts/pnpm-workspace.yaml": "packages:\n  - 'lib/*'\n"},
			wantManager: "pnpm",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"runs a frozen yarn classic install with yarn.lock": {
			files:       map[string]string{"ts/yarn.lock": "# yarn lockfile v1\n"},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"runs an immutable yarn berry install with yarn.lock and .yarnrc.yml": {
			files:       map[string]string{"ts/yarn.lock": "", "ts/.yarnrc.yml": ""},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--immutable", "--mode=skip-build"},
		},
		"takes yarn berry from __metadata in yarn.lock": {
			files:       map[string]string{"ts/yarn.lock": "__metadata:\n  version: 8\n  cacheKey: 10c0\n"},
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
			wantArgs:    pnpmFrozen,
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
		"takes yarn classic from a version range in devEngines below 2": {
			files: map[string]string{
				"ts/package.json": `{"devEngines": {"packageManager": {"name": "yarn", "version": "<2.0.0"}}}`,
				"ts/yarn.lock":    "# yarn lockfile v1\n",
			},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"takes yarn classic from a caret range of 1 in devEngines": {
			files: map[string]string{
				"ts/package.json": `{"devEngines": {"packageManager": {"name": "yarn", "version": "^1"}}}`,
				"ts/yarn.lock":    "",
			},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"takes yarn classic from an x-range of 1 in devEngines": {
			files: map[string]string{
				"ts/package.json": `{"devEngines": {"packageManager": {"name": "yarn", "version": "1.x"}}}`,
				"ts/yarn.lock":    "",
			},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"takes yarn classic from a range starting below 2 in devEngines": {
			files: map[string]string{
				"ts/package.json": `{"devEngines": {"packageManager": {"name": "yarn", "version": "^4.0.0 || >=1.22.0"}}}`,
				"ts/yarn.lock":    "",
			},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--frozen-lockfile", "--ignore-scripts"},
		},
		"takes yarn berry from a range starting at 2 or above in devEngines": {
			files: map[string]string{
				"ts/package.json": `{"devEngines": {"packageManager": {"name": "yarn", "version": ">=3.2.0 <5"}}}`,
				"ts/yarn.lock":    "",
			},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--immutable", "--mode=skip-build"},
		},
		"takes yarn berry from the version range in devEngines": {
			files: map[string]string{
				"ts/package.json": `{"devEngines": {"packageManager": {"name": "yarn", "version": "^4.0.0"}}}`,
				"ts/yarn.lock":    "",
			},
			wantManager: "yarn",
			wantArgs:    []string{"install", "--immutable", "--mode=skip-build"},
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
		"takes the first alternative from devEngines that has a lock file": {
			files: map[string]string{
				"ts/package.json":      `{"devEngines": {"packageManager": [{"name": "pnpm"}, {"name": "npm"}]}}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCI,
		},
		"takes the first alternative from devEngines with a lock file in their order": {
			files: map[string]string{
				"ts/package.json":      `{"devEngines": {"packageManager": [{"name": "pnpm"}, {"name": "npm"}]}}`,
				"ts/package-lock.json": "{}",
				"ts/pnpm-lock.yaml":    "",
			},
			wantManager: "pnpm",
			wantArgs:    pnpmFrozen,
		},
		"takes the first alternative from devEngines that is in PATH without lock files": {
			files:          map[string]string{"ts/package.json": `{"devEngines": {"packageManager": [{"name": "bun"}, {"name": "deno"}, {"name": "npm"}]}}`},
			managersInPath: []string{"npm"},
			createLockFile: true,
			wantManager:    "npm",
			wantArgs:       []string{"install", "--ignore-scripts", "--no-audit", "--no-fund"},
		},
		"installs into a workspace that does not include ts": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["packages/*"]}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"installs into ts excluded from a workspace before it is included": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["!ts", "*"]}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"installs into ts excluded from a pnpm workspace whatever the order of the patterns": {
			files: map[string]string{
				"pnpm-workspace.yaml": "packages:\n  - '*'\n  - '!ts'\n  - 'ts'\n",
				"ts/pnpm-lock.yaml":   "",
			},
			wantManager: "pnpm",
			wantArgs:    pnpmFrozen,
		},
		"installs into ts not included in a pnpm workspace above it": {
			files: map[string]string{
				"pnpm-workspace.yaml": "packages:\n  - 'packages/*'\n",
				"pnpm-lock.yaml":      "",
				"ts/pnpm-lock.yaml":   "",
			},
			wantManager: "pnpm",
			wantArgs:    pnpmFrozen,
		},
		"installs into ts under a dot directory that wildcards do not match": {
			files: map[string]string{
				"package.json":               `{"workspaces": ["**"]}`,
				".helm/ts/package.json":      packageJSON,
				".helm/ts/package-lock.json": "{}",
			},
			tsDir:       ".helm/ts",
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"installs into ts under a dot directory that a wildcard does not match": {
			files: map[string]string{
				"package.json":               `{"workspaces": ["*/ts"]}`,
				".helm/ts/package.json":      packageJSON,
				".helm/ts/package-lock.json": "{}",
			},
			tsDir:       ".helm/ts",
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"installs into ts excluded by a negated character class": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["*", "!t[!x]"]}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"installs into ts in braces without alternatives, which are literal": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["{ts}"]}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"installs into ts under a test directory, which pnpm never takes into a workspace": {
			files: map[string]string{
				"pnpm-workspace.yaml":    "packages:\n  - '**'\n",
				"pnpm-lock.yaml":         "",
				"test/ts/package.json":   packageJSON,
				"test/ts/pnpm-lock.yaml": "",
			},
			tsDir:       "test/ts",
			wantManager: "pnpm",
			wantArgs:    pnpmFrozen,
		},
		"installs into ts under a dot directory that a pnpm exclusion matches": {
			files: map[string]string{
				"pnpm-workspace.yaml":     "packages:\n  - '.helm/*'\n  - '!**/ts'\n",
				"pnpm-lock.yaml":          "",
				".helm/ts/package.json":   packageJSON,
				".helm/ts/pnpm-lock.yaml": "",
			},
			tsDir:       ".helm/ts",
			wantManager: "pnpm",
			wantArgs:    pnpmFrozen,
		},
		"installs into ts listed in package.json of a pnpm workspace that excludes it": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["ts"]}`,
				"pnpm-workspace.yaml":  "packages:\n  - 'packages/*'\n",
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"ignores workspaces in package.json of a pnpm project": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["*"]}`,
				"pnpm-lock.yaml":       "",
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"installs into ts outside the workspace of an ancestor with another package manager": {
			files: map[string]string{
				"package.json":         `{"packageManager": "pnpm@10.34.9", "workspaces": ["ts"]}`,
				"package-lock.json":    "{}",
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"keeps the own workspaces of ts under an ancestor workspace": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["packages/*"]}`,
				"ts/package.json":      `{"workspaces": ["lib/*"]}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCI,
		},
		"ignores an ancestor package.json that cannot be decoded": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["ts"],}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCI,
		},
		"ignores an ancestor package.json with workspaces that cannot be decoded": {
			files: map[string]string{
				"package.json":         `{"workspaces": 42}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCI,
		},
		"ignores an ancestor workspace with a malformed pattern": {
			files: map[string]string{
				"package.json":         `{"workspaces": ["t[s"]}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"ignores an ancestor pnpm-workspace.yaml that cannot be decoded": {
			files: map[string]string{
				"pnpm-workspace.yaml":  "packages: [\n",
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCI,
		},
		"ignores an ancestor workspace whose package manager cannot be detected": {
			files: map[string]string{
				"package.json":         `{"packageManager": "deno@2.9.4", "workspaces": ["ts"]}`,
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCIOutsideWorkspace,
		},
		"reads package.json of ts with a byte order mark": {
			files: map[string]string{
				"ts/package.json":   "\ufeff" + `{"packageManager": "pnpm@10.34.9"}`,
				"ts/pnpm-lock.yaml": "",
			},
			wantManager: "pnpm",
			wantArgs:    pnpmFrozen,
		},
		"ignores pnpm-workspace.yaml of an npm project": {
			files: map[string]string{
				"package.json":         `{"packageManager": "npm@11.20.0"}`,
				"pnpm-workspace.yaml":  "packages:\n  - '*'\n",
				"ts/package-lock.json": "{}",
			},
			wantManager: "npm",
			wantArgs:    npmCI,
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
			wantArgs:       []string{"install", "--ignore-scripts", "--ignore-workspace"},
		},
		"creates the yarn berry lock file in init": {
			files:          map[string]string{"ts/package.json": `{"packageManager": "yarn@4.10.3"}`},
			createLockFile: true,
			wantManager:    "yarn",
			wantArgs:       []string{"install", "--no-immutable", "--mode=skip-build"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			binDir := stubPath(t)
			for _, manager := range lo.Ternary(test.managersInPath != nil, test.managersInPath, allPackageManagers) {
				writeStub(t, binDir, manager, "")
			}

			t.Setenv("NELM_TEST_CANARY", "inherited")

			tsDir := lo.Ternary(test.tsDir != "", test.tsDir, "ts")

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, map[string]string{tsDir + "/package.json": packageJSON})
			writeFiles(t, projectDir, test.files)

			tsPath := filepath.Join(projectDir, filepath.FromSlash(tsDir))
			require.NoError(t, ts.EnsureDependencies(context.Background(), tsPath, test.createLockFile))

			run := readStubRun(t, binDir, test.wantManager)
			assert.Equal(t, test.wantArgs, run.args)
			assert.Equal(t, resolvePath(t, tsPath), run.dir)
			assert.Equal(t, "inherited", run.canary, "the package manager must inherit the environment")
		})
	}

	for name, test := range map[string]struct {
		files          map[string]string
		tsDir          string
		createLockFile bool
		wantErr        []string
	}{
		"fails with lock files of several package managers": {
			files:   map[string]string{"ts/package-lock.json": "{}", "ts/yarn.lock": ""},
			wantErr: []string{"lock files of several package managers: package-lock.json, yarn.lock", "packageManager"},
		},
		"fails with an unsupported package manager": {
			files:   map[string]string{"ts/package.json": `{"packageManager": "deno@2.9.4"}`},
			wantErr: []string{`unsupported package manager "deno" in packageManager of package.json`},
		},
		"fails with only unsupported package managers in devEngines": {
			files:   map[string]string{"ts/package.json": `{"devEngines": {"packageManager": [{"name": "deno"}]}}`},
			wantErr: []string{`unsupported package manager "deno" in devEngines.packageManager of package.json`},
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
		"fails when ts is in a workspace matched with braces and classes": {
			files: map[string]string{
				"package.json":      `{"workspaces": ["{apps/*,t[sx]}"]}`,
				"package-lock.json": "{}",
			},
			wantErr: []string{`run "npm install" in workspace root`},
		},
		"fails when ts is in a workspace matched with a negated character class": {
			files: map[string]string{
				"package.json":      `{"workspaces": ["t[!x]"]}`,
				"package-lock.json": "{}",
			},
			wantErr: []string{`run "npm install" in workspace root`},
		},
		"fails when ts is in a workspace matched with a numeric range": {
			files: map[string]string{
				"package.json":      `{"workspaces": ["chart-{1..3}/ts"]}`,
				"package-lock.json": "{}",
			},
			tsDir:   "chart-2/ts",
			wantErr: []string{`run "npm install" in workspace root`},
		},
		"fails when ts is in a workspace matched with several globstars": {
			files: map[string]string{
				"package.json":      `{"workspaces": ["a/**/b/**/ts"]}`,
				"package-lock.json": "{}",
			},
			tsDir:   "a/x/b/ts",
			wantErr: []string{`run "npm install" in workspace root`},
		},
		"fails when ts under a dot directory is in a workspace naming it": {
			files: map[string]string{
				"package.json":      `{"workspaces": [".helm/*"]}`,
				"package-lock.json": "{}",
			},
			tsDir:   ".helm/ts",
			wantErr: []string{`run "npm install" in workspace root`},
		},
		"fails when ts is in a workspace above the git repository": {
			files: map[string]string{
				"../package.json": `{"workspaces": ["**"]}`,
				".git/":           "",
			},
			wantErr: []string{`run "npm install" in workspace root`},
		},
		"fails when ts is in a workspace with a byte order mark in package.json": {
			files: map[string]string{
				"package.json":      "\ufeff" + `{"workspaces": ["ts"]}`,
				"package-lock.json": "{}",
			},
			wantErr: []string{`run "npm install" in workspace root`},
		},
		"fails when package.json of ts cannot be decoded": {
			files:   map[string]string{"ts/package.json": `{"type": "module",}`, "ts/package-lock.json": "{}"},
			wantErr: []string{"decode", "package.json"},
		},
		"fails in init when ts is in a workspace that is already installed": {
			files: map[string]string{
				"package.json":      `{"workspaces": ["charts/*/ts"]}`,
				"package-lock.json": "{}",
				"node_modules/":     "",
			},
			tsDir:          "charts/new/ts",
			createLockFile: true,
			wantErr:        []string{"The chart files are created", `run "npm install" in workspace root`},
		},
		"fails in init when ts is in a workspace that is not installed": {
			files: map[string]string{
				"package.json":      `{"workspaces": ["charts/*/ts"]}`,
				"package-lock.json": "{}",
			},
			tsDir:          "charts/new/ts",
			createLockFile: true,
			wantErr:        []string{"The chart files are created", `run "npm install" in workspace root`},
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
		"fails when ts is in a pnpm workspace that is not installed yet": {
			files:   map[string]string{"pnpm-workspace.yaml": "packages:\n  - '*'\n"},
			wantErr: []string{`run "pnpm install" in workspace root`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			binDir := stubPath(t)
			for _, manager := range allPackageManagers {
				writeStub(t, binDir, manager, "")
			}

			tsDir := lo.Ternary(test.tsDir != "", test.tsDir, "ts")

			projectDir := filepath.Join(t.TempDir(), "project")
			writeFiles(t, projectDir, map[string]string{tsDir + "/package.json": packageJSON})
			writeFiles(t, projectDir, test.files)

			err := ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, filepath.FromSlash(tsDir)), test.createLockFile)
			require.Error(t, err)

			for _, want := range test.wantErr {
				assert.Contains(t, err.Error(), want)
			}

			for _, manager := range allPackageManagers {
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

			require.NoError(t, ts.EnsureDependencies(context.Background(), filepath.Join(projectDir, "ts"), false))

			requireStubNotRun(t, binDir, "npm")
			requireStubNotRun(t, binDir, "yarn")
		})
	}

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

func TestEnsureDependenciesThroughSymlink(t *testing.T) {
	binDir := stubPath(t)
	writeStub(t, binDir, "npm", "")

	repoDir := filepath.Join(t.TempDir(), "repo")
	writeFiles(t, repoDir, map[string]string{
		"package.json":               `{"workspaces": ["charts/*/ts"]}`,
		"package-lock.json":          "{}",
		"node_modules/":              "",
		"charts/app/ts/package.json": "{}",
	})

	linkDir := filepath.Join(t.TempDir(), "app")
	require.NoError(t, os.Symlink(filepath.Join(repoDir, "charts", "app"), linkDir))

	require.NoError(t, ts.EnsureDependencies(context.Background(), filepath.Join(linkDir, "ts"), false),
		"the workspace must be found among the ancestors of the symlink target")
	requireStubNotRun(t, binDir, "npm")
}

func TestInstallDependencies(t *testing.T) {
	t.Run("installs and creates the lock file", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "npm", "")

		chartDir := filepath.Join(t.TempDir(), "chart")
		writeFiles(t, chartDir, map[string]string{"ts/package.json": "{}"})

		require.NoError(t, ts.InstallDependencies(context.Background(), chartDir))

		run := readStubRun(t, binDir, "npm")
		assert.Equal(t, []string{"install", "--ignore-scripts", "--no-audit", "--no-fund"}, run.args)
		assert.Equal(t, resolvePath(t, filepath.Join(chartDir, "ts")), run.dir)
	})

	t.Run("tells how to finish when installing fails", func(t *testing.T) {
		binDir := stubPath(t)
		writeStub(t, binDir, "npm", "exit 1\n")

		chartDir := filepath.Join(t.TempDir(), "chart")
		writeFiles(t, chartDir, map[string]string{"ts/package.json": "{}"})

		err := ts.InstallDependencies(context.Background(), chartDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "The chart files are created")
		assert.Contains(t, err.Error(), `run "npm install" in `+resolvePath(t, filepath.Join(chartDir, "ts")))
	})

	t.Run("tells how to finish without the package manager in PATH", func(t *testing.T) {
		stubPath(t)

		chartDir := filepath.Join(t.TempDir(), "chart")
		writeFiles(t, chartDir, map[string]string{"ts/package.json": `{"packageManager": "pnpm@10.20.0"}`})

		err := ts.InstallDependencies(context.Background(), chartDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "package manager pnpm is not found in PATH")
		assert.Contains(t, err.Error(), `run "pnpm install" in `)
	})
}
