package ts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/samber/lo"

	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/log"
)

var chartTSEntryPoints = [...]string{common.ChartTSEntryPointTS, common.ChartTSEntryPointJS}

// BundleChartsRecursive builds the bundle of the chart and of its unpacked subcharts, if it is
// missing or if rebuildBundle is set, installing their dependencies first if needed.
func BundleChartsRecursive(ctx context.Context, chart helmchart.Accessor, path string, rebuildBundle bool) error {
	entrypoint, bundle := getEntrypointAndBundle(chart.RuntimeFiles())

	if entrypoint != "" && (bundle == nil || rebuildBundle) {
		log.Default.Info(ctx, "Bundle TypeScript for chart %q (entrypoint: %s)", chart.Name(), entrypoint)

		tsDir, err := filepath.Abs(filepath.Join(path, common.ChartTSSourceDir))
		if err != nil {
			return fmt.Errorf("get absolute path of TypeScript directory: %w", err)
		}

		if err := ensureDependencies(ctx, tsDir, false); err != nil {
			return fmt.Errorf("install TypeScript dependencies of chart %q: %w", chart.Name(), err)
		}

		bundleRes, err := buildBundle(ctx, tsDir, entrypoint)
		if err != nil {
			return fmt.Errorf("build TypeScript bundle: %w", err)
		}

		if bundle != nil {
			chart.RemoveRuntimeFile(common.ChartTSBundleFile)
		}

		chart.AddRuntimeFile(common.ChartTSBundleFile, bundleRes)
	}

	for _, dep := range chart.Dependencies() {
		depAcc, err := helmchart.NewAccessor(dep)
		if err != nil {
			return fmt.Errorf("create accessor for dependency: %w", err)
		}

		depPath := filepath.Join(path, "charts", depAcc.Name())

		if _, err := os.Stat(depPath); err != nil {
			// Subchart loaded from .tgz or missing on disk — skip,
			// bundling needs a real directory to work with.
			continue
		}

		if err := BundleChartsRecursive(ctx, depAcc, depPath, rebuildBundle); err != nil {
			return fmt.Errorf("process dependency %q: %w", depAcc.Name(), err)
		}
	}

	return nil
}

// buildBundle bundles the chart with the esbuild options agreed on with the CLI of
// @nelm/chart-ts-sdk, which bundles the same way for local development.
func buildBundle(ctx context.Context, tsDir, entryPoint string) ([]byte, error) {
	result := api.Build(api.BuildOptions{
		AbsWorkingDir: tsDir,
		// CommonJS dependencies need require for Node.js builtins and __dirname/__filename, which an ES
		// module lacks. Declaring the latter in the banner would clash with ES modules declaring them.
		Banner: map[string]string{
			"js": "import { createRequire as __nelmCreateRequire } from 'node:module'; const require = __nelmCreateRequire(import.meta.url);",
		},
		Bundle: true,
		Define: map[string]string{
			"__dirname":  "import.meta.dirname",
			"__filename": "import.meta.filename",
		},
		Engines:     []api.Engine{{Name: api.EngineNode, Version: "26"}},
		EntryPoints: []string{entryPoint},
		Format:      api.FormatESModule,
		// Does not change the bundle, which must stay the same as the one the SDK builds.
		Metafile: true,
		Platform: api.PlatformNode,
	})

	for _, warning := range api.FormatMessages(result.Warnings, api.FormatMessagesOptions{Kind: api.WarningMessage}) {
		log.Default.Warn(ctx, "%s", strings.TrimSpace(warning))
	}

	if len(result.Errors) > 0 {
		return nil, errors.New(strings.TrimSpace(strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Kind: api.ErrorMessage}), "")))
	}

	if err := checkExternalImports(result.Metafile); err != nil {
		return nil, err
	}

	if len(result.OutputFiles) != 1 {
		return nil, fmt.Errorf("expected 1 output file from esbuild, got %d", len(result.OutputFiles))
	}

	return result.OutputFiles[0].Contents, nil
}

func getEntrypointAndBundle(files []*chartcommon.File) (string, *chartcommon.File) {
	entrypoint := findEntrypointInFiles(files)
	if entrypoint == "" {
		return "", nil
	}

	bundleFile, foundBundle := lo.Find(files, func(f *chartcommon.File) bool {
		return f.Name == common.ChartTSBundleFile
	})

	if !foundBundle {
		return entrypoint, nil
	}

	return entrypoint, bundleFile
}

func hasTSFiles(chart helmchart.Accessor) bool {
	entrypoint := findEntrypointInFiles(chart.RuntimeFiles())
	if entrypoint != "" {
		return true
	}

	for _, dep := range chart.Dependencies() {
		depAcc, err := helmchart.NewAccessor(dep)
		if err != nil {
			continue
		}

		if hasTSFiles(depAcc) {
			return true
		}
	}

	return false
}

// checkExternalImports fails on the imports esbuild left out of the bundle other than Node.js
// built-in modules, such as URL imports, which Node.js does not load.
func checkExternalImports(metafile string) error {
	var meta struct {
		Inputs map[string]struct {
			Imports []struct {
				External bool   `json:"external"`
				Path     string `json:"path"`
			} `json:"imports"`
		} `json:"inputs"`
	}

	if err := json.Unmarshal([]byte(metafile), &meta); err != nil {
		return fmt.Errorf("decode esbuild metafile: %w", err)
	}

	var unsupported []string

	for _, input := range slices.Sorted(maps.Keys(meta.Inputs)) {
		for _, imported := range meta.Inputs[input].Imports {
			// esbuild leaves out bare names only for built-in modules on the node platform.
			builtin := strings.HasPrefix(imported.Path, "node:") || !strings.Contains(imported.Path, ":") && !strings.HasPrefix(imported.Path, "/")
			if imported.External && !builtin {
				unsupported = append(unsupported, fmt.Sprintf("%q imported by %s", imported.Path, input))
			}
		}
	}

	if len(unsupported) > 0 {
		return fmt.Errorf("unsupported imports: %s: Node.js loads only built-in modules at run time, so install the package with the package manager and import it by its name", strings.Join(unsupported, ", "))
	}

	return nil
}

func findEntrypointInFiles(files []*chartcommon.File) string {
	sourceFiles := make(map[string][]byte)

	for _, f := range files {
		if strings.HasPrefix(f.Name, common.ChartTSSourceDir+"src/") {
			sourceFiles[strings.TrimPrefix(f.Name, common.ChartTSSourceDir)] = f.Data
		}
	}

	if len(sourceFiles) == 0 {
		return ""
	}

	for _, ep := range chartTSEntryPoints {
		if _, ok := sourceFiles[ep]; ok {
			return ep
		}
	}

	return ""
}
