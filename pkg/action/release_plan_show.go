package action

import (
	"context"
	"fmt"
	"os"

	"github.com/samber/lo"

	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleasePlanShowLogLevel = log.InfoLevel
)

type ReleasePlanShowOptions struct {
	common.ResourceChangeUDiffOptions

	// PlanArtifactPath is the path to the plan artifact file to execute.
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

	log.Default.Debug(ctx, "Read plan artifact")

	planArtifact, err := plan.ReadPlanArtifact(ctx, opts.PlanArtifactPath, opts.SecretKey, opts.SecretWorkDir)
	if err != nil {
		return fmt.Errorf("read plan artifact from %s: %w", opts.PlanArtifactPath, err)
	}

	if err := logPlannedChanges(ctx, planArtifact.Release.Name, planArtifact.Release.Namespace, planArtifact.Data.Changes, opts.ResourceChangeUDiffOptions); err != nil {
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

	opts.ResourceChangeUDiffOptions.ApplyDefaults()

	return opts, nil
}
