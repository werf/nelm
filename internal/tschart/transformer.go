package tschart

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/nelm/pkg/log"
)

const (
	TSSourceDir      = "ts/"
	VendorBundleFile = "ts/vendor/libs.js"
	VendorBundleDir  = "ts/vendor"
	TSConfigFile     = "tsconfig.json"
)

var (
	EntryPoints = []string{"src/index.ts", "src/index.js"}
)

type Transformer struct{}

func NewTransformer() *Transformer {
	return &Transformer{}
}

type Metafile struct {
	Inputs map[string]struct {
		Bytes int `json:"bytes"`
	} `json:"inputs"`
}

func extractPackageNames(metafileJSON string) ([]string, error) {
	var meta Metafile
	if err := json.Unmarshal([]byte(metafileJSON), &meta); err != nil {
		return nil, fmt.Errorf("parse metafile: %w", err)
	}

	pkgSet := make(map[string]struct{})
	for inputPath := range meta.Inputs {
		if strings.HasPrefix(inputPath, "node_modules/") {
			parts := strings.Split(inputPath, "/")
			var pkgName string
			if len(parts) >= 2 && strings.HasPrefix(parts[1], "@") && len(parts) >= 3 {
				pkgName = parts[1] + "/" + parts[2]
			} else if len(parts) >= 2 {
				pkgName = parts[1]
			}
			if pkgName != "" {
				pkgSet[pkgName] = struct{}{}
			}
		}
	}

	packages := make([]string, 0, len(pkgSet))
	for pkg := range pkgSet {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)
	return packages, nil
}

func generateVendorEntrypoint(packages []string) string {
	var builder strings.Builder
	builder.WriteString("var __NELM_VENDOR__ = {};\n")

	for _, pkg := range packages {
		fmt.Fprintf(&builder, "__NELM_VENDOR__['%s'] = require('%s');\n", pkg, pkg)
	}

	builder.WriteString("if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }\n")
	builder.WriteString("if (typeof exports !== 'undefined') { exports.__NELM_VENDOR__ = __NELM_VENDOR__; }\n")

	return builder.String()
}

func buildVendorBundle(chartTsDir string, entrypoint string) (vendorBundle string, packages []string, err error) {
	absEntrypoint := filepath.Join(chartTsDir, entrypoint)

	scanResult := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:   []string{absEntrypoint},
		Bundle:        true,
		Write:         false,
		Metafile:      true,
		Platform:      esbuild.PlatformNode,
		Format:        esbuild.FormatCommonJS,
		Target:        esbuild.ES2015,
		AbsWorkingDir: chartTsDir,
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
			ResolveDir: chartTsDir,
			Loader:     esbuild.LoaderJS,
		},
		Bundle:        true,
		Write:         false,
		Platform:      esbuild.PlatformNode,
		Format:        esbuild.FormatIIFE,
		Target:        esbuild.ES2015,
		GlobalName:    "__NELM_VENDOR_BUNDLE__",
		AbsWorkingDir: chartTsDir,
	})

	if len(vendorResult.Errors) > 0 {
		return "", nil, formatBuildErrors(vendorResult.Errors)
	}

	if len(vendorResult.OutputFiles) == 0 {
		return "", nil, fmt.Errorf("no output files from vendor bundle build")
	}

	return string(vendorResult.OutputFiles[0].Contents), packages, nil
}

func buildAppBundle(chartTsDir string, entrypoint string, externalPackages []string) (string, error) {
	absEntrypoint := filepath.Join(chartTsDir, entrypoint)

	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:   []string{absEntrypoint},
		Bundle:        true,
		Write:         false,
		Platform:      esbuild.PlatformNode,
		Format:        esbuild.FormatCommonJS,
		Target:        esbuild.ES2015,
		External:      externalPackages,
		AbsWorkingDir: chartTsDir,
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

func loaderFromPath(path string) esbuild.Loader {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ts", ".tsx":
		return esbuild.LoaderTS
	case ".jsx":
		return esbuild.LoaderJSX
	case ".json":
		return esbuild.LoaderJSON
	default:
		return esbuild.LoaderJS
	}
}

func resolvePath(importPath string, resolveDir string, importer string) string {
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		var baseDir string
		if importer != "" {
			baseDir = filepath.Dir(importer)
		} else {
			baseDir = resolveDir
		}
		resolved := filepath.Clean(filepath.Join(baseDir, importPath))

		if filepath.Ext(resolved) == "" {
			for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".json"} {
				if _, ok := getFileFromPath(resolved + ext); ok {
					return resolved + ext
				}
			}
			for _, ext := range []string{".ts", ".js"} {
				indexPath := filepath.Join(resolved, "index"+ext)
				if _, ok := getFileFromPath(indexPath); ok {
					return indexPath
				}
			}
		}
		return resolved
	}
	return importPath
}

func getFileFromPath(path string) ([]byte, bool) {
	return nil, false
}

func createVirtualFSPlugin(files map[string][]byte) esbuild.Plugin {
	fileExists := func(path string) bool {
		_, exists := files[path]
		return exists
	}

	tryResolve := func(basePath string) string {
		if fileExists(basePath) {
			return basePath
		}
		for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".json"} {
			if fileExists(basePath + ext) {
				return basePath + ext
			}
		}
		for _, ext := range []string{".ts", ".js"} {
			indexPath := filepath.Join(basePath, "index"+ext)
			if fileExists(indexPath) {
				return indexPath
			}
		}
		return ""
	}

	return esbuild.Plugin{
		Name: "virtual-fs",
		Setup: func(build esbuild.PluginBuild) {
			build.OnResolve(esbuild.OnResolveOptions{Filter: `.*`},
				func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					if args.Importer == "" {
						finalPath := tryResolve(args.Path)
						if finalPath != "" {
							return esbuild.OnResolveResult{
								Path:      finalPath,
								Namespace: "virtual",
							}, nil
						}
					}

					if !strings.HasPrefix(args.Path, ".") && !strings.HasPrefix(args.Path, "/") && args.Importer != "" {
						return esbuild.OnResolveResult{}, nil
					}

					var resolvedPath string
					if strings.HasPrefix(args.Path, "./") || strings.HasPrefix(args.Path, "../") {
						var baseDir string
						if args.Importer != "" {
							baseDir = filepath.Dir(args.Importer)
						} else {
							baseDir = args.ResolveDir
						}
						resolvedPath = filepath.Clean(filepath.Join(baseDir, args.Path))
					} else {
						resolvedPath = args.Path
					}

					resolvedPath = strings.TrimPrefix(resolvedPath, "./")

					finalPath := tryResolve(resolvedPath)
					if finalPath != "" {
						return esbuild.OnResolveResult{
							Path:      finalPath,
							Namespace: "virtual",
						}, nil
					}

					return esbuild.OnResolveResult{}, nil
				})

			build.OnLoad(esbuild.OnLoadOptions{Filter: `.*`, Namespace: "virtual"},
				func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
					content, exists := files[args.Path]
					if !exists {
						return esbuild.OnLoadResult{}, fmt.Errorf("file not found in virtual fs: %s", args.Path)
					}
					contentStr := string(content)
					loader := loaderFromPath(args.Path)
					return esbuild.OnLoadResult{
						Contents:   &contentStr,
						Loader:     loader,
						ResolveDir: filepath.Dir(args.Path),
					}, nil
				})
		},
	}
}

func buildAppBundleFromFiles(files map[string][]byte, entrypoint string, externalPackages []string) (string, error) {
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entrypoint},
		Bundle:      true,
		Write:       false,
		Platform:    esbuild.PlatformNode,
		Format:      esbuild.FormatCommonJS,
		Target:      esbuild.ES2015,
		External:    externalPackages,
		Sourcemap:   esbuild.SourceMapInline,
		Plugins:     []esbuild.Plugin{createVirtualFSPlugin(files)},
	})

	if len(result.Errors) > 0 {
		return "", formatBuildErrors(result.Errors)
	}

	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("no output files from app bundle build")
	}

	return string(result.OutputFiles[0].Contents), nil
}

func extractPackagesFromVendorBundle(bundle string) []string {
	re := regexp.MustCompile(`__NELM_VENDOR__\[["']([^"']+)["']\]`)
	matches := re.FindAllStringSubmatch(bundle, -1)

	pkgSet := make(map[string]struct{})
	for _, match := range matches {
		if len(match) >= 2 {
			pkgSet[match[1]] = struct{}{}
		}
	}

	packages := make([]string, 0, len(pkgSet))
	for pkg := range pkgSet {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)
	return packages
}

func formatBuildErrors(errors []esbuild.Message) error {
	if len(errors) == 0 {
		return nil
	}

	var errMsg strings.Builder
	errMsg.WriteString("TypeScript transpilation failed:\n")

	for i, msg := range errors {
		if i > 0 {
			errMsg.WriteString("\n")
		}

		if msg.Location != nil {
			errMsg.WriteString(fmt.Sprintf("  File: %s:%d:%d\n",
				msg.Location.File,
				msg.Location.Line,
				msg.Location.Column,
			))

			if msg.Location.LineText != "" {
				errMsg.WriteString(fmt.Sprintf("    %s\n", msg.Location.LineText))

				if msg.Location.Column > 0 {
					spaces := strings.Repeat(" ", msg.Location.Column)
					errMsg.WriteString(fmt.Sprintf("    %s^\n", spaces))
				}
			}
		}

		errMsg.WriteString(fmt.Sprintf("  Error: %s\n", msg.Text))

		if len(msg.Notes) > 0 {
			for _, note := range msg.Notes {
				errMsg.WriteString(fmt.Sprintf("  Note: %s\n", note.Text))
			}
		}
	}

	return fmt.Errorf("%s", errMsg.String())
}

func findEntrypoint(tsDir string) string {
	for _, ep := range EntryPoints {
		epPath := filepath.Join(tsDir, ep)
		if _, err := os.Stat(epPath); err == nil {
			return ep
		}
	}
	return ""
}

func (t *Transformer) TransformChartDir(ctx context.Context, chartPath string) error {
	stat, err := os.Stat(chartPath)
	if err != nil || !stat.IsDir() {
		log.Default.Debug(ctx, "Skipping TypeScript transformation: %s is not a directory", chartPath)
		return nil
	}

	tsDir := filepath.Join(chartPath, TSSourceDir)
	if _, err := os.Stat(tsDir); os.IsNotExist(err) {
		log.Default.Debug(ctx, "No %s directory found, skipping transformation", TSSourceDir)
		return nil
	}

	entrypointFile := findEntrypoint(tsDir)
	if entrypointFile == "" {
		log.Default.Debug(ctx, "No TypeScript entrypoint found, skipping transformation")
		return nil
	}

	nodeModulesPath := filepath.Join(tsDir, "node_modules")
	if _, err := os.Stat(nodeModulesPath); os.IsNotExist(err) {
		log.Default.Debug(ctx, "No node_modules directory found, skipping vendor bundle")
		return nil
	}

	log.Default.Info(ctx, "Building vendor bundle for TypeScript chart: %s", chartPath)

	vendorBundle, packages, err := buildVendorBundle(tsDir, entrypointFile)
	if err != nil {
		return fmt.Errorf("build vendor bundle: %w", err)
	}

	if len(packages) == 0 {
		log.Default.Debug(ctx, "No npm packages used, skipping vendor bundle")
		return nil
	}

	log.Default.Info(ctx, "Bundled %d npm packages: %s", len(packages), strings.Join(packages, ", "))

	vendorPath := filepath.Join(chartPath, VendorBundleFile)
	if err := os.MkdirAll(filepath.Dir(vendorPath), 0755); err != nil {
		return fmt.Errorf("create vendor directory: %w", err)
	}
	if err := os.WriteFile(vendorPath, []byte(vendorBundle), 0644); err != nil {
		return fmt.Errorf("write vendor bundle to %s: %w", vendorPath, err)
	}

	log.Default.Info(ctx, "Wrote vendor bundle to %s", VendorBundleFile)

	return nil
}

func (t *Transformer) TransformChartForRender(ctx context.Context, chartPath string, chart *helmchart.Chart) error {
	stat, err := os.Stat(chartPath)
	if err != nil || !stat.IsDir() {
		hasSourceFiles := false
		for _, f := range chart.Files {
			if strings.HasPrefix(f.Name, TSSourceDir+"src/") {
				for _, ep := range EntryPoints {
					if f.Name == TSSourceDir+ep {
						hasSourceFiles = true
						break
					}
				}
			}
			if hasSourceFiles {
				break
			}
		}

		if !hasSourceFiles {
			log.Default.Debug(ctx, "No TypeScript source found in packaged chart, skipping")
			return nil
		}

		hasVendorBundle := false
		for _, f := range chart.Files {
			if f.Name == VendorBundleFile {
				hasVendorBundle = true
				break
			}
		}

		if !hasVendorBundle {
			log.Default.Debug(ctx, "Packaged chart has TypeScript source but no vendor bundle (may have no npm deps)")
		}

		return nil
	}

	tsDir := filepath.Join(chartPath, TSSourceDir)
	entrypointFile := findEntrypoint(tsDir)
	if entrypointFile == "" {
		log.Default.Debug(ctx, "No TypeScript entrypoint found, skipping transformation")
		return nil
	}

	log.Default.Debug(ctx, "TypeScript chart ready for rendering: %s", chartPath)
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

	entrypointFile := findEntrypoint(tsDir)
	if entrypointFile == "" {
		return "", nil, nil
	}

	if _, err := os.Stat(nodeModulesPath); err == nil {
		log.Default.Debug(ctx, "Building vendor bundle from node_modules")
		return buildVendorBundle(tsDir, entrypointFile)
	}

	if _, err := os.Stat(vendorPath); err == nil {
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

func GetVendorBundleFromFiles(files []*helmchart.File) (string, []string) {
	for _, f := range files {
		if f.Name == VendorBundleFile {
			packages := extractPackagesFromVendorBundle(string(f.Data))
			return string(f.Data), packages
		}
	}
	return "", nil
}

func ExtractSourceFiles(files []*helmchart.File) map[string][]byte {
	sourceFiles := make(map[string][]byte)
	for _, f := range files {
		if strings.HasPrefix(f.Name, TSSourceDir+"src/") {
			relativePath := strings.TrimPrefix(f.Name, TSSourceDir)
			sourceFiles[relativePath] = f.Data
		}
	}
	return sourceFiles
}

func BuildAppBundleFromDir(ctx context.Context, chartPath string, externalPackages []string) (string, error) {
	absChartPath, err := filepath.Abs(chartPath)
	if err != nil {
		return "", fmt.Errorf("get absolute path: %w", err)
	}

	tsDir := filepath.Join(absChartPath, TSSourceDir)
	entrypointFile := findEntrypoint(tsDir)
	if entrypointFile == "" {
		return "", fmt.Errorf("no TypeScript entrypoint found")
	}

	log.Default.Debug(ctx, "Building app bundle from %s", tsDir)
	return buildAppBundle(tsDir, entrypointFile, externalPackages)
}

func BuildAppBundleFromChartFiles(ctx context.Context, files []*helmchart.File, externalPackages []string) (string, error) {
	sourceFiles := ExtractSourceFiles(files)
	if len(sourceFiles) == 0 {
		return "", fmt.Errorf("no source files found in chart")
	}

	var entrypoint string
	for _, ep := range EntryPoints {
		if _, exists := sourceFiles[ep]; exists {
			entrypoint = ep
			break
		}
	}
	if entrypoint == "" {
		return "", fmt.Errorf("no entrypoint found in source files")
	}

	log.Default.Debug(ctx, "Building app bundle from chart files with entrypoint %s", entrypoint)
	return buildAppBundleFromFiles(sourceFiles, entrypoint, externalPackages)
}
