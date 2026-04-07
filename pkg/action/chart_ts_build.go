package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dustin/go-humanize"
	"github.com/gookit/color"
	"github.com/samber/lo"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
	helmchart "github.com/werf/nelm/pkg/helm/pkg/chart"
	"github.com/werf/nelm/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/pkg/helm/pkg/werf/helmopts"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/ts"
)

type ChartTSBuildOptions struct {
	ChartDirPath   string
	DenoBinaryPath string
	TempDirPath    string
}

func ChartTSBuild(ctx context.Context, opts ChartTSBuildOptions) error {
	chartPath := opts.ChartDirPath
	if chartPath == "" {
		chartPath = "."
	}

	absPath, err := filepath.Abs(chartPath)
	if err != nil {
		return fmt.Errorf("get absolute path: %w", err)
	}

	if !featgate.FeatGateTypescript.Enabled() {
		log.Default.Warn(ctx, "TypeScript charts require NELM_FEAT_TYPESCRIPT=true environment variable")

		return fmt.Errorf("TypeScript charts feature is not enabled. Set NELM_FEAT_TYPESCRIPT=true to use this feature")
	}

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Run bundle for ")+"%s", absPath)

	helmOpts := helmopts.HelmOptions{
		ChartLoadOpts: helmopts.ChartLoadOptions{
			ChartType: helmopts.ChartTypeChart,
		},
	}

	chart, err := loader.Load(absPath, helmOpts)
	if err != nil {
		return fmt.Errorf("load chart: %w", err)
	}

	if err = ts.BundleChartsRecursive(ctx, chart, absPath, true, opts.DenoBinaryPath); err != nil {
		return fmt.Errorf("process chart: %w", err)
	}

	bundles := lo.Filter(chart.Raw, func(file *helmchart.File, _ int) bool {
		return strings.Contains(file.Name, common.ChartTSBundleFile)
	})

	if len(bundles) == 0 {
		return nil
	}

	for _, bundle := range bundles {
		bundlePath := filepath.Join(absPath, bundle.Name)
		dirPath := filepath.Dir(bundlePath)

		if err := os.MkdirAll(dirPath, 0o775); err != nil {
			return fmt.Errorf("mkdir %q: %w", dirPath, err)
		}

		if err := os.WriteFile(bundlePath, bundle.Data, 0o644); err != nil {
			return fmt.Errorf("write bundle to file %q: %w", bundlePath, err)
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Bundled: ")+"%s - %s", bundle.Name, humanize.Bytes(uint64(len(bundle.Data))))
	}

	log.Default.Info(ctx, "TypeScript chart bundled successfully")

	return nil
}
