package action

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleasePlanExecuteLogLevel = log.InfoLevel
)

type ReleasePlanExecuteOptions struct {
	common.KubeConnectionOptions
	common.TrackingOptions

	// AutoRollback, when true, automatically rolls back to the previous deployed release on execution failure.
	// Only works if there is a previously successfully deployed release.
	AutoRollback bool
	// InstallGraphPath, if specified, saves the Graphviz representation of the installation plan to this file path.
	// Useful for debugging and visualizing the dependency graph of resource operations.
	InstallGraphPath string
	// InstallReportPath, if specified, saves a JSON report of the execution results to this file path.
	InstallReportPath string
	// NetworkParallelism limits the number of concurrent network-related operations (API calls, resource fetches).
	NetworkParallelism int
	// NoShowNotes, when true, suppresses printing of NOTES.txt after successful execution.
	NoShowNotes bool
	// NoProgressTablePrint, when true, disables real-time progress table printing during resource tracking.
	NoProgressTablePrint bool
	// PlanArtifactLifetime, specifies how long plan artifact be valid.
	PlanArtifactLifetime time.Duration
	// PlanArtifactPath is the path to the plan artifact file to execute.
	PlanArtifactPath string
	// ReleaseHistoryLimit sets the maximum number of release revisions to keep in storage.
	// When exceeded, the oldest revisions are deleted. Defaults to DefaultReleaseHistoryLimit if not set or <= 0.
	// Note: Only release metadata is deleted; actual Kubernetes resources are not affected.
	ReleaseHistoryLimit int
	// RollbackGraphPath, if specified, saves the Graphviz representation of the rollback plan to this file path.
	RollbackGraphPath string
	// SecretKey is the encryption/decryption key for the plan artifact file.
	SecretKey string
	// SecretWorkDir is the working directory for resolving relative paths in secret operations.
	SecretWorkDir string
	// ShowSubchartNotes, when true, shows NOTES.txt from subcharts in addition to the main chart's notes.
	// By default, only the parent chart's NOTES.txt is displayed.
	ShowSubchartNotes bool
	// TempDirPath is the directory for temporary files during execution.
	TempDirPath string
	// Timeout is the maximum duration for the entire release installation operation.
	// If 0, no timeout is applied and the operation runs until completion or error.
	Timeout time.Duration
}

func ReleasePlanExecute(ctx context.Context, opts ReleasePlanExecuteOptions) error {
	ctx, ctxCancelFn := context.WithCancelCause(ctx)

	if opts.Timeout == 0 {
		return releasePlanExecute(ctx, ctxCancelFn, opts)
	}

	ctx, _ = context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("context timed out: action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn(fmt.Errorf("context canceled: action finished"))

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- releasePlanExecute(ctx, ctxCancelFn, opts)
	}()

	for {
		select {
		case err := <-actionCh:
			return err
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
}

func releasePlanExecute(ctx context.Context, ctxCancelFn context.CancelCauseFunc, opts ReleasePlanExecuteOptions) error {
	if opts.SecretKey != "" {
		lo.Must0(os.Setenv("WERF_SECRET_KEY", opts.SecretKey))
	}

	log.Default.Debug(ctx, "Read plan artifact")

	planArtifact, err := plan.ReadPlanArtifact(ctx, opts.PlanArtifactPath, opts.SecretKey, opts.SecretWorkDir)
	if err != nil {
		return fmt.Errorf("read plan artifact from %s: %w", opts.PlanArtifactPath, err)
	}

	installPlan := planArtifact.GetPlan()

	releaseNamespace := planArtifact.Release.Namespace
	releaseName := planArtifact.Release.Name
	// deployType := planArtifact.GetDeployType()

	newRelease, err := planArtifact.GetRelease()
	if err != nil {
		return fmt.Errorf("get release from plan artifact: %w", err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleasePlanExecuteOptionsDefaults(opts, currentDir, homeDir)
	if err != nil {
		return fmt.Errorf("build release plan execute options: %w", err)
	}

	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		KubeConnectionOptions: opts.KubeConnectionOptions,
		KubeContextNamespace:  releaseNamespace,
	})
	if err != nil {
		return fmt.Errorf("construct kube config: %w", err)
	}

	clientFactory, err := kube.NewClientFactory(ctx, kubeConfig)
	if err != nil {
		return fmt.Errorf("construct kube client factory: %w", err)
	}

	releaseStorage, err := release.NewReleaseStorage(ctx, releaseNamespace, planArtifact.Options.ReleaseStorageDriver, clientFactory, release.ReleaseStorageOptions{
		HistoryLimit:  opts.ReleaseHistoryLimit,
		SQLConnection: planArtifact.Options.ReleaseStorageSQLConnection,
	})
	if err != nil {
		return fmt.Errorf("construct release storage: %w", err)
	}

	var lockManager *lock.LockManager
	if m, err := lock.NewLockManager(ctx, releaseNamespace, false, clientFactory); err != nil {
		return fmt.Errorf("construct lock manager: %w", err)
	} else {
		lockManager = m
	}

	if err := createReleaseNamespace(ctx, clientFactory, releaseNamespace); err != nil {
		return fmt.Errorf("create release namespace: %w", err)
	}

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Start release")+" %q (namespace: %q)", releaseName, releaseNamespace)

	if lock, err := lockManager.LockRelease(ctx, releaseName); err != nil {
		return fmt.Errorf("lock release: %w", err)
	} else {
		defer func() {
			_ = lockManager.Unlock(lock)
		}()
	}

	log.Default.Debug(ctx, "Build release history")

	history, err := release.BuildHistory(releaseName, releaseStorage, release.HistoryOptions{})
	if err != nil {
		return fmt.Errorf("build release history: %w", err)
	}

	releases := history.Releases()
	deployedReleases := history.FindAllDeployed()
	prevRelease := lo.LastOrEmpty(releases)
	prevDeployedRelease := lo.LastOrEmpty(deployedReleases)

	newRevision := 1

	if prevRelease != nil {
		newRevision = prevRelease.Version + 1
	}

	instResInfos := planArtifact.GetInstallableResourceInfos()
	relInfos := planArtifact.GetReleaseInfos()

	log.Default.Debug(ctx, "Validate plan artifact")

	if err := plan.ValidatePlanArtifact(planArtifact, newRevision, opts.PlanArtifactLifetime); err != nil {
		return fmt.Errorf("validate plan artifact: %w", err)
	}

	return runReleaseInstallPlan(ctx, ctxCancelFn, releaseName, releaseNamespace, installPlan, prevRelease, newRelease, clientFactory, history, instResInfos, relInfos, prevDeployedRelease, runReleaseInstallPlanOptions{
		TrackingOptions:           opts.TrackingOptions,
		ResourceValidationOptions: planArtifact.Options.ResourceValidationOptions,
		AutoRollback:              opts.AutoRollback,
		DefaultDeletePropagation:  planArtifact.Options.DefaultDeletePropagation,
		ExtraAnnotations:          planArtifact.Options.ExtraAnnotations,
		ExtraLabels:               planArtifact.Options.ExtraLabels,
		ExtraRuntimeAnnotations:   planArtifact.Options.ExtraRuntimeAnnotations,
		ExtraRuntimeLabels:        planArtifact.Options.ExtraRuntimeLabels,
		ForceAdoption:             planArtifact.Options.ForceAdoption,
		InstallGraphPath:          opts.InstallGraphPath,
		InstallReportPath:         opts.InstallReportPath,
		NoRemoveManualChanges:     planArtifact.Options.NoRemoveManualChanges,
		NoShowNotes:               opts.NoShowNotes,
		NoProgressTablePrint:      opts.NoProgressTablePrint,
		NetworkParallelism:        opts.NetworkParallelism,
		ReleaseInfoAnnotations:    planArtifact.Options.ReleaseInfoAnnotations,
		ReleaseLabels:             planArtifact.Options.ReleaseLabels,
		RollbackGraphPath:         opts.RollbackGraphPath,
	})
}

func applyReleasePlanExecuteOptionsDefaults(opts ReleasePlanExecuteOptions, currentDir, homeDir string) (ReleasePlanExecuteOptions, error) {
	var err error

	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleasePlanExecuteOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)
	opts.TrackingOptions.ApplyDefaults()

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = common.DefaultNetworkParallelism
	}

	if opts.SecretWorkDir == "" {
		opts.SecretWorkDir = currentDir
	}

	return opts, nil
}
