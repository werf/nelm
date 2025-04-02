package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/3p-helm/pkg/downloader"
	"github.com/werf/3p-helm/pkg/getter"
	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/werf/chartextender"
	"github.com/werf/3p-helm/pkg/werf/secrets"
	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kubeutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/logboek"
	"github.com/werf/nelm/pkg/chrttree"
	helmcommon "github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/lock_manager"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/opertn"
	"github.com/werf/nelm/pkg/pln"
	"github.com/werf/nelm/pkg/plnbuilder"
	"github.com/werf/nelm/pkg/plnexectr"
	"github.com/werf/nelm/pkg/reprt"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcpatcher"
	"github.com/werf/nelm/pkg/resrcprocssr"
	"github.com/werf/nelm/pkg/rls"
	"github.com/werf/nelm/pkg/rlsdiff"
	"github.com/werf/nelm/pkg/rlshistor"
	"github.com/werf/nelm/pkg/track"
	"github.com/werf/nelm/pkg/utls"
)

const (
	DefaultReleaseInstallReportFilename = "release-install-report.json"
	DefaultReleaseInstallGraphFilename  = "release-install-graph.dot"
	DefaultReleaseInstallLogLevel       = InfoLogLevel
)

// FIXME(ilya-lesikov): this must be done a level higher
// var logboekLogLevel level.Level
// var logrusLogLevel logrus.Level
// switch opts.LogLevel {
// case LogLevelNone:
// 	logrusLogLevel = logrus.WarnLevel
// case LogLevelError:
// 	logrusLogLevel = logrus.ErrorLevel
// case LogLevelWarn:
// 	logrusLogLevel = logrus.WarnLevel
// case LogLevelInfo:
// 	logrusLogLevel = logrus.InfoLevel
// case LogLevelDebug:
// 	logrusLogLevel = logrus.DebugLevel
// default:
// 	panic("unknown log level")
// }
//
// if opts.LogLevel == LogLevelNone {
// 	log.Default = log.DefaultNull
// } else {
// 	log.Default = log.NewLogboekLogger(log.LogboekLoggerOptions{
// 		OutStream: opts.LogStreamOut,
// 		ErrStream: opts.LogStreamErr,
// 		LogLevel:  opts.LogLevel,
// 	})
// }
//
// stdlog.SetOutput(opts.LogStreamOut)
// logrus.StandardLogger().SetOutput(opts.LogStreamOut)
// logrus.StandardLogger().SetLevel(logrusLogLevel)

type ReleaseInstallOptions struct {
	AutoRollback                 bool
	ChartAppVersion              string
	ChartDirPath                 string
	ChartRepositoryInsecure      bool
	ChartRepositorySkipTLSVerify bool
	ChartRepositorySkipUpdate    bool
	DefaultChartAPIVersion       string
	DefaultChartName             string
	DefaultChartVersion          string
	DefaultSecretValuesDisable   bool
	DefaultValuesDisable         bool
	ExtraAnnotations             map[string]string
	ExtraLabels                  map[string]string
	ExtraRuntimeAnnotations      map[string]string
	InstallGraphPath             string
	InstallGraphSave             bool
	InstallReportPath            string
	InstallReportSave            bool
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
	LogColorMode                 string
	LogLevel                     string
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	ProgressTablePrint           bool
	ProgressTablePrintInterval   time.Duration
	RegistryCredentialsPath      string
	ReleaseHistoryLimit          int
	ReleaseInfoAnnotations       map[string]string
	ReleaseStorageDriver         string
	RollbackGraphPath            string
	RollbackGraphSave            bool
	SecretKey                    string
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	SecretWorkDir                string
	SubNotes                     bool
	TempDirPath                  string
	TrackCreationTimeout         time.Duration
	TrackDeletionTimeout         time.Duration
	TrackReadinessTimeout        time.Duration
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
}

func ReleaseInstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseInstallOptions) error {
	actionLock.Lock()
	defer actionLock.Unlock()

	if opts.LogLevel != "" {
		log.Default.SetLevel(ctx, log.Level(opts.LogLevel))
	} else {
		log.Default.SetLevel(ctx, log.Level(DefaultReleaseInstallLogLevel))
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	opts, err = applyReleaseInstallOptionsDefaults(opts, currentDir, currentUser)
	if err != nil {
		return fmt.Errorf("build release install options: %w", err)
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	var kubeConfigPath string
	if len(opts.KubeConfigPaths) > 0 {
		kubeConfigPath = opts.KubeConfigPaths[0]
	}

	kubeConfigGetter, err := kube.NewKubeConfigGetter(
		kube.KubeConfigGetterOptions{
			KubeConfigOptions: kube.KubeConfigOptions{
				Context:             opts.KubeContext,
				ConfigPath:          kubeConfigPath,
				ConfigDataBase64:    opts.KubeConfigBase64,
				ConfigPathMergeList: opts.KubeConfigPaths,
			},
			Namespace:     releaseNamespace,
			BearerToken:   opts.KubeToken,
			APIServer:     opts.KubeAPIServerName,
			CAFile:        opts.KubeCAPath,
			TLSServerName: opts.KubeTLSServerName,
			SkipTLSVerify: opts.KubeSkipTLSVerify,
			QPSLimit:      opts.KubeQPSLimit,
			BurstLimit:    opts.KubeBurstLimit,
		},
	)
	if err != nil {
		return fmt.Errorf("construct kube config getter: %w", err)
	}

	helmSettings := helm_v3.Settings
	*helmSettings.GetConfigP() = kubeConfigGetter
	*helmSettings.GetNamespaceP() = releaseNamespace
	releaseNamespace = helmSettings.Namespace()
	helmSettings.MaxHistory = opts.ReleaseHistoryLimit
	helmSettings.Debug = log.Default.AcceptLevel(ctx, log.Level(DebugLogLevel))

	if opts.KubeContext != "" {
		helmSettings.KubeContext = opts.KubeContext
	}

	if kubeConfigPath != "" {
		helmSettings.KubeConfig = kubeConfigPath
	}

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(log.Default.AcceptLevel(ctx, log.Level(DebugLogLevel))),
		registry.ClientOptWriter(opts.LogRegistryStreamOut),
	}

	if opts.ChartRepositoryInsecure {
		helmRegistryClientOpts = append(
			helmRegistryClientOpts,
			registry.ClientOptPlainHTTP(),
		)
	}

	helmRegistryClientOpts = append(
		helmRegistryClientOpts,
		registry.ClientOptCredentialsFile(opts.RegistryCredentialsPath),
	)

	helmRegistryClient, err := registry.NewClient(helmRegistryClientOpts...)
	if err != nil {
		return fmt.Errorf("construct registry client: %w", err)
	}

	helmActionConfig := &action.Configuration{}
	if err := helmActionConfig.Init(
		helmSettings.RESTClientGetter(),
		releaseNamespace,
		string(opts.ReleaseStorageDriver),
		func(format string, a ...interface{}) {
			log.Default.Debug(ctx, format, a...)
		},
	); err != nil {
		return fmt.Errorf("helm action config init: %w", err)
	}
	helmActionConfig.RegistryClient = helmRegistryClient

	helmReleaseStorage := helmActionConfig.Releases
	helmReleaseStorage.MaxHistory = opts.ReleaseHistoryLimit

	// FIXME(ilya-lesikov): this is not used later
	helmChartPathOptions := action.ChartPathOptions{
		InsecureSkipTLSverify: opts.ChartRepositorySkipTLSVerify,
		PlainHTTP:             opts.ChartRepositoryInsecure,
	}
	helmChartPathOptions.SetRegistryClient(helmRegistryClient)

	clientFactory, err := kubeclnt.NewClientFactory()
	if err != nil {
		return fmt.Errorf("construct kube client factory: %w", err)
	}

	var lockManager *lock_manager.LockManager
	if m, err := lock_manager.NewLockManager(
		releaseNamespace,
		false,
		clientFactory.Static(),
		clientFactory.Dynamic(),
	); err != nil {
		return fmt.Errorf("construct lock manager: %w", err)
	} else {
		lockManager = m
	}

	chartextender.DefaultChartAPIVersion = opts.DefaultChartAPIVersion
	chartextender.DefaultChartName = opts.DefaultChartName
	chartextender.DefaultChartVersion = opts.DefaultChartVersion
	chartextender.ChartAppVersion = opts.ChartAppVersion
	loader.WithoutDefaultSecretValues = opts.DefaultSecretValuesDisable
	loader.WithoutDefaultValues = opts.DefaultValuesDisable
	secrets.CoalesceTablesFunc = chartutil.CoalesceTables
	secrets.SecretsWorkingDir = opts.SecretWorkDir
	loader.SecretValuesFiles = opts.SecretValuesPaths
	secrets.ChartDir = opts.ChartDirPath
	secrets_manager.DisableSecretsDecryption = opts.SecretKeyIgnore

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
	history, err := rlshistor.NewHistory(
		releaseName,
		releaseNamespace,
		helmReleaseStorage,
		rlshistor.HistoryOptions{
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

	var deployType helmcommon.DeployType
	if prevReleaseFound && prevDeployedReleaseFound {
		deployType = helmcommon.DeployTypeUpgrade
	} else if prevReleaseFound {
		deployType = helmcommon.DeployTypeInstall
	} else {
		deployType = helmcommon.DeployTypeInitial
	}

	downloader := &downloader.Manager{
		// FIXME(ilya-lesikov):
		Out:               logboek.Context(ctx).OutStream(),
		ChartPath:         opts.ChartDirPath,
		SkipUpdate:        opts.ChartRepositorySkipUpdate,
		AllowMissingRepos: true,
		Getters:           getter.All(helmSettings),
		RegistryClient:    helmRegistryClient,
		RepositoryConfig:  helmSettings.RepositoryConfig,
		RepositoryCache:   helmSettings.RepositoryCache,
		Debug:             helmSettings.Debug,
	}
	loader.SetChartPathFunc = downloader.SetChartPath
	loader.DepsBuildFunc = downloader.Build

	log.Default.Debug(ctx, "Constructing chart tree")
	chartTree, err := chrttree.NewChartTree(
		ctx,
		opts.ChartDirPath,
		releaseName,
		releaseNamespace,
		newRevision,
		deployType,
		helmActionConfig,
		chrttree.ChartTreeOptions{
			StringSetValues: opts.ValuesStringSets,
			SetValues:       opts.ValuesSets,
			FileValues:      opts.ValuesFileSets,
			ValuesFiles:     opts.ValuesFilesPaths,
			SubNotes:        opts.SubNotes,
			Mapper:          clientFactory.Mapper(),
			DiscoveryClient: clientFactory.Discovery(),
		},
	)
	if err != nil {
		return fmt.Errorf("construct chart tree: %w", err)
	}

	notes := chartTree.Notes()

	var prevRelGeneralResources []*resrc.GeneralResource
	if prevReleaseFound {
		prevRelGeneralResources = prevRelease.GeneralResources()
	}

	log.Default.Debug(ctx, "Processing resources")
	resProcessor := resrcprocssr.NewDeployableResourcesProcessor(
		deployType,
		releaseName,
		releaseNamespace,
		chartTree.StandaloneCRDs(),
		chartTree.HookResources(),
		chartTree.GeneralResources(),
		prevRelGeneralResources,
		resrcprocssr.DeployableResourcesProcessorOptions{
			NetworkParallelism: opts.NetworkParallelism,
			ReleasableHookResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
			},
			ReleasableGeneralResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
			},
			DeployableStandaloneCRDsPatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(
					lo.Assign(opts.ExtraAnnotations, opts.ExtraRuntimeAnnotations), opts.ExtraLabels,
				),
			},
			DeployableHookResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(
					lo.Assign(opts.ExtraAnnotations, opts.ExtraRuntimeAnnotations), opts.ExtraLabels,
				),
			},
			DeployableGeneralResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(
					lo.Assign(opts.ExtraAnnotations, opts.ExtraRuntimeAnnotations), opts.ExtraLabels,
				),
			},
			KubeClient:         clientFactory.KubeClient(),
			Mapper:             clientFactory.Mapper(),
			DiscoveryClient:    clientFactory.Discovery(),
			AllowClusterAccess: true,
		},
	)

	if err := resProcessor.Process(ctx); err != nil {
		return fmt.Errorf("process resources: %w", err)
	}

	log.Default.Debug(ctx, "Constructing new release")
	newRel, err := rls.NewRelease(
		releaseName,
		releaseNamespace,
		newRevision,
		chartTree.ReleaseValues(),
		chartTree.LegacyChart(),
		resProcessor.ReleasableHookResources(),
		resProcessor.ReleasableGeneralResources(),
		notes,
		rls.ReleaseOptions{
			InfoAnnotations: opts.ReleaseInfoAnnotations,
			FirstDeployed:   firstDeployed,
			Mapper:          clientFactory.Mapper(),
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
	deployPlanBuilder := plnbuilder.NewDeployPlanBuilder(
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
		plnbuilder.DeployPlanBuilderOptions{
			PrevRelease:         prevRelease,
			PrevDeployedRelease: prevDeployedRelease,
			CreationTimeout:     opts.TrackCreationTimeout,
			ReadinessTimeout:    opts.TrackReadinessTimeout,
			DeletionTimeout:     opts.TrackDeletionTimeout,
		},
	)

	plan, planBuildErr := deployPlanBuilder.Build(ctx)
	if planBuildErr != nil {
		if _, err := os.Create(opts.InstallGraphPath); err != nil {
			log.Default.Error(ctx, "Error: create release install graph file: %s", err)
			return fmt.Errorf("build deploy plan: %w", planBuildErr)
		}

		if err := plan.SaveDOT(opts.InstallGraphPath); err != nil {
			log.Default.Error(ctx, "Error: save release install graph: %s", err)
		}

		log.Default.Warn(ctx, "Release install graph saved to %q for debugging", opts.InstallGraphPath)

		return fmt.Errorf("build release install plan: %w", planBuildErr)
	}

	if opts.InstallGraphSave {
		if err := plan.SaveDOT(opts.InstallGraphPath); err != nil {
			return fmt.Errorf("save release install graph: %w", err)
		}
	}

	var releaseUpToDate bool
	if prevReleaseFound {
		releaseUpToDate, err = rlsdiff.ReleaseUpToDate(prevRelease, newRel)
		if err != nil {
			return fmt.Errorf("check if release is up to date: %w", err)
		}
	}

	planUseless, err := plan.Useless()
	if err != nil {
		return fmt.Errorf("check if release install plan will do anything useful: %w", err)
	}

	if releaseUpToDate && planUseless {
		if opts.InstallReportSave {
			newRel.Skip()

			report := reprt.NewReport(nil, nil, nil, newRel)

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
			Colorize:         opts.LogColorMode == LogColorModeOn,
		},
	)

	log.Default.Debug(ctx, "Starting tracking")
	stdoutTrackerStopCh := make(chan bool)
	stdoutTrackerFinishedCh := make(chan bool)

	if opts.ProgressTablePrint {
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
	planExecutor := plnexectr.NewPlanExecutor(
		plan,
		plnexectr.PlanExecutorOptions{
			NetworkParallelism: opts.NetworkParallelism,
		},
	)

	var criticalErrs, nonCriticalErrs []error

	planExecutionErr := planExecutor.Execute(ctx)
	if planExecutionErr != nil {
		criticalErrs = append(criticalErrs, fmt.Errorf("execute release install plan: %w", planExecutionErr))
	}

	var worthyCompletedOps []opertn.Operation
	if ops, found, err := plan.WorthyCompletedOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful completed operations: %w", err))
	} else if found {
		worthyCompletedOps = ops
	}

	var worthyCanceledOps []opertn.Operation
	if ops, found, err := plan.WorthyCanceledOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful canceled operations: %w", err))
	} else if found {
		worthyCanceledOps = ops
	}

	var worthyFailedOps []opertn.Operation
	if ops, found, err := plan.WorthyFailedOperations(); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful failed operations: %w", err))
	} else if found {
		worthyFailedOps = ops
	}

	var pendingReleaseCreated bool
	if ops, found, err := plan.OperationsMatch(regexp.MustCompile(fmt.Sprintf(`^%s/%s$`, opertn.TypeCreatePendingReleaseOperation, newRel.ID()))); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get pending release operation: %w", err))
	} else if !found {
		panic("no pending release operation found")
	} else {
		pendingReleaseCreated = ops[0].Status() == opertn.StatusCompleted
	}

	if planExecutionErr != nil && pendingReleaseCreated {
		wcompops, wfailops, wcancops, criterrs, noncriterrs := runFailureDeployPlan(
			ctx,
			releaseNamespace,
			deployType,
			plan,
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
				opts.RollbackGraphSave,
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

	if opts.ProgressTablePrint {
		stdoutTrackerStopCh <- true
		<-stdoutTrackerFinishedCh
	}

	report := reprt.NewReport(
		worthyCompletedOps,
		worthyCanceledOps,
		worthyFailedOps,
		newRel,
	)

	report.Print(ctx)

	if opts.InstallReportSave {
		if err := report.Save(opts.InstallReportPath); err != nil {
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("save release install report: %w", err))
		}
	}

	if len(criticalErrs) == 0 {
		printNotes(ctx, notes)
	}

	if len(criticalErrs) > 0 {
		return utls.Multierrorf("failed release %q (namespace: %q)", append(criticalErrs, nonCriticalErrs...), releaseName, releaseNamespace)
	} else if len(nonCriticalErrs) > 0 {
		return utls.Multierrorf("succeeded release %q (namespace: %q), but non-critical errors encountered", nonCriticalErrs, releaseName, releaseNamespace)
	} else {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Succeeded release %q (namespace: %q)", releaseName, releaseNamespace)))

		return nil
	}
}

func applyReleaseInstallOptionsDefaults(
	opts ReleaseInstallOptions,
	currentDir string,
	currentUser *user.User,
) (ReleaseInstallOptions, error) {
	if opts.ChartDirPath == "" {
		opts.ChartDirPath = currentDir
	}

	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseInstallOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.InstallGraphPath == "" {
		opts.InstallGraphPath = filepath.Join(opts.TempDirPath, DefaultReleaseInstallGraphFilename)
	}

	if opts.RollbackGraphPath == "" {
		opts.RollbackGraphPath = filepath.Join(opts.TempDirPath, DefaultReleaseRollbackGraphFilename)
	}

	if opts.InstallReportPath == "" {
		opts.InstallReportPath = filepath.Join(opts.TempDirPath, DefaultReleaseInstallReportFilename)
	}

	if opts.KubeConfigBase64 == "" && len(opts.KubeConfigPaths) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(currentUser.HomeDir, ".kube", "config")}
	}

	if opts.LogRegistryStreamOut == nil {
		opts.LogRegistryStreamOut = os.Stdout
	}

	opts.LogColorMode = applyLogColorModeDefault(opts.LogColorMode, false)

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
	clientFactory *kubeclnt.ClientFactory,
	releaseNamespace string,
) error {
	releaseNamespaceResource := resrc.NewReleaseNamespace(
		&unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]interface{}{
					"name": releaseNamespace,
				},
			},
		}, resrc.ReleaseNamespaceOptions{
			Mapper: clientFactory.Mapper(),
		},
	)

	if _, err := clientFactory.KubeClient().Get(
		ctx,
		releaseNamespaceResource.ResourceID,
		kubeclnt.KubeClientGetOptions{
			TryCache: true,
		},
	); err != nil {
		if errors.IsNotFound(err) {
			log.Default.Debug(ctx, "Creating release namespace %q", releaseNamespace)

			createOp := opertn.NewCreateResourceOperation(
				releaseNamespaceResource.ResourceID,
				releaseNamespaceResource.Unstructured(),
				clientFactory.KubeClient(),
				opertn.CreateResourceOperationOptions{
					ManageableBy: resrc.ManageableByAnyone,
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

	log.Default.InfoBlock(ctx, color.Style{color.Bold, color.Blue}.Render("Release notes")).Do(func() {
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
			logboek.Context(ctx).LogBlock(header).Do(func() {
				tables[header].SuppressTrailingSpaces()
				logboek.Context(ctx).LogLn(tables[header].Render())
			})
		}
	}

	if tables, nonEmpty := tablesBuilder.BuildLogTables(); nonEmpty {
		headers := lo.Keys(tables)
		sort.Strings(headers)

		for _, header := range headers {
			logboek.Context(ctx).LogBlock(header).Do(func() {
				tables[header].SuppressTrailingSpaces()
				logboek.Context(ctx).LogLn(tables[header].Render())
			})
		}
	}

	if table, nonEmpty := tablesBuilder.BuildProgressTable(); nonEmpty {
		logboek.Context(ctx).LogBlock(color.Style{color.Bold, color.Blue}.Render("Progress status")).Do(func() {
			table.SuppressTrailingSpaces()
			logboek.Context(ctx).LogLn(table.Render())
		})
	}
}

func runFailureDeployPlan(
	ctx context.Context,
	releaseNamespace string,
	deployType helmcommon.DeployType,
	failedPlan *pln.Plan,
	taskStore *statestore.TaskStore,
	resProcessor *resrcprocssr.DeployableResourcesProcessor,
	newRel, prevRelease *rls.Release,
	history *rlshistor.History,
	clientFactory *kubeclnt.ClientFactory,
	networkParallelism int,
) (
	worthyCompletedOps []opertn.Operation,
	worthyFailedOps []opertn.Operation,
	worthyCanceledOps []opertn.Operation,
	criticalErrs []error,
	nonCriticalErrs []error,
) {
	log.Default.Debug(ctx, "Building failure deploy plan")
	failurePlanBuilder := plnbuilder.NewDeployFailurePlanBuilder(
		releaseNamespace,
		deployType,
		failedPlan,
		taskStore,
		resProcessor.DeployableHookResourcesInfos(),
		resProcessor.DeployableGeneralResourcesInfos(),
		newRel,
		history,
		clientFactory.KubeClient(),
		clientFactory.Dynamic(),
		clientFactory.Mapper(),
		plnbuilder.DeployFailurePlanBuilderOptions{
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
	failurePlanExecutor := plnexectr.NewPlanExecutor(
		failurePlan,
		plnexectr.PlanExecutorOptions{
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
	deployType helmcommon.DeployType,
	failedRelease *rls.Release,
	prevDeployedRelease *rls.Release,
	failedRevision int,
	history *rlshistor.History,
	clientFactory *kubeclnt.ClientFactory,
	userExtraAnnotations map[string]string,
	serviceAnnotations map[string]string,
	userExtraLabels map[string]string,
	trackCreationTimeout time.Duration,
	trackReadinessTimeout time.Duration,
	trackDeletionTimeout time.Duration,
	saveRollbackGraph bool,
	rollbackGraphPath string,
	networkParallelism int,
) (
	worthyCompletedOps []opertn.Operation,
	worthyFailedOps []opertn.Operation,
	worthyCanceledOps []opertn.Operation,
	notes string,
	criticalErrs []error,
	nonCriticalErrs []error,
) {
	log.Default.Debug(ctx, "Processing rollback resources")
	resProcessor := resrcprocssr.NewDeployableResourcesProcessor(
		helmcommon.DeployTypeRollback,
		releaseName,
		releaseNamespace,
		nil,
		prevDeployedRelease.HookResources(),
		prevDeployedRelease.GeneralResources(),
		failedRelease.GeneralResources(),
		resrcprocssr.DeployableResourcesProcessorOptions{
			NetworkParallelism: networkParallelism,
			ReleasableHookResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(userExtraAnnotations, userExtraLabels),
			},
			ReleasableGeneralResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(userExtraAnnotations, userExtraLabels),
			},
			DeployableStandaloneCRDsPatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(
					lo.Assign(userExtraAnnotations, serviceAnnotations), userExtraLabels,
				),
			},
			DeployableHookResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(
					lo.Assign(userExtraAnnotations, serviceAnnotations), userExtraLabels,
				),
			},
			DeployableGeneralResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(
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
	rollbackRel, err := rls.NewRelease(
		releaseName,
		releaseNamespace,
		rollbackRevision,
		prevDeployedRelease.Values(),
		prevDeployedRelease.LegacyChart(),
		resProcessor.ReleasableHookResources(),
		resProcessor.ReleasableGeneralResources(),
		prevDeployedRelease.Notes(),
		rls.ReleaseOptions{
			FirstDeployed: prevDeployedRelease.FirstDeployed(),
			Mapper:        clientFactory.Mapper(),
		},
	)
	if err != nil {
		return nil, nil, nil, "", []error{fmt.Errorf("construct rollback release: %w", err)}, nonCriticalErrs
	}

	log.Default.Debug(ctx, "Constructing rollback plan")
	rollbackPlanBuilder := plnbuilder.NewDeployPlanBuilder(
		releaseNamespace,
		helmcommon.DeployTypeRollback,
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
		plnbuilder.DeployPlanBuilderOptions{
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

	if saveRollbackGraph {
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
	rollbackPlanExecutor := plnexectr.NewPlanExecutor(
		rollbackPlan,
		plnexectr.PlanExecutorOptions{
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
	if ops, found, err := rollbackPlan.OperationsMatch(regexp.MustCompile(fmt.Sprintf(`^%s/%s$`, opertn.TypeCreatePendingReleaseOperation, rollbackRel.ID()))); err != nil {
		nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get pending rollback release operation: %w", err))
	} else if !found {
		panic("no pending rollback release operation found")
	} else {
		pendingRollbackReleaseCreated = ops[0].Status() == opertn.StatusCompleted
	}

	if rollbackPlanExecutionErr != nil && pendingRollbackReleaseCreated {
		wcompops, wfailops, wcancops, criterrs, noncriterrs := runFailureDeployPlan(
			ctx,
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
