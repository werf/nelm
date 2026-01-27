package tschart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"

	"github.com/werf/nelm/pkg/log"
)

func (t *Transformer) TransformChartDir(ctx context.Context, chartPath string) error {
	absChartPath, err := filepath.Abs(chartPath)
	if err != nil {
		return fmt.Errorf("get absolute path: %w", err)
	}

	stat, err := os.Stat(absChartPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Default.Debug(ctx, "Skipping TypeScript transformation: %s does not exist", absChartPath)
			return nil
		}

		return fmt.Errorf("stat %s: %w", absChartPath, err)
	}

	if !stat.IsDir() {
		return fmt.Errorf("%s is not a directory", absChartPath)
	}

	tsDir := filepath.Join(absChartPath, TSSourceDir)
	if _, err := os.Stat(tsDir); err != nil {
		if os.IsNotExist(err) {
			log.Default.Debug(ctx, "No %s directory found, skipping transformation", TSSourceDir)
			return nil
		}

		return fmt.Errorf("stat %s: %w", tsDir, err)
	}

	entrypointFile, err := findEntrypointInDir(tsDir)
	if err != nil {
		return fmt.Errorf("find entrypoint: %w", err)
	}

	if entrypointFile == "" {
		log.Default.Debug(ctx, "No TypeScript entrypoint found, skipping transformation")
		return nil
	}

	nodeModulesPath := filepath.Join(tsDir, "node_modules")
	if _, err := os.Stat(nodeModulesPath); err != nil {
		if os.IsNotExist(err) {
			log.Default.Debug(ctx, "No node_modules directory found, skipping vendor bundle")
			return nil
		}

		return fmt.Errorf("stat %s: %w", nodeModulesPath, err)
	}

	log.Default.Info(ctx, "Building vendor bundle for TypeScript chart: %s", absChartPath)

	vendorBundle, packages, err := buildVendorBundleInDir(tsDir, entrypointFile)
	if err != nil {
		return fmt.Errorf("build vendor bundle: %w", err)
	}

	if len(packages) == 0 {
		log.Default.Debug(ctx, "No npm packages used, skipping vendor bundle")
		return nil
	}

	log.Default.Info(ctx, "Bundled %d npm packages: %s", len(packages), strings.Join(packages, ", "))

	vendorPath := filepath.Join(absChartPath, VendorBundleFile)
	if err := os.MkdirAll(filepath.Dir(vendorPath), 0o755); err != nil {
		return fmt.Errorf("create vendor directory: %w", err)
	}

	if err := os.WriteFile(vendorPath, []byte(vendorBundle), 0o644); err != nil { //nolint:gosec // Chart files should be world-readable
		return fmt.Errorf("write vendor bundle to %s: %w", vendorPath, err)
	}

	log.Default.Info(ctx, "Wrote vendor bundle to %s", VendorBundleFile)

	return nil
}

func GetVendorBundleFromDir(ctx context.Context, chartPath string) (string, []string, error) {
	absChartPath, err := filepath.Abs(chartPath)
	if err != nil {
		return "", nil, fmt.Errorf("get absolute path: %w", err)
	}

	tsDir := filepath.Join(absChartPath, TSSourceDir)
	nodeModulesPath := filepath.Join(tsDir, "node_modules")
	vendorPath := filepath.Join(absChartPath, VendorBundleFile)

	entrypointFile, err := findEntrypointInDir(tsDir)
	if err != nil {
		return "", nil, fmt.Errorf("find entrypoint: %w", err)
	}

	if entrypointFile == "" {
		return "", nil, nil
	}

	_, err = os.Stat(nodeModulesPath)
	if err != nil && !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("stat %s: %w", nodeModulesPath, err)
	}

	if err == nil {
		log.Default.Debug(ctx, "Building vendor bundle from node_modules")
		return buildVendorBundleInDir(tsDir, entrypointFile)
	}

	_, err = os.Stat(vendorPath)
	if err != nil && !os.IsNotExist(err) {
		return "", nil, fmt.Errorf("stat %s: %w", vendorPath, err)
	}

	if err == nil {
		log.Default.Debug(ctx, "Using pre-built vendor bundle from %s", vendorPath)

		vendorBytes, err := os.ReadFile(vendorPath)
		if err != nil {
			return "", nil, fmt.Errorf("read vendor bundle: %w", err)
		}

		packages := extractPackagesFromVendorBundle(string(vendorBytes))

		return string(vendorBytes), packages, nil
	}

	log.Default.Debug(ctx, "No vendor dependencies found")

	return "", nil, nil
}

func BuildAppBundleFromDir(ctx context.Context, chartPath string, externalPackages []string) (string, error) {
	absChartPath, err := filepath.Abs(chartPath)
	if err != nil {
		return "", fmt.Errorf("get absolute path: %w", err)
	}

	tsDir := filepath.Join(absChartPath, TSSourceDir)

	entrypointFile, err := findEntrypointInDir(tsDir)
	if err != nil {
		return "", fmt.Errorf("find entrypoint: %w", err)
	}

	if entrypointFile == "" {
		return "", fmt.Errorf("no TypeScript entrypoint found")
	}

	log.Default.Debug(ctx, "Building app bundle from %s", tsDir)

	return buildAppBundle(tsDir, entrypointFile, externalPackages)
}

func buildVendorBundleInDir(chartTSDir, entrypoint string) (vendorBundle string, packages []string, err error) {
	absEntrypoint := filepath.Join(chartTSDir, entrypoint)

	scanResult := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:   []string{absEntrypoint},
		Bundle:        true,
		Write:         false,
		Metafile:      true,
		Platform:      esbuild.PlatformNode,
		Format:        esbuild.FormatCommonJS,
		Target:        esbuild.ES2015,
		AbsWorkingDir: chartTSDir,
	})

	if len(scanResult.Errors) > 0 {
		return "", nil, formatBuildErrors(scanResult.Errors)
	}

	packages, err = extractPackageNames(scanResult.Metafile)
	if err != nil {
		return "", nil, err
	}

	if len(packages) == 0 {
		return "", packages, nil
	}

	virtualEntry := generateVendorEntrypoint(packages)

	vendorResult := esbuild.Build(esbuild.BuildOptions{
		Stdin: &esbuild.StdinOptions{
			Contents:   virtualEntry,
			ResolveDir: chartTSDir,
			Loader:     esbuild.LoaderJS,
		},
		Bundle:        true,
		Write:         false,
		Platform:      esbuild.PlatformNode,
		Format:        esbuild.FormatIIFE,
		Target:        esbuild.ES2015,
		GlobalName:    "__NELM_VENDOR_BUNDLE__",
		AbsWorkingDir: chartTSDir,
	})

	if len(vendorResult.Errors) > 0 {
		return "", nil, formatBuildErrors(vendorResult.Errors)
	}

	if len(vendorResult.OutputFiles) == 0 {
		return "", nil, fmt.Errorf("no output files from vendor bundle build")
	}

	return string(vendorResult.OutputFiles[0].Contents), packages, nil
}

func buildAppBundle(chartTSDir, entrypoint string, externalPackages []string) (string, error) {
	absEntrypoint := filepath.Join(chartTSDir, entrypoint)

	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:   []string{absEntrypoint},
		Bundle:        true,
		Write:         false,
		Platform:      esbuild.PlatformNode,
		Format:        esbuild.FormatCommonJS,
		Target:        esbuild.ES2015,
		External:      externalPackages,
		AbsWorkingDir: chartTSDir,
		Sourcemap:     esbuild.SourceMapInline,
	})

	if len(result.Errors) > 0 {
		return "", formatBuildErrors(result.Errors)
	}

	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("no output files from app bundle build")
	}

	return string(result.OutputFiles[0].Contents), nil
}

func findEntrypointInDir(tsDir string) (string, error) {
	for _, ep := range EntryPoints {
		epPath := filepath.Join(tsDir, ep)

		_, err := os.Stat(epPath)
		if err == nil {
			return ep, nil
		}

		if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", epPath, err)
		}
	}

	return "", nil
}
