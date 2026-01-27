package tschart

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

const (
	TSSourceDir      = "ts/"
	VendorBundleFile = "ts/vendor/libs.js"
	VendorBundleDir  = "ts/vendor"
	TSConfigFile     = "tsconfig.json"
)

var EntryPoints = []string{"src/index.ts", "src/index.js"}

type Transformer struct{}

func NewTransformer() *Transformer {
	return &Transformer{}
}

type Metafile struct {
	Inputs map[string]struct {
		Bytes int `json:"bytes"`
	} `json:"inputs"`
}

// --- Internal utilities ---

func extractPackageNames(metafileJSON string) ([]string, error) {
	var meta Metafile
	if err := json.Unmarshal([]byte(metafileJSON), &meta); err != nil {
		return nil, fmt.Errorf("parse metafile: %w", err)
	}

	pkgSet := make(map[string]struct{})

	for inputPath := range meta.Inputs {
		// Handle both regular paths and virtual namespace paths (e.g., "virtual:node_modules/...")
		normalizedPath := inputPath
		if strings.HasPrefix(inputPath, "virtual:") {
			normalizedPath = strings.TrimPrefix(inputPath, "virtual:")
		}

		if strings.HasPrefix(normalizedPath, "node_modules/") {
			parts := strings.Split(normalizedPath, "/")

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
