package ts

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/log"
)

var (
	installedDependenciesMarkers = []string{"node_modules", ".pnp.cjs"}
	foreignLockFiles             = []string{"yarn.lock", "pnpm-lock.yaml", "bun.lock", "bun.lockb"}
)

// RunNPMInstall installs the dependencies of the TypeScript chart in chartPath with npm, without
// running their install scripts. nodeBinaryPath is the Node.js binary to run npm with instead of the
// embedded or downloaded one.
func RunNPMInstall(ctx context.Context, chartPath, nodeBinaryPath string) error {
	return runNPM(ctx, filepath.Join(chartPath, common.ChartTSSourceDir), nodeBinaryPath, "install", "--ignore-scripts")
}

func ensureDependencies(ctx context.Context, tsDir, nodeBinaryPath string) error {
	for _, marker := range installedDependenciesMarkers {
		exists, err := fileExists(filepath.Join(tsDir, marker))
		if err != nil {
			return err
		}

		if exists {
			log.Default.Debug(ctx, "Dependencies in %s are already installed: %s exists", tsDir, marker)

			return nil
		}
	}

	// Without a package.json here npm would look for one in the parent directories and install
	// into whatever project it finds there.
	if exists, err := fileExists(filepath.Join(tsDir, "package.json")); err != nil {
		return err
	} else if !exists {
		log.Default.Debug(ctx, "No package.json in %s, nothing to install", tsDir)

		return nil
	}

	if exists, err := fileExists(filepath.Join(tsDir, "package-lock.json")); err != nil {
		return err
	} else if exists {
		return runNPM(ctx, tsDir, nodeBinaryPath, "ci", "--ignore-scripts")
	}

	for _, lockFile := range foreignLockFiles {
		exists, err := fileExists(filepath.Join(tsDir, lockFile))
		if err != nil {
			return err
		}

		if exists {
			return fmt.Errorf("dependencies in %s are not installed: install them with the package manager %s belongs to", tsDir, lockFile)
		}
	}

	return runNPM(ctx, tsDir, nodeBinaryPath, "install", "--ignore-scripts")
}

func runNPM(ctx context.Context, dir, nodeBinaryPath string, args ...string) error {
	nodeBin, err := getNodeBinary(ctx, nodeBinaryPath)
	if err != nil {
		return fmt.Errorf("ensure Node.js is available: %w", err)
	}

	npmCLI, err := getNPMCLI(ctx)
	if err != nil {
		return fmt.Errorf("ensure npm is available: %w", err)
	}

	npmCommand := "npm " + strings.Join(args, " ")

	log.Default.Info(ctx, "Run %q in %s", npmCommand, dir)

	cmd := exec.CommandContext(ctx, nodeBin, append([]string{npmCLI}, args...)...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			log.Default.Error(ctx, "%s error: %s", npmCommand, string(output))
		}

		return fmt.Errorf("run %s: %w", npmCommand, err)
	}

	log.Default.Debug(ctx, "%s output: %s", npmCommand, string(output))

	return nil
}
