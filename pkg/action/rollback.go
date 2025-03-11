package action

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"time"

	"github.com/gookit/color"
	"github.com/xo/terminfo"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kubeutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	helmcommon "github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/lock_manager"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/opertn"
	"github.com/werf/nelm/pkg/plnbuilder"
	"github.com/werf/nelm/pkg/plnexectr"
	"github.com/werf/nelm/pkg/reprt"
	"github.com/werf/nelm/pkg/resrcpatcher"
	"github.com/werf/nelm/pkg/resrcprocssr"
	"github.com/werf/nelm/pkg/rls"
	"github.com/werf/nelm/pkg/rlsdiff"
	"github.com/werf/nelm/pkg/rlshistor"
	"github.com/werf/nelm/pkg/track"
	"github.com/werf/nelm/pkg/utls"
)

const (
	DefaultRollbackReportFilename = "rollback-report.json"
	DefaultRollbackGraphFilename  = "rollback-graph.dot"
)

type RollbackOptions struct {
	ExtraRuntimeAnnotations    map[string]string
	KubeAPIServerName          string
	KubeBurstLimit             int
	KubeCAPath                 string
	KubeConfigBase64           string
	KubeConfigPaths            []string
	KubeContext                string
	KubeQPSLimit               int
	KubeSkipTLSVerify          bool
	KubeTLSServerName          string
	KubeToken                  string
	LogColorMode               LogColorMode
	LogLevel                   log.Level
	NetworkParallelism         int
	ProgressTablePrint         bool
	ProgressTablePrintInterval time.Duration
	ReleaseHistoryLimit        int
	ReleaseName                string
	ReleaseNamespace           string
	ReleaseStorageDriver       ReleaseStorageDriver
	Revision                   int
	RollbackGraphPath          string
	RollbackGraphSave          bool
	RollbackReportPath         string
	RollbackReportSave         bool
	TempDirPath                string
	TrackCreationTimeout       time.Duration
	TrackDeletionTimeout       time.Duration
	TrackReadinessTimeout      time.Duration
}

func Rollback(ctx context.Context, opts RollbackOptions) error {
	log.Default.SetLevel(ctx, opts.LogLevel)

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	opts, err = applyRollbackOptionsDefaults(opts, currentUser)
	if err != nil {
		return fmt.Errorf("build rollback options: %w", err)
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
			Namespace:     opts.ReleaseNamespace,
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
	*helmSettings.GetNamespaceP() = opts.ReleaseNamespace
	opts.ReleaseNamespace = helmSettings.Namespace()
	helmSettings.MaxHistory = opts.ReleaseHistoryLimit
	helmSettings.Debug = log.Default.AcceptLevel(ctx, log.DebugLevel)

	if opts.KubeContext != "" {
		helmSettings.KubeContext = opts.KubeContext
	}

	if kubeConfigPath != "" {
		helmSettings.KubeConfig = kubeConfigPath
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

	helmReleaseStorage := helmActionConfig.Releases
	helmReleaseStorage.MaxHistory = opts.ReleaseHistoryLimit

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

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Starting rollback of release")+" %q (namespace: %q)", opts.ReleaseName, opts.ReleaseNamespace)

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
	} else if !prevReleaseFound {
		return fmt.Errorf("not found release %q (namespace: %q)", opts.ReleaseName, opts.ReleaseNamespace)
	}

	prevDeployedRelease, _, err := history.LastDeployedRelease()
	if err != nil {
		return fmt.Errorf("get last deployed release: %w", err)
	}

	var releaseToRollback *rls.Release
	if opts.Revision == 0 {
		prevDeployedReleaseExceptLastRelease, found, err := history.LastDeployedReleaseExceptLastRelease()
		if err != nil {
			return fmt.Errorf("get last deployed release except last release: %w", err)
		}

		if !found {
			return fmt.Errorf("not found successfully deployed (except last) release %q (namespace: %q)", opts.ReleaseName, opts.ReleaseNamespace)
		}

		releaseToRollback = prevDeployedReleaseExceptLastRelease
	} else {
		var found bool
		releaseToRollback, found, err = history.Release(opts.Revision)
		if err != nil {
			return fmt.Errorf("get release revision %q: %w", opts.Revision, err)
		} else if !found {
			return fmt.Errorf("not found revision %q for release %q (namespace: %q)", opts.Revision, opts.ReleaseName, opts.ReleaseNamespace)
		}
	}

	var newRevision int
	var firstDeployed time.Time
	if prevReleaseFound {
		newRevision = prevRelease.Revision() + 1
		firstDeployed = prevRelease.FirstDeployed()
	} else {
		newRevision = 1
	}

	deployType := helmcommon.DeployTypeRollback
	notes := releaseToRollback.Notes()

	log.Default.Info(ctx, "Processing rollback resources")
	resProcessor := resrcprocssr.NewDeployableResourcesProcessor(
		deployType,
		opts.ReleaseName,
		opts.ReleaseNamespace,
		nil,
		releaseToRollback.HookResources(),
		releaseToRollback.GeneralResources(),
		prevRelease.GeneralResources(),
		resrcprocssr.DeployableResourcesProcessorOptions{
			NetworkParallelism: opts.NetworkParallelism,
			DeployableHookResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(
					opts.ExtraRuntimeAnnotations, nil,
				),
			},
			DeployableGeneralResourcePatchers: []resrcpatcher.ResourcePatcher{
				resrcpatcher.NewExtraMetadataPatcher(
					opts.ExtraRuntimeAnnotations, nil,
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

	log.Default.Info(ctx, "Constructing new rollback release")
	newRel, err := rls.NewRelease(
		opts.ReleaseName,
		opts.ReleaseNamespace,
		newRevision,
		releaseToRollback.Values(),
		releaseToRollback.LegacyChart(),
		resProcessor.ReleasableHookResources(),
		resProcessor.ReleasableGeneralResources(),
		notes,
		rls.ReleaseOptions{
			FirstDeployed: firstDeployed,
			Mapper:        clientFactory.Mapper(),
		},
	)
	if err != nil {
		return fmt.Errorf("construct new rollback release: %w", err)
	}

	taskStore := statestore.NewTaskStore()
	logStore := kubeutil.NewConcurrent(
		logstore.NewLogStore(),
	)

	log.Default.Info(ctx, "Constructing new rollback plan")
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
		if _, err := os.Create(opts.RollbackGraphPath); err != nil {
			log.Default.Error(ctx, "Error: create rollback graph file: %s", err)
			return fmt.Errorf("build rollback plan: %w", planBuildErr)
		}

		if err := plan.SaveDOT(opts.RollbackGraphPath); err != nil {
			log.Default.Error(ctx, "Error: save rollback graph: %s", err)
		}

		log.Default.Warn(ctx, "Rollback graph saved to %q for debugging", opts.RollbackGraphPath)

		return fmt.Errorf("build rollback plan: %w", planBuildErr)
	}

	if opts.RollbackGraphSave {
		if err := plan.SaveDOT(opts.RollbackGraphPath); err != nil {
			return fmt.Errorf("save rollback graph: %w", err)
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
		return fmt.Errorf("check if rollback plan will do anything useful: %w", err)
	}

	if releaseUpToDate && planUseless {
		if opts.RollbackReportSave {
			newRel.Skip()

			report := reprt.NewReport(nil, nil, nil, newRel)

			if err := report.Save(opts.RollbackReportPath); err != nil {
				log.Default.Error(ctx, "Error: save rollback report: %s", err)
			}
		}

		printNotes(ctx, notes)

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped rollback of release %q (namespace: %q): cluster resources already as desired", opts.ReleaseName, opts.ReleaseNamespace)))

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

	log.Default.Info(ctx, "Executing rollback plan")
	planExecutor := plnexectr.NewPlanExecutor(
		plan,
		plnexectr.PlanExecutorOptions{
			NetworkParallelism: opts.NetworkParallelism,
		},
	)

	var criticalErrs, nonCriticalErrs []error

	planExecutionErr := planExecutor.Execute(ctx)
	if planExecutionErr != nil {
		criticalErrs = append(criticalErrs, fmt.Errorf("execute rollback plan: %w", planExecutionErr))
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

	if opts.RollbackReportSave {
		if err := report.Save(opts.RollbackReportPath); err != nil {
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("save rollback report: %w", err))
		}
	}

	if len(criticalErrs) == 0 {
		printNotes(ctx, notes)
	}

	if len(criticalErrs) > 0 {
		return utls.Multierrorf("failed rollback of release %q (namespace: %q)", append(criticalErrs, nonCriticalErrs...), opts.ReleaseName, opts.ReleaseNamespace)
	} else if len(nonCriticalErrs) > 0 {
		return utls.Multierrorf("succeeded rollback of release %q (namespace: %q), but non-critical errors encountered", nonCriticalErrs, opts.ReleaseName, opts.ReleaseNamespace)
	} else {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Succeeded rollback of release %q (namespace: %q)", opts.ReleaseName, opts.ReleaseNamespace)))

		return nil
	}
}

func applyRollbackOptionsDefaults(
	opts RollbackOptions,
	currentUser *user.User,
) (RollbackOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return RollbackOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.RollbackGraphPath == "" {
		opts.RollbackGraphPath = filepath.Join(opts.TempDirPath, DefaultRollbackGraphFilename)
	}

	if opts.RollbackReportPath == "" {
		opts.RollbackReportPath = filepath.Join(opts.TempDirPath, DefaultRollbackReportFilename)
	}

	if opts.KubeConfigBase64 == "" && len(opts.KubeConfigPaths) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(currentUser.HomeDir, ".kube", "config")}
	}

	if opts.LogColorMode == LogColorModeUnspecified || opts.LogColorMode == LogColorModeAuto {
		if color.DetectColorLevel() == terminfo.ColorLevelNone {
			opts.LogColorMode = LogColorModeOff
		} else {
			opts.LogColorMode = LogColorModeOn
		}
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

	if opts.ReleaseName == "" {
		return RollbackOptions{}, fmt.Errorf("release name not specified")
	}

	if opts.ReleaseStorageDriver == ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = ReleaseStorageDriverSecrets
	} else if opts.ReleaseStorageDriver == ReleaseStorageDriverMemory {
		return RollbackOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	return opts, nil
}
