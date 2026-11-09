package ts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
	"github.com/samber/lo"
	"sigs.k8s.io/yaml"

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

type packageJSON struct {
	DevEngines struct {
		PackageManager json.RawMessage `json:"packageManager"`
	} `json:"devEngines"`
	PackageManager string          `json:"packageManager"`
	Workspaces     json.RawMessage `json:"workspaces"`
}

type packageManager struct {
	name    string
	version string
}

// InstallDependencies installs the dependencies of the TypeScript chart in chartPath, as chart ts init
// does: unlike before bundling, a missing lock file is created rather than being an error.
func InstallDependencies(ctx context.Context, chartPath string) error {
	return ensureDependencies(ctx, filepath.Join(chartPath, common.ChartTSSourceDir), true)
}

func ensureDependencies(ctx context.Context, tsDir string, createLockFile bool) error {
	absTSDir, err := filepath.Abs(tsDir)
	if err != nil {
		return fmt.Errorf("get absolute path of %s: %w", tsDir, err)
	}

	if exists, err := fileExists(filepath.Join(absTSDir, "package.json")); err != nil {
		return err
	} else if !exists {
		log.Default.Debug(ctx, "No package.json in %s, nothing to install", absTSDir)

		return nil
	}

	// The workspace the chart belongs to is found among the ancestors of where it really is.
	tsDir, err = filepath.EvalSymlinks(absTSDir)
	if err != nil {
		return fmt.Errorf("resolve symlinks in %s: %w", absTSDir, err)
	}

	root, err := findWorkspaceRoot(tsDir)
	if err != nil {
		return fmt.Errorf("find workspace root of %s: %w", tsDir, err)
	}

	for _, marker := range []string{"node_modules", ".pnp.cjs"} {
		if exists, err := fileExists(filepath.Join(root, marker)); err != nil {
			return err
		} else if exists {
			log.Default.Debug(ctx, "Dependencies in %s are already installed: %s exists", root, marker)

			return nil
		}
	}

	manager, err := detectPackageManager(root)
	if err != nil {
		return fmt.Errorf("detect package manager of %s: %w", root, err)
	}

	if root != tsDir {
		return fmt.Errorf("dependencies of %s are not installed: run %q in workspace root %s", tsDir, manager.name+" install", root)
	}

	managerPath, err := exec.LookPath(manager.name)
	if err != nil {
		return fmt.Errorf("install dependencies of %s: package manager %s is not found in PATH", root, manager.name)
	}

	hasLockFile, err := hasLockFileOf(root, manager.name)
	if err != nil {
		return err
	}

	if !hasLockFile && !createLockFile {
		return fmt.Errorf("no lock file of %s in %s: run %q there and commit the lock file", manager.name, root, manager.name+" install")
	}

	yarnBerry, err := isYarnBerry(root, manager)
	if err != nil {
		return err
	}

	return runPackageManager(ctx, root, managerPath, manager.name, installArgs(manager.name, yarnBerry, hasLockFile))
}

// findWorkspaceRoot returns the closest ancestor of tsDir, up to the root of the git repository or of
// the filesystem, that declares a workspace including tsDir, or tsDir itself if there is none.
func findWorkspaceRoot(tsDir string) (string, error) {
	if isGitRoot, err := fileExists(filepath.Join(tsDir, ".git")); err != nil {
		return "", err
	} else if isGitRoot {
		return tsDir, nil
	}

	for dir := filepath.Dir(tsDir); ; dir = filepath.Dir(dir) {
		patterns, err := readWorkspacePatterns(dir)
		if err != nil {
			return "", err
		}

		relPath, err := filepath.Rel(dir, tsDir)
		if err != nil {
			return "", fmt.Errorf("get path of %s relative to %s: %w", tsDir, dir, err)
		}

		matched, err := matchesWorkspacePatterns(patterns, filepath.ToSlash(relPath))
		if err != nil {
			return "", fmt.Errorf("match workspace patterns of %s: %w", dir, err)
		}

		if matched {
			return dir, nil
		}

		isGitRoot, err := fileExists(filepath.Join(dir, ".git"))
		if err != nil {
			return "", err
		}

		if isGitRoot || filepath.Dir(dir) == dir {
			return tsDir, nil
		}
	}
}

// detectPackageManager takes the package manager from the packageManager or devEngines.packageManager
// field of package.json in root, or else from the lock file in root, defaulting to npm.
func detectPackageManager(root string) (packageManager, error) {
	manifest, _, err := readPackageJSON(root)
	if err != nil {
		return packageManager{}, err
	}

	if manifest.PackageManager != "" {
		name, version, _ := strings.Cut(manifest.PackageManager, "@")
		version, _, _ = strings.Cut(version, "+")

		return validatePackageManager(packageManager{name: name, version: version}, "packageManager")
	}

	if name, err := devEnginesPackageManagerName(manifest.DevEngines.PackageManager); err != nil {
		return packageManager{}, fmt.Errorf("decode devEngines.packageManager: %w", err)
	} else if name != "" {
		return validatePackageManager(packageManager{name: name}, "devEngines.packageManager")
	}

	var foundLockFiles, foundManagers []string

	for _, lockFile := range lockFiles {
		exists, err := fileExists(filepath.Join(root, lockFile.name))
		if err != nil {
			return packageManager{}, err
		}

		if exists {
			foundLockFiles = append(foundLockFiles, lockFile.name)
			foundManagers = append(foundManagers, lockFile.packageManager)
		}
	}

	switch foundManagers = lo.Uniq(foundManagers); len(foundManagers) {
	case 0:
		return packageManager{name: packageManagerNPM}, nil
	case 1:
		return packageManager{name: foundManagers[0]}, nil
	default:
		return packageManager{}, fmt.Errorf("lock files of several package managers: %s, set the packageManager field in package.json to choose one", strings.Join(foundLockFiles, ", "))
	}
}

// matchesWorkspacePatterns reports whether relPath, slash-separated, is matched by the workspace glob
// patterns, where a "!"-prefixed pattern excludes what the patterns before it included.
func matchesWorkspacePatterns(patterns []string, relPath string) (bool, error) {
	var included bool

	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		pattern = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(pattern, "!"), "./"), "/")

		matched, err := matchWorkspacePattern(pattern, relPath)
		if err != nil {
			return false, err
		}

		if matched {
			included = !negated
		}
	}

	return included, nil
}

func readWorkspacePatterns(dir string) ([]string, error) {
	var patterns []string

	manifest, found, err := readPackageJSON(dir)
	if err != nil {
		return nil, err
	}

	if found && len(manifest.Workspaces) > 0 {
		var npmPatterns []string
		if err := json.Unmarshal(manifest.Workspaces, &npmPatterns); err != nil {
			// Yarn classic also accepts {"packages": [...]}.
			var yarnWorkspaces struct {
				Packages []string `json:"packages"`
			}

			if err := json.Unmarshal(manifest.Workspaces, &yarnWorkspaces); err != nil {
				return nil, fmt.Errorf("decode workspaces in %s: %w", filepath.Join(dir, "package.json"), err)
			}

			npmPatterns = yarnWorkspaces.Packages
		}

		patterns = append(patterns, npmPatterns...)
	}

	pnpmWorkspacePath := filepath.Join(dir, "pnpm-workspace.yaml")

	pnpmWorkspaceBytes, err := os.ReadFile(pnpmWorkspacePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", pnpmWorkspacePath, err)
	}

	if err == nil {
		var pnpmWorkspace struct {
			Packages []string `json:"packages"`
		}

		if err := yaml.Unmarshal(pnpmWorkspaceBytes, &pnpmWorkspace); err != nil {
			return nil, fmt.Errorf("decode %s: %w", pnpmWorkspacePath, err)
		}

		patterns = append(patterns, pnpmWorkspace.Packages...)
	}

	return patterns, nil
}

// devEnginesPackageManagerName returns the name from devEngines.packageManager, which is an object or
// an array of objects, the first of which is the preferred one.
func devEnginesPackageManagerName(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	type engine struct {
		Name string `json:"name"`
	}

	var single engine
	if err := json.Unmarshal(raw, &single); err == nil {
		return single.Name, nil
	}

	var list []engine
	if err := json.Unmarshal(raw, &list); err != nil {
		return "", fmt.Errorf("decode as an object or a list of objects: %w", err)
	}

	if len(list) == 0 {
		return "", nil
	}

	return list[0].Name, nil
}

func hasLockFileOf(root, managerName string) (bool, error) {
	for _, lockFile := range lockFiles {
		if lockFile.packageManager != managerName {
			continue
		}

		if exists, err := fileExists(filepath.Join(root, lockFile.name)); err != nil || exists {
			return exists, err
		}
	}

	return false, nil
}

func installArgs(managerName string, yarnBerry, frozen bool) []string {
	switch {
	case managerName == packageManagerNPM && frozen:
		return []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}
	case managerName == packageManagerNPM:
		return []string{"install", "--ignore-scripts", "--no-audit", "--no-fund"}
	case yarnBerry && frozen:
		return []string{"install", "--immutable", "--mode=skip-build"}
	case yarnBerry:
		return []string{"install", "--mode=skip-build"}
	case frozen:
		return []string{"install", "--frozen-lockfile", "--ignore-scripts"}
	default:
		return []string{"install", "--ignore-scripts"}
	}
}

// isYarnBerry tells Yarn 2+ from Yarn classic, which take different install flags.
func isYarnBerry(root string, manager packageManager) (bool, error) {
	if manager.name != packageManagerYarn {
		return false, nil
	}

	if exists, err := fileExists(filepath.Join(root, ".yarnrc.yml")); err != nil || exists {
		return exists, err
	}

	if manager.version == "" {
		return false, nil
	}

	major, err := nodeMajorVersion(manager.version)
	if err != nil {
		return false, fmt.Errorf("parse yarn version from packageManager of package.json: %w", err)
	}

	return major >= 2, nil
}

func matchWorkspacePattern(pattern, relPath string) (bool, error) {
	compiled, err := glob.Compile(pattern, '/')
	if err != nil {
		return false, fmt.Errorf("compile workspace pattern %q: %w", pattern, err)
	}

	if compiled.Match(relPath) {
		return true, nil
	}

	// Unlike in gobwas/glob, "**/" also matches no directories at all.
	if withoutGlobstars := strings.ReplaceAll(pattern, "**/", ""); withoutGlobstars != pattern {
		return matchWorkspacePattern(withoutGlobstars, relPath)
	}

	return false, nil
}

func readPackageJSON(dir string) (*packageJSON, bool, error) {
	path := filepath.Join(dir, "package.json")

	manifestBytes, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &packageJSON{}, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}

	var manifest packageJSON
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", path, err)
	}

	return &manifest, true, nil
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

func validatePackageManager(manager packageManager, field string) (packageManager, error) {
	if !lo.Contains([]string{packageManagerBun, packageManagerNPM, packageManagerPNPM, packageManagerYarn}, manager.name) {
		return packageManager{}, fmt.Errorf("unsupported package manager %q in %s of package.json", manager.name, field)
	}

	return manager, nil
}
