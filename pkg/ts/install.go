package ts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	absTSDir, err := filepath.Abs(tsDir)
	if err != nil {
		return fmt.Errorf("get absolute path of %s: %w", tsDir, err)
	}

	hasPackageJSON, err := fileExists(filepath.Join(absTSDir, "package.json"))
	if err != nil {
		return fmt.Errorf("check package.json: %w", err)
	}

	if !hasPackageJSON {
		return checkNotDenoChart(ctx, absTSDir)
	}

	// The lock file is looked for among the ancestors of where the chart really is.
	tsDir, err = filepath.EvalSymlinks(absTSDir)
	if err != nil {
		return fmt.Errorf("resolve symlinks in %s: %w", absTSDir, err)
	}

	installDir, managerName, err := findLockFile(tsDir)
	if err != nil {
		return fmt.Errorf("find lock file of %s: %w", tsDir, err)
	}

	// Without a lock file the dependencies go to tsDir, as chart ts init installs them.
	lockFileFound := installDir != ""
	if !lockFileFound {
		installDir, managerName = tsDir, packageManagerNPM
	}

	for _, marker := range []string{"node_modules", ".pnp.cjs"} {
		if exists, err := fileExists(filepath.Join(installDir, marker)); err != nil {
			return fmt.Errorf("check installed dependencies: %w", err)
		} else if exists {
			log.Default.Debug(ctx, "Dependencies in %s are already installed: %s exists", installDir, marker)

			return nil
		}
	}

	if !lockFileFound && !createLockFile {
		return fmt.Errorf("no lock file in %s or above: run \"npm install\" in %s and commit the lock file", tsDir, tsDir)
	}

	args := []string{"install", "--ignore-scripts", "--no-audit", "--no-fund"}
	if lockFileFound {
		if args, err = installArgs(installDir, managerName); err != nil {
			return err
		}
	}

	// Otherwise npm and pnpm would install a workspace above that does not include the chart.
	if installDir == tsDir && managerName == packageManagerNPM {
		args = append(args, "--workspaces=false")
	} else if installDir == tsDir && managerName == packageManagerPNPM {
		args = append(args, "--ignore-workspace")
	}

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

	return nextStep(runPackageManager(ctx, installDir, managerPath, managerName, args))
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

func installArgs(dir, managerName string) ([]string, error) {
	switch managerName {
	case packageManagerNPM:
		return []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}, nil
	case packageManagerYarn:
		lockFile, err := os.ReadFile(filepath.Join(dir, "yarn.lock"))
		if err != nil {
			return nil, fmt.Errorf("read yarn.lock: %w", err)
		}

		if bytes.Contains(lockFile, []byte("__metadata:")) {
			return []string{"install", "--immutable", "--mode=skip-build"}, nil
		}

		return []string{"install", "--frozen-lockfile", "--ignore-scripts"}, nil
	default:
		return []string{"install", "--frozen-lockfile", "--ignore-scripts"}, nil
	}
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
