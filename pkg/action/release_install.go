package action

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/3p-helm/pkg/registry"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kubeutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm-for-werf-helm/pkg/resrcinfo"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/plan/resinfo"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/meta"
	"github.com/werf/nelm/internal/track"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleaseInstallLogLevel = InfoLogLevel
)

type ReleaseInstallOptions struct {
	AutoRollback                 bool
	Chart                        string
	ChartAppVersion              string
	ChartDirPath                 string // TODO(v2): get rid
	ChartRepositoryInsecure      bool
	ChartRepositorySkipTLSVerify bool
	ChartRepositorySkipUpdate    bool
	ChartVersion                 string
	DefaultChartAPIVersion       string
	DefaultChartName             string
	DefaultChartVersion          string
	DefaultSecretValuesDisable   bool
	DefaultValuesDisable         bool
	ExtraAnnotations             map[string]string
	ExtraLabels                  map[string]string
	ExtraRuntimeAnnotations      map[string]string
	ForceAdoption                bool
	InstallGraphPath             string
	InstallReportPath            string
	KubeAPIServerName            string
	KubeBurstLimit               int
	KubeCAPath                   string
	KubeConfigBase64             string
	KubeConfigPaths              []string
	KubeContext                  string
	KubeQPSLimit                 int
	KubeSkipTLSVerify            bool
	KubeTLSServerName            string
	KubeToken                    string
	LegacyChartType              helmopts.ChartType
	LegacyExtraValues            map[string]interface{}
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	NoInstallCRDs                bool
	NoPodLogs                    bool
	NoProgressTablePrint         bool
	ProgressTablePrintInterval   time.Duration
	RegistryCredentialsPath      string
	ReleaseHistoryLimit          int
	ReleaseInfoAnnotations       map[string]string
	ReleaseLabels                map[string]string
	ReleaseStorageDriver         string
	RollbackGraphPath            string
	SQLConnectionString          string
	SecretKey                    string
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	SecretWorkDir                string
	SubNotes                     bool
	TempDirPath                  string
	Timeout                      time.Duration
	TrackCreationTimeout         time.Duration
	TrackDeletionTimeout         time.Duration
	TrackReadinessTimeout        time.Duration
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
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
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	if len(opts.KubeConfigPaths) > 0 {
		var splitPaths []string
		for _, path := range opts.KubeConfigPaths {
			splitPaths = append(splitPaths, filepath.SplitList(path)...)
		}

		opts.KubeConfigPaths = lo.Compact(splitPaths)
	}

	// TODO(ilya-lesikov): some options are not propagated from cli/actions
	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		BurstLimit:            opts.KubeBurstLimit,
		CertificateAuthority:  opts.KubeCAPath,
		CurrentContext:        opts.KubeContext,
		InsecureSkipTLSVerify: opts.KubeSkipTLSVerify,
		KubeConfigBase64:      opts.KubeConfigBase64,
		Namespace:             releaseNamespace,
		QPSLimit:              opts.KubeQPSLimit,
		Server:                opts.KubeAPIServerName,
		TLSServerName:         opts.KubeTLSServerName,
		Token:                 opts.KubeToken,
	})
	if err != nil {
		return fmt.Errorf("construct kube config: %w", err)
	}

	clientFactory, err := kube.NewClientFactory(ctx, kubeConfig)
	if err != nil {
		return fmt.Errorf("construct kube client factory: %w", err)
	}

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(log.Default.AcceptLevel(ctx, log.Level(DebugLogLevel))),
		registry.ClientOptWriter(opts.LogRegistryStreamOut),
		registry.ClientOptCredentialsFile(opts.RegistryCredentialsPath),
	}

	if opts.ChartRepositoryInsecure {
		helmRegistryClientOpts = append(
			helmRegistryClientOpts,
			registry.ClientOptPlainHTTP(),
		)
	}

	helmRegistryClient, err := registry.NewClient(helmRegistryClientOpts...)
	if err != nil {
		return fmt.Errorf("construct registry client: %w", err)
	}

	releaseStorage, err := release.NewReleaseStorage(ctx, releaseNamespace, opts.ReleaseStorageDriver, release.ReleaseStorageOptions{
		StaticClient:        clientFactory.Static().(*kubernetes.Clientset),
		HistoryLimit:        opts.ReleaseHistoryLimit,
		SQLConnectionString: opts.SQLConnectionString,
	})
	if err != nil {
		return fmt.Errorf("construct release storage: %w", err)
	}

	var lockManager *lock.LockManager
	if m, err := lock.NewLockManager(releaseNamespace, false, clientFactory.Static(), clientFactory.Dynamic()); err != nil {
		return fmt.Errorf("construct lock manager: %w", err)
	} else {
		lockManager = m
	}

	helmOptions := helmopts.HelmOptions{
		ChartLoadOpts: helmopts.ChartLoadOptions{
			ChartAppVersion:        opts.ChartAppVersion,
			ChartType:              opts.LegacyChartType,
			DefaultChartAPIVersion: opts.DefaultChartAPIVersion,
			DefaultChartName:       opts.DefaultChartName,
			DefaultChartVersion:    opts.DefaultChartVersion,
			ExtraValues:            opts.LegacyExtraValues,
			NoDecryptSecrets:       opts.SecretKeyIgnore,
			NoDefaultSecretValues:  opts.DefaultSecretValuesDisable,
			NoDefaultValues:        opts.DefaultValuesDisable,
			SecretValuesFiles:      opts.SecretValuesPaths,
			SecretsWorkingDir:      opts.SecretWorkDir,
		},
	}

	if err := createReleaseNamespace(ctx, clientFactory, releaseNamespace); err != nil {
		return fmt.Errorf("create release namespace: %w", err)
	}

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Start release")+" %q (namespace: %q)", releaseName, releaseNamespace)

	if lock, err := lockManager.LockRelease(ctx, releaseName); err != nil {
		return fmt.Errorf("lock release: %w", err)
	} else {
		defer lockManager.Unlock(lock)
	}

	log.Default.Debug(ctx, "Construct release history")
	history, err := release.BuildHistory(releaseName, releaseStorage, release.HistoryOptions{})
	if err != nil {
		return fmt.Errorf("construct release history: %w", err)
	}

	var prevRelease *helmrelease.Release
	var prevDeployedRelease *helmrelease.Release
	var prevReleaseFailed bool

	releases := history.Releases()
	deployedReleases := history.FindAllDeployed()
	if len(releases) > 0 {
		prevRelease = lo.LastOrEmpty(releases)
		prevDeployedRelease = lo.LastOrEmpty(deployedReleases)
	}

	var newRevision int
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
	renderChartResult, err := chart.RenderChart(ctx, opts.Chart, releaseName, releaseNamespace, newRevision, deployType, chart.RenderChartOptions{
		ChartRepoInsecure:      opts.ChartRepositoryInsecure,
		ChartRepoSkipTLSVerify: opts.ChartRepositorySkipTLSVerify,
		ChartRepoSkipUpdate:    opts.ChartRepositorySkipUpdate,
		ChartVersion:           opts.ChartVersion,
		DiscoveryClient:        clientFactory.Discovery(),
		FileValues:             opts.ValuesFileSets,
		HelmOptions:            helmOptions,
		KubeCAPath:             opts.KubeCAPath,
		KubeConfig:             clientFactory.KubeConfig(),
		Mapper:                 clientFactory.Mapper(),
		NoStandaloneCRDs:       opts.NoInstallCRDs,
		RegistryClient:         helmRegistryClient,
		SetValues:              opts.ValuesSets,
		StringSetValues:        opts.ValuesStringSets,
		SubNotes:               opts.SubNotes,
		ValuesFiles:            opts.ValuesFilesPaths,
	})
	if err != nil {
		return fmt.Errorf("render chart: %w", err)
	}

	log.Default.Debug(ctx, "Build transformed resource specs")
	transformedResSpecs, err := resource.BuildTransformedResourceSpecs(ctx, releaseNamespace, renderChartResult.ResourceSpecs, []resource.ResourceTransformer{
		resource.NewResourceListsTransformer(),
		resource.NewDropInvalidAnnotationsAndLabelsTransformer(),
	})
	if err != nil {
		return fmt.Errorf("build transformed resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build releasable resource specs")
	releasableResSpecs, err := resource.BuildReleasableResourceSpecs(ctx, releaseNamespace, transformedResSpecs, []resource.ResourcePatcher{
		resource.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
	})
	if err != nil {
		return fmt.Errorf("build releasable resource specs: %w", err)
	}

	newRelease, err := release.NewRelease(releaseName, releaseNamespace, newRevision, deployType, releasableResSpecs, release.ReleaseOptions{
		InfoAnnotations: opts.ReleaseInfoAnnotations,
		Labels:          opts.ReleaseLabels,
		Notes:           renderChartResult.Notes,
	})
	if err != nil {
		return fmt.Errorf("construct new release: %w", err)
	}

	log.Default.Debug(ctx, "Build resources")
	instResources, delResources, err := resource.BuildResources(ctx, deployType, releaseNamespace, prevRelease, newRelease, []resource.ResourcePatcher{
		resource.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
		resource.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, nil),
	}, resource.BuildResourcesOptions{
		Mapper: clientFactory.Mapper(),
	})
	if err != nil {
		return fmt.Errorf("build resources: %w", err)
	}

	log.Default.Debug(ctx, "Locally validate resources")
	if err := resource.ValidateLocal(releaseNamespace, instResources); err != nil {
		return fmt.Errorf("locally validate resources: %w", err)
	}

	log.Default.Debug(ctx, "Build resource infos")
	instResInfos, delResInfos, err := plan.BuildResourceInfos(ctx, releaseName, releaseNamespace, instResources, delResources, prevReleaseFailed, clientFactory.KubeClient(), clientFactory.Mapper(), opts.NetworkParallelism)
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
	installPlan, err := plan.BuildPlan(instResInfos, delResInfos, relInfos)
	if err != nil {
		handleBuildInstallPlanErr(ctx, installPlan, err, opts.InstallGraphPath, opts.TempDirPath)
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

	_, installPlanIsUseless := lo.Find(installPlan.Operations(), func(op *plan.Operation) bool {
		return op.Type != plan.OperationTypeNoop
	})

	if releaseIsUpToDate && installPlanIsUseless {
		if opts.InstallReportPath != "" {
			if err := saveReport(opts.InstallReportPath, &reportV2{
				Version:   2,
				Release:   releaseName,
				Namespace: releaseNamespace,
				Revision:  newRelease.Version,
				Status:    helmrelease.StatusSkipped,
			}); err != nil {
				return fmt.Errorf("save release install report: %w", err)
			}
		}

		printNotes(ctx, renderChartResult.Notes)

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q (namespace: %q): cluster resources already as desired", releaseName, releaseNamespace)))

		return nil
	}

	taskStore := kubeutil.NewConcurrent(statestore.NewTaskStore())
	logStore := kubeutil.NewConcurrent(logstore.NewLogStore())
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
	executePlanErr := plan.ExecutePlan(ctx, installPlan, taskStore, logStore, informerFactory, history, clientFactory.KubeClient(), clientFactory.Static(), clientFactory.Dynamic(), clientFactory.Discovery(), clientFactory.Mapper(), plan.ExecutePlanOptions{
		NetworkParallelism: opts.NetworkParallelism,
		ReadinessTimeout:   opts.TrackReadinessTimeout,
		PresenceTimeout:    opts.TrackCreationTimeout,
		AbsenceTimeout:     opts.TrackDeletionTimeout,
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

	_, newReleaseCreated := lo.Find(installPlan.Operations(), func(op *plan.Operation) bool {
		return op.Type == plan.OperationTypeCreateRelease &&
			op.Status == plan.OperationStatusCompleted &&
			op.Config.(*plan.OperationConfigCreateRelease).Release.ID() == newRelease.ID() &&
			op.Config.(*plan.OperationConfigCreateRelease).Release.Info.Status.IsPending()
	})

	if executePlanErr != nil && newReleaseCreated {
		runFailureInstallPlanResult, nonCritErrs, critErrs := runFailureInstallPlan(ctx, installPlan, instResInfos, relInfos, taskStore, logStore, informerFactory, history, clientFactory, runFailureInstallPlanOptions{
			NetworkParallelism:    opts.NetworkParallelism,
			TrackReadinessTimeout: opts.TrackReadinessTimeout,
			TrackCreationTimeout:  opts.TrackCreationTimeout,
			TrackDeletionTimeout:  opts.TrackDeletionTimeout,
		})

		criticalErrs = append(criticalErrs, critErrs...)
		nonCriticalErrs = append(nonCriticalErrs, nonCritErrs...)
		completedResourceOps = append(completedResourceOps, runFailureInstallPlanResult.CompletedResourceOps...)
		canceledResourceOps = append(canceledResourceOps, runFailureInstallPlanResult.CanceledResourceOps...)
		failedResourceOps = append(failedResourceOps, runFailureInstallPlanResult.FailedResourceOps...)

		if opts.AutoRollback && prevDeployedRelease != nil {
			wcompops, wfailops, wcancops, notes, criterrs, noncriterrs = fixmeRunRollbackPlan(
				ctx,
				taskStore,
				logStore,
				informerFactory,
				releaseName,
				releaseNamespace,
				deployType,
				newRel,
				prevDeployedRelease,
				newRevision,
				history,
				clientFactory,
				opts.ExtraAnnotations,
				opts.ExtraRuntimeAnnotations,
				opts.ExtraLabels,
				opts.TrackCreationTimeout,
				opts.TrackReadinessTimeout,
				opts.TrackDeletionTimeout,
				opts.RollbackGraphPath,
				opts.NetworkParallelism,
				opts.ForceAdoption,
			)

			worthyCompletedOps = append(worthyCompletedOps, wcompops...)
			worthyFailedOps = append(worthyFailedOps, wfailops...)
			worthyCanceledOps = append(worthyCanceledOps, wcancops...)
			criticalErrs = append(criticalErrs, criterrs...)
			nonCriticalErrs = append(nonCriticalErrs, noncriterrs...)
		}
	}

	if !opts.NoProgressTablePrint {
		progressPrinter.Stop()
		progressPrinter.Wait()
	}

	report := newReport(
		worthyCompletedOps,
		worthyCanceledOps,
		worthyFailedOps,
		newRel,
	)

	report.Print(ctx)

	if opts.InstallReportPath != "" {
		if err := report.Save(opts.InstallReportPath); err != nil {
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("save release install report: %w", err))
		}
	}

	if len(criticalErrs) == 0 {
		printNotes(ctx, notes)
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

func applyReleaseInstallOptionsDefaults(
	opts ReleaseInstallOptions,
	currentDir string,
	homeDir string,
) (ReleaseInstallOptions, error) {
	if opts.Chart == "" && opts.ChartDirPath != "" {
		opts.Chart = opts.ChartDirPath
	} else if opts.ChartDirPath == "" && opts.Chart == "" {
		opts.Chart = currentDir
	}

	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseInstallOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.KubeConfigBase64 == "" && len(lo.Compact(opts.KubeConfigPaths)) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(homeDir, ".kube", "config")}
	}

	if opts.LogRegistryStreamOut == nil {
		opts.LogRegistryStreamOut = io.Discard
	}

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = DefaultNetworkParallelism
	}

	if opts.KubeQPSLimit <= 0 {
		opts.KubeQPSLimit = DefaultQPSLimit
	}

	if opts.KubeBurstLimit <= 0 {
		opts.KubeBurstLimit = DefaultBurstLimit
	}

	if opts.ProgressTablePrintInterval <= 0 {
		opts.ProgressTablePrintInterval = DefaultProgressPrintInterval
	}

	if opts.ReleaseHistoryLimit <= 0 {
		opts.ReleaseHistoryLimit = DefaultReleaseHistoryLimit
	}

	if opts.ReleaseStorageDriver == ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = ReleaseStorageDriverSecrets
	} else if opts.ReleaseStorageDriver == ReleaseStorageDriverMemory {
		return ReleaseInstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	if opts.SecretWorkDir == "" {
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return ReleaseInstallOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = DefaultRegistryCredentialsPath
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

	resSpec := resource.NewResourceSpec(unstruct, releaseNamespace, resource.ResourceSpecOptions{})

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

type runFailureInstallPlanOptions struct {
	NetworkParallelism    int
	TrackReadinessTimeout time.Duration
	TrackCreationTimeout  time.Duration
	TrackDeletionTimeout  time.Duration
}

type runFailureInstallPlanResult struct {
	CompletedResourceOps []*plan.Operation
	CanceledResourceOps  []*plan.Operation
	FailedResourceOps    []*plan.Operation
}

func runFailureInstallPlan(
	ctx context.Context,
	failedPlan *plan.Plan,
	installableInfos []*plan.InstallableResourceInfo,
	releaseInfos []*plan.ReleaseInfo,
	taskStore *kubeutil.Concurrent[*statestore.TaskStore],
	logStore *kubeutil.Concurrent[*logstore.LogStore],
	informerFactory *kubeutil.Concurrent[*informer.InformerFactory],
	history *release.History,
	clientFactory *kube.ClientFactory,
	opts runFailureInstallPlanOptions,
) (result *runFailureInstallPlanResult, nonCritErrs []error, critErrs []error) {
	log.Default.Debug(ctx, "Build failure plan")
	failurePlan, err := plan.BuildFailurePlan(failedPlan, installableInfos, releaseInfos)
	if err != nil {
		critErrs = append(critErrs, fmt.Errorf("build failure plan: %w", err))
		return nil, nonCritErrs, critErrs
	}

	if _, planIsUseless := lo.Find(failurePlan.Operations(), func(op *plan.Operation) bool {
		return op.Type != plan.OperationTypeNoop
	}); planIsUseless {
		return &runFailureInstallPlanResult{}, nonCritErrs, critErrs
	}

	log.Default.Debug(ctx, "Execute failure plan")
	executePlanErr := plan.ExecutePlan(ctx, failurePlan, taskStore, logStore, informerFactory, history, clientFactory.KubeClient(), clientFactory.Static(), clientFactory.Dynamic(), clientFactory.Discovery(), clientFactory.Mapper(), plan.ExecutePlanOptions{
		NetworkParallelism: opts.NetworkParallelism,
		ReadinessTimeout:   opts.TrackReadinessTimeout,
		PresenceTimeout:    opts.TrackCreationTimeout,
		AbsenceTimeout:     opts.TrackDeletionTimeout,
	})
	if executePlanErr != nil {
		critErrs = append(critErrs, fmt.Errorf("execute failure plan: %w", executePlanErr))
	}

	resourceOps := lo.Filter(failurePlan.Operations(), func(op *plan.Operation, _ int) bool {
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

	return &runFailureInstallPlanResult{
		CompletedResourceOps: completedResourceOps,
		CanceledResourceOps:  canceledResourceOps,
		FailedResourceOps:    failedResourceOps,
	}, nonCritErrs, critErrs
}

type runRollbackPlanOptions struct {
	NetworkParallelism    int
	TrackReadinessTimeout time.Duration
	TrackCreationTimeout  time.Duration
	TrackDeletionTimeout  time.Duration
}

type runRollbackPlanResult struct {
	CompletedResourceOps []*plan.Operation
	CanceledResourceOps  []*plan.Operation
	FailedResourceOps    []*plan.Operation
}

func runRollbackPlan(
	ctx context.Context,
	releaseName string,
	releaseNamespace string,
	deployType common.DeployType,
	failedRelease *helmrelease.Release,
	prevDeployedRelease *helmrelease.Release,
	opts runRollbackPlanOptions,
) (result *runRollbackPlanResult, nonCritErrs []error, critErrs []error) {
	resSpecs, err := release.ReleaseToResourceSpecs(prevDeployedRelease, releaseNamespace)
	if err != nil {
		critErrs = append(critErrs, fmt.Errorf("convert previous deployed release to resource specs: %w", err))
		return nil, nonCritErrs, critErrs
	}

	newRelease, err := release.NewRelease(releaseName, releaseNamespace, failedRelease.Version+1, deployType, releasableResSpecs, release.ReleaseOptions{
		InfoAnnotations: opts.ReleaseInfoAnnotations,
		Labels:          opts.ReleaseLabels,
		Notes:           renderChartResult.Notes,
	})
	if err != nil {
		return fmt.Errorf("construct new release: %w", err)
	}

	log.Default.Debug(ctx, "Build rollback plan")
	rollbackPlan, err := plan.BuildPlan(failedPlan, installableInfos, releaseInfos)
	if err != nil {
		critErrs = append(critErrs, fmt.Errorf("build rollback plan: %w", err))
		return nil, critErrs
	}

	if _, planIsUseless := lo.Find(rollbackPlan.Operations(), func(op *plan.Operation) bool {
		return op.Type != plan.OperationTypeNoop
	}); planIsUseless {
		return &runRollbackPlanResult{}, critErrs
	}

	log.Default.Debug(ctx, "Execute rollback plan")
	executePlanErr := plan.ExecutePlan(ctx, rollbackPlan, taskStore, logStore, informerFactory, history, clientFactory.KubeClient(), clientFactory.Static(), clientFactory.Dynamic(), clientFactory.Discovery(), clientFactory.Mapper(), plan.ExecutePlanOptions{
		NetworkParallelism: opts.NetworkParallelism,
		ReadinessTimeout:   opts.TrackReadinessTimeout,
		PresenceTimeout:    opts.TrackCreationTimeout,
		AbsenceTimeout:     opts.TrackDeletionTimeout,
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

	return &runRollbackPlanResult{
		CompletedResourceOps: completedResourceOps,
		CanceledResourceOps:  canceledResourceOps,
		FailedResourceOps:    failedResourceOps,
		NonCriticalErrs:      nonCritErrs,
	}, critErrs
}

func fixmeRunRollbackPlan(
	ctx context.Context,
	taskStore *statestore.TaskStore,
	logStore *kubeutil.Concurrent[*logstore.LogStore],
	informerFactory *kubeutil.Concurrent[*informer.InformerFactory],
	releaseName string,
	releaseNamespace string,
	deployType common.DeployType,
	failedRelease *release.Release,
	prevDeployedRelease *release.Release,
	failedRevision int,
	history *release.History,
	clientFactory *kube.ClientFactory,
	userExtraAnnotations map[string]string,
	serviceAnnotations map[string]string,
	userExtraLabels map[string]string,
	trackCreationTimeout time.Duration,
	trackReadinessTimeout time.Duration,
	trackDeletionTimeout time.Duration,
	rollbackGraphPath string,
	networkParallelism int,
	forceAdoption bool,
) (
	worthyCompletedOps []operation.FixmeOperation,
	worthyFailedOps []operation.FixmeOperation,
	worthyCanceledOps []operation.FixmeOperation,
	notes string,
	criticalErrs []error,
	nonCriticalErrs []error,
) {
	log.Default.Debug(ctx, "Processing rollback resources")
	resProcessor := resinfo.NewDeployableResourcesProcessor(
		common.DeployTypeRollback,
		releaseName,
		releaseNamespace,
		nil,
		prevDeployedRelease.HookResources(),
		prevDeployedRelease.GeneralResources(),
		nil,
		failedRelease.GeneralResources(),
		resinfo.DeployableResourcesProcessorOptions{
			NetworkParallelism: networkParallelism,
			ExtraReleasableResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(userExtraAnnotations, userExtraLabels),
			},
			ExtraDeployableResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(serviceAnnotations, nil),
			},
			KubeClient:         clientFactory.KubeClient(),
			Mapper:             clientFactory.Mapper(),
			DiscoveryClient:    clientFactory.Discovery(),
			AllowClusterAccess: true,
			ForceAdoption:      forceAdoption,
		},
	)

	if err := resProcessor.Process(ctx); err != nil {
		return nil, nil, nil, "", []error{fmt.Errorf("process rollback resources: %w", err)}, nonCriticalErrs
	}

	rollbackRevision := failedRevision + 1

	log.Default.Debug(ctx, "Constructing rollback release")
	rollbackRel, err := release.NewRelease(
		releaseName,
		releaseNamespace,
		rollbackRevision,
		prevDeployedRelease.OverrideValues(),
		prevDeployedRelease.LegacyChart(),
		resProcessor.ReleasableHookResources(),
		resProcessor.ReleasableGeneralResources(),
		prevDeployedRelease.Notes(),
		release.ReleaseOptions{
			InfoAnnotations: prevDeployedRelease.InfoAnnotations(),
			FirstDeployed:   prevDeployedRelease.FirstDeployed(),
			Mapper:          clientFactory.Mapper(),
			Labels:          prevDeployedRelease.Labels(),
		},
	)
	if err != nil {
		return nil, nil, nil, "", []error{fmt.Errorf("construct rollback release: %w", err)}, nonCriticalErrs
	}

	log.Default.Debug(ctx, "Constructing rollback plan")
	rollbackPlanBuilder := plan.NewDeployPlanBuilder(
		releaseNamespace,
		common.DeployTypeRollback,
		taskStore,
		logStore,
		informerFactory,
		nil,
		resProcessor.DeployableHookResourcesInfos(),
		resProcessor.DeployableGeneralResourcesInfos(),
		resProcessor.DeployablePrevReleaseGeneralResourcesInfos(),
		rollbackRel,
		history,
		clientFactory.KubeClient(),
		clientFactory.Static(),
		clientFactory.Dynamic(),
		clientFactory.Discovery(),
		clientFactory.Mapper(),
		plan.DeployPlanBuilderOptions{
			PrevRelease:         failedRelease,
			PrevDeployedRelease: prevDeployedRelease,
			CreationTimeout:     trackCreationTimeout,
			ReadinessTimeout:    trackReadinessTimeout,
			DeletionTimeout:     trackDeletionTimeout,
		},
	)

	rollbackPlan, err := rollbackPlanBuilder.Build(ctx)
	if err != nil {
		return nil, nil, nil, "", []error{fmt.Errorf("build rollback plan: %w", err)}, nonCriticalErrs
	}

	if rollbackGraphPath != "" {
		if err := rollbackPlan.SaveDOT(rollbackGraphPath); err != nil {
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("save rollback graph: %w", err))
		}
	}

	if useless, err := rollbackPlan.Useless(); err != nil {
		return nil, nil, nil, "", []error{fmt.Errorf("check if rollback plan will do anything useful: %w", err)}, nonCriticalErrs
	} else if useless {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Skipped rollback release")+" %q (namespace: %q): cluster resources already as desired", releaseName, releaseNamespace)

		return nil, nil, nil, "", criticalErrs, nonCriticalErrs
	}

	log.Default.Debug(ctx, "Executing rollback plan")
	rollbackPlanExecutor := plan.NewPlanExecutor(
		rollbackPlan,
		plan.PlanExecutorOptions{
			NetworkParallelism: networkParallelism,
		},
	)

	rollbackPlanExecutionErr := rollbackPlanExecutor.Execute(ctx)
	if rollbackPlanExecutionErr != nil {
		criticalErrs = append(criticalErrs, fmt.Errorf("execute rollback plan: %w", rollbackPlanExecutionErr))
	}

	if ops, found, err := rollbackPlan.WorthyCompletedOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful completed operations: %w", err))
	} else if found {
		worthyCompletedOps = ops
	}

	if ops, found, err := rollbackPlan.WorthyFailedOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful failed operations: %w", err))
	} else if found {
		worthyFailedOps = ops
	}

	if ops, found, err := rollbackPlan.WorthyCanceledOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful canceled operations: %w", err))
	} else if found {
		worthyCanceledOps = ops
	}

	var pendingRollbackReleaseCreated bool
	if ops, found, err := rollbackPlan.OperationsMatch(regexp.MustCompile(fmt.Sprintf(`^%s/%s$`, operation.TypeCreatePendingReleaseOperation, rollbackRel.ID()))); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get pending rollback release operation: %w", err))
	} else if !found {
		panic("no pending rollback release operation found")
	} else {
		pendingRollbackReleaseCreated = ops[0].Status() == operation.StatusCompleted
	}

	if rollbackPlanExecutionErr != nil && pendingRollbackReleaseCreated {
		wcompops, wfailops, wcancops, criterrs, noncriterrs := fixmeRunFailureDeployPlan(
			ctx,
			releaseName,
			releaseNamespace,
			deployType,
			rollbackPlan,
			taskStore,
			informerFactory,
			resProcessor,
			rollbackRel,
			failedRelease,
			history,
			clientFactory,
			networkParallelism,
		)
		worthyCompletedOps = append(worthyCompletedOps, wcompops...)
		worthyFailedOps = append(worthyFailedOps, wfailops...)
		worthyCanceledOps = append(worthyCanceledOps, wcancops...)
		criticalErrs = append(criticalErrs, criterrs...)
		nonCriticalErrs = append(nonCriticalErrs, noncriterrs...)
	}

	return worthyCompletedOps, worthyFailedOps, worthyCanceledOps, rollbackRel.Notes(), criticalErrs, nonCriticalErrs
}

func handleBuildInstallPlanErr(ctx context.Context, installPlan *plan.Plan, planErr error, installGraphPath, tempDirPath string) {
	var graphPath string
	if installGraphPath != "" {
		graphPath = installGraphPath
	} else {
		graphPath = filepath.Join(tempDirPath, "release-install-graph.dot")
	}

	if err := savePlanAsDot(installPlan, graphPath); err != nil {
		log.Default.Error(ctx, "Error: save release install graph: %s", err)
		return
	}

	log.Default.Warn(ctx, "Release install graph saved to %q for debugging", graphPath)
	return
}

func savePlanAsDot(plan *plan.Plan, path string) error {
	dotByte, err := plan.ToDOT()
	if err != nil {
		return fmt.Errorf("convert plan to DOT file: %w", err)
	}

	if err := os.WriteFile(path, dotByte, 0o644); err != nil {
		return fmt.Errorf("write DOT graph file at %q: %w", path, err)
	}

	return nil
}
