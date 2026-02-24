package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/werf/nelm/internal/ts"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

type ChartInitOptions struct {
	ChartDirPath string
	TS           bool
	TempDirPath  string
}

func ChartInit(ctx context.Context, opts ChartInitOptions) error {
	chartPath := opts.ChartDirPath
	if chartPath == "" {
		chartPath = "."
	}

	absPath, err := filepath.Abs(chartPath)
	if err != nil {
		return fmt.Errorf("get absolute path: %w", err)
	}

	chartName := filepath.Base(absPath)

	if !opts.TS {
		return fmt.Errorf("non-TypeScript chart initialization not implemented yet, use --ts flag")
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

	if err := ts.InitTSBoilerplate(ctx, absPath, chartName); err != nil {
		return fmt.Errorf("init TypeScript boilerplate: %w", err)
	}

	if err := ts.EnsureGitignore(absPath); err != nil {
		return fmt.Errorf("ensure .gitignore: %w", err)
	}

	log.Default.Info(ctx, "Initialized TypeScript chart in %s", absPath)

	return nil
}
