package action

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/werf/nelm/internal/ts"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

type ChartTSBuildOptions struct {
	ChartDirPath string
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

	if err := ts.BuildBundleToFile(ctx, absPath); err != nil {
		return fmt.Errorf("build TypeScript bundle: %w", err)
	}

	log.Default.Info(ctx, "TypeScript chart bundled in %s", absPath)

	return nil
}
