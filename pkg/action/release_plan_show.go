package action

import (
	"context"
	"fmt"
	"os"

	"github.com/samber/lo"

	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/pkg/log"
)

type ReleasePlanShowOptions struct {
	plan.CalculatePlannedChangesOptions

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
	if opts.SecretKey != "" {
		lo.Must0(os.Setenv("WERF_SECRET_KEY", opts.SecretKey))
	}

	log.Default.Debug(ctx, "Read plan artifact")

	planArtifact, err := plan.ReadPlanArtifact(ctx, opts.PlanArtifactPath, opts.SecretKey, opts.SecretWorkDir)
	if err != nil {
		return fmt.Errorf("read plan artifact from %s: %w", opts.PlanArtifactPath, err)
	}

	if err := logPlannedChanges(ctx, planArtifact.Release.Name, planArtifact.Release.Namespace, planArtifact.GetChanges(), plan.CalculatePlannedChangesOptions{
		DiffContextLines:       opts.DiffContextLines,
		ShowVerboseCRDDiffs:    opts.ShowVerboseCRDDiffs,
		ShowVerboseDiffs:       opts.ShowVerboseDiffs,
		ShowSensitiveDiffs:     opts.ShowSensitiveDiffs,
		ShowInsignificantDiffs: opts.ShowInsignificantDiffs,
	}); err != nil {
		return fmt.Errorf("log planned changes: %w", err)
	}

	return nil
}
