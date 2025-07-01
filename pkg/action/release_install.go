package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kubeutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/logboek"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/plan/resourceinfo"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
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
	if opts.Timeout == 0 {
		return releaseInstall(ctx, releaseName, releaseNamespace, opts)
	}

	ctx, ctxCancelFn := context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn()

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- releaseInstall(ctx, releaseName, releaseNamespace, opts)
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

func releaseInstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseInstallOptions) error {
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

	releaseStorage, err := release.NewReleaseStorage(
		ctx,
		releaseNamespace,
		opts.ReleaseStorageDriver,
		release.ReleaseStorageOptions{
			StaticClient:        clientFactory.Static().(*kubernetes.Clientset),
			HistoryLimit:        opts.ReleaseHistoryLimit,
			SQLConnectionString: opts.SQLConnectionString,
		},
	)
	if err != nil {
		return fmt.Errorf("construct release storage: %w", err)
	}

	var lockManager *lock.LockManager
	if m, err := lock.NewLockManager(
		releaseNamespace,
		false,
		clientFactory.Static(),
		clientFactory.Dynamic(),
	); err != nil {
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

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Starting release")+" %q (namespace: %q)", releaseName, releaseNamespace)

	if lock, err := lockManager.LockRelease(ctx, releaseName); err != nil {
		return fmt.Errorf("lock release: %w", err)
	} else {
		defer lockManager.Unlock(lock)
	}

	log.Default.Debug(ctx, "Constructing release history")
	history, err := release.NewHistory(
		releaseName,
		releaseNamespace,
		releaseStorage,
		release.HistoryOptions{
			Mapper:          clientFactory.Mapper(),
			DiscoveryClient: clientFactory.Discovery(),
		},
	)
	if err != nil {
		return fmt.Errorf("construct release history: %w", err)
	}

	prevRelease, prevReleaseFound, err := history.LastRelease()
	if err != nil {
		return fmt.Errorf("get last release: %w", err)
	}

	prevDeployedRelease, prevDeployedReleaseFound, err := history.LastDeployedRelease()
	if err != nil {
		return fmt.Errorf("get last deployed release: %w", err)
	}

	var newRevision int
	var firstDeployed time.Time
	if prevReleaseFound {
		newRevision = prevRelease.Revision() + 1
		firstDeployed = prevRelease.FirstDeployed()
	} else {
		newRevision = 1
	}

	var deployType common.DeployType
	if prevReleaseFound && prevDeployedReleaseFound {
		deployType = common.DeployTypeUpgrade
	} else if prevReleaseFound {
		deployType = common.DeployTypeInstall
	} else {
		deployType = common.DeployTypeInitial
	}

	log.Default.Debug(ctx, "Constructing chart tree")
	chartTree, err := chart.NewChartTree(
		ctx,
		opts.Chart,
		releaseName,
		releaseNamespace,
		newRevision,
		deployType,
		chart.ChartTreeOptions{
			ChartRepoInsecure:      opts.ChartRepositoryInsecure,
			ChartRepoSkipTLSVerify: opts.ChartRepositorySkipTLSVerify,
			ChartRepoSkipUpdate:    opts.ChartRepositorySkipUpdate,
			ChartVersion:           opts.ChartVersion,
			DiscoveryClient:        clientFactory.Discovery(),
			FileValues:             opts.ValuesFileSets,
			KubeCAPath:             opts.KubeCAPath,
			KubeConfig:             clientFactory.KubeConfig(),
			HelmOptions:            helmOptions,
			Mapper:                 clientFactory.Mapper(),
			NoStandaloneCRDs:       opts.NoInstallCRDs,
			RegistryClient:         helmRegistryClient,
			SetValues:              opts.ValuesSets,
			StringSetValues:        opts.ValuesStringSets,
			SubNotes:               opts.SubNotes,
			ValuesFiles:            opts.ValuesFilesPaths,
		},
	)
	if err != nil {
		return fmt.Errorf("construct chart tree: %w", err)
	}

	notes := chartTree.Notes()

	var prevRelGeneralResources []*resource.GeneralResource
	if prevReleaseFound {
		prevRelGeneralResources = prevRelease.GeneralResources()
	}

	log.Default.Debug(ctx, "Processing resources")
	resProcessor := resourceinfo.NewDeployableResourcesProcessor(
		deployType,
		releaseName,
		releaseNamespace,
		chartTree.StandaloneCRDs(),
		chartTree.HookResources(),
		chartTree.GeneralResources(),
		nil,
		prevRelGeneralResources,
		resourceinfo.DeployableResourcesProcessorOptions{
			NetworkParallelism: opts.NetworkParallelism,
			ReleasableHookResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
			},
			ReleasableGeneralResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
			},
			DeployableStandaloneCRDsPatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(
					lo.Assign(opts.ExtraAnnotations, opts.ExtraRuntimeAnnotations), opts.ExtraLabels,
				),
			},
			DeployableHookResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(
					lo.Assign(opts.ExtraAnnotations, opts.ExtraRuntimeAnnotations), opts.ExtraLabels,
				),
			},
			DeployableGeneralResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(
					lo.Assign(opts.ExtraAnnotations, opts.ExtraRuntimeAnnotations), opts.ExtraLabels,
				),
			},
			KubeClient:         clientFactory.KubeClient(),
			Mapper:             clientFactory.Mapper(),
			DiscoveryClient:    clientFactory.Discovery(),
			AllowClusterAccess: true,
			ForceAdoption:      opts.ForceAdoption,
		},
	)

	if err := resProcessor.Process(ctx); err != nil {
		return fmt.Errorf("process resources: %w", err)
	}

	log.Default.Debug(ctx, "Constructing new release")
	newRel, err := release.NewRelease(
		releaseName,
		releaseNamespace,
		newRevision,
		chartTree.OverrideValues(),
		chartTree.LegacyChart(),
		resProcessor.ReleasableHookResources(),
		resProcessor.ReleasableGeneralResources(),
		notes,
		release.ReleaseOptions{
			InfoAnnotations: opts.ReleaseInfoAnnotations,
			FirstDeployed:   firstDeployed,
			Mapper:          clientFactory.Mapper(),
			Labels:          opts.ReleaseLabels,
		},
	)
	if err != nil {
		return fmt.Errorf("construct new release: %w", err)
	}

	taskStore := statestore.NewTaskStore()
	logStore := kubeutil.NewConcurrent(
		logstore.NewLogStore(),
	)

	log.Default.Debug(ctx, "Constructing new deploy plan")
	deployPlanBuilder := plan.NewDeployPlanBuilder(
		releaseNamespace,
		deployType,
		taskStore,
		logStore,
		resProcessor.DeployableStandaloneCRDsInfos(),
		resProcessor.DeployableHookResourcesInfos(),
		resProcessor.DeployableGeneralResourcesInfos(),
		resProcessor.DeployablePrevReleaseGeneralResourcesInfos(),
		newRel,
		history,
		clientFactory.KubeClient(),
		clientFactory.Static(),
		clientFactory.Dynamic(),
		clientFactory.Discovery(),
		clientFactory.Mapper(),
		plan.DeployPlanBuilderOptions{
			PrevRelease:         prevRelease,
			PrevDeployedRelease: prevDeployedRelease,
			CreationTimeout:     opts.TrackCreationTimeout,
			ReadinessTimeout:    opts.TrackReadinessTimeout,
			DeletionTimeout:     opts.TrackDeletionTimeout,
		},
	)

	deployPlan, planBuildErr := deployPlanBuilder.Build(ctx)
	if planBuildErr != nil {
		var graphPath string
		if opts.InstallGraphPath != "" {
			graphPath = opts.InstallGraphPath
		} else {
			graphPath = filepath.Join(opts.TempDirPath, "release-install-graph.dot")
		}

		if _, err := os.Create(graphPath); err != nil {
			log.Default.Error(ctx, "Error: create release install graph file: %s", err)
			return fmt.Errorf("build deploy plan: %w", planBuildErr)
		}

		if err := deployPlan.SaveDOT(graphPath); err != nil {
			log.Default.Error(ctx, "Error: save release install graph: %s", err)
		}

		log.Default.Warn(ctx, "Release install graph saved to %q for debugging", graphPath)

		return fmt.Errorf("build release install plan: %w", planBuildErr)
	}

	if opts.InstallGraphPath != "" {
		if err := deployPlan.SaveDOT(opts.InstallGraphPath); err != nil {
			return fmt.Errorf("save release install graph: %w", err)
		}
	}

	var releaseUpToDate bool
	if prevReleaseFound {
		releaseUpToDate, err = release.ReleaseUpToDate(prevRelease, newRel)
		if err != nil {
			return fmt.Errorf("check if release is up to date: %w", err)
		}
	}

	planUseless, err := deployPlan.Useless()
	if err != nil {
		return fmt.Errorf("check if release install plan will do anything useful: %w", err)
	}

	if releaseUpToDate && planUseless {
		if opts.InstallReportPath != "" {
			newRel.Skip()

			report := newReport(nil, nil, nil, newRel)

			if err := report.Save(opts.InstallReportPath); err != nil {
				log.Default.Error(ctx, "Error: save release install report: %s", err)
			}
		}

		printNotes(ctx, notes)

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q (namespace: %q): cluster resources already as desired", releaseName, releaseNamespace)))

		return nil
	}

	tablesBuilder := track.NewTablesBuilder(
		taskStore,
		logStore,
		track.TablesBuilderOptions{
			DefaultNamespace: releaseNamespace,
		},
	)

	log.Default.Debug(ctx, "Starting tracking")
	stdoutTrackerStopCh := make(chan bool)
	stdoutTrackerFinishedCh := make(chan bool)

	if !opts.NoProgressTablePrint {
		go func() {
			ticker := time.NewTicker(opts.ProgressTablePrintInterval)
			defer func() {
				ticker.Stop()
				stdoutTrackerFinishedCh <- true
			}()

			for {
				select {
				case <-ticker.C:
					printTables(ctx, tablesBuilder)
				case <-stdoutTrackerStopCh:
					printTables(ctx, tablesBuilder)
					return
				}
			}
		}()
	}

	log.Default.Debug(ctx, "Executing release install plan")
	planExecutor := plan.NewPlanExecutor(
		deployPlan,
		plan.PlanExecutorOptions{
			NetworkParallelism: opts.NetworkParallelism,
		},
	)

	var criticalErrs, nonCriticalErrs []error

	log.Default.Debug(ctx, "Actually executing release install plan")
	planExecutionErr := planExecutor.Execute(ctx)
	if planExecutionErr != nil {
		log.Default.Debug(ctx, "Error: execute release install plan: %s", planExecutionErr)
		criticalErrs = append(criticalErrs, fmt.Errorf("execute release install plan: %w", planExecutionErr))
	}
	log.Default.Debug(ctx, "Release install plan executed")

	var worthyCompletedOps []operation.Operation
	if ops, found, err := deployPlan.WorthyCompletedOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful completed operations: %w", err))
	} else if found {
		worthyCompletedOps = ops
	}

	var worthyCanceledOps []operation.Operation
	if ops, found, err := deployPlan.WorthyCanceledOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful canceled operations: %w", err))
	} else if found {
		worthyCanceledOps = ops
	}

	var worthyFailedOps []operation.Operation
	if ops, found, err := deployPlan.WorthyFailedOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful failed operations: %w", err))
	} else if found {
		worthyFailedOps = ops
	}

	var pendingReleaseCreated bool
	if ops, found, err := deployPlan.OperationsMatch(regexp.MustCompile(fmt.Sprintf(`^%s/%s$`, operation.TypeCreatePendingReleaseOperation, newRel.ID()))); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get pending release operation: %w", err))
	} else if !found {
		panic("no pending release operation found")
	} else {
		pendingReleaseCreated = ops[0].Status() == operation.StatusCompleted
	}

	if planExecutionErr != nil && pendingReleaseCreated {
		wcompops, wfailops, wcancops, criterrs, noncriterrs := runFailureDeployPlan(
			ctx,
			releaseName,
			releaseNamespace,
			deployType,
			deployPlan,
			taskStore,
			resProcessor,
			newRel,
			prevRelease,
			history,
			clientFactory,
			opts.NetworkParallelism,
		)

		worthyCompletedOps = append(worthyCompletedOps, wcompops...)
		worthyFailedOps = append(worthyFailedOps, wfailops...)
		worthyCanceledOps = append(worthyCanceledOps, wcancops...)
		criticalErrs = append(criticalErrs, criterrs...)
		nonCriticalErrs = append(nonCriticalErrs, noncriterrs...)

		if opts.AutoRollback && prevDeployedReleaseFound {
			wcompops, wfailops, wcancops, notes, criterrs, noncriterrs = runRollbackPlan(
				ctx,
				taskStore,
				logStore,
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
			)

			worthyCompletedOps = append(worthyCompletedOps, wcompops...)
			worthyFailedOps = append(worthyFailedOps, wfailops...)
			worthyCanceledOps = append(worthyCanceledOps, wcancops...)
			criticalErrs = append(criticalErrs, criterrs...)
			nonCriticalErrs = append(nonCriticalErrs, noncriterrs...)
		}
	}

	if !opts.NoProgressTablePrint {
		stdoutTrackerStopCh <- true
		<-stdoutTrackerFinishedCh
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

func createReleaseNamespace(
	ctx context.Context,
	clientFactory *kube.ClientFactory,
	releaseNamespace string,
) error {
	releaseNamespaceResource := resource.NewReleaseNamespace(
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": releaseNamespace,
				},
			},
		}, resource.ReleaseNamespaceOptions{
			Mapper: clientFactory.Mapper(),
		},
	)

	if _, err := clientFactory.KubeClient().Get(
		ctx,
		releaseNamespaceResource.ResourceID,
		kube.KubeClientGetOptions{
			TryCache: true,
		},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Default.Debug(ctx, "Creating release namespace %q", releaseNamespace)

			createOp := operation.NewCreateResourceOperation(
				releaseNamespaceResource.ResourceID,
				releaseNamespaceResource.Unstructured(),
				clientFactory.KubeClient(),
				operation.CreateResourceOperationOptions{
					ManageableBy: resource.ManageableByAnyone,
				},
			)

			if err := createOp.Execute(ctx); err != nil {
				return fmt.Errorf("create release namespace: %w", err)
			}
		} else if errors.IsForbidden(err) {
		} else {
			return fmt.Errorf("get release namespace: %w", err)
		}
	}

	return nil
}

func printNotes(ctx context.Context, notes string) {
	if notes == "" {
		return
	}

	log.Default.InfoBlock(ctx, log.BlockOptions{
		BlockTitle: color.Style{color.Bold, color.Blue}.Render("Release notes"),
	}, func() {
		log.Default.Info(ctx, notes)
	})
}

func printTables(
	ctx context.Context,
	tablesBuilder *track.TablesBuilder,
) {
	maxTableWidth := logboek.Context(ctx).Streams().ContentWidth() - 2
	tablesBuilder.SetMaxTableWidth(maxTableWidth)

	if tables, nonEmpty := tablesBuilder.BuildEventTables(); nonEmpty {
		headers := lo.Keys(tables)
		sort.Strings(headers)

		for _, header := range headers {
			log.Default.InfoBlock(ctx, log.BlockOptions{
				BlockTitle: header,
			}, func() {
				log.Default.Info(ctx, tables[header].Render())
			})
		}
	}

	if tables, nonEmpty := tablesBuilder.BuildLogTables(); nonEmpty {
		headers := lo.Keys(tables)
		sort.Strings(headers)

		for _, header := range headers {
			log.Default.InfoBlock(ctx, log.BlockOptions{
				BlockTitle: header,
			}, func() {
				log.Default.Info(ctx, tables[header].Render())
			})
		}
	}

	if table, nonEmpty := tablesBuilder.BuildProgressTable(); nonEmpty {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: color.Style{color.Bold, color.Blue}.Render("Progress status"),
		}, func() {
			fmt.Printf("LESIKOVTEST: progress table line: %s\n", table.Render())
			log.Default.Info(ctx, table.Render())
		})
	}
}

func runFailureDeployPlan(
	ctx context.Context,
	releaseName string,
	releaseNamespace string,
	deployType common.DeployType,
	failedPlan *plan.Plan,
	taskStore *statestore.TaskStore,
	resProcessor *resourceinfo.DeployableResourcesProcessor,
	newRel, prevRelease *release.Release,
	history *release.History,
	clientFactory *kube.ClientFactory,
	networkParallelism int,
) (
	worthyCompletedOps []operation.Operation,
	worthyFailedOps []operation.Operation,
	worthyCanceledOps []operation.Operation,
	criticalErrs []error,
	nonCriticalErrs []error,
) {
	log.Default.Debug(ctx, "Building failure deploy plan")
	failurePlanBuilder := plan.NewDeployFailurePlanBuilder(
		releaseName,
		releaseNamespace,
		deployType,
		failedPlan,
		taskStore,
		resProcessor.DeployableHookResourcesInfos(),
		resProcessor.DeployableGeneralResourcesInfos(),
		history,
		clientFactory.KubeClient(),
		clientFactory.Dynamic(),
		clientFactory.Mapper(),
		plan.DeployFailurePlanBuilderOptions{
			NewRelease:  newRel,
			PrevRelease: prevRelease,
		},
	)

	failurePlan, err := failurePlanBuilder.Build(ctx)
	if err != nil {
		return nil, nil, nil, []error{fmt.Errorf("build failure plan: %w", err)}, nil
	}

	if useless, err := failurePlan.Useless(); err != nil {
		return nil, nil, nil, []error{fmt.Errorf("check if failure plan do anything useful: %w", err)}, nil
	} else if useless {
		return nil, nil, nil, nil, nil
	}

	log.Default.Debug(ctx, "Executing failure deploy plan")
	failurePlanExecutor := plan.NewPlanExecutor(
		failurePlan,
		plan.PlanExecutorOptions{
			NetworkParallelism: networkParallelism,
		},
	)

	if err := failurePlanExecutor.Execute(ctx); err != nil {
		criticalErrs = append(criticalErrs, fmt.Errorf("execute failure plan: %w", err))
	}

	if ops, found, err := failurePlan.WorthyCompletedOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful completed operations: %w", err))
	} else if found {
		worthyCompletedOps = append(worthyCompletedOps, ops...)
	}

	if ops, found, err := failurePlan.WorthyFailedOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful failed operations: %w", err))
	} else if found {
		worthyFailedOps = append(worthyFailedOps, ops...)
	}

	if ops, found, err := failurePlan.WorthyCanceledOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful canceled operations: %w", err))
	} else if found {
		worthyCanceledOps = append(worthyCanceledOps, ops...)
	}

	return worthyCompletedOps, worthyFailedOps, worthyCanceledOps, criticalErrs, nonCriticalErrs
}

func runRollbackPlan(
	ctx context.Context,
	taskStore *statestore.TaskStore,
	logStore *kubeutil.Concurrent[*logstore.LogStore],
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
) (
	worthyCompletedOps []operation.Operation,
	worthyFailedOps []operation.Operation,
	worthyCanceledOps []operation.Operation,
	notes string,
	criticalErrs []error,
	nonCriticalErrs []error,
) {
	log.Default.Debug(ctx, "Processing rollback resources")
	resProcessor := resourceinfo.NewDeployableResourcesProcessor(
		common.DeployTypeRollback,
		releaseName,
		releaseNamespace,
		nil,
		prevDeployedRelease.HookResources(),
		prevDeployedRelease.GeneralResources(),
		nil,
		failedRelease.GeneralResources(),
		resourceinfo.DeployableResourcesProcessorOptions{
			NetworkParallelism: networkParallelism,
			ReleasableHookResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(userExtraAnnotations, userExtraLabels),
			},
			ReleasableGeneralResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(userExtraAnnotations, userExtraLabels),
			},
			DeployableStandaloneCRDsPatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(
					lo.Assign(userExtraAnnotations, serviceAnnotations), userExtraLabels,
				),
			},
			DeployableHookResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(
					lo.Assign(userExtraAnnotations, serviceAnnotations), userExtraLabels,
				),
			},
			DeployableGeneralResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(
					lo.Assign(userExtraAnnotations, serviceAnnotations), userExtraLabels,
				),
			},
			KubeClient:         clientFactory.KubeClient(),
			Mapper:             clientFactory.Mapper(),
			DiscoveryClient:    clientFactory.Discovery(),
			AllowClusterAccess: true,
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
		wcompops, wfailops, wcancops, criterrs, noncriterrs := runFailureDeployPlan(
			ctx,
			releaseName,
			releaseNamespace,
			deployType,
			rollbackPlan,
			taskStore,
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
