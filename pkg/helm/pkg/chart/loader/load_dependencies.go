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

	"github.com/google/uuid"
	"sigs.k8s.io/yaml"

	"github.com/werf/common-go/pkg/locker"
	"github.com/werf/common-go/pkg/util"
	"github.com/werf/lockgate"
	nelmcommon "github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/helm/intern/sympath"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader/archive"
	chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/pkg/helm/pkg/ignore"
	"github.com/werf/nelm/pkg/log"
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
		if len(chartMetadata.Dependencies) > 0 && NoChartLockWarning != "" {
			log.Default.Warn(ctx, "%s", NoChartLockWarning)
		}

		return res, nil
	}

	conf := newChartDependenciesConfiguration(chartMetadata, chartMetadataLock)

	for _, chartDep := range chartMetadataLock.Dependencies {
		if !strings.HasPrefix(chartDep.Repository, "file://") {
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

	haveExternalDependencies, metadataFile, metadataLockFile, err := conf.GetExternalDependenciesFiles(loadedChartFiles)
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

func (conf *chartDependenciesConfiguration) GetExternalDependenciesFiles(loadedChartFiles []*nelmcommon.BufferedFile) (bool, *nelmcommon.BufferedFile, *nelmcommon.BufferedFile, error) {
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

	var externalDependenciesNames []string
	isExternalDependency := func(depName string) bool {
		for _, externalDepName := range externalDependenciesNames {
			if depName == externalDepName {
				return true
			}
		}

		return false
	}

FindExternalDependencies:
	for _, depLock := range metadataLock.Dependencies {
		if depLock.Repository == "" || strings.HasPrefix(depLock.Repository, "file://") {
			continue
		}

		for _, loadedFile := range loadedChartFiles {
			if strings.HasPrefix(loadedFile.Name, "charts/") {
				if filepath.Base(loadedFile.Name) == makeDependencyArchiveName(depLock.Name, depLock.Version) {
					continue FindExternalDependencies
				}
			}
		}

		externalDependenciesNames = append(externalDependenciesNames, depLock.Name)
	}

	var filteredLockDependencies []*chart.Dependency
	for _, depLock := range metadataLock.Dependencies {
		if isExternalDependency(depLock.Name) {
			filteredLockDependencies = append(filteredLockDependencies, depLock)
		}
	}
	metadataLock.Dependencies = filteredLockDependencies

	var filteredDependencies []*chart.Dependency
	for _, dep := range metadata.Dependencies {
		if isExternalDependency(dep.Name) {
			filteredDependencies = append(filteredDependencies, dep)
		}
	}
	metadata.Dependencies = filteredDependencies

	if len(metadata.Dependencies) == 0 {
		return false, nil, nil, nil
	}

	for _, dep := range metadata.Dependencies {
		for _, depLock := range metadataLock.Dependencies {
			if dep.Name == depLock.Name {
				dep.Repository = depLock.Repository
				break
			}
		}
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

func makeDependencyArchiveName(depName, depVersion string) string {
	return fmt.Sprintf("%s-%s.tgz", depName, depVersion)
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
			if rules.Ignore(n, fi) {
				return filepath.SkipDir
			}
			return nil
		}

		if rules.Ignore(n, fi) {
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
