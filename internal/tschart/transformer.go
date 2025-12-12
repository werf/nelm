package tschart

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/nelm/pkg/log"
)

const (
	TSSourceDir = "ts/"
	BundleFile = "ts/chart_render_main.js"
	TSConfigFile = "tsconfig.json"
)

var (
	EntryPoints = []string{"src/index.ts", "src/index.js"}
)

type Transformer struct {
	// Future: add options like cache, custom esbuild config, etc.
}

func NewTransformer() *Transformer {
	return &Transformer{}
}

func buildTSFromDir(ctx context.Context, tsDir string, entryPoint string) (esbuild.BuildResult, error) {
	fullEntryPoint := filepath.Join(tsDir, entryPoint)

	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{fullEntryPoint},
		Bundle:      true,
		Write:       false,
		Outfile:     BundleFile,
		Platform:    esbuild.PlatformNode,
		Format:      esbuild.FormatCommonJS,
		Sourcemap:   esbuild.SourceMapInline,
		Target:      esbuild.ES2015,

		// NOTE: Minify options are breaking stack traces and error messages, disable for now
		// MinifyWhitespace: true,
		// MinifyIdentifiers: true,

		// It does not work for CommonJS
		// TreeShaking: esbuild.TreeShakingTrue,
		External:    []string{},
	})

	return result, nil
}

// formatBuildErrors formats esbuild errors into a human-readable error message.
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

				// Add a caret pointer to the error column
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

// TransformChartDir transpiles TypeScript files in a chart directory to JavaScript
// and writes the result to disk.
//
// The bundle is written to: <chartPath>/ts/chart_render_main.js
//
// Behavior:
// - If chartPath is not a directory, returns nil (no-op)
// - If no ts/src/index.ts or ts/src/index.js exists, returns nil (no-op)
// - If TypeScript source exists, always overwrites existing bundle
//
// Returns error if:
// - TypeScript files exist but transpilation fails
// - Cannot write output file
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

	var entrypointFile string
	for _, ep := range EntryPoints {
		epPath := filepath.Join(tsDir, ep)
		if _, err := os.Stat(epPath); err == nil {
			entrypointFile = ep
			break
		}
	}

	if entrypointFile == "" {
		log.Default.Debug(ctx, "No TypeScript entrypoint found, skipping transformation")
		return nil
	}

	log.Default.Info(ctx, "Transpiling TypeScript chart: %s", chartPath)

	result, err := buildTSFromDir(ctx, tsDir, entrypointFile)
	if err != nil {
		return fmt.Errorf("build TypeScript: %w", err)
	}

	if len(result.Errors) > 0 {
		return formatBuildErrors(result.Errors)
	}

	for _, warn := range result.Warnings {
		log.Default.Warn(ctx, "TypeScript build warning: %s", warn.Text)
	}

	if len(result.OutputFiles) == 0 {
		return fmt.Errorf("no output files from TypeScript build")
	}

	bundlePath := filepath.Join(chartPath, BundleFile)
	if err := os.WriteFile(bundlePath, result.OutputFiles[0].Contents, 0644); err != nil {
		return fmt.Errorf("write bundle to %s: %w", bundlePath, err)
	}

	log.Default.Info(ctx, "Wrote TypeScript bundle to %s", BundleFile)

	return nil
}

// TransformChartForRender prepares TypeScript bundle for chart rendering.
//
// This function ensures the chart has a JavaScript bundle ready for execution:
// - If bundle already exists in chart.Files (loaded by Helm or pre-built), returns nil
// - If local directory with entrypoint: builds from filesystem, adds to chart.Files
// - If packaged/remote chart with source but no bundle: returns error
//
// Unlike TransformChartDir (which writes to disk for packaging), this function
// only adds the bundle to chart.Files for in-memory rendering.
func (t *Transformer) TransformChartForRender(ctx context.Context, chartPath string, chart *helmchart.Chart) error {
	// Check if bundle already exists in chart.Files
	// (either loaded by Helm from disk, or pre-built in packaged chart)
	for _, f := range chart.Files {
		if f.Name == BundleFile {
			log.Default.Debug(ctx, "Found existing %s in chart.Files, skipping transformation", BundleFile)
			return nil
		}
	}

	stat, err := os.Stat(chartPath)
	if err == nil && stat.IsDir() {
		tsDir := filepath.Join(chartPath, TSSourceDir)

		var entrypointFile string
		for _, ep := range EntryPoints {
			epPath := filepath.Join(tsDir, ep)
			if _, err := os.Stat(epPath); err == nil {
				entrypointFile = ep
				break
			}
		}

		if entrypointFile == "" {
			log.Default.Debug(ctx, "No TypeScript entrypoint found, skipping transformation")
			return nil
		}

		log.Default.Info(ctx, "Transpiling TypeScript chart: %s", chartPath)

		result, err := buildTSFromDir(ctx, tsDir, entrypointFile)
		if err != nil {
			return fmt.Errorf("build TypeScript: %w", err)
		}

		if len(result.Errors) > 0 {
			return formatBuildErrors(result.Errors)
		}

		for _, warn := range result.Warnings {
			log.Default.Warn(ctx, "TypeScript build warning: %s", warn.Text)
		}

		if len(result.OutputFiles) == 0 {
			return fmt.Errorf("no output files from TypeScript build")
		}

		chart.Files = append(chart.Files, &helmchart.File{
			Name: BundleFile,
			Data: result.OutputFiles[0].Contents,
		})
		log.Default.Debug(ctx, "Added transpiled JavaScript to inmemory chart.Files as %s", BundleFile)

		return nil
	}

	hasSourceFiles := false
	for _, f := range chart.Files {
		if strings.HasPrefix(f.Name, TSSourceDir) {
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

	if hasSourceFiles {
		return fmt.Errorf("packaged chart has TypeScript source (%s) but no pre-built bundle (%s). "+
			"Re-package the chart with NELM_FEAT_TS_CHARTS=true or include a pre-built bundle",
			TSSourceDir, BundleFile)
	}

	log.Default.Debug(ctx, "No TypeScript source found, skipping transformation")
	return nil
}
