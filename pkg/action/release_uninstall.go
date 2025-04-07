package action

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	helm_kube "github.com/werf/3p-helm/pkg/kube"
	"github.com/werf/3p-helm/pkg/storage/driver"
	kdkube "github.com/werf/kubedog/pkg/kube"
	"github.com/werf/logboek"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/legacy/deploy"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/log"
	"github.com/werf/nelm/internal/plan/operation"
	"github.com/werf/nelm/internal/resource/id"
)

const (
	DefaultReleaseUninstallLogLevel = InfoLogLevel
)

type ReleaseUninstallOptions struct {
	DeleteHooks                bool
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
	LogColorMode               string
	NetworkParallelism         int
	ProgressTablePrintInterval time.Duration
	ReleaseHistoryLimit        int
	ReleaseStorageDriver       string
	TempDirPath                string
}

func ReleaseUninstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseUninstallOptions) error {
	actionLock.Lock()
	defer actionLock.Unlock()

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	opts, err = applyReleaseUninstallOptionsDefaults(opts, currentDir, currentUser)
	if err != nil {
		return fmt.Errorf("build release uninstall options: %w", err)
	}

	var kubeConfigPath string
	if len(opts.KubeConfigPaths) > 0 {
		kubeConfigPath = opts.KubeConfigPaths[0]
	}

	kubeConfigGetter, err := kdkube.NewKubeConfigGetter(
		kdkube.KubeConfigGetterOptions{
			KubeConfigOptions: kdkube.KubeConfigOptions{
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

	clientFactory, err := kube.NewClientFactory()
	if err != nil {
		return fmt.Errorf("construct kube client factory: %w", err)
	}

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
			logboek.Context(ctx).OutStream(),
			helm_v3.UninstallCmdOptions{
				StagesSplitter:      deploy.NewStagesSplitter(),
				DeleteHooks:         lo.ToPtr(opts.DeleteHooks),
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

func applyReleaseUninstallOptionsDefaults(opts ReleaseUninstallOptions, currentDir string, currentUser *user.User) (ReleaseUninstallOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseUninstallOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.KubeConfigBase64 == "" && len(opts.KubeConfigPaths) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(currentUser.HomeDir, ".kube", "config")}
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
		return ReleaseUninstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	return opts, nil
}
