package action

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/helm/intern/chart/v3/util"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/ts"
)

type ChartTSInitOptions struct {
	ChartDirPath      string
	ChartName         string
	DenoBinaryPath    string
	RenderContextType string
	TempDirPath       string
}

func ChartTSInit(ctx context.Context, opts ChartTSInitOptions) error {
	chartPath := opts.ChartDirPath
	if chartPath == "" {
		chartPath = "."
	}

	absPath, err := filepath.Abs(chartPath)
	if err != nil {
		return fmt.Errorf("get absolute path: %w", err)
	}

	var chartName string
	if opts.ChartName != "" {
		chartName = opts.ChartName
	} else {
		meta, err := util.LoadChartfile(filepath.Join(absPath, "Chart.yaml"))
		if err != nil {
			return fmt.Errorf("load Chart.yaml: %w", err)
		}

		if meta.Name == "" {
			return errors.New("name must not be empty in Chart.yaml")
		}

		chartName = meta.Name
	}

	if !featgate.FeatGateTypescript.Enabled() {
		log.Default.Warn(ctx, "TypeScript charts require NELM_FEAT_TYPESCRIPT=true environment variable")

		return fmt.Errorf("TypeScript charts feature is not enabled. Set NELM_FEAT_TYPESCRIPT=true to use this feature")
	}

	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", absPath, err)
	}

	if err := ts.InitChartStructure(ctx, absPath, chartName); err != nil {
		return fmt.Errorf("init chart structure: %w", err)
	}

	if err := ts.InitTSBoilerplate(ctx, absPath, chartName, ts.InitTSBoilerplateOptions{
		RenderContextType: opts.RenderContextType,
	}); err != nil {
		return fmt.Errorf("init TypeScript boilerplate: %w", err)
	}

	if err := ts.EnsureGitignore(absPath); err != nil {
		return fmt.Errorf("ensure .gitignore: %w", err)
	}

	if err := ts.RunDenoInstall(ctx, absPath, opts.DenoBinaryPath); err != nil {
		return fmt.Errorf("run deno install: %w", err)
	}

	log.Default.Info(ctx, "Initialized TypeScript files in %s", absPath)

	return nil
}
