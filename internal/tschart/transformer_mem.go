package tschart

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/nelm/pkg/log"
)

// GetVendorBundleFromFiles returns the vendor bundle and list of packages.
// If node_modules are present in files, it builds the vendor bundle in-memory.
// Otherwise, it looks for a pre-built vendor bundle at ts/vendor/libs.js.
func GetVendorBundleFromFiles(files []*helmchart.File) (string, []string, error) {
	// Check if node_modules are present - if so, build vendor bundle in-memory
	if hasNodeModules(files) {
		filesMap := prepareFilesMap(files)

		// Find entrypoint
		var entrypoint string

		for _, ep := range EntryPoints {
			if _, exists := filesMap[ep]; exists {
				entrypoint = ep
				break
			}
		}

		if entrypoint == "" {
			return "", nil, nil // No entrypoint, no vendor bundle needed
		}

		vendorBundle, packages, err := buildVendorBundleFromFiles(filesMap, entrypoint)
		if err != nil {
			return "", nil, fmt.Errorf("build vendor bundle from node_modules: %w", err)
		}

		return vendorBundle, packages, nil
	}

	// Fall back to pre-built vendor bundle
	for _, f := range files {
		if f.Name == VendorBundleFile {
			packages := extractPackagesFromVendorBundle(string(f.Data))
			return string(f.Data), packages, nil
		}
	}

	return "", nil, nil
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

// buildVendorBundleFromFiles builds a vendor bundle from in-memory source files and node_modules.
// It scans the entrypoint to find which packages are used, then bundles them.
func buildVendorBundleFromFiles(files map[string][]byte, entrypoint string) (vendorBundle string, packages []string, err error) {
	scanResult := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{entrypoint},
		Bundle:      true,
		Write:       false,
		Metafile:    true,
		Platform:    esbuild.PlatformNode,
		Format:      esbuild.FormatCommonJS,
		Target:      esbuild.ES2015,
		Plugins:     []esbuild.Plugin{createVirtualFSPluginWithNodeModules(files)},
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
			ResolveDir: ".",
			Loader:     esbuild.LoaderJS,
		},
		Bundle:     true,
		Write:      false,
		Platform:   esbuild.PlatformNode,
		Format:     esbuild.FormatIIFE,
		Target:     esbuild.ES2015,
		GlobalName: "__NELM_VENDOR_BUNDLE__",
		Plugins:    []esbuild.Plugin{createVirtualFSPluginWithNodeModules(files)},
	})

	if len(vendorResult.Errors) > 0 {
		return "", nil, formatBuildErrors(vendorResult.Errors)
	}

	if len(vendorResult.OutputFiles) == 0 {
		return "", nil, fmt.Errorf("no output files from vendor bundle build")
	}

	return string(vendorResult.OutputFiles[0].Contents), packages, nil
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

// prepareFilesMap converts []*helmchart.File to map[string][]byte, stripping the ts/ prefix.
// This prepares files for use with the virtual FS plugin.
func prepareFilesMap(files []*helmchart.File) map[string][]byte {
	result := make(map[string][]byte)

	for _, f := range files {
		// Strip ts/ prefix so paths become like "src/index.ts" and "node_modules/lodash/index.js"
		name := strings.TrimPrefix(f.Name, TSSourceDir)
		result[name] = f.Data
	}

	return result
}

func hasNodeModules(files []*helmchart.File) bool {
	for _, f := range files {
		if strings.HasPrefix(f.Name, TSSourceDir+"node_modules/") {
			return true
		}
	}

	return false
}

// virtualFSResolver provides file resolution for virtual filesystem plugins.
type virtualFSResolver struct {
	files map[string][]byte
}

func newVirtualFSResolver(files map[string][]byte) *virtualFSResolver {
	return &virtualFSResolver{files: files}
}

func (r *virtualFSResolver) exists(path string) bool {
	_, exists := r.files[path]
	return exists
}

func (r *virtualFSResolver) resolve(basePath string) string {
	if r.exists(basePath) {
		return basePath
	}

	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".json"} {
		if r.exists(basePath + ext) {
			return basePath + ext
		}
	}

	for _, ext := range []string{".ts", ".js"} {
		indexPath := filepath.Join(basePath, "index"+ext)
		if r.exists(indexPath) {
			return indexPath
		}
	}

	return ""
}

func (r *virtualFSResolver) resolveNodeModule(pkgName string) string {
	pkgPath := filepath.Join("node_modules", pkgName)

	pkgJSONPath := filepath.Join(pkgPath, "package.json")
	if pkgJSON, exists := r.files[pkgJSONPath]; exists {
		var pkg struct {
			Main    string      `json:"main"`
			Module  string      `json:"module"`
			Exports interface{} `json:"exports"`
		}
		if err := json.Unmarshal(pkgJSON, &pkg); err == nil {
			if pkg.Main != "" {
				mainPath := filepath.Join(pkgPath, pkg.Main)
				if resolved := r.resolve(mainPath); resolved != "" {
					return resolved
				}
			}

			if pkg.Module != "" {
				modulePath := filepath.Join(pkgPath, pkg.Module)
				if resolved := r.resolve(modulePath); resolved != "" {
					return resolved
				}
			}
		}
	}

	if resolved := r.resolve(pkgPath); resolved != "" {
		return resolved
	}

	return ""
}

func (r *virtualFSResolver) load(path string) (esbuild.OnLoadResult, error) {
	content, exists := r.files[path]
	if !exists {
		return esbuild.OnLoadResult{}, fmt.Errorf("file not found in virtual fs: %s", path)
	}

	contentStr := string(content)
	loader := loaderFromPath(path)

	return esbuild.OnLoadResult{
		Contents:   &contentStr,
		Loader:     loader,
		ResolveDir: filepath.Dir(path),
	}, nil
}

func createVirtualFSPlugin(files map[string][]byte) esbuild.Plugin {
	r := newVirtualFSResolver(files)

	return esbuild.Plugin{
		Name: "virtual-fs",
		Setup: func(build esbuild.PluginBuild) {
			build.OnResolve(esbuild.OnResolveOptions{Filter: `.*`},
				func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					if args.Importer == "" {
						finalPath := r.resolve(args.Path)
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

					finalPath := r.resolve(resolvedPath)
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
					return r.load(args.Path)
				})
		},
	}
}

// createVirtualFSPluginWithNodeModules creates an esbuild plugin that resolves both
// source files and node_modules from in-memory files map.
func createVirtualFSPluginWithNodeModules(files map[string][]byte) esbuild.Plugin {
	r := newVirtualFSResolver(files)

	return esbuild.Plugin{
		Name: "virtual-fs-with-node-modules",
		Setup: func(build esbuild.PluginBuild) {
			build.OnResolve(esbuild.OnResolveOptions{Filter: `.*`},
				func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
					if args.Importer == "" {
						finalPath := r.resolve(args.Path)
						if finalPath != "" {
							return esbuild.OnResolveResult{
								Path:      finalPath,
								Namespace: "virtual",
							}, nil
						}
					}

					if !strings.HasPrefix(args.Path, ".") && !strings.HasPrefix(args.Path, "/") {
						parts := strings.SplitN(args.Path, "/", 2)
						pkgName := parts[0]

						if strings.HasPrefix(pkgName, "@") && len(parts) > 1 {
							subparts := strings.SplitN(parts[1], "/", 2)

							pkgName = pkgName + "/" + subparts[0]
							if len(subparts) > 1 {
								parts = []string{pkgName, subparts[1]}
							} else {
								parts = []string{pkgName}
							}
						}

						if len(parts) == 2 {
							subPath := filepath.Join("node_modules", pkgName, parts[1])
							if resolved := r.resolve(subPath); resolved != "" {
								return esbuild.OnResolveResult{
									Path:      resolved,
									Namespace: "virtual",
								}, nil
							}
						} else {
							if resolved := r.resolveNodeModule(pkgName); resolved != "" {
								return esbuild.OnResolveResult{
									Path:      resolved,
									Namespace: "virtual",
								}, nil
							}
						}

						return esbuild.OnResolveResult{}, fmt.Errorf("cannot resolve package %q in virtual filesystem", args.Path)
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

					finalPath := r.resolve(resolvedPath)
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
					return r.load(args.Path)
				})
		},
	}
}
