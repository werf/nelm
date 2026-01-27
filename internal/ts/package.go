package ts

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func extractPackageNames(metafileJSON string) ([]string, error) {
	var meta struct {
		Inputs map[string]json.RawMessage `json:"inputs"`
	}
	if err := json.Unmarshal([]byte(metafileJSON), &meta); err != nil {
		return nil, fmt.Errorf("parse metafile: %w", err)
	}

	pkgSet := make(map[string]struct{})
	for inputPath := range meta.Inputs {
		normalizedPath := filepath.ToSlash(strings.TrimPrefix(inputPath, "virtual:"))
		if strings.HasPrefix(normalizedPath, "node_modules/") {
			normalizedPath = "/" + normalizedPath
		}

		nodeModulesIdx := strings.LastIndex(normalizedPath, "/node_modules/")
		if nodeModulesIdx == -1 {
			continue
		}

		parts := strings.Split(strings.TrimPrefix(normalizedPath[nodeModulesIdx:], "/node_modules/"), "/")
		if len(parts) < 2 {
			continue
		}

		if strings.HasPrefix(parts[0], "@") {
			if len(parts) >= 3 {
				pkgSet[parts[0]+"/"+parts[1]] = struct{}{}
			}
		} else {
			pkgSet[parts[0]] = struct{}{}
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
	re := regexp.MustCompile(`__NELM_VENDOR__\[['"]([^'"]+)['"]\]`)
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
	var sb strings.Builder
	sb.WriteString("var __NELM_VENDOR__ = {};\n")

	for _, pkg := range packages {
		fmt.Fprintf(&sb, "__NELM_VENDOR__['%s'] = require('%s');\n", pkg, pkg)
	}

	sb.WriteString("if (typeof global !== 'undefined') { global.__NELM_VENDOR__ = __NELM_VENDOR__; }\n")
	sb.WriteString("if (typeof exports !== 'undefined') { exports.__NELM_VENDOR__ = __NELM_VENDOR__; }\n")

	return sb.String()
}
