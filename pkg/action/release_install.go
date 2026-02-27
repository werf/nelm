package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/3p-helm/pkg/registry"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/track"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/legacy/progrep"
	"github.com/werf/nelm/pkg/log"
)

const DefaultReleaseInstallLogLevel = log.InfoLevel

type ReleaseInstallOptions struct {
	common.ChartRepoConnectionOptions
	common.KubeConnectionOptions
	common.ReleaseInstallRuntimeOptions
	common.SecretValuesOptions
	common.TrackingOptions
	common.ValuesOptions

	// AutoRollback, when true, automatically rolls back to the previous deployed release on installation failure.
	// Only works if there is a previously successfully deployed release.
	AutoRollback bool
	// Chart specifies the chart to install. Can be a local directory path, chart archive,
	// OCI registry URL (oci://registry/chart), or chart repository reference (repo/chart).
	// Defaults to current directory if not specified.
	Chart string
	// ChartAppVersion overrides the appVersion field in Chart.yaml.
	// Used to set application version metadata without modifying the chart file.
	ChartAppVersion string
	// ChartDirPath is deprecated
	ChartDirPath string // TODO(major): get rid
	// ChartProvenanceKeyring is the path to a keyring file containing public keys
	// used to verify chart provenance signatures. Used with signed charts for security.
	ChartProvenanceKeyring string
	// ChartProvenanceStrategy defines how to verify chart provenance.
	// Defaults to DefaultChartProvenanceStrategy if not set.
	ChartProvenanceStrategy string
	// ChartRepoSkipUpdate, when true, skips updating the chart repository cache before fetching the chart.
	// Useful for offline operations or when repository is known to be up-to-date.
	ChartRepoSkipUpdate bool
	// ChartVersion specifies the version of the chart to install (e.g., "1.2.3").
	// If not specified, the latest version is used.
	ChartVersion string
	// DefaultChartAPIVersion sets the default Chart API version when Chart.yaml doesn't specify one.
	DefaultChartAPIVersion string
	// DefaultChartName sets the default chart name when Chart.yaml doesn't specify one.
	DefaultChartName string
	// DefaultChartVersion sets the default chart version when Chart.yaml doesn't specify one.
	DefaultChartVersion string
	// DenoBinaryPath, if specified, uses this path as the Deno binary instead of auto-downloading.
	DenoBinaryPath string
	// InstallGraphPath, if specified, saves the Graphviz representation of the install plan to this file path.
	// Useful for debugging and visualizing the dependency graph of resource operations.
	InstallGraphPath string
	// InstallReportPath, if specified, saves a JSON report of the installation results to this file path.
	// The report includes the release status and lists of completed, canceled, and failed operations.
	InstallReportPath string
	// LegacyChartType specifies the chart type for legacy compatibility.
	// Used internally for backward compatibility with werf integration.
	LegacyChartType helmopts.ChartType
	// LegacyExtraValues provides additional values programmatically.
	// Used internally for backward compatibility with werf integration.
	LegacyExtraValues map[string]interface{}
	// LegacyLogRegistryStreamOut is the output writer for Helm registry client logs.
	// Defaults to io.Discard if not set. Used for debugging registry operations.
	LegacyLogRegistryStreamOut io.Writer
	// LegacyProgressReportCh, when non-nil, receives ProgressReport snapshots during deployment.
	// Must be a buffered channel with capacity >= 1. The caller owns the channel and is responsible
	// for its lifecycle. Intermediate reports may be dropped if the consumer is slow; the final
	// report is guaranteed (blocking send). ReleaseInstall does not close this channel.
	LegacyProgressReportCh chan<- progrep.ProgressReport
	// NetworkParallelism limits the number of concurrent network-related operations (API calls, resource fetches).
	// Defaults to DefaultNetworkParallelism if not set or <= 0.
	NetworkParallelism int
	// NoShowNotes, when true, suppresses printing of NOTES.txt after successful installation.
	// NOTES.txt typically contains usage instructions and next steps.
	NoShowNotes bool
	// PlanArtifactLifetime, specifies how long plan artifact be valid.
	PlanArtifactLifetime time.Duration
	// PlanArtifactPath, if specified, saves the install plan artifact to this file path.
	PlanArtifactPath string
	// RebuildTSBundle, when true, forces rebuilding the Deno bundle even if it already exists.
	RebuildTSBundle bool
	// RegistryCredentialsPath is the path to Docker config.json file with registry credentials.
	// Defaults to DefaultRegistryCredentialsPath (~/.docker/config.json) if not set.
	// Used for authenticating to OCI registries when pulling charts.
	RegistryCredentialsPath string
	// RollbackGraphPath, if specified, saves the Graphviz representation of the rollback plan (if auto-rollback occurs)
	// to this file path. Only used when AutoRollback is true and rollback is triggered.
	RollbackGraphPath string
	// ShowSubchartNotes, when true, shows NOTES.txt from subcharts in addition to the main chart's notes.
	// By default, only the parent chart's NOTES.txt is displayed.
	ShowSubchartNotes bool
	// TempDirPath is the directory for temporary files during the operation.
	// A temporary directory is created automatically if not specified.
	TempDirPath string
	// TemplatesAllowDNS, when true, enables DNS lookups in chart templates using template functions.
	// WARNING: This can make template rendering non-deterministic and slower.
	TemplatesAllowDNS bool
	// Timeout is the maximum duration for the entire release installation operation.
	// If 0, no timeout is applied and the operation runs until completion or error.
	Timeout time.Duration
}

type runRollbackPlanOptions struct {
	common.ReleaseInstallRuntimeOptions
	common.TrackingOptions

	LegacyProgressReporter *plan.LegacyProgressReporter
	NetworkParallelism     int
	RollbackGraphPath      string
}

type runRollbackPlanResult struct {
	CanceledResourceOps  []*plan.Operation
	CompletedResourceOps []*plan.Operation
	FailedResourceOps    []*plan.Operation
}

func ReleaseInstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseInstallOptions) error {
	ctx, ctxCancelFn := context.WithCancelCause(ctx)

	if opts.Timeout == 0 {
		return releaseInstall(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
	}

	ctx, _ = context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("context timed out: action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn(fmt.Errorf("context canceled: action finished"))

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- releaseInstall(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
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

func releaseInstall(ctx context.Context, ctxCancelFn context.CancelCauseFunc, releaseName, releaseNamespace string, opts ReleaseInstallOptions) error {
	usePlan := opts.PlanArtifactPath != ""

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleaseInstallOptionsDefaults(opts, currentDir, homeDir)
	if err != nil {
		return fmt.Errorf("build release install options: %w", err)
	}

	if opts.SecretKey != "" {
		lo.Must0(os.Setenv("WERF_SECRET_KEY", opts.SecretKey))
	}

	var planArtifact *plan.PlanArtifact
	if usePlan {
		log.Default.Info(ctx, "Using %s plan artifact", opts.PlanArtifactPath)

		log.Default.Debug(ctx, "Read plan artifact")

		planArtifact, err = plan.ReadPlanArtifact(ctx, opts.PlanArtifactPath, opts.SecretKey, opts.SecretWorkDir)
		if err != nil {
			return fmt.Errorf("read plan artifact from %s: %w", opts.PlanArtifactPath, err)
		}

		log.Default.Debug(ctx, "Validate plan artifact")

		if err := plan.ValidatePlanArtifact(planArtifact, opts.PlanArtifactLifetime); err != nil {
			return fmt.Errorf("validate plan artifact: %w", err)
		}

		releaseNamespace = planArtifact.Release.Namespace
		releaseName = planArtifact.Release.Name

		opts.ReleaseInstallRuntimeOptions = planArtifact.Data.Options
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

	var helmRegistryClient *registry.Client
	if !usePlan {
		helmRegistryClientOpts := []registry.ClientOption{
			registry.ClientOptDebug(log.Default.AcceptLevel(ctx, log.DebugLevel)), // TODO(log):
			registry.ClientOptWriter(opts.LegacyLogRegistryStreamOut),
			registry.ClientOptCredentialsFile(opts.RegistryCredentialsPath),
		}

		if opts.ChartRepoInsecure {
			helmRegistryClientOpts = append(
				helmRegistryClientOpts,
				registry.ClientOptPlainHTTP(),
			)
		}

		helmRegistryClient, err = registry.NewClient(helmRegistryClientOpts...)
		if err != nil {
			return fmt.Errorf("construct registry client: %w", err)
		}
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

	newRevision := 1
	if prevRelease != nil {
		newRevision = prevRelease.Version + 1
	}

	var (
		installPlan  *plan.Plan
		newRelease   *helmrelease.Release
		instResInfos []*plan.InstallableResourceInfo
		relInfos     []*plan.ReleaseInfo
	)

	if usePlan {
		if planArtifact.Release.Revision != newRevision {
			return fmt.Errorf("plan artifact release revision mismatch: expected %d, got %d",
				planArtifact.Release.Revision, newRevision)
		}

		installPlan = planArtifact.Data.Plan
		newRelease = planArtifact.Data.Release
		instResInfos = planArtifact.Data.InstallableResourceInfos
		relInfos = planArtifact.Data.ReleaseInfos
	} else {
		prevReleaseFailed := prevRelease != nil && prevRelease.IsStatusFailed()

		var deployType common.DeployType
		if prevDeployedRelease != nil {
			deployType = common.DeployTypeUpgrade
		} else if prevRelease != nil {
			deployType = common.DeployTypeInstall
		} else {
			deployType = common.DeployTypeInitial
		}

		helmOptions := helmopts.HelmOptions{
			ChartLoadOpts: helmopts.ChartLoadOptions{
				ChartAppVersion:            opts.ChartAppVersion,
				ChartType:                  opts.LegacyChartType,
				DefaultChartAPIVersion:     opts.DefaultChartAPIVersion,
				DefaultChartName:           opts.DefaultChartName,
				DefaultChartVersion:        opts.DefaultChartVersion,
				DefaultSecretValuesDisable: opts.DefaultSecretValuesDisable,
				DefaultValuesDisable:       opts.DefaultValuesDisable,
				ExtraValues:                opts.LegacyExtraValues,
				SecretKeyIgnore:            opts.SecretKeyIgnore,
				SecretValuesFiles:          opts.SecretValuesFiles,
				SecretWorkDir:              opts.SecretWorkDir,
			},
		}

		log.Default.Debug(ctx, "Render chart")

		renderChartResult, err := chart.RenderChart(ctx, opts.Chart, releaseName, releaseNamespace, newRevision, deployType, helmRegistryClient, clientFactory, chart.RenderChartOptions{
			ChartRepoConnectionOptions: opts.ChartRepoConnectionOptions,
			ValuesOptions:              opts.ValuesOptions,
			ChartProvenanceKeyring:     opts.ChartProvenanceKeyring,
			ChartProvenanceStrategy:    opts.ChartProvenanceStrategy,
			ChartRepoNoUpdate:          opts.ChartRepoSkipUpdate,
			ChartVersion:               opts.ChartVersion,
			HelmOptions:                helmOptions,
			NoStandaloneCRDs:           opts.NoInstallStandaloneCRDs,
			Remote:                     true,
			SubchartNotes:              opts.ShowSubchartNotes,
			TemplatesAllowDNS:          opts.TemplatesAllowDNS,
			RebuildTSBundle:            opts.RebuildTSBundle,
			DenoBinaryPath:             opts.DenoBinaryPath,
			TempDirPath:                opts.TempDirPath,
		})
		if err != nil {
			return fmt.Errorf("render chart: %w", err)
		}

		log.Default.Debug(ctx, "Build transformed resource specs")

		transformedResSpecs, err := spec.BuildTransformedResourceSpecs(ctx, releaseNamespace, renderChartResult.ResourceSpecs, []spec.ResourceTransformer{
			spec.NewResourceListsTransformer(),
			spec.NewDropInvalidAnnotationsAndLabelsTransformer(),
		})
		if err != nil {
			return fmt.Errorf("build transformed resource specs: %w", err)
		}

		log.Default.Debug(ctx, "Build releasable resource specs")

		patchers := []spec.ResourcePatcher{
			spec.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
			spec.NewSecretStringDataPatcher(),
		}

		if opts.LegacyHelmCompatibleTracking {
			patchers = append(patchers, spec.NewLegacyOnlyTrackJobsPatcher())
		}

		releasableResSpecs, err := spec.BuildReleasableResourceSpecs(ctx, releaseNamespace, transformedResSpecs, patchers)
		if err != nil {
			return fmt.Errorf("build releasable resource specs: %w", err)
		}

		newRelease, err = release.NewRelease(releaseName, releaseNamespace, newRevision, deployType, releasableResSpecs, renderChartResult.Chart, renderChartResult.ReleaseConfig, release.ReleaseOptions{
			InfoAnnotations: opts.ReleaseInfoAnnotations,
			Labels:          opts.ReleaseLabels,
			Notes:           renderChartResult.Notes,
		})
		if err != nil {
			return fmt.Errorf("construct new release: %w", err)
		}

		log.Default.Debug(ctx, "Convert previous release to resource specs")

		var prevRelResSpecs []*spec.ResourceSpec
		if prevRelease != nil {
			prevRelResSpecs, err = release.ReleaseToResourceSpecs(prevRelease, releaseNamespace, false)
			if err != nil {
				return fmt.Errorf("convert previous release to resource specs: %w", err)
			}
		}

		log.Default.Debug(ctx, "Convert new release to resource specs")

		newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, releaseNamespace, false)
		if err != nil {
			return fmt.Errorf("convert new release to resource specs: %w", err)
		}

		log.Default.Debug(ctx, "Build resources")

		instResources, delResources, err := resource.BuildResources(ctx, deployType, releaseNamespace, prevRelResSpecs, newRelResSpecs, []spec.ResourcePatcher{
			spec.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
			spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
		}, clientFactory, resource.BuildResourcesOptions{
			Remote:                   true,
			DefaultDeletePropagation: metav1.DeletionPropagation(opts.DefaultDeletePropagation),
		})
		if err != nil {
			return fmt.Errorf("build resources: %w", err)
		}

		log.Default.Debug(ctx, "Locally validate resources")

		if err := resource.ValidateLocal(ctx, releaseNamespace, instResources, opts.ResourceValidationOptions); err != nil {
			return fmt.Errorf("locally validate resources: %w", err)
		}

		log.Default.Debug(ctx, "Build resource infos")

		var delResInfos []*plan.DeletableResourceInfo

		instResInfos, delResInfos, err = plan.BuildResourceInfos(ctx, deployType, releaseName, releaseNamespace, instResources, delResources, prevReleaseFailed, clientFactory, plan.BuildResourceInfosOptions{
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

		relInfos, err = plan.BuildReleaseInfos(ctx, deployType, releases, newRelease)
		if err != nil {
			return fmt.Errorf("build release infos: %w", err)
		}

		log.Default.Debug(ctx, "Build install plan")

		installPlan, err = plan.BuildPlan(instResInfos, delResInfos, relInfos, plan.BuildPlanOptions{
			NoFinalTracking: opts.NoFinalTracking,
		})
		if err != nil {
			handleBuildPlanErr(ctx, installPlan, err, opts.InstallGraphPath, opts.TempDirPath, "release-install-graph.dot")

			return fmt.Errorf("build install plan: %w", err)
		}
	}

	if opts.InstallGraphPath != "" {
		if err := savePlanAsDot(installPlan, opts.InstallGraphPath); err != nil {
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
		if opts.InstallReportPath != "" {
			if err := saveReport(opts.InstallReportPath, &releaseReportV3{
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

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q (namespace: %q): cluster resources already as desired", releaseName, releaseNamespace)))

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

	criticalErrs := &util.MultiError{}
	nonCriticalErrs := &util.MultiError{}

	var reporter *plan.LegacyProgressReporter
	if opts.LegacyProgressReportCh != nil {
		reporter = plan.NewLegacyProgressReporter(opts.LegacyProgressReportCh)
	}

	log.Default.Debug(ctx, "Execute release install plan")

	executePlanErr := plan.ExecutePlan(ctx, releaseNamespace, installPlan, taskStore, logStore, informerFactory, history, clientFactory, plan.ExecutePlanOptions{
		LegacyProgressReporter: reporter,
		TrackingOptions:        opts.TrackingOptions,
		NetworkParallelism:     opts.NetworkParallelism,
	})
	if executePlanErr != nil {
		criticalErrs.Add(fmt.Errorf("execute release install plan: %w", executePlanErr))
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
			LegacyProgressReporter: reporter,
			TrackingOptions:        opts.TrackingOptions,
			NetworkParallelism:     opts.NetworkParallelism,
		})

		criticalErrs.Add(critErrs)
		nonCriticalErrs.Add(nonCritErrs)

		if runFailurePlanResult != nil {
			completedResourceOps = append(completedResourceOps, runFailurePlanResult.CompletedResourceOps...)
			canceledResourceOps = append(canceledResourceOps, runFailurePlanResult.CanceledResourceOps...)
			failedResourceOps = append(failedResourceOps, runFailurePlanResult.FailedResourceOps...)
		}

		if opts.AutoRollback && prevDeployedRelease != nil {
			runRollbackPlanResult, nonCritErrs, critErrs := runRollbackPlan(ctx, releaseName, releaseNamespace, newRelease, prevDeployedRelease, taskStore, logStore, informerFactory, history, clientFactory, runRollbackPlanOptions{
				ReleaseInstallRuntimeOptions: opts.ReleaseInstallRuntimeOptions,
				TrackingOptions:              opts.TrackingOptions,
				LegacyProgressReporter:       reporter,
				NetworkParallelism:           opts.NetworkParallelism,
				RollbackGraphPath:            opts.RollbackGraphPath,
			})

			criticalErrs.Add(critErrs)
			nonCriticalErrs.Add(nonCritErrs)

			if runRollbackPlanResult != nil {
				completedResourceOps = append(completedResourceOps, runRollbackPlanResult.CompletedResourceOps...)
				canceledResourceOps = append(canceledResourceOps, runRollbackPlanResult.CanceledResourceOps...)
				failedResourceOps = append(failedResourceOps, runRollbackPlanResult.FailedResourceOps...)
			}
		}
	}

	if !opts.NoProgressTablePrint {
		progressPrinter.Stop()
		progressPrinter.Wait()
	}

	if reporter != nil {
		reporter.Stop(ctx)
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
		Status:              lo.Ternary(executePlanErr == nil, helmrelease.StatusDeployed, helmrelease.StatusFailed),
		CompletedOperations: reportCompletedOps,
		CanceledOperations:  reportCanceledOps,
		FailedOperations:    reportFailedOps,
	}

	printReport(ctx, report)

	if opts.InstallReportPath != "" {
		if err := saveReport(opts.InstallReportPath, report); err != nil {
			nonCriticalErrs.Add(fmt.Errorf("save release install report: %w", err))
		}
	}

	if !criticalErrs.HasErrors() && !opts.NoShowNotes {
		printNotes(ctx, newRelease.Info.Notes)
	}

	if criticalErrs.HasErrors() {
		allErrs := &util.MultiError{}
		allErrs.Add(criticalErrs, nonCriticalErrs)

		return fmt.Errorf("failed release %q (namespace: %q): %w", releaseName, releaseNamespace, allErrs)
	} else if nonCriticalErrs.HasErrors() {
		return fmt.Errorf("succeeded release %q (namespace: %q), but non-critical errors encountered: %w", releaseName, releaseNamespace, nonCriticalErrs)
	}

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Succeeded release %q (namespace: %q)", releaseName, releaseNamespace)))

	return nil
}

func applyReleaseInstallOptionsDefaults(opts ReleaseInstallOptions, currentDir, homeDir string) (ReleaseInstallOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseInstallOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)
	opts.ChartRepoConnectionOptions.ApplyDefaults()
	opts.ValuesOptions.ApplyDefaults()
	opts.SecretValuesOptions.ApplyDefaults(currentDir)
	opts.TrackingOptions.ApplyDefaults()

	if opts.Chart == "" && opts.ChartDirPath != "" {
		opts.Chart = opts.ChartDirPath
	} else if opts.ChartDirPath == "" && opts.Chart == "" {
		opts.Chart = currentDir
	}

	if opts.LegacyLogRegistryStreamOut == nil {
		opts.LegacyLogRegistryStreamOut = io.Discard
	}

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
		return ReleaseInstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = common.DefaultRegistryCredentialsPath
	}

	if opts.ChartProvenanceStrategy == "" {
		opts.ChartProvenanceStrategy = common.DefaultChartProvenanceStrategy
	}

	if opts.DefaultDeletePropagation == "" {
		opts.DefaultDeletePropagation = string(common.DefaultDeletePropagation)
	}

	return opts, nil
}

func createReleaseNamespace(ctx context.Context, clientFactory *kube.ClientFactory, releaseNamespace string) error {
	unstruct := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": releaseNamespace,
			},
		},
	}

	resSpec := spec.NewResourceSpec(unstruct, releaseNamespace, spec.ResourceSpecOptions{})

	if _, err := clientFactory.KubeClient().Get(ctx, resSpec.ResourceMeta, kube.KubeClientGetOptions{
		TryCache: true,
	}); err != nil {
		if kube.IsNotFoundErr(err) {
			log.Default.Debug(ctx, "Create release namespace %q", releaseNamespace)

			if _, err := clientFactory.KubeClient().Create(ctx, resSpec, kube.KubeClientCreateOptions{}); err != nil {
				return fmt.Errorf("create release namespace: %w", err)
			}
		} else if errors.IsForbidden(err) {
		} else {
			return fmt.Errorf("get release namespace: %w", err)
		}
	}

	return nil
}

func runRollbackPlan(ctx context.Context, releaseName, releaseNamespace string, failedRelease, prevDeployedRelease *helmrelease.Release, taskStore *kdutil.Concurrent[*statestore.TaskStore], logStore *kdutil.Concurrent[*logstore.LogStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], history *release.History, clientFactory *kube.ClientFactory, opts runRollbackPlanOptions) (result *runRollbackPlanResult, nonCritErrs, critErrs *util.MultiError) {
	critErrs = &util.MultiError{}
	nonCritErrs = &util.MultiError{}

	log.Default.Debug(ctx, "Convert prev deployed release to resource specs")

	resSpecs, err := release.ReleaseToResourceSpecs(prevDeployedRelease, releaseNamespace, false)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("convert previous deployed release to resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Build transformed resource specs")

	transformedResSpecs, err := spec.BuildTransformedResourceSpecs(ctx, releaseNamespace, resSpecs, []spec.ResourceTransformer{
		spec.NewResourceListsTransformer(),
		spec.NewDropInvalidAnnotationsAndLabelsTransformer(),
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build transformed resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Build releasable resource specs")

	patchers := []spec.ResourcePatcher{
		spec.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
		spec.NewSecretStringDataPatcher(),
	}

	if opts.LegacyHelmCompatibleTracking {
		patchers = append(patchers, spec.NewLegacyOnlyTrackJobsPatcher())
	}

	releasableResSpecs, err := spec.BuildReleasableResourceSpecs(ctx, releaseNamespace, transformedResSpecs, patchers)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build releasable resource specs: %w", err))
	}

	newRelease, err := release.NewRelease(releaseName, releaseNamespace, failedRelease.Version+1, common.DeployTypeRollback, releasableResSpecs, prevDeployedRelease.Chart, prevDeployedRelease.Config, release.ReleaseOptions{
		InfoAnnotations: opts.ReleaseInfoAnnotations,
		Labels:          opts.ReleaseLabels,
		Notes:           prevDeployedRelease.Info.Notes,
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("construct new release: %w", err))
	}

	log.Default.Debug(ctx, "Convert failed release to resource specs")

	failedRelResSpecs, err := release.ReleaseToResourceSpecs(failedRelease, releaseNamespace, false)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("convert previous release to resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, releaseNamespace, false)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("convert new release to resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Build resources")

	instResources, delResources, err := resource.BuildResources(ctx, common.DeployTypeRollback, releaseNamespace, failedRelResSpecs, newRelResSpecs, []spec.ResourcePatcher{
		spec.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
		spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
	}, clientFactory, resource.BuildResourcesOptions{
		Remote:                   true,
		DefaultDeletePropagation: metav1.DeletionPropagation(opts.DefaultDeletePropagation),
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build resources: %w", err))
	}

	log.Default.Debug(ctx, "Locally validate resources")

	if err := resource.ValidateLocal(ctx, releaseNamespace, instResources, opts.ResourceValidationOptions); err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("locally validate resources: %w", err))
	}

	log.Default.Debug(ctx, "Build resource infos")

	instResInfos, delResInfos, err := plan.BuildResourceInfos(ctx, common.DeployTypeRollback, releaseName, releaseNamespace, instResources, delResources, true, clientFactory, plan.BuildResourceInfosOptions{
		NetworkParallelism:    opts.NetworkParallelism,
		NoRemoveManualChanges: opts.NoRemoveManualChanges,
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build resource infos: %w", err))
	}

	log.Default.Debug(ctx, "Remotely validate resources")

	if err := plan.ValidateRemote(releaseName, releaseNamespace, instResInfos, opts.ForceAdoption); err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("remotely validate resources: %w", err))
	}

	releases := history.Releases()

	log.Default.Debug(ctx, "Build release infos")

	relInfos, err := plan.BuildReleaseInfos(ctx, common.DeployTypeRollback, releases, newRelease)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build release infos: %w", err))
	}

	log.Default.Debug(ctx, "Build rollback plan")

	rollbackPlan, err := plan.BuildPlan(instResInfos, delResInfos, relInfos, plan.BuildPlanOptions{
		NoFinalTracking: opts.NoFinalTracking,
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build rollback plan: %w", err))
	}

	if opts.RollbackGraphPath != "" {
		if err := savePlanAsDot(rollbackPlan, opts.RollbackGraphPath); err != nil {
			return nil, nonCritErrs, critErrs.Add(fmt.Errorf("save rollback graph: %w", err))
		}
	}

	releaseIsUpToDate, err := release.IsReleaseUpToDate(failedRelease, newRelease)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("check if release is up to date: %w", err))
	}

	planIsUseless := lo.NoneBy(rollbackPlan.Operations(), func(op *plan.Operation) bool {
		switch op.Category {
		case plan.OperationCategoryResource, plan.OperationCategoryTrack:
			return true
		default:
			return false
		}
	})

	if releaseIsUpToDate && planIsUseless {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Skipped rollback release")+" %q (namespace: %q): cluster resources already as desired", releaseName, releaseNamespace)

		return &runRollbackPlanResult{}, nonCritErrs, critErrs
	}

	log.Default.Debug(ctx, "Execute rollback plan")

	executePlanErr := plan.ExecutePlan(ctx, releaseNamespace, rollbackPlan, taskStore, logStore, informerFactory, history, clientFactory, plan.ExecutePlanOptions{
		LegacyProgressReporter: opts.LegacyProgressReporter,
		TrackingOptions:        opts.TrackingOptions,
		NetworkParallelism:     opts.NetworkParallelism,
	})
	if executePlanErr != nil {
		critErrs.Add(fmt.Errorf("execute rollback plan: %w", executePlanErr))
	}

	resourceOps := lo.Filter(rollbackPlan.Operations(), func(op *plan.Operation, _ int) bool {
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
		runFailurePlanResult, nonCrErrs, crErrs := runFailurePlan(ctx, releaseNamespace, rollbackPlan, instResInfos, relInfos, taskStore, logStore, informerFactory, history, clientFactory, runFailureInstallPlanOptions{
			LegacyProgressReporter: opts.LegacyProgressReporter,
			TrackingOptions:        opts.TrackingOptions,
			NetworkParallelism:     opts.NetworkParallelism,
		})

		critErrs.Add(crErrs)
		nonCritErrs.Add(nonCrErrs)

		if runFailurePlanResult != nil {
			completedResourceOps = append(completedResourceOps, runFailurePlanResult.CompletedResourceOps...)
			canceledResourceOps = append(canceledResourceOps, runFailurePlanResult.CanceledResourceOps...)
			failedResourceOps = append(failedResourceOps, runFailurePlanResult.FailedResourceOps...)
		}
	}

	return &runRollbackPlanResult{
		CanceledResourceOps:  canceledResourceOps,
		CompletedResourceOps: completedResourceOps,
		FailedResourceOps:    failedResourceOps,
	}, nonCritErrs, critErrs
}
