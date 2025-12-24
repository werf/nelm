package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/track"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleaseRollbackLogLevel = log.InfoLevel
)

type ReleaseRollbackOptions struct {
	common.KubeConnectionOptions
	common.TrackingOptions

	// DefaultDeletePropagation sets the deletion propagation policy for resource deletions.
	DefaultDeletePropagation string
	// ExtraRuntimeAnnotations are additional annotations to add to resources at runtime during rollback.
	// These are added during resource creation/update but not stored in the release.
	ExtraRuntimeAnnotations map[string]string
	// ExtraRuntimeLabels are additional labels to add to resources at runtime during rollback.
	// These are added during resource creation/update but not stored in the release.
	ExtraRuntimeLabels map[string]string
	// ForceAdoption, when true, allows adopting resources that belong to a different Helm release.
	// WARNING: This can lead to conflicts if resources are managed by multiple releases.
	ForceAdoption bool
	// NetworkParallelism limits the number of concurrent network-related operations (API calls, resource fetches).
	// Defaults to DefaultNetworkParallelism if not set or <= 0.
	NetworkParallelism int
	// NoRemoveManualChanges, when true, preserves fields manually added to resources in the cluster
	// that are not present in the chart manifests. By default, such fields are removed during rollback.
	NoRemoveManualChanges bool
	// NoShowNotes, when true, suppresses printing of NOTES.txt after successful rollback.
	// NOTES.txt typically contains usage instructions and next steps.
	NoShowNotes bool
	// ReleaseHistoryLimit sets the maximum number of release revisions to keep in storage.
	// When exceeded, the oldest revisions are deleted. Defaults to DefaultReleaseHistoryLimit if not set or <= 0.
	// Note: Only release metadata is deleted; actual Kubernetes resources are not affected.
	ReleaseHistoryLimit int
	// ReleaseInfoAnnotations are custom annotations to add to the new rollback release metadata (stored in Secret/ConfigMap).
	// These do not affect resources but can be used for tagging releases.
	ReleaseInfoAnnotations map[string]string
	// ReleaseLabels are labels to add to the new rollback release storage object (Secret/ConfigMap).
	// Used for filtering and organizing releases in storage.
	ReleaseLabels map[string]string
	// ReleaseStorageDriver specifies how release metadata is stored in Kubernetes.
	// Valid values: "secret" (default), "configmap", "sql".
	// Defaults to "secret" if not specified or set to "default".
	ReleaseStorageDriver string
	// ReleaseStorageSQLConnection is the SQL connection string when using SQL storage driver.
	// Only used when ReleaseStorageDriver is "sql".
	ReleaseStorageSQLConnection string
	// Revision specifies which release revision to roll back to.
	// If 0, rolls back to the previous deployed revision.
	Revision int
	// RollbackGraphPath, if specified, saves the Graphviz representation of the rollback plan to this file path.
	// Useful for debugging and visualizing the dependency graph of resource operations.
	RollbackGraphPath string
	// RollbackReportPath, if specified, saves a JSON report of the rollback results to this file path.
	// The report includes lists of completed, canceled, and failed operations.
	RollbackReportPath string
	// TempDirPath is the directory for temporary files during the operation.
	// A temporary directory is created automatically if not specified.
	TempDirPath string
	// Timeout is the maximum duration for the entire rollback operation.
	// If 0, no timeout is applied and the operation runs until completion or error.
	Timeout time.Duration
}

func ReleaseRollback(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseRollbackOptions) error {
	ctx, ctxCancelFn := context.WithCancelCause(ctx)

	if opts.Timeout == 0 {
		return releaseRollback(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
	}

	ctx, _ = context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("context timed out: action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn(fmt.Errorf("context canceled: action finished"))

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- releaseRollback(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
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

func releaseRollback(ctx context.Context, ctxCancelFn context.CancelCauseFunc, releaseName, releaseNamespace string, opts ReleaseRollbackOptions) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleaseRollbackOptionsDefaults(opts, homeDir)
	if err != nil {
		return fmt.Errorf("build release rollback options: %w", err)
	}

	if len(opts.KubeConfigPaths) > 0 {
		var splitPaths []string
		for _, path := range opts.KubeConfigPaths {
			splitPaths = append(splitPaths, filepath.SplitList(path)...)
		}

		opts.KubeConfigPaths = lo.Compact(splitPaths)
	}

	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		KubeConnectionOptions: opts.KubeConnectionOptions,
		KubeContextNamespace:  releaseNamespace, // TODO: unset it everywhere
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

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Start rollback of release")+" %q (namespace: %q)", releaseName, releaseNamespace)

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
	if len(releases) == 0 {
		return fmt.Errorf("not found release %q (namespace: %q)", releaseName, releaseNamespace)
	}

	deployedReleases := history.FindAllDeployed()
	prevRelease := lo.LastOrEmpty(releases)
	prevDeployedRelease := lo.LastOrEmpty(deployedReleases)

	var rollbackRelease *helmrelease.Release
	if opts.Revision == 0 {
		if len(deployedReleases) == 0 {
			return fmt.Errorf("not found successfully deployed release %q (namespace: %q)", releaseName, releaseNamespace)
		}

		if prevDeployedRelease.Version != prevRelease.Version {
			rollbackRelease = prevDeployedRelease
		} else {
			if len(deployedReleases) < 2 {
				return fmt.Errorf("not found successfully deployed (except last) release %q (namespace: %q)", releaseName, releaseNamespace)
			}

			rollbackRelease = deployedReleases[len(deployedReleases)-2]
		}
	} else {
		var found bool

		rollbackRelease, found = lo.Find(releases, func(rel *helmrelease.Release) bool {
			return rel.Version == opts.Revision
		})
		if !found {
			return fmt.Errorf("not found revision %d for release %q (namespace: %q)", opts.Revision, releaseName, releaseNamespace)
		}
	}

	var (
		newRevision       int
		prevReleaseFailed bool
	)

	if prevRelease != nil {
		newRevision = prevRelease.Version + 1
		prevReleaseFailed = prevRelease.IsStatusFailed()
	} else {
		newRevision = 1
	}

	deployType := common.DeployTypeRollback

	log.Default.Debug(ctx, "Convert release to resource specs")

	rollbackReleaseResSpecs, err := release.ReleaseToResourceSpecs(rollbackRelease, releaseNamespace, false)
	if err != nil {
		return fmt.Errorf("convert release to rollback to resource specs: %w", err)
	}

	newRelease, err := release.NewRelease(releaseName, releaseNamespace, newRevision, deployType, rollbackReleaseResSpecs, rollbackRelease.Chart, rollbackRelease.Config, release.ReleaseOptions{
		InfoAnnotations: lo.Assign(rollbackRelease.Info.Annotations, opts.ReleaseInfoAnnotations),
		Labels:          lo.Assign(rollbackRelease.Labels, opts.ReleaseLabels),
		Notes:           rollbackRelease.Info.Notes,
	})
	if err != nil {
		return fmt.Errorf("construct new release: %w", err)
	}

	log.Default.Debug(ctx, "Convert previous release to resource specs")

	prevRelResSpecs, err := release.ReleaseToResourceSpecs(prevRelease, releaseNamespace, false)
	if err != nil {
		return fmt.Errorf("convert previous release to resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, releaseNamespace, false)
	if err != nil {
		return fmt.Errorf("convert new release to resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build resources")

	patchers := []spec.ResourcePatcher{
		spec.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
		spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
	}

	if opts.LegacyHelmCompatibleTracking {
		patchers = append(patchers, spec.NewLegacyOnlyTrackJobsPatcher())
	}

	instResources, delResources, err := resource.BuildResources(ctx, deployType, releaseNamespace, prevRelResSpecs, newRelResSpecs, patchers, clientFactory, resource.BuildResourcesOptions{
		Remote:                   true,
		DefaultDeletePropagation: metav1.DeletionPropagation(opts.DefaultDeletePropagation),
	})
	if err != nil {
		return fmt.Errorf("build resources: %w", err)
	}

	log.Default.Debug(ctx, "Locally validate resources")

	if err := resource.ValidateLocal(releaseNamespace, instResources); err != nil {
		return fmt.Errorf("locally validate resources: %w", err)
	}

	log.Default.Debug(ctx, "Build resource infos")

	instResInfos, delResInfos, err := plan.BuildResourceInfos(ctx, deployType, releaseName, releaseNamespace, instResources, delResources, prevReleaseFailed, clientFactory, plan.BuildResourceInfosOptions{
		NetworkParallelism:    opts.NetworkParallelism,
		NoRemoveManualChanges: opts.NoRemoveManualChanges,
	})
	if err != nil {
		return fmt.Errorf("build resource infos: %w", err)
	}

	log.Default.Debug(ctx, "Remotely validate resources")

	if err := plan.ValidateRemote(releaseName, releaseNamespace, instResInfos, opts.ForceAdoption); err != nil {
		return fmt.Errorf("remotely validate resources: %w", err)
	}

	log.Default.Debug(ctx, "Build release infos")

	relInfos, err := plan.BuildReleaseInfos(ctx, deployType, releases, newRelease)
	if err != nil {
		return fmt.Errorf("build release infos: %w", err)
	}

	log.Default.Debug(ctx, "Build install plan")

	installPlan, err := plan.BuildPlan(instResInfos, delResInfos, relInfos, plan.BuildPlanOptions{
		NoFinalTracking: opts.NoFinalTracking,
	})
	if err != nil {
		handleBuildPlanErr(ctx, installPlan, err, opts.RollbackGraphPath, opts.TempDirPath, "release-rollback-graph.dot")
		return fmt.Errorf("build install plan: %w", err)
	}

	if opts.RollbackGraphPath != "" {
		if err := savePlanAsDot(installPlan, opts.RollbackGraphPath); err != nil {
			return fmt.Errorf("save release install graph: %w", err)
		}
	}

	releaseIsUpToDate, err := release.IsReleaseUpToDate(prevRelease, newRelease)
	if err != nil {
		return fmt.Errorf("check if release is up to date: %w", err)
	}

	installPlanIsUseless := lo.NoneBy(installPlan.Operations(), func(op *plan.Operation) bool {
		switch op.Category {
		case plan.OperationCategoryResource, plan.OperationCategoryTrack:
			return true
		default:
			return false
		}
	})

	if releaseIsUpToDate && installPlanIsUseless {
		if opts.RollbackReportPath != "" {
			if err := saveReport(opts.RollbackReportPath, &releaseReportV3{
				Version:   3,
				Release:   releaseName,
				Namespace: releaseNamespace,
				Revision:  newRelease.Version,
				Status:    helmrelease.StatusSkipped,
			}); err != nil {
				return fmt.Errorf("save release install report: %w", err)
			}
		}

		if !opts.NoShowNotes {
			printNotes(ctx, newRelease.Info.Notes)
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped rollback of release %q (namespace: %q): cluster resources already as desired", releaseName, releaseNamespace)))

		return nil
	}

	taskStore := kdutil.NewConcurrent(statestore.NewTaskStore())
	logStore := kdutil.NewConcurrent(logstore.NewLogStore())
	watchErrCh := make(chan error, 1)
	informerFactory := informer.NewConcurrentInformerFactory(ctx.Done(), watchErrCh, clientFactory.Dynamic(), informer.ConcurrentInformerFactoryOptions{})

	log.Default.Debug(ctx, "Start tracking")

	go func() {
		if err := <-watchErrCh; err != nil {
			ctxCancelFn(fmt.Errorf("context canceled: watch error: %w", err))
		}
	}()

	var progressPrinter *track.ProgressTablesPrinter
	if !opts.NoProgressTablePrint {
		progressPrinter = track.NewProgressTablesPrinter(taskStore, logStore, track.ProgressTablesPrinterOptions{
			DefaultNamespace: releaseNamespace,
		})
		progressPrinter.Start(ctx, opts.ProgressTablePrintInterval)
	}

	var criticalErrs, nonCriticalErrs []error

	log.Default.Debug(ctx, "Execute release install plan")

	executePlanErr := plan.ExecutePlan(ctx, releaseNamespace, installPlan, taskStore, logStore, informerFactory, history, clientFactory, plan.ExecutePlanOptions{
		TrackingOptions:    opts.TrackingOptions,
		NetworkParallelism: opts.NetworkParallelism,
	})
	if executePlanErr != nil {
		criticalErrs = append(criticalErrs, fmt.Errorf("execute release install plan: %w", executePlanErr))
	}

	resourceOps := lo.Filter(installPlan.Operations(), func(op *plan.Operation, _ int) bool {
		return op.Category == plan.OperationCategoryResource
	})

	completedResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusCompleted
	})

	canceledResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusPending || op.Status == plan.OperationStatusUnknown
	})

	failedResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusFailed
	})

	if executePlanErr != nil {
		runFailurePlanResult, nonCritErrs, critErrs := runFailurePlan(ctx, releaseNamespace, installPlan, instResInfos, relInfos, taskStore, logStore, informerFactory, history, clientFactory, runFailureInstallPlanOptions{
			TrackingOptions:    opts.TrackingOptions,
			NetworkParallelism: opts.NetworkParallelism,
		})

		criticalErrs = append(criticalErrs, critErrs...)
		nonCriticalErrs = append(nonCriticalErrs, nonCritErrs...)

		if runFailurePlanResult != nil {
			completedResourceOps = append(completedResourceOps, runFailurePlanResult.CompletedResourceOps...)
			canceledResourceOps = append(canceledResourceOps, runFailurePlanResult.CanceledResourceOps...)
			failedResourceOps = append(failedResourceOps, runFailurePlanResult.FailedResourceOps...)
		}
	}

	if !opts.NoProgressTablePrint {
		progressPrinter.Stop()
		progressPrinter.Wait()
	}

	reportCompletedOps := lo.Map(completedResourceOps, func(op *plan.Operation, _ int) string {
		return op.IDHuman()
	})

	reportCanceledOps := lo.Map(canceledResourceOps, func(op *plan.Operation, _ int) string {
		return op.IDHuman()
	})

	reportFailedOps := lo.Map(failedResourceOps, func(op *plan.Operation, _ int) string {
		return op.IDHuman()
	})

	sort.Strings(reportCompletedOps)
	sort.Strings(reportCanceledOps)
	sort.Strings(reportFailedOps)

	report := &releaseReportV3{
		Version:             3,
		Release:             releaseName,
		Namespace:           releaseNamespace,
		Revision:            newRelease.Version,
		Status:              helmrelease.StatusDeployed,
		CompletedOperations: reportCompletedOps,
		CanceledOperations:  reportCanceledOps,
		FailedOperations:    reportFailedOps,
	}

	printReport(ctx, report)

	if opts.RollbackReportPath != "" {
		if err := saveReport(opts.RollbackReportPath, report); err != nil {
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("save release install report: %w", err))
		}
	}

	if len(criticalErrs) == 0 && !opts.NoShowNotes {
		printNotes(ctx, newRelease.Info.Notes)
	}

	if len(criticalErrs) > 0 {
		return util.Multierrorf("failed rollback of release %q (namespace: %q)", append(criticalErrs, nonCriticalErrs...), releaseName, releaseNamespace)
	} else if len(nonCriticalErrs) > 0 {
		return util.Multierrorf("succeeded rollback of release %q (namespace: %q), but non-critical errors encountered", nonCriticalErrs, releaseName, releaseNamespace)
	} else {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Succeeded rollback of release %q (namespace: %q)", releaseName, releaseNamespace)))

		return nil
	}
}

func applyReleaseRollbackOptionsDefaults(opts ReleaseRollbackOptions, homeDir string) (ReleaseRollbackOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseRollbackOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)
	opts.TrackingOptions.ApplyDefaults()

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = common.DefaultNetworkParallelism
	}

	if opts.ProgressTablePrintInterval <= 0 {
		opts.ProgressTablePrintInterval = common.DefaultProgressPrintInterval
	}

	if opts.ReleaseHistoryLimit <= 0 {
		opts.ReleaseHistoryLimit = common.DefaultReleaseHistoryLimit
	}

	switch opts.ReleaseStorageDriver {
	case common.ReleaseStorageDriverDefault:
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
	case common.ReleaseStorageDriverMemory:
		return ReleaseRollbackOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	if opts.DefaultDeletePropagation == "" {
		opts.DefaultDeletePropagation = string(common.DefaultDeletePropagation)
	}

	return opts, nil
}
