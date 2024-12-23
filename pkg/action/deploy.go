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
	"github.com/xo/terminfo"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/cli"
	"github.com/werf/3p-helm/pkg/registry"
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
	DefaultDeployReportFilename  = "deploy-report.json"
	DefaultDeployGraphFilename   = "deploy-graph.dot"
	DefaultRollbackGraphFilename = "rollback-graph.dot"
)

// FIXME(ilya-lesikov): this is old... need to check
// 1. if last succeeded release was cleaned up because of release limit, werf will see
// current release as first install. We might want to not delete last succeeded or last
// uninstalled release ever.
// 2. don't forget errs.FormatTemplatingError if any errors occurs

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

type DeployOptions struct {
	AutoRollback                 bool
	ChartDirPath                 string
	ChartRepositoryInsecure      bool
	ChartRepositorySkipTLSVerify bool
	ChartRepositorySkipUpdate    bool
	DefaultSecretValuesDisable   bool
	DefaultValuesDisable         bool
	DeployGraphPath              string
	DeployGraphSave              bool
	DeployReportPath             string
	DeployReportSave             bool
	ExtraAnnotations             map[string]string
	ExtraLabels                  map[string]string
	ExtraRuntimeAnnotations      map[string]string
	KubeConfigBase64             string
	KubeConfigPaths              []string
	KubeContext                  string
	LogColorMode                 LogColorMode
	LogDebug                     bool
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	ProgressTablePrint           bool
	ProgressTablePrintInterval   time.Duration
	RegistryCredentialsPath      string
	ReleaseHistoryLimit          int
	ReleaseName                  string
	ReleaseNamespace             string
	ReleaseStorageDriver         ReleaseStorageDriver
	RollbackGraphPath            string
	RollbackGraphSave            bool
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	TempDirPath                  string
	TrackCreationTimeout         time.Duration
	TrackDeletionTimeout         time.Duration
	TrackReadinessTimeout        time.Duration
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
	SubNotes                     bool
	LegacyPreDeployHook          func(
		ctx context.Context,
		releaseNamespace string,
		helmRegistryClient *registry.Client,
		registryCredentialsPath string,
		chartRepositorySkipUpdate bool,
		secretValuesPaths []string,
		extraAnnotations map[string]string,
		extraLabels map[string]string,
		defaultValuesDisable bool,
		defaultSecretValuesDisable bool,
		helmSettings *cli.EnvSettings,
	) error
}

func Deploy(ctx context.Context, opts DeployOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	opts, err = applyDeployOptionsDefaults(opts, currentDir, currentUser)
	if err != nil {
		return fmt.Errorf("build deploy options: %w", err)
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
			Namespace: opts.ReleaseNamespace,
		},
	)
	if err != nil {
		return fmt.Errorf("construct kube config getter: %w", err)
	}

	helmSettings := helm_v3.Settings
	*helmSettings.GetConfigP() = kubeConfigGetter
	*helmSettings.GetNamespaceP() = opts.ReleaseNamespace
	opts.ReleaseNamespace = helmSettings.Namespace()
	helmSettings.MaxHistory = opts.ReleaseHistoryLimit
	helmSettings.Debug = opts.LogDebug

	if opts.KubeContext != "" {
		helmSettings.KubeContext = opts.KubeContext
	}

	if kubeConfigPath != "" {
		helmSettings.KubeConfig = kubeConfigPath
	}

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(opts.LogDebug),
		registry.ClientOptWriter(opts.LogRegistryStreamOut),
	}

	if opts.ChartRepositoryInsecure {
		helmRegistryClientOpts = append(
			helmRegistryClientOpts,
			registry.ClientOptPlainHTTP(),
		)
	}

	if opts.RegistryCredentialsPath != "" {
		helmRegistryClientOpts = append(
			helmRegistryClientOpts,
			registry.ClientOptCredentialsFile(opts.RegistryCredentialsPath),
		)
	}

	helmRegistryClient, err := registry.NewClient(helmRegistryClientOpts...)
	if err != nil {
		return fmt.Errorf("construct registry client: %w", err)
	}

	helmActionConfig := &action.Configuration{}
	if err := helmActionConfig.Init(
		helmSettings.RESTClientGetter(),
		opts.ReleaseNamespace,
		string(opts.ReleaseStorageDriver),
		func(format string, a ...interface{}) {
			log.Default.Info(ctx, format, a...)
		},
	); err != nil {
		return fmt.Errorf("helm action config init: %w", err)
	}
	helmActionConfig.RegistryClient = helmRegistryClient

	helmReleaseStorage := helmActionConfig.Releases
	helmReleaseStorage.MaxHistory = opts.ReleaseHistoryLimit

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
		opts.ReleaseNamespace,
		false,
		clientFactory.Static(),
		clientFactory.Dynamic(),
	); err != nil {
		return fmt.Errorf("construct lock manager: %w", err)
	} else {
		lockManager = m
	}

	secrets_manager.DefaultManager = secrets_manager.NewSecretsManager(
		secrets_manager.SecretsManagerOptions{
			DisableSecretsDecryption: opts.SecretKeyIgnore,
		},
	)

	if opts.LegacyPreDeployHook != nil {
		if err := opts.LegacyPreDeployHook(
			ctx,
			opts.ReleaseNamespace,
			helmRegistryClient,
			opts.RegistryCredentialsPath,
			opts.ChartRepositorySkipUpdate,
			opts.SecretValuesPaths,
			opts.ExtraAnnotations,
			opts.ExtraLabels,
			opts.DefaultValuesDisable,
			opts.DefaultSecretValuesDisable,
			helmSettings,
		); err != nil {
			return fmt.Errorf("legacy pre deploy hook: %w", err)
		}
	}

	if err := createReleaseNamespace(ctx, clientFactory, opts.ReleaseNamespace); err != nil {
		return fmt.Errorf("create release namespace: %w", err)
	}

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Starting release")+" %q (namespace: %q)", opts.ReleaseName, opts.ReleaseNamespace)

	if lock, err := lockManager.LockRelease(ctx, opts.ReleaseName); err != nil {
		return fmt.Errorf("lock release: %w", err)
	} else {
		defer lockManager.Unlock(lock)
	}

	log.Default.Info(ctx, "Constructing release history")
	history, err := rlshistor.NewHistory(
		opts.ReleaseName,
		opts.ReleaseNamespace,
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

	log.Default.Info(ctx, "Constructing chart tree")
	chartTree, err := chrttree.NewChartTree(
		ctx,
		opts.ChartDirPath,
		opts.ReleaseName,
		opts.ReleaseNamespace,
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

	log.Default.Info(ctx, "Processing resources")
	resProcessor := resrcprocssr.NewDeployableResourcesProcessor(
		deployType,
		opts.ReleaseName,
		opts.ReleaseNamespace,
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

	log.Default.Info(ctx, "Constructing new release")
	newRel, err := rls.NewRelease(
		opts.ReleaseName,
		opts.ReleaseNamespace,
		newRevision,
		chartTree.ReleaseValues(),
		chartTree.LegacyChart(),
		resProcessor.ReleasableHookResources(),
		resProcessor.ReleasableGeneralResources(),
		notes,
		rls.ReleaseOptions{
			FirstDeployed: firstDeployed,
			Mapper:        clientFactory.Mapper(),
		},
	)
	if err != nil {
		return fmt.Errorf("construct new release: %w", err)
	}

	taskStore := statestore.NewTaskStore()
	logStore := kubeutil.NewConcurrent(
		logstore.NewLogStore(),
	)

	log.Default.Info(ctx, "Constructing new deploy plan")
	deployPlanBuilder := plnbuilder.NewDeployPlanBuilder(
		opts.ReleaseNamespace,
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
		if _, err := os.Create(opts.DeployGraphPath); err != nil {
			log.Default.Error(ctx, "Error: create deploy graph file: %s", err)
			return fmt.Errorf("build deploy plan: %w", planBuildErr)
		}

		if err := plan.SaveDOT(opts.DeployGraphPath); err != nil {
			log.Default.Error(ctx, "Error: save deploy graph: %s", err)
		}

		log.Default.Warn(ctx, "Deploy graph saved to %q for debugging", opts.DeployGraphPath)

		return fmt.Errorf("build deploy plan: %w", planBuildErr)
	}

	if opts.DeployGraphSave {
		if err := plan.SaveDOT(opts.DeployGraphPath); err != nil {
			return fmt.Errorf("save deploy graph: %w", err)
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
		return fmt.Errorf("check if deploy plan will do anything useful: %w", err)
	}

	if releaseUpToDate && planUseless {
		if opts.DeployReportSave {
			newRel.Skip()

			report := reprt.NewReport(nil, nil, nil, newRel)

			if err := report.Save(opts.DeployReportPath); err != nil {
				log.Default.Error(ctx, "Error: save deploy report: %s", err)
			}
		}

		printNotes(ctx, notes)

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q (namespace: %q): cluster resources already as desired", opts.ReleaseName, opts.ReleaseNamespace)))

		return nil
	}

	tablesBuilder := track.NewTablesBuilder(
		taskStore,
		logStore,
		track.TablesBuilderOptions{
			DefaultNamespace: opts.ReleaseNamespace,
			Colorize:         opts.LogColorMode == LogColorModeOn,
		},
	)

	log.Default.Info(ctx, "Starting tracking")
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

	log.Default.Info(ctx, "Executing deploy plan")
	planExecutor := plnexectr.NewPlanExecutor(
		plan,
		plnexectr.PlanExecutorOptions{
			NetworkParallelism: opts.NetworkParallelism,
		},
	)

	var criticalErrs, nonCriticalErrs []error

	planExecutionErr := planExecutor.Execute(ctx)
	if planExecutionErr != nil {
		criticalErrs = append(criticalErrs, fmt.Errorf("execute deploy plan: %w", planExecutionErr))
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
			opts.ReleaseNamespace,
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
				opts.ReleaseName,
				opts.ReleaseNamespace,
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

	if opts.DeployReportSave {
		if err := report.Save(opts.DeployReportPath); err != nil {
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("save deploy report: %w", err))
		}
	}

	if len(criticalErrs) == 0 {
		printNotes(ctx, notes)
	}

	if len(criticalErrs) > 0 {
		return utls.Multierrorf("failed release %q (namespace: %q)", append(criticalErrs, nonCriticalErrs...), opts.ReleaseName, opts.ReleaseNamespace)
	} else if len(nonCriticalErrs) > 0 {
		return utls.Multierrorf("succeeded release %q (namespace: %q), but non-critical errors encountered", nonCriticalErrs, opts.ReleaseName, opts.ReleaseNamespace)
	} else {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Succeeded release %q (namespace: %q)", opts.ReleaseName, opts.ReleaseNamespace)))

		return nil
	}
}

func applyDeployOptionsDefaults(
	opts DeployOptions,
	currentDir string,
	currentUser *user.User,
) (DeployOptions, error) {
	if opts.ChartDirPath == "" {
		opts.ChartDirPath = currentDir
	}

	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return DeployOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.DeployGraphPath == "" {
		opts.DeployGraphPath = filepath.Join(opts.TempDirPath, DefaultDeployGraphFilename)
	}

	if opts.RollbackGraphPath == "" {
		opts.RollbackGraphPath = filepath.Join(opts.TempDirPath, DefaultRollbackGraphFilename)
	}

	if opts.DeployReportPath == "" {
		opts.DeployReportPath = filepath.Join(opts.TempDirPath, DefaultDeployReportFilename)
	}

	if opts.KubeConfigBase64 == "" && len(opts.KubeConfigPaths) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(currentUser.HomeDir, ".kube", "config")}
	}

	if opts.LogRegistryStreamOut == nil {
		opts.LogRegistryStreamOut = os.Stdout
	}

	if opts.LogColorMode == LogColorModeDefault {
		if color.DetectColorLevel() == terminfo.ColorLevelNone {
			opts.LogColorMode = LogColorModeOff
		} else {
			opts.LogColorMode = LogColorModeOn
		}
	}

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = 30
	}

	if opts.ProgressTablePrintInterval <= 0 {
		opts.ProgressTablePrintInterval = 5 * time.Second
	}

	if opts.ReleaseHistoryLimit <= 0 {
		opts.ReleaseHistoryLimit = 10
	}

	if opts.ReleaseName == "" {
		return DeployOptions{}, fmt.Errorf("release name not specified")
	}

	if opts.ReleaseStorageDriver == ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = ReleaseStorageDriverSecrets
	} else if opts.ReleaseStorageDriver == ReleaseStorageDriverMemory {
		return DeployOptions{}, fmt.Errorf("memory release storage driver is not supported")
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
			log.Default.Info(ctx, "Creating release namespace %q", releaseNamespace)

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
	log.Default.Info(ctx, "Building failure deploy plan")
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

	log.Default.Info(ctx, "Executing failure deploy plan")
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
	log.Default.Info(ctx, "Processing rollback resources")
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

	log.Default.Info(ctx, "Constructing rollback release")
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

	log.Default.Info(ctx, "Constructing rollback plan")
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

	log.Default.Info(ctx, "Executing rollback plan")
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
