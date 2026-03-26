package action

import (
	"context"
	"fmt"
	"os"

	"github.com/samber/lo"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/plan"
)

const DefaultReleasePlanShowLogLevel = log.InfoLevel

type ReleasePlanShowOptions struct {
	common.ResourceDiffOptions

	// LegacyPlanArtifact provides plan artifact to review changes.
	LegacyPlanArtifact *plan.Artifact
	// PlanArtifactPath is the path to the plan artifact file to review changes.
	PlanArtifactPath string
	// SecretKey is the encryption/decryption key for the plan artifact file.
	SecretKey string
	// SecretWorkDir is the working directory for resolving relative paths in secret operations.
	SecretWorkDir string
	// TempDirPath is the directory for temporary files during execution.
	TempDirPath string
}

func ReleasePlanShow(ctx context.Context, opts ReleasePlanShowOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	opts, err = applyReleasePlanShowOptionsDefaults(opts, currentDir)
	if err != nil {
		return fmt.Errorf("build release plan show options: %w", err)
	}

	if opts.SecretKey != "" {
		lo.Must0(os.Setenv("WERF_SECRET_KEY", opts.SecretKey))
	}

	var planArtifact *plan.Artifact

	if opts.LegacyPlanArtifact != nil {
		planArtifact = opts.LegacyPlanArtifact
	} else {
		var err error

		log.Default.Debug(ctx, "Read plan artifact")

		planArtifact, err = plan.ReadArtifact(ctx, opts.PlanArtifactPath, opts.SecretKey, opts.SecretWorkDir)
		if err != nil {
			return fmt.Errorf("read plan artifact from %s: %w", opts.PlanArtifactPath, err)
		}
	}

	if err := logPlannedChanges(ctx, planArtifact, opts.ResourceDiffOptions); err != nil {
		return fmt.Errorf("log planned changes: %w", err)
	}

	return nil
}

func applyReleasePlanShowOptionsDefaults(opts ReleasePlanShowOptions, currentDir string) (ReleasePlanShowOptions, error) {
	var err error

	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleasePlanShowOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.SecretWorkDir == "" {
		opts.SecretWorkDir = currentDir
	}

	opts.ResourceDiffOptions.ApplyDefaults()

	return opts, nil
}
