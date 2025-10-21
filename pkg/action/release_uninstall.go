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
	"k8s.io/apimachinery/pkg/runtime/schema"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/track"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleaseUninstallLogLevel = log.InfoLevel
)

type ReleaseUninstallOptions struct {
	DeleteReleaseNamespace      bool
	KubeAPIServerAddress        string
	KubeAuthProviderConfig      map[string]string
	KubeAuthProviderName        string
	KubeBasicAuthPassword       string
	KubeBasicAuthUsername       string
	KubeBearerTokenData         string
	KubeBearerTokenPath         string
	KubeBurstLimit              int
	KubeConfigBase64            string
	KubeConfigPaths             []string
	KubeContextCluster          string
	KubeContextCurrent          string
	KubeContextUser             string
	KubeImpersonateGroups       []string
	KubeImpersonateUID          string
	KubeImpersonateUser         string
	KubeProxyURL                string
	KubeQPSLimit                int
	KubeRequestTimeout          string
	KubeSkipTLSVerify           bool
	KubeTLSCAData               string
	KubeTLSCAPath               string
	KubeTLSClientCertData       string
	KubeTLSClientCertPath       string
	KubeTLSClientKeyData        string
	KubeTLSClientKeyPath        string
	KubeTLSServerName           string
	NetworkParallelism          int
	NoFinalTracking             bool
	NoPodLogs                   bool
	NoProgressTablePrint        bool
	NoRemoveManualChanges       bool
	ProgressTablePrintInterval  time.Duration
	ReleaseHistoryLimit         int
	ReleaseStorageDriver        string
	ReleaseStorageSQLConnection string
	TempDirPath                 string
	Timeout                     time.Duration
	TrackCreationTimeout        time.Duration
	TrackDeletionTimeout        time.Duration
	TrackReadinessTimeout       time.Duration
	UninstallGraphPath          string
	UninstallReportPath         string
}

func ReleaseUninstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseUninstallOptions) error {
	ctx, ctxCancelFn := context.WithCancelCause(ctx)

	if opts.Timeout == 0 {
		return releaseUninstall(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
	}

	ctx, _ = context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("context timed out: action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn(fmt.Errorf("context canceled: action finished"))

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- releaseUninstall(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
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

func releaseUninstall(ctx context.Context, ctxCancelFn context.CancelCauseFunc, releaseName, releaseNamespace string, opts ReleaseUninstallOptions) error {
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

	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		APIServerAddress:   opts.KubeAPIServerAddress,
		AuthProviderConfig: opts.KubeAuthProviderConfig,
		AuthProviderName:   opts.KubeAuthProviderName,
		BasicAuthPassword:  opts.KubeBasicAuthPassword,
		BasicAuthUsername:  opts.KubeBasicAuthUsername,
		BearerTokenData:    opts.KubeBearerTokenData,
		BearerTokenPath:    opts.KubeBearerTokenPath,
		BurstLimit:         opts.KubeBurstLimit,
		ContextCluster:     opts.KubeContextCluster,
		ContextCurrent:     opts.KubeContextCurrent,
		ContextNamespace:   releaseNamespace, // TODO: unset it everywhere
		ContextUser:        opts.KubeContextUser,
		ImpersonateGroups:  opts.KubeImpersonateGroups,
		ImpersonateUID:     opts.KubeImpersonateUID,
		ImpersonateUser:    opts.KubeImpersonateUser,
		KubeConfigBase64:   opts.KubeConfigBase64,
		ProxyURL:           opts.KubeProxyURL,
		QPSLimit:           opts.KubeQPSLimit,
		RequestTimeout:     opts.KubeRequestTimeout,
		SkipTLSVerify:      opts.KubeSkipTLSVerify,
		TLSCAData:          opts.KubeTLSCAData,
		TLSCAPath:          opts.KubeTLSCAPath,
		TLSClientCertData:  opts.KubeTLSClientCertData,
		TLSClientCertPath:  opts.KubeTLSClientCertPath,
		TLSClientKeyData:   opts.KubeTLSClientKeyData,
		TLSClientKeyPath:   opts.KubeTLSClientKeyPath,
		TLSServerName:      opts.KubeTLSServerName,
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
	if m, err := lock.NewLockManager(releaseNamespace, false, clientFactory); err != nil {
		return fmt.Errorf("construct lock manager: %w", err)
	} else {
		lockManager = m
	}

	nsMeta := spec.NewResourceMeta(releaseNamespace, "", releaseNamespace, "", schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, nil, nil)

	if exists, err := isReleaseNamespaceExist(ctx, clientFactory, nsMeta); err != nil {
		return fmt.Errorf("check release namespace existence: %w", err)
	} else if !exists {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q uninstall: no release namespace %q found", releaseName, releaseNamespace)))

		return nil
	}

	if err := func() error {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Delete release")+" %q (namespace: %q)", releaseName, releaseNamespace)

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
			log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q (namespace: %q) uninstall: no release found", releaseName, releaseNamespace)))

			return nil
		}

		prevRelease := lo.LastOrEmpty(releases)
		prevReleaseFailed := prevRelease.IsStatusFailed()
		deployType := common.DeployTypeUninstall

		log.Default.Debug(ctx, "Convert previous release to resource specs")
		prevRelResSpecs, err := release.ReleaseToResourceSpecs(prevRelease, releaseNamespace)
		if err != nil {
			return fmt.Errorf("convert previous release to resource specs: %w", err)
		}

		log.Default.Debug(ctx, "Build resources")
		instResources, delResources, err := resource.BuildResources(ctx, deployType, releaseNamespace, prevRelResSpecs, nil, []spec.ResourcePatcher{
			spec.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
		}, clientFactory, resource.BuildResourcesOptions{
			Remote: true,
		})
		if err != nil {
			return fmt.Errorf("build resources: %w", err)
		}

		log.Default.Debug(ctx, "Build resource infos")
		instResInfos, delResInfos, err := plan.BuildResourceInfos(ctx, deployType, releaseName, releaseNamespace, instResources, delResources, prevReleaseFailed, clientFactory, plan.BuildResourceInfosOptions{
			NetworkParallelism:    opts.NetworkParallelism,
			NoRemoveManualChanges: opts.NoRemoveManualChanges,
		})
		if err != nil {
			return fmt.Errorf("build resource infos: %w", err)
		}

		log.Default.Debug(ctx, "Build release infos")
		relInfos, err := plan.BuildReleaseInfos(ctx, deployType, releases, nil)
		if err != nil {
			return fmt.Errorf("build release infos: %w", err)
		}

		log.Default.Debug(ctx, "Build delete plan")
		deletePlan, err := plan.BuildPlan(instResInfos, delResInfos, relInfos, plan.BuildPlanOptions{
			NoFinalTracking: opts.NoFinalTracking,
		})
		if err != nil {
			handleBuildPlanErr(ctx, deletePlan, err, opts.UninstallGraphPath, opts.TempDirPath, "release-uninstall-graph.dot")
			return fmt.Errorf("build delete plan: %w", err)
		}

		if opts.UninstallGraphPath != "" {
			if err := savePlanAsDot(deletePlan, opts.UninstallGraphPath); err != nil {
				return fmt.Errorf("save release delete graph: %w", err)
			}
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

		log.Default.Debug(ctx, "Execute release delete plan")
		executePlanErr := plan.ExecutePlan(ctx, releaseNamespace, deletePlan, taskStore, logStore, informerFactory, history, clientFactory, plan.ExecutePlanOptions{
			NetworkParallelism:    opts.NetworkParallelism,
			TrackCreationTimeout:  opts.TrackCreationTimeout,
			TrackDeletionTimeout:  opts.TrackDeletionTimeout,
			TrackReadinessTimeout: opts.TrackReadinessTimeout,
		})
		if executePlanErr != nil {
			criticalErrs = append(criticalErrs, fmt.Errorf("execute release delete plan: %w", executePlanErr))
		}

		resourceOps := lo.Filter(deletePlan.Operations(), func(op *plan.Operation, _ int) bool {
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
			runFailurePlanResult, nonCritErrs, critErrs := runFailurePlan(ctx, releaseNamespace, deletePlan, instResInfos, relInfos, taskStore, logStore, informerFactory, history, clientFactory, runFailureInstallPlanOptions{
				NetworkParallelism:    opts.NetworkParallelism,
				NoFinalTracking:       opts.NoFinalTracking,
				TrackReadinessTimeout: opts.TrackReadinessTimeout,
				TrackCreationTimeout:  opts.TrackCreationTimeout,
				TrackDeletionTimeout:  opts.TrackDeletionTimeout,
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
			Revision:            prevRelease.Version,
			Status:              helmrelease.StatusUninstalled,
			CompletedOperations: reportCompletedOps,
			CanceledOperations:  reportCanceledOps,
			FailedOperations:    reportFailedOps,
		}

		printReport(ctx, report)

		if opts.UninstallReportPath != "" {
			if err := saveReport(opts.UninstallReportPath, report); err != nil {
				nonCriticalErrs = append(nonCriticalErrs, fmt.Errorf("save release delete report: %w", err))
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
	}(); err != nil {
		return err
	}

	if opts.DeleteReleaseNamespace {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Delete release namespace %q", nsMeta.Name)))

		if err := clientFactory.KubeClient().Delete(ctx, nsMeta, kube.KubeClientDeleteOptions{}); err != nil {
			return fmt.Errorf("delete release namespace: %w", err)
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Deleted release namespace %q", nsMeta.Name)))
	}

	return nil
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

	switch opts.ReleaseStorageDriver {
	case ReleaseStorageDriverDefault:
		opts.ReleaseStorageDriver = ReleaseStorageDriverSecrets
	case ReleaseStorageDriverMemory:
		return ReleaseUninstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	return opts, nil
}

func isReleaseNamespaceExist(ctx context.Context, clientFactory *kube.ClientFactory, nsMeta *spec.ResourceMeta) (bool, error) {
	if _, err := clientFactory.KubeClient().Get(ctx, nsMeta, kube.KubeClientGetOptions{
		TryCache: true,
	}); err != nil {
		if kube.IsNotFoundErr(err) {
			return false, nil
		} else {
			return false, fmt.Errorf("get release namespace: %w", err)
		}
	}

	return true, nil
}
