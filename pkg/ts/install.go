package ts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

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
	pnpmWorkspaceFile  = "pnpm-workspace.yaml"
)

var (
	supportedPackageManagers = []string{packageManagerBun, packageManagerNPM, packageManagerPNPM, packageManagerYarn}
	lockFiles                = []struct {
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
)

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

	hasPackageJSON, err := fileExists(filepath.Join(absTSDir, "package.json"))
	if err != nil {
		return fmt.Errorf("check package.json: %w", err)
	}

	if !hasPackageJSON {
		return checkNotDenoChart(ctx, absTSDir)
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
			return fmt.Errorf("check installed dependencies: %w", err)
		} else if exists {
			log.Default.Debug(ctx, "Dependencies in %s are already installed: %s exists", root, marker)

			return nil
		}
	}

	isPNPMWorkspace, err := fileExists(filepath.Join(root, pnpmWorkspaceFile))
	if err != nil {
		return fmt.Errorf("check %s: %w", pnpmWorkspaceFile, err)
	}

	manager, err := detectPackageManager(root, lo.Ternary(isPNPMWorkspace, packageManagerPNPM, packageManagerNPM))
	if err != nil {
		return fmt.Errorf("detect package manager of %s: %w", root, err)
	}

	installCommand := manager.name + " install"

	if root != tsDir {
		return fmt.Errorf("dependencies of %s are not installed: run %q in workspace root %s", tsDir, installCommand, root)
	}

	// In chart ts init the chart files are already created, so the user only has to finish the install.
	nextStep := func(err error) error {
		if err == nil || !createLockFile {
			return err
		}

		return fmt.Errorf("%w\nThe chart files are created: once the problem is fixed, run %q in %s to install the dependencies", err, installCommand, root)
	}

	managerPath, err := exec.LookPath(manager.name)
	if err != nil {
		return nextStep(fmt.Errorf("install dependencies of %s: package manager %s is not found in PATH: %w", root, manager.name, err))
	}

	hasLockFile, err := hasLockFileOf(root, manager.name)
	if err != nil {
		return fmt.Errorf("check lock file: %w", err)
	}

	if !hasLockFile && !createLockFile {
		return fmt.Errorf("no lock file of %s in %s: run %q there and commit the lock file", manager.name, root, installCommand)
	}

	yarnBerry, err := isYarnBerry(root, manager)
	if err != nil {
		return fmt.Errorf("detect yarn version: %w", err)
	}

	args := installArgs(manager.name, yarnBerry, hasLockFile)

	// Otherwise pnpm would install a pnpm-workspace.yaml above, that does not include the chart,
	// instead of the chart.
	if manager.name == packageManagerPNPM && !isPNPMWorkspace {
		args = append(args, "--ignore-workspace")
	}

	return nextStep(runPackageManager(ctx, root, managerPath, manager.name, args))
}

// findWorkspaceRoot returns the workspace root including tsDir as the package managers find it, or
// tsDir itself if there is none. npm, yarn and bun take the closest ancestor whose package.json
// lists tsDir in workspaces, and pnpm the closest pnpm-workspace.yaml, if it lists tsDir. Both look
// up to the root of the filesystem, and neither reads the workspaces of the other.
func findWorkspaceRoot(tsDir string) (string, error) {
	pnpmWorkspaceFound := false

	for dir := tsDir; ; dir = filepath.Dir(dir) {
		relPath, err := filepath.Rel(dir, tsDir)
		if err != nil {
			return "", fmt.Errorf("get path of %s relative to %s: %w", tsDir, dir, err)
		}

		relPath = filepath.ToSlash(relPath)

		if !pnpmWorkspaceFound {
			patterns, found, err := readPNPMWorkspacePatterns(dir)
			if err != nil {
				return "", err
			}

			pnpmWorkspaceFound = found

			if found && dir != tsDir {
				root, err := workspaceRootIfIncluded(dir, relPath, patterns, packageManagerPNPM)
				if err != nil || root != "" {
					return root, err
				}
			}
		}

		if dir != tsDir {
			patterns, err := readPackageJSONWorkspacePatterns(dir)
			if err != nil {
				return "", err
			}

			if len(patterns) > 0 {
				root, err := workspaceRootIfIncluded(dir, relPath, patterns, packageManagerNPM)
				if err != nil || root != "" {
					return root, err
				}
			}
		}

		if filepath.Dir(dir) == dir {
			return tsDir, nil
		}
	}
}

// workspaceRootIfIncluded returns dir if the patterns include relPath and the package manager of dir
// reads them: pnpm only reads pnpm-workspace.yaml, the others only package.json.
func workspaceRootIfIncluded(dir, relPath string, patterns []string, defaultManager string) (string, error) {
	included, err := matchesWorkspacePatterns(patterns, relPath)
	if err != nil {
		return "", fmt.Errorf("match workspace patterns of %s: %w", dir, err)
	}

	if !included {
		return "", nil
	}

	manager, err := detectPackageManager(dir, defaultManager)
	if err != nil {
		return "", fmt.Errorf("detect package manager of %s: %w", dir, err)
	}

	if (manager.name == packageManagerPNPM) != (defaultManager == packageManagerPNPM) {
		return "", nil
	}

	return dir, nil
}

// detectPackageManager takes the package manager from the packageManager or devEngines.packageManager
// field of package.json in root, or else from the lock file in root, or else defaultManager.
func detectPackageManager(root, defaultManager string) (packageManager, error) {
	manifest, _, err := readPackageJSON(root)
	if err != nil {
		return packageManager{}, err
	}

	if manifest.PackageManager != "" {
		name, version, _ := strings.Cut(manifest.PackageManager, "@")
		version, _, _ = strings.Cut(version, "+")

		if !lo.Contains(supportedPackageManagers, name) {
			return packageManager{}, fmt.Errorf("unsupported package manager %q in packageManager of package.json", name)
		}

		return packageManager{name: name, version: version}, nil
	}

	alternatives, err := devEnginesPackageManagers(manifest.DevEngines.PackageManager)
	if err != nil {
		return packageManager{}, fmt.Errorf("decode devEngines.packageManager: %w", err)
	}

	if len(alternatives) > 0 {
		return chooseDevEnginesPackageManager(root, alternatives)
	}

	var foundLockFiles, foundManagers []string

	for _, lockFile := range lockFiles {
		exists, err := fileExists(filepath.Join(root, lockFile.name))
		if err != nil {
			return packageManager{}, fmt.Errorf("check lock file: %w", err)
		}

		if exists {
			foundLockFiles = append(foundLockFiles, lockFile.name)
			foundManagers = append(foundManagers, lockFile.packageManager)
		}
	}

	switch foundManagers = lo.Uniq(foundManagers); len(foundManagers) {
	case 0:
		return packageManager{name: defaultManager}, nil
	case 1:
		return packageManager{name: foundManagers[0]}, nil
	default:
		return packageManager{}, fmt.Errorf("lock files of several package managers: %s, set the packageManager field in package.json to choose one", strings.Join(foundLockFiles, ", "))
	}
}

// chooseDevEnginesPackageManager takes the first supported alternative that has a lock file in root,
// or else the first one in PATH, or else the first one.
func chooseDevEnginesPackageManager(root string, alternatives []packageManager) (packageManager, error) {
	supported := lo.Filter(alternatives, func(manager packageManager, _ int) bool {
		return lo.Contains(supportedPackageManagers, manager.name)
	})

	if len(supported) == 0 {
		return packageManager{}, fmt.Errorf("unsupported package manager %q in devEngines.packageManager of package.json", alternatives[0].name)
	}

	for _, manager := range supported {
		hasLockFile, err := hasLockFileOf(root, manager.name)
		if err != nil {
			return packageManager{}, fmt.Errorf("check lock file: %w", err)
		}

		if hasLockFile {
			return manager, nil
		}
	}

	for _, manager := range supported {
		if _, err := exec.LookPath(manager.name); err == nil {
			return manager, nil
		}
	}

	return supported[0], nil
}

// isYarnBerry tells Yarn 2+ from Yarn classic, which take different install flags, by .yarnrc.yml,
// by the format of yarn.lock, or by the version in package.json.
func isYarnBerry(root string, manager packageManager) (bool, error) {
	if manager.name != packageManagerYarn {
		return false, nil
	}

	if exists, err := fileExists(filepath.Join(root, ".yarnrc.yml")); err != nil {
		return false, fmt.Errorf("check .yarnrc.yml: %w", err)
	} else if exists {
		return true, nil
	}

	lockPath := filepath.Join(root, "yarn.lock")

	lock, err := os.ReadFile(lockPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read %s: %w", lockPath, err)
	}

	if bytes.Contains(lock, []byte("\n__metadata:")) || bytes.HasPrefix(lock, []byte("__metadata:")) {
		return true, nil
	}

	major, found := versionRangeMajor(manager.version)

	return found && major >= 2, nil
}

// matchesWorkspacePatterns reports whether relPath, slash-separated, is matched by a workspace glob
// pattern and not excluded by a "!" pattern, wherever it is in the list.
func matchesWorkspacePatterns(patterns []string, relPath string) (bool, error) {
	var included, excluded bool

	for _, pattern := range patterns {
		negated := strings.HasPrefix(pattern, "!")
		pattern = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(pattern, "!"), "./"), "/")

		for _, expanded := range expandBraces(pattern) {
			matched, err := matchWorkspaceGlob(strings.Split(expanded, "/"), strings.Split(relPath, "/"))
			if err != nil {
				return false, fmt.Errorf("match workspace pattern %q: %w", pattern, err)
			}

			if matched && negated {
				excluded = true
			} else if matched {
				included = true
			}
		}
	}

	return included && !excluded, nil
}

func readPackageJSONWorkspacePatterns(dir string) ([]string, error) {
	manifest, found, err := readPackageJSON(dir)
	if err != nil || !found || len(manifest.Workspaces) == 0 {
		return nil, err
	}

	var patterns []string
	if err := json.Unmarshal(manifest.Workspaces, &patterns); err == nil {
		return patterns, nil
	}

	// Yarn classic also accepts {"packages": [...]}.
	var yarnWorkspaces struct {
		Packages []string `json:"packages"`
	}

	if err := json.Unmarshal(manifest.Workspaces, &yarnWorkspaces); err != nil {
		return nil, fmt.Errorf("decode workspaces in %s: %w", filepath.Join(dir, "package.json"), err)
	}

	return yarnWorkspaces.Packages, nil
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

// devEnginesPackageManagers returns the entries of devEngines.packageManager, which is an object or
// an array of alternatives.
func devEnginesPackageManagers(raw json.RawMessage) ([]packageManager, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	type engine struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	var list []engine
	if err := json.Unmarshal(raw, &list); err != nil {
		var single engine
		if err := json.Unmarshal(raw, &single); err != nil {
			return nil, fmt.Errorf("decode as an object or a list of objects: %w", err)
		}

		list = []engine{single}
	}

	return lo.Map(list, func(e engine, _ int) packageManager {
		return packageManager{name: e.Name, version: e.Version}
	}), nil
}

// expandBraces expands "{a,b}" alternatives, nested ones included, into separate patterns.
func expandBraces(pattern string) []string {
	start := strings.IndexByte(pattern, '{')
	if start < 0 {
		return []string{pattern}
	}

	depth := 0
	alternativeStart := start + 1

	var alternatives []string

	for i := start; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			depth++
		case ',':
			if depth == 1 {
				alternatives = append(alternatives, pattern[alternativeStart:i])
				alternativeStart = i + 1
			}
		case '}':
			depth--
			if depth > 0 {
				continue
			}

			alternatives = append(alternatives, pattern[alternativeStart:i])

			var expanded []string
			for _, alternative := range alternatives {
				expanded = append(expanded, expandBraces(pattern[:start]+alternative+pattern[i+1:])...)
			}

			return expanded
		}
	}

	return []string{pattern}
}

func hasLockFileOf(root, managerName string) (bool, error) {
	for _, lockFile := range lockFiles {
		if lockFile.packageManager != managerName {
			continue
		}

		exists, err := fileExists(filepath.Join(root, lockFile.name))
		if err != nil {
			return false, fmt.Errorf("check %s: %w", lockFile.name, err)
		}

		if exists {
			return true, nil
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
		// Yarn Berry turns --immutable on by itself in CI, where it would refuse to create the lock file.
		return []string{"install", "--no-immutable", "--mode=skip-build"}
	case frozen:
		return []string{"install", "--frozen-lockfile", "--ignore-scripts"}
	default:
		return []string{"install", "--ignore-scripts"}
	}
}

// matchWorkspaceGlob matches path segments against pattern segments. As in the glob libraries of the
// package managers, wildcards and "**" do not match names starting with a dot.
func matchWorkspaceGlob(patternSegments, pathSegments []string) (bool, error) {
	if len(patternSegments) == 0 {
		return len(pathSegments) == 0, nil
	}

	if patternSegments[0] == "**" {
		if matched, err := matchWorkspaceGlob(patternSegments[1:], pathSegments); err != nil || matched {
			return matched, err
		}

		if len(pathSegments) == 0 || strings.HasPrefix(pathSegments[0], ".") {
			return false, nil
		}

		return matchWorkspaceGlob(patternSegments, pathSegments[1:])
	}

	if len(pathSegments) == 0 {
		return false, nil
	}

	if strings.HasPrefix(pathSegments[0], ".") && !strings.HasPrefix(patternSegments[0], ".") {
		return false, nil
	}

	matched, err := path.Match(patternSegments[0], pathSegments[0])
	if err != nil {
		return false, fmt.Errorf("match %q: %w", patternSegments[0], err)
	}

	if !matched {
		return false, nil
	}

	return matchWorkspaceGlob(patternSegments[1:], pathSegments[1:])
}

func readPNPMWorkspacePatterns(dir string) ([]string, bool, error) {
	workspacePath := filepath.Join(dir, pnpmWorkspaceFile)

	workspaceBytes, err := os.ReadFile(workspacePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", workspacePath, err)
	}

	var workspace struct {
		Packages []string `json:"packages"`
	}

	if err := yaml.Unmarshal(workspaceBytes, &workspace); err != nil {
		return nil, false, fmt.Errorf("decode %s: %w", workspacePath, err)
	}

	return workspace.Packages, true, nil
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

// versionRangeMajor returns the first number of a version or a version range such as "^4.0.0".
func versionRangeMajor(version string) (int, bool) {
	start := strings.IndexFunc(version, func(r rune) bool { return r >= '0' && r <= '9' })
	if start < 0 {
		return 0, false
	}

	digits := version[start:]
	if end := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); end >= 0 {
		digits = digits[:end]
	}

	major, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}

	return major, true
}
