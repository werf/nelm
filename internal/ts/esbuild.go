package ts

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

// virtualFSResolver resolves imports from in-memory file map.
type virtualFSResolver struct {
	files map[string][]byte
}

func (r *virtualFSResolver) load(filePath string) (esbuild.OnLoadResult, error) {
	content, ok := r.files[filePath]
	if !ok {
		return esbuild.OnLoadResult{}, fmt.Errorf("load file: %s not found", filePath)
	}

	s := string(content)

	return esbuild.OnLoadResult{
		Contents:   &s,
		Loader:     esbuildLoaderFromPath(filePath),
		ResolveDir: path.Dir(filePath),
	}, nil
}

func (r *virtualFSResolver) resolve(basePath string) string {
	if _, ok := r.files[basePath]; ok {
		return basePath
	}

	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".json"} {
		if _, ok := r.files[basePath+ext]; ok {
			return basePath + ext
		}
	}

	for _, ext := range []string{".ts", ".js"} {
		p := path.Join(basePath, "index"+ext)
		if _, ok := r.files[p]; ok {
			return p
		}
	}

	return ""
}

func (r *virtualFSResolver) resolvePackageImport(importPath string) string {
	pkgName, subPath := parsePackageImport(importPath)
	if subPath != "" {
		return r.resolve(path.Join("node_modules", pkgName, subPath))
	}

	pkgPath := path.Join("node_modules", pkgName)
	if pkgJSON, ok := r.files[path.Join(pkgPath, "package.json")]; ok {
		var pkg struct {
			Main   string `json:"main"`
			Module string `json:"module"`
		}
		if json.Unmarshal(pkgJSON, &pkg) == nil {
			for _, entry := range []string{pkg.Main, pkg.Module} {
				if entry != "" {
					if resolved := r.resolve(path.Join(pkgPath, entry)); resolved != "" {
						return resolved
					}
				}
			}
		}
	}

	return r.resolve(pkgPath)
}

func runEsbuildBundle(opts esbuild.BuildOptions) (string, error) {
	result := esbuild.Build(opts)
	if len(result.Errors) > 0 {
		return "", formatEsbuildErrors(result.Errors)
	}

	if len(result.OutputFiles) == 0 {
		return "", fmt.Errorf("run esbuild bundle: no output files")
	}

	return string(result.OutputFiles[0].Contents), nil
}

func esbuildLoaderFromPath(filePath string) esbuild.Loader {
	switch strings.ToLower(path.Ext(filePath)) {
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

func formatEsbuildErrors(errors []esbuild.Message) error {
	if len(errors) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("TypeScript transpilation failed:\n")

	for _, msg := range errors {
		if loc := msg.Location; loc != nil {
			fmt.Fprintf(&sb, "  %s:%d:%d: %s\n", loc.File, loc.Line, loc.Column, msg.Text)

			if loc.LineText != "" {
				fmt.Fprintf(&sb, "    %s\n    %s^\n", loc.LineText, strings.Repeat(" ", loc.Column))
			}
		} else {
			fmt.Fprintf(&sb, "  %s\n", msg.Text)
		}

		for _, note := range msg.Notes {
			fmt.Fprintf(&sb, "  Note: %s\n", note.Text)
		}
	}

	return fmt.Errorf("%s", sb.String())
}

func newEsbuildOptions() esbuild.BuildOptions {
	return esbuild.BuildOptions{
		Bundle:   true,
		Write:    false,
		Platform: esbuild.PlatformNode,
		Format:   esbuild.FormatCommonJS,
		Target:   esbuild.ES2015,
	}
}

func newVendorEsbuildOptions(packages []string, resolveDir string) esbuild.BuildOptions {
	return esbuild.BuildOptions{
		Stdin: &esbuild.StdinOptions{
			Contents:   generateVendorEntrypoint(packages),
			ResolveDir: resolveDir,
			Loader:     esbuild.LoaderJS,
		},
		Bundle:     true,
		Write:      false,
		Platform:   esbuild.PlatformNode,
		Format:     esbuild.FormatIIFE,
		Target:     esbuild.ES2015,
		GlobalName: "__NELM_VENDOR_BUNDLE__",
	}
}

func newVirtualFSPlugin(files map[string][]byte, resolveNodeModules bool) esbuild.Plugin {
	r := &virtualFSResolver{
		files: files,
	}

	return esbuild.Plugin{
		Name: "virtual-fs",
		Setup: func(build esbuild.PluginBuild) {
			build.OnResolve(esbuild.OnResolveOptions{Filter: `.*`}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
				if args.Importer == "" {
					if resolved := r.resolve(args.Path); resolved != "" {
						return esbuild.OnResolveResult{Path: resolved, Namespace: "virtual"}, nil
					}
				}

				if !strings.HasPrefix(args.Path, ".") && !strings.HasPrefix(args.Path, "/") {
					if !resolveNodeModules {
						return esbuild.OnResolveResult{}, nil
					}

					if resolved := r.resolvePackageImport(args.Path); resolved != "" {
						return esbuild.OnResolveResult{Path: resolved, Namespace: "virtual"}, nil
					}

					return esbuild.OnResolveResult{}, fmt.Errorf("resolve import: cannot resolve package %q", args.Path)
				}

				baseDir := args.ResolveDir
				if args.Importer != "" {
					baseDir = path.Dir(args.Importer)
				}

				resolvedPath := strings.TrimPrefix(path.Clean(path.Join(baseDir, args.Path)), "./")
				if resolved := r.resolve(resolvedPath); resolved != "" {
					return esbuild.OnResolveResult{Path: resolved, Namespace: "virtual"}, nil
				}

				return esbuild.OnResolveResult{}, nil
			})

			build.OnLoad(esbuild.OnLoadOptions{Filter: `.*`, Namespace: "virtual"}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
				return r.load(args.Path)
			})
		},
	}
}

func parsePackageImport(importPath string) (pkgName, subPath string) {
	parts := strings.SplitN(importPath, "/", 2)
	if strings.HasPrefix(parts[0], "@") && len(parts) > 1 {
		subparts := strings.SplitN(parts[1], "/", 2)

		pkgName = parts[0] + "/" + subparts[0]
		if len(subparts) > 1 {
			subPath = subparts[1]
		}
	} else {
		pkgName = parts[0]
		if len(parts) > 1 {
			subPath = parts[1]
		}
	}

	return pkgName, subPath
}
