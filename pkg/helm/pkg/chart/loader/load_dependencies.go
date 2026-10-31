package loader

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"sigs.k8s.io/yaml"

	"github.com/werf/common-go/pkg/locker"
	"github.com/werf/common-go/pkg/util"
	"github.com/werf/lockgate"
	nelmcommon "github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/helm/intern/sympath"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
	chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/v2/pkg/helm/pkg/ignore"
	"github.com/werf/nelm/v2/pkg/log"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

var utf8bom = []byte{0xEF, 0xBB, 0xBF}

var localCacheDir string
var serviceDir string

var NoChartLockWarning = `Cannot automatically download chart dependencies without Chart.lock or requirements.lock.`

func LocalCacheDir() (string, error) {
	if localCacheDir == "" {
		userHomeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home dir: %w", err)
		}

		localCacheDir = filepath.Join(userHomeDir, ".werf", "local_cache")
	}

	return localCacheDir, nil
}

func SetLocalCacheDir(dir string) {
	localCacheDir = dir
}

func ServiceDir() (string, error) {
	if serviceDir == "" {
		userHomeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home dir: %w", err)
		}

		serviceDir = filepath.Join(userHomeDir, ".werf", "service")
	}

	return serviceDir, nil
}

func SetServiceDir(dir string) {
	serviceDir = dir
}

func LoadChartDependencies(
	ctx context.Context,
	loadChartDirFunc func(ctx context.Context, dir string) ([]*nelmcommon.BufferedFile, error),
	chartDir string,
	loadedChartFiles []*nelmcommon.BufferedFile,
	opts nelmcommon.HelmOptions,
) ([]*nelmcommon.BufferedFile, error) {
	res := loadedChartFiles

	var chartMetadata *chart.Metadata
	var chartMetadataLock *chart.Lock

	for _, f := range loadedChartFiles {
		switch f.Name {
		case "Chart.yaml":
			chartMetadata = new(chart.Metadata)
			if err := yaml.Unmarshal(f.Data, chartMetadata); err != nil {
				return nil, fmt.Errorf("cannot load Chart.yaml: %w", err)
			}
			if chartMetadata.APIVersion == "" {
				chartMetadata.APIVersion = chart.APIVersionV1
			}

		case "Chart.lock":
			chartMetadataLock = new(chart.Lock)
			if err := yaml.Unmarshal(f.Data, chartMetadataLock); err != nil {
				return nil, fmt.Errorf("cannot load Chart.lock: %w", err)
			}
		}
	}

	for _, f := range loadedChartFiles {
		switch f.Name {
		case "requirements.yaml":
			if chartMetadata == nil {
				chartMetadata = new(chart.Metadata)
			}
			if err := yaml.Unmarshal(f.Data, chartMetadata); err != nil {
				return nil, fmt.Errorf("cannot load requirements.yaml: %w", err)
			}

		case "requirements.lock":
			if chartMetadataLock == nil {
				chartMetadataLock = new(chart.Lock)
			}
			if err := yaml.Unmarshal(f.Data, chartMetadataLock); err != nil {
				return nil, fmt.Errorf("cannot load requirements.lock: %w", err)
			}
		}
	}

	if chartMetadata == nil {
		return res, nil
	}

	if chartMetadataLock == nil {
		if err := checkVendoredDependenciesConstraints(loadedChartFiles, chartMetadata.Dependencies); err != nil {
			return nil, err
		}

		if len(chartMetadata.Dependencies) > 0 && NoChartLockWarning != "" {
			log.Default.Warn(ctx, "%s", NoChartLockWarning)
		}

		return res, nil
	}

	vendoredDeps, err := findVendoredDependencies(loadedChartFiles, chartMetadataLock.Dependencies)
	if err != nil {
		return nil, err
	}

	for _, chartDep := range chartMetadataLock.Dependencies {
		if vendoredDeps[newDependencyIdentity(chartDep)] && chartDep.Repository != "" {
			log.Default.Debug(ctx, "Use dependency %q %s vendored under charts/ instead of %s", chartDep.Name, chartDep.Version, chartDep.Repository)
		}
	}

	conf := newChartDependenciesConfiguration(chartMetadata, chartMetadataLock)

	for _, chartDep := range chartMetadataLock.Dependencies {
		if !strings.HasPrefix(chartDep.Repository, "file://") || vendoredDeps[newDependencyIdentity(chartDep)] {
			continue
		}

		relativeLocalChartPath := strings.TrimPrefix(chartDep.Repository, "file://")
		localChartPath := filepath.Join(chartDir, relativeLocalChartPath)

		chartFiles, err := loadChartDirFunc(ctx, localChartPath)
		if err != nil {
			return nil, fmt.Errorf("unable to load custom subchart dir %q: %w", localChartPath, err)
		}

		for _, f := range chartFiles {
			f.Name = filepath.Join("charts", chartDep.Name, f.Name)
		}

		res = append(res, chartFiles...)
	}

	haveExternalDependencies, metadataFile, metadataLockFile, err := conf.GetExternalDependenciesFiles(vendoredDeps)
	if err != nil {
		return nil, fmt.Errorf("unable to get external dependencies chart configuration files: %w", err)
	}
	if !haveExternalDependencies {
		return res, nil
	}

	depsDir, err := getPreparedChartDependenciesDir(ctx, metadataFile, metadataLockFile, opts)
	if err != nil {
		return nil, fmt.Errorf("error preparing chart dependencies: %w", err)
	}
	localFiles, err := getFilesFromLocalFilesystem(depsDir)
	if err != nil {
		return nil, err
	}

	for _, f := range localFiles {
		if strings.HasPrefix(f.Name, "charts/") {
			f1 := new(nelmcommon.BufferedFile)
			*f1 = nelmcommon.BufferedFile(*f)
			res = append(res, f1)
		}
	}

	return res, nil
}

func getChartDependenciesCacheDir() (string, error) {
	localCacheDir, err := LocalCacheDir()
	if err != nil {
		return "", fmt.Errorf("get local cache dir: %w", err)
	}

	return filepath.Join(localCacheDir, "helm_chart_dependencies", "1"), nil
}

func getChartDependenciesLocksDir() (string, error) {
	svcDir, err := ServiceDir()
	if err != nil {
		return "", fmt.Errorf("get service dir: %w", err)
	}

	return filepath.Join(svcDir, "locks"), nil
}

func prepareDependenciesDir(ctx context.Context, metadataBytes, metadataLockBytes []byte, prepareFunc func(tmpDepsDir string) error) (string, error) {
	chartDependenciesCacheDir, err := getChartDependenciesCacheDir()
	if err != nil {
		return "", fmt.Errorf("get chart dependencies cache dir: %w", err)
	}

	chartDependenciesLocksDir, err := getChartDependenciesLocksDir()
	if err != nil {
		return "", fmt.Errorf("get chart dependencies locks dir: %w", err)
	}

	hostLocker, err := locker.NewHostLocker(chartDependenciesLocksDir)
	if err != nil {
		return "", fmt.Errorf("unable to create host locker: %w", err)
	}

	depsDir := filepath.Join(chartDependenciesCacheDir, util.Sha256Hash(string(metadataLockBytes)))

	_, err = os.Stat(depsDir)
	switch {
	case os.IsNotExist(err):
		if err := func() error {
			_, lock, err := hostLocker.AcquireLock(ctx, depsDir, lockgate.AcquireOptions{})
			if err != nil {
				return fmt.Errorf("error acquiring lock for %q: %w", depsDir, err)
			}
			defer hostLocker.ReleaseLock(lock)

			switch _, err := os.Stat(depsDir); {
			case os.IsNotExist(err):
			case err != nil:
				return fmt.Errorf("error accessing %s: %w", depsDir, err)
			default:
				return nil
			}

			tmpDepsDir := fmt.Sprintf("%s.tmp.%s", depsDir, uuid.NewString())

			if err := createChartDependenciesDir(tmpDepsDir, metadataBytes, metadataLockBytes); err != nil {
				return err
			}

			if err := prepareFunc(tmpDepsDir); err != nil {
				return err
			}

			if err := os.Rename(tmpDepsDir, depsDir); err != nil {
				return fmt.Errorf("error renaming %q to %q: %w", tmpDepsDir, depsDir, err)
			}
			return nil
		}(); err != nil {
			return "", err
		}
	case err != nil:
		return "", fmt.Errorf("error accessing %q: %w", depsDir, err)
	}

	return depsDir, nil
}

func createChartDependenciesDir(destDir string, metadataBytes, metadataLockBytes []byte) error {
	if err := os.MkdirAll(destDir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating dir %q: %w", destDir, err)
	}

	files := []*nelmcommon.BufferedFile{
		{Name: "Chart.yaml", Data: metadataBytes},
		{Name: "Chart.lock", Data: metadataLockBytes},
	}

	for _, f := range files {
		if f == nil {
			continue
		}

		path := filepath.Join(destDir, f.Name)
		if err := os.WriteFile(path, f.Data, 0o644); err != nil {
			return fmt.Errorf("error writing %q: %w", path, err)
		}
	}

	return nil
}

func getPreparedChartDependenciesDir(ctx context.Context, metadataFile, metadataLockFile *nelmcommon.BufferedFile, opts nelmcommon.HelmOptions) (string, error) {
	return prepareDependenciesDir(ctx, metadataFile.Data, metadataLockFile.Data, func(tmpDepsDir string) error {
		if err := buildChartDependenciesInDir(ctx, tmpDepsDir, opts); err != nil {
			return fmt.Errorf("error building chart dependencies: %w", err)
		}
		return nil
	})
}

type chartDependenciesConfiguration struct {
	ChartMetadata     *chart.Metadata
	ChartMetadataLock *chart.Lock
}

func newChartDependenciesConfiguration(chartMetadata *chart.Metadata, chartMetadataLock *chart.Lock) *chartDependenciesConfiguration {
	return &chartDependenciesConfiguration{ChartMetadata: chartMetadata, ChartMetadataLock: chartMetadataLock}
}

func (conf *chartDependenciesConfiguration) GetExternalDependenciesFiles(vendoredDeps map[dependencyIdentity]bool) (bool, *nelmcommon.BufferedFile, *nelmcommon.BufferedFile, error) {
	metadataBytes, err := yaml.Marshal(conf.ChartMetadata)
	if err != nil {
		return false, nil, nil, fmt.Errorf("unable to marshal original chart metadata into yaml: %w", err)
	}
	metadata := new(chart.Metadata)
	if err := yaml.Unmarshal(metadataBytes, metadata); err != nil {
		return false, nil, nil, fmt.Errorf("unable to unmarshal original chart metadata yaml: %w", err)
	}

	metadataLockBytes, err := yaml.Marshal(conf.ChartMetadataLock)
	if err != nil {
		return false, nil, nil, fmt.Errorf("unable to marshal original chart metadata lock into yaml: %w", err)
	}
	metadataLock := new(chart.Lock)
	if err := yaml.Unmarshal(metadataLockBytes, metadataLock); err != nil {
		return false, nil, nil, fmt.Errorf("unable to unmarshal original chart metadata lock yaml: %w", err)
	}

	metadata.APIVersion = chart.APIVersionV2

	// Lock entries are matched to Chart.yaml dependencies by name and version, so that aliases
	// of one chart at different versions are told apart.
	usedDependencies := map[*chart.Dependency]bool{}
	var (
		filteredDependencies     []*chart.Dependency
		filteredLockDependencies []*chart.Dependency
	)

	for _, depLock := range metadataLock.Dependencies {
		dep := matchLockedDependency(metadata.Dependencies, depLock, usedDependencies)
		if dep != nil {
			usedDependencies[dep] = true
		}

		if depLock.Repository == "" || strings.HasPrefix(depLock.Repository, "file://") || vendoredDeps[newDependencyIdentity(depLock)] || dep == nil {
			continue
		}

		dep.Repository = depLock.Repository
		filteredDependencies = append(filteredDependencies, dep)
		filteredLockDependencies = append(filteredLockDependencies, depLock)
	}

	metadataLock.Dependencies = filteredLockDependencies
	metadata.Dependencies = lo.Filter(metadata.Dependencies, func(dep *chart.Dependency, _ int) bool {
		return lo.Contains(filteredDependencies, dep)
	})

	if len(metadata.Dependencies) == 0 {
		return false, nil, nil, nil
	}

	if newDigest, err := hashReq(metadata.Dependencies, metadataLock.Dependencies); err != nil {
		return false, nil, nil, fmt.Errorf("unable to calculate external dependencies Chart.yaml digest: %w", err)
	} else {
		metadataLock.Digest = newDigest
	}

	metadataFile := &nelmcommon.BufferedFile{Name: "Chart.yaml"}
	if data, err := yaml.Marshal(metadata); err != nil {
		return false, nil, nil, fmt.Errorf("unable to marshal chart metadata file with external dependencies: %w", err)
	} else {
		metadataFile.Data = data
	}

	metadataLockFile := &nelmcommon.BufferedFile{Name: "Chart.lock"}
	if data, err := yaml.Marshal(metadataLock); err != nil {
		return false, nil, nil, fmt.Errorf("unable to marshal chart metadata lock file with external dependencies: %w", err)
	} else {
		metadataLockFile.Data = data
	}

	return true, metadataFile, metadataLockFile, nil
}

func hashReq(req, lock []*chart.Dependency) (string, error) {
	data, err := json.Marshal([2][]*chart.Dependency{req, lock})
	if err != nil {
		return "", err
	}

	s, err := digest(bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}

	return "sha256:" + s, nil
}

func digest(in io.Reader) (string, error) {
	hash := crypto.SHA256.New()
	if _, err := io.Copy(hash, in); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

type dependencyIdentity struct {
	name    string
	version string
}

func newDependencyIdentity(dep *chart.Dependency) dependencyIdentity {
	return dependencyIdentity{name: dep.Name, version: dep.Version}
}

// matchLockedDependency returns the first unused Chart.yaml dependency with the name of the lock
// entry whose version constraint the locked version satisfies.
func matchLockedDependency(deps []*chart.Dependency, depLock *chart.Dependency, used map[*chart.Dependency]bool) *chart.Dependency {
	lockVersion, lockVersionErr := semver.NewVersion(depLock.Version)

	for _, dep := range deps {
		if used[dep] || dep.Name != depLock.Name {
			continue
		}

		if lockVersionErr != nil || dep.Version == "" {
			return dep
		}

		constraint, err := semver.NewConstraint(dep.Version)
		if err != nil || constraint.Check(lockVersion) {
			return dep
		}
	}

	return nil
}

// findVendoredDependencies returns the locked dependencies the chart files already have under
// charts/, either as an archive or as the directory a saved chart has. A vendored copy of a
// locked chart at a version no lock entry has, or two copies of one locked version, is an error,
// since it would be used instead of or next to the locked version.
func findVendoredDependencies(loadedChartFiles []*nelmcommon.BufferedFile, deps []*chart.Dependency) (map[dependencyIdentity]bool, error) {
	candidates, err := findVendoredCharts(loadedChartFiles)
	if err != nil {
		return nil, err
	}

	vendoredPaths := map[dependencyIdentity]string{}
	for _, candidate := range candidates {
		locks := lo.Filter(deps, func(dep *chart.Dependency, _ int) bool { return dep.Name == candidate.name })
		if len(locks) == 0 {
			continue
		}

		depLock, found := lo.Find(locks, func(dep *chart.Dependency) bool { return sameVersion(candidate.version, dep.Version) })
		if !found {
			lockedVersions := lo.Map(locks, func(dep *chart.Dependency, _ int) string { return dep.Version })
			hint := "remove it"
			if lo.ContainsBy(locks, func(dep *chart.Dependency) bool { return dep.Repository == "" }) {
				hint = "replace it with the locked version, for example with \"helm dependency update\""
			}

			return nil, fmt.Errorf("%s is version %s but Chart.lock requires %s: %s", candidate.path, candidate.version, strings.Join(lockedVersions, " or "), hint)
		}

		identity := newDependencyIdentity(depLock)
		if previousPath, ok := vendoredPaths[identity]; ok {
			return nil, fmt.Errorf("dependency %q %s is vendored more than once (%s, %s): keep one of them", identity.name, identity.version, previousPath, candidate.path)
		}

		vendoredPaths[identity] = candidate.path
	}

	return lo.MapValues(vendoredPaths, func(string, dependencyIdentity) bool { return true }), nil
}

// checkVendoredDependenciesConstraints fails if a chart vendored under charts/ of a chart without
// Chart.lock doesn't match the version constraint of its dependency in Chart.yaml.
func checkVendoredDependenciesConstraints(loadedChartFiles []*nelmcommon.BufferedFile, deps []*chart.Dependency) error {
	candidates, err := findVendoredCharts(loadedChartFiles)
	if err != nil {
		return err
	}

	for _, dep := range deps {
		if dep.Version == "" {
			continue
		}

		constraint, err := semver.NewConstraint(dep.Version)
		if err != nil {
			continue
		}

		for _, candidate := range candidates {
			if candidate.name != dep.Name {
				continue
			}

			if parsedVersion, err := semver.NewVersion(candidate.version); err == nil && constraint.Check(parsedVersion) {
				continue
			}

			return fmt.Errorf("%s is version %s but Chart.yaml requires %s: remove it or vendor a matching version", candidate.path, candidate.version, dep.Version)
		}
	}

	return nil
}

type vendoredChart struct {
	path    string
	name    string
	version string
}

// findVendoredCharts finds the charts directly under charts/, as archives or directories, with
// their names and versions from their Chart.yaml.
func findVendoredCharts(loadedChartFiles []*nelmcommon.BufferedFile) ([]vendoredChart, error) {
	var charts []vendoredChart
	for _, f := range loadedChartFiles {
		name, ok := strings.CutPrefix(f.Name, "charts/")
		if !ok {
			continue
		}

		dir, fileName, isInDir := strings.Cut(name, "/")

		var chartYAML []byte
		switch {
		case !isInDir && strings.HasSuffix(name, ".tgz"):
			archiveFiles, err := archive.LoadArchiveFiles(bytes.NewReader(f.Data))
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", f.Name, err)
			}

			chartFile, found := lo.Find(archiveFiles, func(archiveFile *archive.BufferedFile) bool { return archiveFile.Name == "Chart.yaml" })
			if !found {
				return nil, fmt.Errorf("read %s: Chart.yaml not found", f.Name)
			}

			chartYAML = chartFile.Data
		case isInDir && fileName == "Chart.yaml":
			chartYAML = f.Data
			name = dir
		default:
			continue
		}

		metadata := new(chart.Metadata)
		if err := yaml.Unmarshal(chartYAML, metadata); err != nil || metadata.Name == "" {
			continue
		}

		charts = append(charts, vendoredChart{path: "charts/" + name, name: metadata.Name, version: metadata.Version})
	}

	return charts, nil
}

// sameVersion compares versions exactly, build metadata included.
func sameVersion(a, b string) bool {
	parsedA, errA := semver.NewVersion(a)
	parsedB, errB := semver.NewVersion(b)
	if errA != nil || errB != nil {
		return a == b
	}

	return parsedA.String() == parsedB.String()
}

func buildChartDependenciesInDir(ctx context.Context, targetDir string, opts nelmcommon.HelmOptions) error {
	if opts.ChartLoadOpts.ChartDepsDownloader == nil {
		return fmt.Errorf("dependency downloader is required")
	}

	opts.ChartLoadOpts.ChartType = nelmcommon.LegacyChartTypeChartStub
	opts.ChartLoadOpts.ChartDepsDownloader.SetChartPath(targetDir)
	ctx = nelmcommon.ContextWithHelmOptions(ctx, opts)

	if err := opts.ChartLoadOpts.ChartDepsDownloader.Build(ctx); err != nil {
		return fmt.Errorf("build dependencies: %w", err)
	}

	return nil
}

func getFilesFromLocalFilesystem(dir string) ([]*nelmcommon.BufferedFile, error) {
	topdir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	rules := ignore.Empty()
	ifile := filepath.Join(topdir, ignore.HelmIgnore)
	if _, err := os.Stat(ifile); err == nil {
		r, err := ignore.ParseFile(ifile)
		if err != nil {
			return nil, err
		}
		rules = r
	}
	rules.AddDefaults()

	var files []*nelmcommon.BufferedFile
	topdir += string(filepath.Separator)

	walk := func(name string, fi os.FileInfo, err error) error {
		n := strings.TrimPrefix(name, topdir)
		if n == "" {
			return nil
		}

		n = filepath.ToSlash(n)

		if err != nil {
			return err
		}
		if fi.IsDir() {
			if !werfdata.IsWerfDataFileInTree(n) && rules.Ignore(n, fi) {
				return filepath.SkipDir
			}
			return nil
		}

		if !werfdata.IsWerfDataFileInTree(n) && rules.Ignore(n, fi) {
			return nil
		}

		if !fi.Mode().IsRegular() {
			return fmt.Errorf("cannot load irregular file %s as it has file mode type bits set", name)
		}

		if fi.Size() > archive.MaxDecompressedFileSize {
			return fmt.Errorf("chart file %q is larger than the maximum file size %d", fi.Name(), archive.MaxDecompressedFileSize)
		}

		data, err := os.ReadFile(name)
		if err != nil {
			return fmt.Errorf("error reading %s: %w", n, err)
		}

		data = bytes.TrimPrefix(data, utf8bom)

		files = append(files, &nelmcommon.BufferedFile{Name: n, Data: data})
		return nil
	}
	if err = sympath.Walk(topdir, walk); err != nil {
		return nil, err
	}

	return files, nil
}
