package ts

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		Platform:    api.PlatformNode,
	})

	for _, warning := range api.FormatMessages(result.Warnings, api.FormatMessagesOptions{Kind: api.WarningMessage}) {
		log.Default.Warn(ctx, "%s", strings.TrimSpace(warning))
	}

	if len(result.Errors) > 0 {
		err := errors.New(strings.TrimSpace(strings.Join(api.FormatMessages(result.Errors, api.FormatMessagesOptions{Kind: api.ErrorMessage}), "")))

		unresolved := lo.ContainsBy(result.Errors, func(message api.Message) bool {
			return strings.HasPrefix(message.Text, "Could not resolve")
		})
		if !unresolved {
			return nil, err
		}

		hint := fmt.Sprintf("Install the dependencies of %s with its package manager and make sure every imported package is in package.json.", tsDir)

		return nil, fmt.Errorf("%w\n%s", err, hint)
	}

	if len(result.OutputFiles) != 1 {
		return nil, fmt.Errorf("expected 1 output file from esbuild, got %d", len(result.OutputFiles))
	}

	return result.OutputFiles[0].Contents, nil
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
