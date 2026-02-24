package action

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	helmv3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/cli"
	helmkube "github.com/werf/3p-helm/pkg/kube"
	"github.com/werf/3p-helm/pkg/storage/driver"
	"github.com/werf/kubedog/pkg/display"
	kdkube "github.com/werf/kubedog/pkg/kube"
	"github.com/werf/logboek"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/legacy/deploy"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const DefaultLegacyReleaseUninstallLogLevel = log.InfoLevel

var legacyUninstallLock sync.Mutex

type LegacyReleaseUninstallOptions struct {
	common.KubeConnectionOptions
	common.TrackingOptions

	DeleteReleaseNamespace bool
	NetworkParallelism     int
	NoDeleteHooks          bool
	ReleaseHistoryLimit    int
	ReleaseStorageDriver   string
	TempDirPath            string
	Timeout                time.Duration
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

	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		KubeConnectionOptions: opts.KubeConnectionOptions,
		KubeContextNamespace:  releaseNamespace,
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
	helmSettings.Debug = log.Default.AcceptLevel(ctx, log.Level(log.DebugLevel))

	if opts.KubeContextCurrent != "" {
		helmSettings.KubeContext = opts.KubeContextCurrent
	}

	var kubeConfigPath string
	if len(opts.KubeConfigPaths) > 0 {
		kubeConfigPath = opts.KubeConfigPaths[0]
	}

	helmSettings.KubeConfig = kubeConfigPath

	if err := kdkube.Init(kdkube.InitOptions{
		KubeConfigOptions: kdkube.KubeConfigOptions{
			Context:             opts.KubeContextCurrent,
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

	helmKubeClient := helmActionConfig.KubeClient.(*helmkube.Client)
	helmKubeClient.Namespace = releaseNamespace
	helmKubeClient.ResourcesWaiter = deploy.NewResourcesWaiter(
		helmKubeClient,
		time.Now(),
		opts.ProgressTablePrintInterval,
		opts.ProgressTablePrintInterval,
	)

	nsMeta := spec.NewResourceMeta(releaseNamespace, "", releaseNamespace, "", schema.GroupVersionKind{Version: "v1", Kind: "Namespace"}, nil, nil)

	if _, err := clientFactory.KubeClient().Get(
		ctx,
		nsMeta,
		kube.KubeClientGetOptions{
			TryCache: true,
		},
	); err != nil {
		if apierrors.IsNotFound(err) {
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
		if m, err := lock.NewLockManager(ctx, releaseNamespace, false, clientFactory); err != nil {
			return fmt.Errorf("construct lock manager: %w", err)
		} else {
			lockManager = m
		}

		if lock, err := lockManager.LockRelease(ctx, releaseName); err != nil {
			return fmt.Errorf("lock release: %w", err)
		} else {
			defer lockManager.Unlock(lock)
		}

		helmUninstallCmd := helmv3.NewUninstallCmd(
			helmActionConfig,
			os.Stdout,
			helmv3.UninstallCmdOptions{
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
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Deleting release namespace %q", releaseNamespace)))

		if err := clientFactory.KubeClient().Delete(ctx, nsMeta, kube.KubeClientDeleteOptions{}); err != nil {
			return fmt.Errorf("delete release namespace: %w", err)
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Deleted release namespace %q", releaseNamespace)))
	}

	return nil
}

func initKubedog(ctx context.Context) error {
	flag.CommandLine.Parse([]string{})

	display.SetOut(os.Stdout)
	display.SetErr(os.Stderr)

	if err := silenceKlog(ctx); err != nil {
		return fmt.Errorf("silence klog: %w", err)
	}

	if err := silenceKlogV2(ctx); err != nil {
		return fmt.Errorf("silence klog v2: %w", err)
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

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)
	opts.TrackingOptions.ApplyDefaults()

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = common.DefaultNetworkParallelism
	}

	if opts.ReleaseHistoryLimit <= 0 {
		opts.ReleaseHistoryLimit = common.DefaultReleaseHistoryLimit
	}

	if opts.ReleaseStorageDriver == common.ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
	} else if opts.ReleaseStorageDriver == common.ReleaseStorageDriverMemory {
		return LegacyReleaseUninstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	return opts, nil
}

func silenceKlog(ctx context.Context) error {
	fs := flag.NewFlagSet("klog", flag.PanicOnError)
	klog.InitFlags(fs)

	if err := fs.Set("logtostderr", "false"); err != nil {
		return fmt.Errorf("set logtostderr: %w", err)
	}

	if err := fs.Set("alsologtostderr", "false"); err != nil {
		return fmt.Errorf("set alsologtostderr: %w", err)
	}

	if err := fs.Set("stderrthreshold", "5"); err != nil {
		return fmt.Errorf("set stderrthreshold: %w", err)
	}

	// Suppress info and warnings from client-go reflector
	klog.SetOutputBySeverity("INFO", ioutil.Discard)
	klog.SetOutputBySeverity("WARNING", ioutil.Discard)
	klog.SetOutputBySeverity("ERROR", ioutil.Discard)
	klog.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())

	return nil
}

func silenceKlogV2(ctx context.Context) error {
	fs := flag.NewFlagSet("klog", flag.PanicOnError)
	klogv2.InitFlags(fs)

	if err := fs.Set("logtostderr", "false"); err != nil {
		return fmt.Errorf("set logtostderr: %w", err)
	}

	if err := fs.Set("alsologtostderr", "false"); err != nil {
		return fmt.Errorf("set alsologtostderr: %w", err)
	}

	if err := fs.Set("stderrthreshold", "5"); err != nil {
		return fmt.Errorf("set stderrthreshold: %w", err)
	}

	// Suppress info and warnings from client-go reflector
	klogv2.SetOutputBySeverity("INFO", ioutil.Discard)
	klogv2.SetOutputBySeverity("WARNING", ioutil.Discard)
	klogv2.SetOutputBySeverity("ERROR", ioutil.Discard)
	klogv2.SetOutputBySeverity("FATAL", logboek.Context(ctx).ErrStream())

	return nil
}
