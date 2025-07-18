package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kubeutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/plan/resourceinfo"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/internal/track"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleaseUninstallLogLevel = InfoLogLevel
)

type ReleaseUninstallOptions struct {
	DeleteReleaseNamespace     bool
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
	NetworkParallelism         int
	NoProgressTablePrint       bool
	ProgressTablePrintInterval time.Duration
	ReleaseHistoryLimit        int
	ReleaseStorageDriver       string
	SQLConnectionString        string
	TempDirPath                string
	Timeout                    time.Duration
	TrackCreationTimeout       time.Duration
	TrackDeletionTimeout       time.Duration
	TrackReadinessTimeout      time.Duration
	UninstallGraphPath         string
	UninstallReportPath        string
}

func ReleaseUninstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseUninstallOptions) error {
	if opts.Timeout == 0 {
		return releaseUninstall(ctx, releaseName, releaseNamespace, opts)
	}

	ctx, ctxCancelFn := context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("context timed out: action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn()

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- releaseUninstall(ctx, releaseName, releaseNamespace, opts)
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

func releaseUninstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseUninstallOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleaseUninstallOptionsDefaults(opts, currentDir, homeDir)
	if err != nil {
		return fmt.Errorf("build  release uninstall options: %w", err)
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

	namespaceID := id.NewResourceID(
		releaseNamespace,
		"",
		schema.GroupVersionKind{Version: "v1", Kind: "Namespace"},
		id.ResourceIDOptions{Mapper: clientFactory.Mapper()},
	)

	if exists, err := isReleaseNamespaceExist(ctx, clientFactory, namespaceID); err != nil {
		return fmt.Errorf("check release namespace existence: %w", err)
	} else if !exists {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q uninstall: no release namespace %q found", releaseName, releaseNamespace)))

		return nil
	}

	if err := func() error {
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

		if !prevReleaseFound {
			log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q (namespace: %q) uninstall: no release found", releaseName, releaseNamespace)))

			return nil
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Deleting release")+" %q (namespace: %q)", releaseName, releaseNamespace)

		log.Default.Debug(ctx, "Processing resources")
		resProcessor := resourceinfo.NewDeployableResourcesProcessor(
			common.DeployTypeUninstall,
			releaseName,
			releaseNamespace,
			nil,
			nil,
			nil,
			prevRelease.HookResources(),
			prevRelease.GeneralResources(),
			resourceinfo.DeployableResourcesProcessorOptions{
				NetworkParallelism: opts.NetworkParallelism,
				KubeClient:         clientFactory.KubeClient(),
				Mapper:             clientFactory.Mapper(),
				DiscoveryClient:    clientFactory.Discovery(),
				AllowClusterAccess: true,
			},
		)

		if err := resProcessor.Process(ctx); err != nil {
			return fmt.Errorf("process resources: %w", err)
		}

		taskStore := statestore.NewTaskStore()
		logStore := kubeutil.NewConcurrent(
			logstore.NewLogStore(),
		)

		log.Default.Debug(ctx, "Constructing new uninstall plan")
		uninstallPlanBuilder := plan.NewUninstallPlanBuilder(
			releaseName,
			releaseNamespace,
			taskStore,
			logStore,
			resProcessor.DeployablePrevReleaseHookResourcesInfos(),
			resProcessor.DeployablePrevReleaseGeneralResourcesInfos(),
			prevRelease,
			history,
			clientFactory.KubeClient(),
			clientFactory.Static(),
			clientFactory.Dynamic(),
			clientFactory.Discovery(),
			clientFactory.Mapper(),
			plan.UninstallPlanBuilderOptions{
				CreationTimeout:  opts.TrackCreationTimeout,
				DeletionTimeout:  opts.TrackDeletionTimeout,
				ReadinessTimeout: opts.TrackReadinessTimeout,
			},
		)

		uninstallPlan, planBuildErr := uninstallPlanBuilder.Build(ctx)
		if planBuildErr != nil {
			var graphPath string
			if opts.UninstallGraphPath != "" {
				graphPath = opts.UninstallGraphPath
			} else {
				graphPath = filepath.Join(opts.TempDirPath, "release-uninstall-graph.dot")
			}

			if _, err := os.Create(graphPath); err != nil {
				log.Default.Error(ctx, "Error: create release uninstall graph file: %s", err)
				return fmt.Errorf("build uninstall plan: %w", planBuildErr)
			}

			if err := uninstallPlan.SaveDOT(graphPath); err != nil {
				log.Default.Error(ctx, "Error: save release uninstall graph: %s", err)
			}

			log.Default.Warn(ctx, "Release uninstall graph saved to %q for debugging", graphPath)

			return fmt.Errorf("build release uninstall plan: %w", planBuildErr)
		}

		if opts.UninstallGraphPath != "" {
			if err := uninstallPlan.SaveDOT(opts.UninstallGraphPath); err != nil {
				return fmt.Errorf("save release uninstall graph: %w", err)
			}
		}

		tablesBuilder := track.NewTablesBuilder(
			taskStore,
			logStore,
			track.TablesBuilderOptions{
				DefaultNamespace: releaseNamespace,
			},
		)

		log.Default.Debug(ctx, "Starting tracking")
		progressPrinter := newProgressTablePrinter(ctx, opts.ProgressTablePrintInterval, opts.Timeout)

		if !opts.NoProgressTablePrint {
			progressPrinter.Start(func() {
				printTables(ctx, tablesBuilder)
			})
		}

		log.Default.Debug(ctx, "Executing release uninstall plan")
		planExecutor := plan.NewPlanExecutor(
			uninstallPlan,
			plan.PlanExecutorOptions{
				NetworkParallelism: opts.NetworkParallelism,
			},
		)

		var criticalErrs, nonCriticalErrs []error

		planExecutionErr := planExecutor.Execute(ctx)
		if planExecutionErr != nil {
			criticalErrs = append(criticalErrs, fmt.Errorf("execute release uninstall plan: %w", planExecutionErr))
		}

		var worthyCompletedOps []operation.Operation
		if ops, found, err := uninstallPlan.WorthyCompletedOperations(); err != nil {
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful completed operations: %w", err))
		} else if found {
			worthyCompletedOps = ops
		}

		var worthyCanceledOps []operation.Operation
		if ops, found, err := uninstallPlan.WorthyCanceledOperations(); err != nil {
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful canceled operations: %w", err))
		} else if found {
			worthyCanceledOps = ops
		}

		var worthyFailedOps []operation.Operation
		if ops, found, err := uninstallPlan.WorthyFailedOperations(); err != nil {
			nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("get meaningful failed operations: %w", err))
		} else if found {
			worthyFailedOps = ops
		}

		if planExecutionErr != nil {
			wcompops, wfailops, wcancops, criterrs, noncriterrs := runFailureDeployPlan(
				ctx,
				releaseName,
				releaseNamespace,
				common.DeployTypeUninstall,
				uninstallPlan,
				taskStore,
				resProcessor,
				nil,
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

		if !opts.NoProgressTablePrint {
			progressPrinter.Stop()
			progressPrinter.Wait()
		}

		report := newReport(
			worthyCompletedOps,
			worthyCanceledOps,
			worthyFailedOps,
			prevRelease,
		)

		report.Print(ctx)

		if opts.UninstallReportPath != "" {
			if err := report.Save(opts.UninstallReportPath); err != nil {
				nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("save release uninstall report: %w", err))
			}
		}

		if len(criticalErrs) > 0 {
			return util.Multierrorf("uninstall failed for release %q (namespace: %q)", append(criticalErrs, nonCriticalErrs...), releaseName, releaseNamespace)
		} else if len(nonCriticalErrs) > 0 {
			return util.Multierrorf("uninstall succeeded for release %q (namespace: %q), but non-critical errors encountered", nonCriticalErrs, releaseName, releaseNamespace)
		} else {
			log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Uninstalled release %q (namespace: %q)", releaseName, releaseNamespace)))

			return nil
		}

		return nil
	}(); err != nil {
		return err
	}

	if opts.DeleteReleaseNamespace {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Deleting release namespace %q", namespaceID.Name())))

		deleteOp := operation.NewDeleteResourceOperation(
			namespaceID,
			clientFactory.KubeClient(),
			operation.DeleteResourceOperationOptions{},
		)

		if err := deleteOp.Execute(ctx); err != nil {
			return fmt.Errorf("delete release namespace: %w", err)
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Deleted release namespace %q", namespaceID.Name())))
	}

	return nil
}

func isReleaseNamespaceExist(ctx context.Context, clientFactory *kube.ClientFactory, namespaceID *id.ResourceID) (bool, error) {
	if _, err := clientFactory.KubeClient().Get(
		ctx,
		namespaceID,
		kube.KubeClientGetOptions{
			TryCache: true,
		},
	); err != nil {
		if api_errors.IsNotFound(err) {
			return false, nil
		} else {
			return false, fmt.Errorf("get release namespace: %w", err)
		}
	}

	return true, nil
}

func applyReleaseUninstallOptionsDefaults(opts ReleaseUninstallOptions, currentDir, homeDir string) (ReleaseUninstallOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseUninstallOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.KubeConfigBase64 == "" && len(lo.Compact(opts.KubeConfigPaths)) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(homeDir, ".kube", "config")}
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
		return ReleaseUninstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	return opts, nil
}
