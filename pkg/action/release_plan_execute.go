package action

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleasePlanExecuteLogLevel = log.InfoLevel
)

type ReleasePlanExecuteOptions struct {
	common.KubeConnectionOptions
	common.ResourceValidationOptions
	common.TrackingOptions

	// AutoRollback, when true, automatically rolls back to the previous deployed release on execution failure.
	// Only works if there is a previously successfully deployed release.
	AutoRollback bool
	// ExtraAnnotations are additional Kubernetes annotations to add to all chart resources during rollback.
	ExtraAnnotations map[string]string
	// ExtraLabels are additional Kubernetes labels to add to all chart resources during rollback.
	ExtraLabels map[string]string
	// ExtraRuntimeAnnotations are additional annotations to add to resources at runtime during rollback.
	ExtraRuntimeAnnotations map[string]string
	// ExtraRuntimeLabels are additional labels to add to resources at runtime during rollback.
	ExtraRuntimeLabels map[string]string
	// ForceAdoption, when true, allows adopting resources that belong to a different Helm release during rollback.
	ForceAdoption bool
	// InstallGraphPath, if specified, saves the Graphviz representation of the install plan to this file path.
	// Useful for debugging and visualizing the dependency graph of resource operations.
	InstallGraphPath string
	// InstallReportPath, if specified, saves a JSON report of the execution results to this file path.
	InstallReportPath string
	// NetworkParallelism limits the number of concurrent network-related operations (API calls, resource fetches).
	NetworkParallelism int
	// NoInstallStandaloneCRDs, when true, skips installation of CustomResourceDefinitions from the "crds/" directory.
	// By default, CRDs are installed first before other chart resources.
	NoInstallStandaloneCRDs bool
	// NoRemoveManualChanges, when true, preserves fields manually added to resources in the cluster during rollback.
	NoRemoveManualChanges bool
	// NoShowNotes, when true, suppresses printing of NOTES.txt after successful execution.
	NoShowNotes bool
	// NoProgressTablePrint, when true, disables real-time progress table printing during resource tracking.
	NoProgressTablePrint bool
	// PlanArtifactPath is the path to the plan artifact file to execute.
	PlanArtifactPath string
	// ReleaseHistoryLimit is the maximum number of release revisions to keep in storage.
	ReleaseHistoryLimit int
	// ReleaseInfoAnnotations are annotations to add to the release metadata.
	ReleaseInfoAnnotations map[string]string
	// ReleaseLabels are labels to add to the release metadata.
	ReleaseLabels map[string]string
	// ReleaseStorageDriver specifies where to store release metadata ("secrets" or "sql").
	ReleaseStorageDriver string
	// ReleaseStorageSQLConnection is the SQL connection string when using SQL storage driver.
	ReleaseStorageSQLConnection string
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

	planArtifact, err := plan.ReadInstallPlanArtifact(ctx, opts.PlanArtifactPath, opts.SecretKey, opts.SecretWorkDir)
	if err != nil {
		return fmt.Errorf("read plan artifact from %s: %w", opts.PlanArtifactPath, err)
	}

	installPlan, err := planArtifact.GetInstallPlan()
	if err != nil {
		return fmt.Errorf("get install plan from plan artifact: %w", err)
	}

	releaseNamespace := planArtifact.Release.Namespace
	releaseName := planArtifact.Release.Name
	deployType := planArtifact.GetDeployType()

	resourceSpecs, err := planArtifact.GetResourceSpecs()
	if err != nil {
		return fmt.Errorf("get resource specs from plan artifact: %w", err)
	}

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

	releaseStorage, err := release.NewReleaseStorage(ctx, releaseNamespace, opts.ReleaseStorageDriver, clientFactory, release.ReleaseStorageOptions{
		HistoryLimit:  opts.ReleaseHistoryLimit,
		SQLConnection: opts.ReleaseStorageSQLConnection,
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

	var prevReleaseFailed bool

	newRevision := 1

	if prevRelease != nil {
		newRevision = prevRelease.Version + 1
		prevReleaseFailed = prevRelease.IsStatusFailed()
	}

	log.Default.Debug(ctx, "Validate plan artifact")

	if err := plan.ValidateInstallPlanArtifact(planArtifact, newRevision); err != nil {
		return fmt.Errorf("validate plan artifact: %w", err)
	}

	log.Default.Debug(ctx, "Reconstruct installable resources from plan artifact")

	var instResources []*resource.InstallableResource

	for _, resSpec := range resourceSpecs.InstallableSpecs {
		instRes, err := resource.NewInstallableResource(resSpec, releaseNamespace, clientFactory, resource.InstallableResourceOptions{
			Remote:                   true,
			DefaultDeletePropagation: metav1.DeletionPropagation(planArtifact.DefaultDeletePropagation),
		})
		if err != nil {
			return fmt.Errorf("construct installable resource: %w", err)
		}

		instResources = append(instResources, instRes)
	}

	log.Default.Debug(ctx, "Reconstruct deletable resources from plan artifact")

	var delResources []*resource.DeletableResource

	for _, resSpec := range resourceSpecs.DeletableSpecs {
		delRes := resource.NewDeletableResource(resSpec, releaseNamespace, resource.DeletableResourceOptions{
			DefaultDeletePropagation: metav1.DeletionPropagation(planArtifact.DefaultDeletePropagation),
		})

		delResources = append(delResources, delRes)
	}

	log.Default.Debug(ctx, "Build resource infos from plan artifact")

	instResInfos, _, err := plan.BuildResourceInfos(ctx, deployType, releaseName, releaseNamespace, instResources, delResources, prevReleaseFailed, clientFactory, plan.BuildResourceInfosOptions{
		NetworkParallelism:    opts.NetworkParallelism,
		NoRemoveManualChanges: opts.NoRemoveManualChanges,
	})
	if err != nil {
		return fmt.Errorf("build resource info: %w", err)
	}

	log.Default.Debug(ctx, "Build release infos from plan artifact")

	relInfos, err := plan.BuildReleaseInfos(ctx, deployType, releases, newRelease)
	if err != nil {
		return fmt.Errorf("build release info: %w", err)
	}

	return runInstallPlan(ctx, ctxCancelFn, releaseName, releaseNamespace, installPlan, prevRelease, newRelease, clientFactory, history, instResInfos, relInfos, prevDeployedRelease, runInstallPlanOptions{
		TrackingOptions: opts.TrackingOptions,
		runRollbackPlanOptions: runRollbackPlanOptions{
			TrackingOptions:           opts.TrackingOptions,
			ResourceValidationOptions: opts.ResourceValidationOptions,
			DefaultDeletePropagation:  planArtifact.DefaultDeletePropagation,
			ExtraAnnotations:          opts.ExtraAnnotations,
			ExtraLabels:               opts.ExtraLabels,
			ExtraRuntimeAnnotations:   opts.ExtraRuntimeAnnotations,
			ExtraRuntimeLabels:        opts.ExtraRuntimeLabels,
			ForceAdoption:             opts.ForceAdoption,
			NetworkParallelism:        opts.NetworkParallelism,
			NoRemoveManualChanges:     opts.NoRemoveManualChanges,
			ReleaseInfoAnnotations:    opts.ReleaseInfoAnnotations,
			ReleaseLabels:             opts.ReleaseLabels,
			RollbackGraphPath:         opts.RollbackGraphPath,
		},
		AutoRollback:               opts.AutoRollback,
		InstallGraphPath:           opts.InstallReportPath,
		InstallReportPath:          opts.InstallReportPath,
		NoShowNotes:                opts.NoShowNotes,
		NoProgressTablePrint:       opts.NoProgressTablePrint,
		ProgressTablePrintInterval: opts.ProgressTablePrintInterval,
		NetworkParallelism:         opts.NetworkParallelism,
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

	if opts.ReleaseHistoryLimit <= 0 {
		opts.ReleaseHistoryLimit = common.DefaultReleaseHistoryLimit
	}

	switch opts.ReleaseStorageDriver {
	case common.ReleaseStorageDriverDefault:
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
	case common.ReleaseStorageDriverMemory:
		return ReleasePlanExecuteOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	if opts.SecretWorkDir == "" {
		opts.SecretWorkDir = currentDir
	}

	return opts, nil
}
