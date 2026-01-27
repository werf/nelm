package ts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

// BuildVendorBundleToDir scans the chart's TypeScript source for npm dependencies
// and creates a vendor bundle file at ts/vendor/libs.js.
func BuildVendorBundleToDir(ctx context.Context, chartPath string) error {
	absChartPath, err := filepath.Abs(chartPath)
	if err != nil {
		return fmt.Errorf("get absolute path: %w", err)
	}

	stat, err := os.Stat(absChartPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Default.Debug(ctx, "Skipping vendor bundle: chart path %s does not exist", absChartPath)

			return nil
		}

		return fmt.Errorf("stat %s: %w", absChartPath, err)
	}

	if !stat.IsDir() {
		return fmt.Errorf("build vendor bundle to dir: %s is not a directory", absChartPath)
	}

	tsDir := filepath.Join(absChartPath, common.ChartTSSourceDir)
	if _, err := os.Stat(tsDir); err != nil {
		if os.IsNotExist(err) {
			log.Default.Debug(ctx, "Skipping vendor bundle: no %s directory", common.ChartTSSourceDir)

			return nil
		}

		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	entrypoint, err := findEntrypointInDir(tsDir)
	if err != nil {
		return fmt.Errorf("find entrypoint: %w", err)
	}

	if entrypoint == "" {
		log.Default.Debug(ctx, "Skipping vendor bundle: no entrypoint found")

		return nil
	}

	nodeModulesPath := filepath.Join(tsDir, "node_modules")
	if _, err := os.Stat(nodeModulesPath); err != nil {
		if os.IsNotExist(err) {
			log.Default.Debug(ctx, "Skipping vendor bundle: no node_modules directory")

			return nil
		}

		return fmt.Errorf("stat %s: %w", nodeModulesPath, err)
	}

	log.Default.Info(ctx, "Building vendor bundle for TypeScript chart: %s", absChartPath)

	vendorBundle, packages, err := buildVendorBundleFromDir(ctx, tsDir, entrypoint)
	if err != nil {
		return fmt.Errorf("build vendor bundle: %w", err)
	}

	if len(packages) == 0 {
		log.Default.Debug(ctx, "Skipping vendor bundle: no npm packages used")

		return nil
	}

	log.Default.Info(ctx, "Bundled %d npm packages: %s", len(packages), strings.Join(packages, ", "))

	vendorPath := filepath.Join(absChartPath, common.ChartTSVendorBundleFile)
	if err := os.MkdirAll(filepath.Dir(vendorPath), 0o755); err != nil {
		return fmt.Errorf("create vendor directory: %w", err)
	}

	if err := os.WriteFile(vendorPath, []byte(vendorBundle), 0o644); err != nil {
		return fmt.Errorf("write vendor bundle to %s: %w", vendorPath, err)
	}

	log.Default.Info(ctx, "Wrote vendor bundle to %s", common.ChartTSVendorBundleFile)

	return nil
}

func resolveVendorBundle(ctx context.Context, files []*helmchart.File) (string, []string, error) {
	// Check if node_modules exists in files
	hasNodeModules := false
	for _, f := range files {
		if strings.HasPrefix(f.Name, common.ChartTSSourceDir+"node_modules/") {
			hasNodeModules = true
			break
		}
	}

	if hasNodeModules {
		filesMap := make(map[string][]byte)
		for _, f := range files {
			if !strings.HasPrefix(f.Name, common.ChartTSSourceDir) {
				continue
			}

			filesMap[strings.TrimPrefix(f.Name, common.ChartTSSourceDir)] = f.Data
		}

		entrypoint := findEntrypointInFiles(filesMap)
		if entrypoint == "" {
			return "", nil, nil
		}

		return buildVendorBundleFromFiles(ctx, filesMap, entrypoint)
	}

	// Look for pre-built vendor bundle
	for _, f := range files {
		if f.Name == common.ChartTSVendorBundleFile {
			return string(f.Data), extractPackagesFromVendorBundle(string(f.Data)), nil
		}
	}

	return "", nil, nil
}

func buildVendorBundleFromDir(ctx context.Context, tsDir, entrypoint string) (string, []string, error) {
	packages, err := scanPackagesFromDir(ctx, tsDir, entrypoint)
	if err != nil {
		return "", nil, err
	}

	if len(packages) == 0 {
		return "", nil, nil
	}

	vendorOpts := newVendorEsbuildOptions(packages, tsDir)
	vendorOpts.AbsWorkingDir = tsDir

	bundle, err := runEsbuildBundle(vendorOpts)
	if err != nil {
		return "", nil, err
	}

	return bundle, packages, nil
}

func buildVendorBundleFromFiles(ctx context.Context, files map[string][]byte, entrypoint string) (string, []string, error) {
	packages, err := scanPackagesFromFiles(ctx, entrypoint, newVirtualFSPlugin(files, true))
	if err != nil {
		return "", nil, err
	}

	if len(packages) == 0 {
		return "", nil, nil
	}

	vendorOpts := newVendorEsbuildOptions(packages, ".")
	vendorOpts.Plugins = []esbuild.Plugin{newVirtualFSPlugin(files, true)}

	bundle, err := runEsbuildBundle(vendorOpts)
	if err != nil {
		return "", nil, err
	}

	return bundle, packages, nil
}

func buildAppBundleFromFiles(ctx context.Context, files map[string][]byte, externalPackages []string) (string, error) {
	entrypoint := findEntrypointInFiles(files)
	if entrypoint == "" {
		return "", fmt.Errorf("build app bundle: no entrypoint found")
	}

	log.Default.Debug(ctx, "Building app bundle from chart files with entrypoint %s", entrypoint)

	opts := newEsbuildOptions()
	opts.EntryPoints = []string{entrypoint}
	opts.External = externalPackages
	opts.Sourcemap = esbuild.SourceMapInline
	opts.Plugins = []esbuild.Plugin{newVirtualFSPlugin(files, false)}

	return runEsbuildBundle(opts)
}

func scanPackagesFromDir(ctx context.Context, workDir, entrypoint string) ([]string, error) {
	scanOpts := newEsbuildOptions()
	scanOpts.EntryPoints = []string{entrypoint}
	scanOpts.Metafile = true
	scanOpts.AbsWorkingDir = workDir

	scanResult := esbuild.Build(scanOpts)
	if len(scanResult.Errors) > 0 {
		return nil, formatEsbuildErrors(scanResult.Errors)
	}

	return extractPackageNames(scanResult.Metafile)
}

func scanPackagesFromFiles(ctx context.Context, entrypoint string, plugin esbuild.Plugin) ([]string, error) {
	scanOpts := newEsbuildOptions()
	scanOpts.EntryPoints = []string{entrypoint}
	scanOpts.Metafile = true
	scanOpts.Plugins = []esbuild.Plugin{plugin}

	scanResult := esbuild.Build(scanOpts)
	if len(scanResult.Errors) > 0 {
		return nil, formatEsbuildErrors(scanResult.Errors)
	}

	return extractPackageNames(scanResult.Metafile)
}
