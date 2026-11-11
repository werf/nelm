package ts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/log"
)

const (
	packageManagerBun  = "bun"
	packageManagerNPM  = "npm"
	packageManagerPNPM = "pnpm"
	packageManagerYarn = "yarn"
)

var lockFiles = []struct {
	name           string
	packageManager string
}{
	{name: "package-lock.json", packageManager: packageManagerNPM},
	{name: "npm-shrinkwrap.json", packageManager: packageManagerNPM},
	{name: "pnpm-lock.yaml", packageManager: packageManagerPNPM},
	{name: "yarn.lock", packageManager: packageManagerYarn},
	{name: "bun.lock", packageManager: packageManagerBun},
	{name: "bun.lockb", packageManager: packageManagerBun},
}

// InstallDependencies installs the dependencies of the TypeScript chart in chartPath, as chart ts init
// does: unlike before bundling, a missing lock file is created rather than being an error.
func InstallDependencies(ctx context.Context, chartPath string) error {
	return ensureDependencies(ctx, filepath.Join(chartPath, common.ChartTSSourceDir), true)
}

// ensureDependencies installs the dependencies of tsDir, unless they are installed, with the package
// manager of the closest lock file, in its directory. Whether that directory is a workspace with tsDir
// in it is up to the user, as with Pulumi.
func ensureDependencies(ctx context.Context, tsDir string, createLockFile bool) error {
	// The lock file is looked for among the ancestors of where the chart really is, so this path is
	// also the one in the messages.
	tsDir, err := resolvePath(tsDir)
	if err != nil {
		return err
	}

	hasPackageJSON, err := fileExists(filepath.Join(tsDir, "package.json"))
	if err != nil {
		return fmt.Errorf("check package.json: %w", err)
	}

	if !hasPackageJSON {
		return checkNotDenoChart(ctx, tsDir)
	}

	installDir, managerName, err := findLockFile(tsDir)
	if err != nil {
		return fmt.Errorf("find lock file of %s: %w", tsDir, err)
	}

	// The workspace of the lock file and its lock file are up to the user, so chart ts init only tells
	// how to install.
	if createLockFile && installDir != "" && installDir != tsDir {
		log.Default.Warn(ctx, "Dependencies of %s are not installed: the closest lock file is in %s. Set up the workspace and/or install the dependencies manually.", tsDir, installDir)

		return nil
	}

	if installDir == "" && createLockFile {
		args := append([]string{"install", "--ignore-scripts", "--no-audit", "--no-fund"}, isolationArgs(packageManagerNPM)...)

		return runInstall(ctx, tsDir, packageManagerNPM, args, createLockFile)
	}

	if installDir == "" {
		installed, err := dependenciesInstalled(ctx, tsDir)
		if err != nil {
			return err
		}

		if !installed {
			return fmt.Errorf("no lock file in %s or above: run %q in %s and commit the lock file", tsDir, "npm install", tsDir)
		}

		log.Default.Warn(ctx, "No lock file in %s or above, so the dependencies installed there may differ elsewhere: run %q in %s and commit the lock file", tsDir, "npm install", tsDir)

		return nil
	}

	if installed, err := dependenciesInstalled(ctx, installDir); err != nil || installed {
		return err
	}

	yarnBerry, err := isYarnBerryLockFile(installDir, managerName)
	if err != nil {
		return err
	}

	args := installArgs(managerName, yarnBerry)
	if installDir == tsDir {
		args = append(args, isolationArgs(managerName)...)
	}

	return runInstall(ctx, installDir, managerName, args, createLockFile)
}

// runInstall runs the package manager, after checking that a yarn in PATH reads a yarn.lock of
// its format, which yarn classic would otherwise rewrite while running scripts of packages.
func runInstall(ctx context.Context, installDir, managerName string, args []string, createLockFile bool) error {
	// In chart ts init the chart files are already created, so the user only has to finish the install.
	nextStep := func(err error) error {
		if err == nil || !createLockFile {
			return err
		}

		return fmt.Errorf("%w\nThe chart files are created: once the problem is fixed, run %q in %s to install the dependencies", err, managerName+" "+strings.Join(args, " "), installDir)
	}

	managerPath, err := exec.LookPath(managerName)
	if err != nil {
		return nextStep(fmt.Errorf("install dependencies in %s: package manager %s is not found in PATH: %w", installDir, managerName, err))
	}

	if managerName == packageManagerYarn {
		if err := checkYarnVersion(ctx, installDir, managerPath); err != nil {
			return nextStep(err)
		}
	}

	return nextStep(runPackageManager(ctx, installDir, managerPath, managerName, args))
}

func checkYarnVersion(ctx context.Context, installDir, yarnPath string) error {
	yarnBerry, err := isYarnBerryLockFile(installDir, packageManagerYarn)
	if err != nil {
		return err
	}

	// Run in installDir, where .yarnrc.yml may set the yarn to use.
	cmd := exec.CommandContext(ctx, yarnPath, "--version")
	cmd.Dir = installDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("run \"yarn --version\" in %s: %w: %s", installDir, err, strings.TrimSpace(string(output)))
	}

	version := strings.TrimSpace(string(output))

	major, err := strconv.Atoi(strings.Split(version, ".")[0])
	if err != nil {
		return fmt.Errorf("parse version %q printed by \"yarn --version\" in %s: %w", version, installDir, err)
	}

	switch {
	case yarnBerry && major < 2:
		return fmt.Errorf("yarn.lock in %s is of Yarn 2 or later, but yarn in PATH is %s: put the yarn of the project in PATH", installDir, version)
	case !yarnBerry && major >= 2:
		return fmt.Errorf("yarn.lock in %s is of Yarn 1, but yarn in PATH is %s: put the yarn of the project in PATH", installDir, version)
	}

	return nil
}

// installHint tells how to install the dependencies of tsDir, for the errors of bundling.
func installHint(tsDir string) (string, error) {
	tsDir, err := resolvePath(tsDir)
	if err != nil {
		return "", err
	}

	installDir, managerName, err := findLockFile(tsDir)
	if err != nil {
		return "", fmt.Errorf("find lock file of %s: %w", tsDir, err)
	}

	switch installDir {
	case "":
		return fmt.Sprintf("run %q in %s and commit the lock file", "npm install", tsDir), nil
	case tsDir:
		yarnBerry, err := isYarnBerryLockFile(installDir, managerName)
		if err != nil {
			return "", err
		}

		args := append(installArgs(managerName, yarnBerry), isolationArgs(managerName)...)

		return fmt.Sprintf("run %q in %s", managerName+" "+strings.Join(args, " "), tsDir), nil
	default:
		return fmt.Sprintf("the closest lock file is in %s, so make sure %s is in the workspace there and run %q in %s", installDir, tsDir, managerName+" install", installDir), nil
	}
}

func checkNotDenoChart(ctx context.Context, tsDir string) error {
	for _, denoConfig := range []string{"deno.json", "deno.jsonc"} {
		if exists, err := fileExists(filepath.Join(tsDir, denoConfig)); err != nil {
			return fmt.Errorf("check %s: %w", denoConfig, err)
		} else if exists {
			return fmt.Errorf("%s has %s but no package.json: TypeScript charts now run on Node.js. Create package.json and tsconfig.json there like \"nelm chart ts init\" generates them, move the dependencies from %s to package.json and remove %s", tsDir, denoConfig, denoConfig, denoConfig)
		}
	}

	log.Default.Debug(ctx, "No package.json in %s, nothing to install", tsDir)

	return nil
}

func dependenciesInstalled(ctx context.Context, installDir string) (bool, error) {
	for _, marker := range []string{"node_modules", ".pnp.cjs"} {
		if exists, err := fileExists(filepath.Join(installDir, marker)); err != nil {
			return false, fmt.Errorf("check installed dependencies: %w", err)
		} else if exists {
			log.Default.Debug(ctx, "Dependencies in %s are already installed: %s exists", installDir, marker)

			return true, nil
		}
	}

	return false, nil
}

// findLockFile returns the closest directory to tsDir, tsDir included, with a lock file and the
// package manager of the lock file. It looks up to the first directory with .git or to the root of
// the filesystem, and returns an empty directory if there is no lock file.
func findLockFile(tsDir string) (string, string, error) {
	for dir := tsDir; ; dir = filepath.Dir(dir) {
		var found, managers []string

		for _, lockFile := range lockFiles {
			if exists, err := fileExists(filepath.Join(dir, lockFile.name)); err != nil {
				return "", "", fmt.Errorf("check %s: %w", lockFile.name, err)
			} else if exists {
				found = append(found, lockFile.name)
				managers = lo.Uniq(append(managers, lockFile.packageManager))
			}
		}

		if len(managers) > 1 {
			return "", "", fmt.Errorf("lock files of several package managers in %s: %s: keep the one of the package manager in use", dir, strings.Join(found, ", "))
		}

		if len(managers) == 1 {
			return dir, managers[0], nil
		}

		if isGitRoot, err := fileExists(filepath.Join(dir, ".git")); err != nil {
			return "", "", fmt.Errorf("check .git: %w", err)
		} else if isGitRoot || filepath.Dir(dir) == dir {
			return "", "", nil
		}
	}
}

func installArgs(managerName string, yarnBerry bool) []string {
	switch managerName {
	case packageManagerNPM:
		return []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}
	case packageManagerPNPM, packageManagerBun:
		return []string{"install", "--frozen-lockfile", "--ignore-scripts"}
	case packageManagerYarn:
		if yarnBerry {
			return []string{"install", "--immutable", "--mode=skip-build"}
		}

		return []string{"install", "--frozen-lockfile", "--ignore-scripts"}
	default:
		panic(fmt.Sprintf("unexpected package manager %q", managerName))
	}
}

func isYarnBerryLockFile(dir, managerName string) (bool, error) {
	if managerName != packageManagerYarn {
		return false, nil
	}

	lockFile, err := os.ReadFile(filepath.Join(dir, "yarn.lock"))
	if err != nil {
		return false, fmt.Errorf("read yarn.lock: %w", err)
	}

	return bytes.Contains(lockFile, []byte("__metadata:")), nil
}

// isolationArgs keep npm and pnpm, installing in ts/, from installing a workspace above instead.
func isolationArgs(managerName string) []string {
	switch managerName {
	case packageManagerNPM:
		return []string{"--workspaces=false"}
	case packageManagerPNPM:
		return []string{"--ignore-workspace"}
	case packageManagerYarn, packageManagerBun:
		return nil
	default:
		panic(fmt.Sprintf("unexpected package manager %q", managerName))
	}
}

func resolvePath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("get absolute path of %s: %w", path, err)
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks in %s: %w", absPath, err)
	}

	return resolvedPath, nil
}

func runPackageManager(ctx context.Context, dir, managerPath, managerName string, args []string) error {
	command := managerName + " " + strings.Join(args, " ")

	log.Default.Info(ctx, "Run %q in %s", command, dir)

	var output bytes.Buffer

	cmd := exec.CommandContext(ctx, managerPath, args...)
	cmd.Dir = dir
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		log.Default.Error(ctx, "%s output: %s", command, output.String())

		return fmt.Errorf("run %q in %s: %w", command, dir, err)
	}

	log.Default.Debug(ctx, "%s output: %s", command, output.String())

	return nil
}
