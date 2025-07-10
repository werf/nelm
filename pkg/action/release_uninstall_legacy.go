package action

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/cli"
	helm_kube "github.com/werf/3p-helm/pkg/kube"
	"github.com/werf/3p-helm/pkg/storage/driver"
	kdkube "github.com/werf/kubedog/pkg/kube"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/legacy/deploy"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultLegacyReleaseUninstallLogLevel = InfoLogLevel
)

var legacyUninstallLock sync.Mutex

type LegacyReleaseUninstallOptions struct {
	NoDeleteHooks              bool
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
	ProgressTablePrintInterval time.Duration
	ReleaseHistoryLimit        int
	ReleaseStorageDriver       string
	TempDirPath                string
	Timeout                    time.Duration
}

func LegacyReleaseUninstall(ctx context.Context, releaseName, releaseNamespace string, opts LegacyReleaseUninstallOptions) error {
	legacyUninstallLock.Lock()
	defer legacyUninstallLock.Unlock()

	if opts.Timeout == 0 {
		return legacyReleaseUninstall(ctx, releaseName, releaseNamespace, opts)
	}

	ctx, ctxCancelFn := context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("context timed out: action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn()

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- legacyReleaseUninstall(ctx, releaseName, releaseNamespace, opts)
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

func legacyReleaseUninstall(ctx context.Context, releaseName, releaseNamespace string, opts LegacyReleaseUninstallOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyLegacyReleaseUninstallOptionsDefaults(opts, currentDir, homeDir)
	if err != nil {
		return fmt.Errorf("build legacy release uninstall options: %w", err)
	}

	if len(opts.KubeConfigPaths) > 0 {
		var splitPaths []string
		for _, path := range opts.KubeConfigPaths {
			splitPaths = append(splitPaths, filepath.SplitList(path)...)
		}

		opts.KubeConfigPaths = lo.Compact(splitPaths)

		// Don't even ask... This way we force ClientConfigLoadingRules.ExplicitPath to always be
		// empty, otherwise KUBECONFIG with multiple files doesn't work. Eventually should switch
		// from Kubedog to Nelm for initializing K8s Clients like in other actions and get rid of
		// this.
		opts.KubeConfigPaths = append([]string{""}, opts.KubeConfigPaths...)
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

	helmSettings := cli.New()
	*helmSettings.GetConfigP() = clientFactory.LegacyClientGetter()
	*helmSettings.GetNamespaceP() = releaseNamespace
	releaseNamespace = helmSettings.Namespace()
	helmSettings.MaxHistory = opts.ReleaseHistoryLimit
	helmSettings.Debug = log.Default.AcceptLevel(ctx, log.Level(DebugLogLevel))

	if opts.KubeContext != "" {
		helmSettings.KubeContext = opts.KubeContext
	}

	var kubeConfigPath string
	if len(opts.KubeConfigPaths) > 0 {
		kubeConfigPath = opts.KubeConfigPaths[0]
	}

	helmSettings.KubeConfig = kubeConfigPath

	if err := kdkube.Init(kdkube.InitOptions{
		KubeConfigOptions: kdkube.KubeConfigOptions{
			Context:             opts.KubeContext,
			ConfigPath:          kubeConfigPath,
			ConfigDataBase64:    opts.KubeConfigBase64,
			ConfigPathMergeList: opts.KubeConfigPaths,
		},
	}); err != nil {
		return fmt.Errorf("initialize kubedog kube client: %w", err)
	}

	if err := initKubedog(ctx); err != nil {
		return fmt.Errorf("initialize kubedog: %w", err)
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

	helmReleaseStorage := helmActionConfig.Releases
	helmReleaseStorage.MaxHistory = opts.ReleaseHistoryLimit

	helmKubeClient := helmActionConfig.KubeClient.(*helm_kube.Client)
	helmKubeClient.Namespace = releaseNamespace
	helmKubeClient.ResourcesWaiter = deploy.NewResourcesWaiter(
		helmKubeClient,
		time.Now(),
		opts.ProgressTablePrintInterval,
		opts.ProgressTablePrintInterval,
	)

	namespaceID := id.NewResourceID(
		releaseNamespace,
		"",
		schema.GroupVersionKind{Version: "v1", Kind: "Namespace"},
		id.ResourceIDOptions{Mapper: clientFactory.Mapper()},
	)

	if _, err := clientFactory.KubeClient().Get(
		ctx,
		namespaceID,
		kube.KubeClientGetOptions{
			TryCache: true,
		},
	); err != nil {
		if api_errors.IsNotFound(err) {
			log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q removal: no release namespace %q found", releaseName, releaseNamespace)))

			return nil
		} else {
			return fmt.Errorf("get release namespace: %w", err)
		}
	}

	if err := func() error {
		var releaseFound bool
		if _, err := helmActionConfig.Releases.History(releaseName); err != nil {
			if !errors.Is(err, driver.ErrReleaseNotFound) {
				return fmt.Errorf("get release history: %w", err)
			}
		} else {
			releaseFound = true
		}

		if !releaseFound {
			log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q (namespace: %q) uninstall: no release found", releaseName, releaseNamespace)))

			return nil
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Deleting release")+" %q (namespace: %q)", releaseName, releaseNamespace)

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

		if lock, err := lockManager.LockRelease(ctx, releaseName); err != nil {
			return fmt.Errorf("lock release: %w", err)
		} else {
			defer lockManager.Unlock(lock)
		}

		helmUninstallCmd := helm_v3.NewUninstallCmd(
			helmActionConfig,
			os.Stdout,
			helm_v3.UninstallCmdOptions{
				StagesSplitter:      deploy.NewStagesSplitter(),
				DeleteHooks:         lo.ToPtr(!opts.NoDeleteHooks),
				DontFailIfNoRelease: lo.ToPtr(true),
			},
		)

		if err := helmUninstallCmd.RunE(helmUninstallCmd, []string{releaseName}); err != nil {
			return fmt.Errorf("run uninstall command: %w", err)
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Uninstalled release %q (namespace: %q)", releaseName, releaseNamespace)))

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

func applyLegacyReleaseUninstallOptionsDefaults(opts LegacyReleaseUninstallOptions, currentDir, homeDir string) (LegacyReleaseUninstallOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return LegacyReleaseUninstallOptions{}, fmt.Errorf("create temp dir: %w", err)
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
		return LegacyReleaseUninstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	return opts, nil
}
