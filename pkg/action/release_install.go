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
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleaseInstallLogLevel = log.InfoLevel
)

type ReleaseInstallOptions struct {
	common.KubeConnectionOptions
	common.ChartRepoConnectionOptions
	common.ValuesOptions
	common.SecretValuesOptions
	common.TrackingOptions

	AutoRollback                bool
	Chart                       string
	ChartAppVersion             string
	ChartDirPath                string // TODO(v2): get rid
	ChartProvenanceKeyring      string
	ChartProvenanceStrategy     string
	ChartRepoSkipUpdate         bool
	ChartVersion                string
	DefaultChartAPIVersion      string
	DefaultChartName            string
	DefaultChartVersion         string
	ExtraAnnotations            map[string]string
	ExtraLabels                 map[string]string
	ExtraRuntimeAnnotations     map[string]string
	ExtraRuntimeLabels          map[string]string
	ForceAdoption               bool
	InstallGraphPath            string
	InstallReportPath           string
	LegacyChartType             helmopts.ChartType
	LegacyExtraValues           map[string]interface{}
	LegacyLogRegistryStreamOut  io.Writer
	NetworkParallelism          int
	NoInstallStandaloneCRDs     bool
	NoRemoveManualChanges       bool
	NoShowNotes                 bool
	RegistryCredentialsPath     string
	ReleaseHistoryLimit         int
	ReleaseInfoAnnotations      map[string]string
	ReleaseLabels               map[string]string
	ReleaseStorageDriver        string
	ReleaseStorageSQLConnection string
	RollbackGraphPath           string
	ShowSubchartNotes           bool
	TempDirPath                 string
	TemplatesAllowDNS           bool
	Timeout                     time.Duration
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

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(log.Default.AcceptLevel(ctx, log.DebugLevel)),
		// TODO(log):
		registry.ClientOptWriter(opts.LegacyLogRegistryStreamOut),
		registry.ClientOptCredentialsFile(opts.RegistryCredentialsPath),
	}

	if opts.ChartRepoInsecure {
		helmRegistryClientOpts = append(
			helmRegistryClientOpts,
			registry.ClientOptPlainHTTP(),
		)
	}

	helmRegistryClient, err := registry.NewClient(helmRegistryClientOpts...)
	if err != nil {
		return fmt.Errorf("construct registry client: %w", err)
	}

	releaseStorage, err := release.NewReleaseStorage(ctx, releaseNamespace, opts.ReleaseStorageDriver, clientFactory, release.ReleaseStorageOptions{
		HistoryLimit:  opts.ReleaseHistoryLimit,
		SQLConnection: opts.ReleaseStorageSQLConnection,
	})
	if err != nil {
		return fmt.Errorf("construct release storage: %w", err)
	}

	var lockManager *lock.LockManager
	if m, err := lock.NewLockManager(releaseNamespace, false, clientFactory); err != nil {
		return fmt.Errorf("construct lock manager: %w", err)
	} else {
		lockManager = m
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

	var deployType common.DeployType
	if prevDeployedRelease != nil {
		deployType = common.DeployTypeUpgrade
	} else if prevRelease != nil {
		deployType = common.DeployTypeInstall
	} else {
		deployType = common.DeployTypeInitial
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

	releasableResSpecs, err := spec.BuildReleasableResourceSpecs(ctx, releaseNamespace, transformedResSpecs, []spec.ResourcePatcher{
		spec.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
	})
	if err != nil {
		return fmt.Errorf("build releasable resource specs: %w", err)
	}

	newRelease, err := release.NewRelease(releaseName, releaseNamespace, newRevision, deployType, releasableResSpecs, renderChartResult.Chart, renderChartResult.ReleaseConfig, release.ReleaseOptions{
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
		prevRelResSpecs, err = release.ReleaseToResourceSpecs(prevRelease, releaseNamespace)
		if err != nil {
			return fmt.Errorf("convert previous release to resource specs: %w", err)
		}
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, releaseNamespace)
	if err != nil {
		return fmt.Errorf("convert new release to resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build resources")

	instResources, delResources, err := resource.BuildResources(ctx, deployType, releaseNamespace, prevRelResSpecs, newRelResSpecs, []spec.ResourcePatcher{
		spec.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
		spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
	}, clientFactory, resource.BuildResourcesOptions{
		Remote: true,
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
		handleBuildPlanErr(ctx, installPlan, err, opts.InstallGraphPath, opts.TempDirPath, "release-install-graph.dot")
		return fmt.Errorf("build install plan: %w", err)
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
			printNotes(ctx, renderChartResult.Notes)
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

		if opts.AutoRollback && prevDeployedRelease != nil {
			runRollbackPlanResult, nonCritErrs, critErrs := runRollbackPlan(ctx, releaseName, releaseNamespace, newRelease, prevDeployedRelease, taskStore, logStore, informerFactory, history, clientFactory, runRollbackPlanOptions{
				TrackingOptions:         opts.TrackingOptions,
				ExtraAnnotations:        opts.ExtraAnnotations,
				ExtraLabels:             opts.ExtraLabels,
				ExtraRuntimeAnnotations: opts.ExtraRuntimeAnnotations,
				ExtraRuntimeLabels:      opts.ExtraRuntimeLabels,
				ForceAdoption:           opts.ForceAdoption,
				NetworkParallelism:      opts.NetworkParallelism,
				NoRemoveManualChanges:   opts.NoRemoveManualChanges,
				ReleaseInfoAnnotations:  opts.ReleaseInfoAnnotations,
				ReleaseLabels:           opts.ReleaseLabels,
				RollbackGraphPath:       opts.RollbackGraphPath,
			})

			criticalErrs = append(criticalErrs, critErrs...)
			nonCriticalErrs = append(nonCriticalErrs, nonCritErrs...)

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
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("save release install report: %w", err))
		}
	}

	if len(criticalErrs) == 0 && !opts.NoShowNotes {
		printNotes(ctx, renderChartResult.Notes)
	}

	if len(criticalErrs) > 0 {
		return util.Multierrorf("failed release %q (namespace: %q)", append(criticalErrs, nonCriticalErrs...), releaseName, releaseNamespace)
	} else if len(nonCriticalErrs) > 0 {
		return util.Multierrorf("succeeded release %q (namespace: %q), but non-critical errors encountered", nonCriticalErrs, releaseName, releaseNamespace)
	} else {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Succeeded release %q (namespace: %q)", releaseName, releaseNamespace)))

		return nil
	}
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

type runRollbackPlanOptions struct {
	common.TrackingOptions

	ExtraAnnotations        map[string]string
	ExtraLabels             map[string]string
	ExtraRuntimeAnnotations map[string]string
	ExtraRuntimeLabels      map[string]string
	ForceAdoption           bool
	NetworkParallelism      int
	NoRemoveManualChanges   bool
	ReleaseInfoAnnotations  map[string]string
	ReleaseLabels           map[string]string
	RollbackGraphPath       string
}

type runRollbackPlanResult struct {
	CompletedResourceOps []*plan.Operation
	CanceledResourceOps  []*plan.Operation
	FailedResourceOps    []*plan.Operation
}

func runRollbackPlan(ctx context.Context, releaseName, releaseNamespace string, failedRelease, prevDeployedRelease *helmrelease.Release, taskStore *kdutil.Concurrent[*statestore.TaskStore], logStore *kdutil.Concurrent[*logstore.LogStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], history *release.History, clientFactory *kube.ClientFactory, opts runRollbackPlanOptions) (result *runRollbackPlanResult, nonCritErrs, critErrs []error) {
	log.Default.Debug(ctx, "Convert prev deployed release to resource specs")

	resSpecs, err := release.ReleaseToResourceSpecs(prevDeployedRelease, releaseNamespace)
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("convert previous deployed release to resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Build transformed resource specs")

	transformedResSpecs, err := spec.BuildTransformedResourceSpecs(ctx, releaseNamespace, resSpecs, []spec.ResourceTransformer{
		spec.NewResourceListsTransformer(),
		spec.NewDropInvalidAnnotationsAndLabelsTransformer(),
	})
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("build transformed resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Build releasable resource specs")

	releasableResSpecs, err := spec.BuildReleasableResourceSpecs(ctx, releaseNamespace, transformedResSpecs, []spec.ResourcePatcher{
		spec.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
	})
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("build releasable resource specs: %w", err))
	}

	newRelease, err := release.NewRelease(releaseName, releaseNamespace, failedRelease.Version+1, common.DeployTypeRollback, releasableResSpecs, prevDeployedRelease.Chart, prevDeployedRelease.Config, release.ReleaseOptions{
		InfoAnnotations: opts.ReleaseInfoAnnotations,
		Labels:          opts.ReleaseLabels,
		Notes:           prevDeployedRelease.Info.Notes,
	})
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("construct new release: %w", err))
	}

	log.Default.Debug(ctx, "Convert failed release to resource specs")

	failedRelResSpecs, err := release.ReleaseToResourceSpecs(failedRelease, releaseNamespace)
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("convert previous release to resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, releaseNamespace)
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("convert new release to resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Build resources")

	instResources, delResources, err := resource.BuildResources(ctx, common.DeployTypeRollback, releaseNamespace, failedRelResSpecs, newRelResSpecs, []spec.ResourcePatcher{
		spec.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
		spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
	}, clientFactory, resource.BuildResourcesOptions{
		Remote: true,
	})
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("build resources: %w", err))
	}

	log.Default.Debug(ctx, "Locally validate resources")

	if err := resource.ValidateLocal(releaseNamespace, instResources); err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("locally validate resources: %w", err))
	}

	log.Default.Debug(ctx, "Build resource infos")

	instResInfos, delResInfos, err := plan.BuildResourceInfos(ctx, common.DeployTypeRollback, releaseName, releaseNamespace, instResources, delResources, true, clientFactory, plan.BuildResourceInfosOptions{
		NetworkParallelism:    opts.NetworkParallelism,
		NoRemoveManualChanges: opts.NoRemoveManualChanges,
	})
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("build resource infos: %w", err))
	}

	log.Default.Debug(ctx, "Remotely validate resources")

	if err := plan.ValidateRemote(releaseName, releaseNamespace, instResInfos, opts.ForceAdoption); err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("remotely validate resources: %w", err))
	}

	releases := history.Releases()

	log.Default.Debug(ctx, "Build release infos")

	relInfos, err := plan.BuildReleaseInfos(ctx, common.DeployTypeRollback, releases, newRelease)
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("build release infos: %w", err))
	}

	log.Default.Debug(ctx, "Build rollback plan")

	rollbackPlan, err := plan.BuildPlan(instResInfos, delResInfos, relInfos, plan.BuildPlanOptions{
		NoFinalTracking: opts.NoFinalTracking,
	})
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("build rollback plan: %w", err))
	}

	if opts.RollbackGraphPath != "" {
		if err := savePlanAsDot(rollbackPlan, opts.RollbackGraphPath); err != nil {
			return nil, nonCritErrs, append(critErrs, fmt.Errorf("save rollback graph: %w", err))
		}
	}

	releaseIsUpToDate, err := release.IsReleaseUpToDate(failedRelease, newRelease)
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("check if release is up to date: %w", err))
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
		TrackingOptions:    opts.TrackingOptions,
		NetworkParallelism: opts.NetworkParallelism,
	})
	if executePlanErr != nil {
		critErrs = append(critErrs, fmt.Errorf("execute rollback plan: %w", executePlanErr))
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
			TrackingOptions:    opts.TrackingOptions,
			NetworkParallelism: opts.NetworkParallelism,
		})

		critErrs = append(critErrs, crErrs...)
		nonCritErrs = append(nonCritErrs, nonCrErrs...)

		if runFailurePlanResult != nil {
			completedResourceOps = append(completedResourceOps, runFailurePlanResult.CompletedResourceOps...)
			canceledResourceOps = append(canceledResourceOps, runFailurePlanResult.CanceledResourceOps...)
			failedResourceOps = append(failedResourceOps, runFailurePlanResult.FailedResourceOps...)
		}
	}

	return &runRollbackPlanResult{
		CompletedResourceOps: completedResourceOps,
		CanceledResourceOps:  canceledResourceOps,
		FailedResourceOps:    failedResourceOps,
	}, nonCritErrs, critErrs
}
