package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"

	"github.com/samber/lo"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
	"github.com/werf/3p-helm/pkg/action"
	"github.com/werf/3p-helm/pkg/chart/loader"
	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/3p-helm/pkg/downloader"
	"github.com/werf/3p-helm/pkg/getter"
	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/storage"
	"github.com/werf/3p-helm/pkg/storage/driver"
	"github.com/werf/3p-helm/pkg/werf/chartextender"
	"github.com/werf/3p-helm/pkg/werf/secrets"
	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/logboek"

	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/nelm/pkg/chrttree"
	helmcommon "github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcpatcher"
	"github.com/werf/nelm/pkg/resrcprocssr"
	"github.com/werf/nelm/pkg/rlshistor"
)

const (
	DefaultChartLintLogLevel = log.InfoLevel
)

type ChartLintOptions struct {
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
	Local                        bool
	LocalKubeVersion             string
	LogColorMode                 LogColorMode
	LogLevel                     log.Level
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	RegistryCredentialsPath      string
	ReleaseName                  string
	ReleaseNamespace             string
	ReleaseStorageDriver         ReleaseStorageDriver
	SecretKey                    string
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	SecretWorkDir                string
	TempDirPath                  string
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
}

func ChartLint(ctx context.Context, opts ChartLintOptions) error {
	actionLock.Lock()
	defer actionLock.Unlock()

	if opts.LogLevel != "" {
		log.Default.SetLevel(ctx, opts.LogLevel)
	} else {
		log.Default.SetLevel(ctx, DefaultChartLintLogLevel)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	opts, err = applyChartLintOptionsDefaults(opts, currentDir, currentUser)
	if err != nil {
		return fmt.Errorf("build chart lint options: %w", err)
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
	helmSettings.Debug = log.Default.AcceptLevel(ctx, log.DebugLevel)

	if opts.KubeContext != "" {
		helmSettings.KubeContext = opts.KubeContext
	}

	if kubeConfigPath != "" {
		helmSettings.KubeConfig = kubeConfigPath
	}

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(log.Default.AcceptLevel(ctx, log.DebugLevel)),
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
		opts.ReleaseNamespace,
		string(opts.ReleaseStorageDriver),
		func(format string, a ...interface{}) {
			log.Default.Debug(ctx, format, a...)
		},
	); err != nil {
		return fmt.Errorf("helm action config init: %w", err)
	}
	helmActionConfig.RegistryClient = helmRegistryClient

	var clientFactory *kubeclnt.ClientFactory
	if opts.Local {
		helmReleaseStorageDriver := driver.NewMemory()
		helmReleaseStorageDriver.SetNamespace(opts.ReleaseNamespace)
		helmActionConfig.Releases = storage.Init(helmReleaseStorageDriver)
		helmActionConfig.Capabilities = chartutil.DefaultCapabilities.Copy()

		kubeVersion, err := chartutil.ParseKubeVersion(opts.LocalKubeVersion)
		if err != nil {
			return fmt.Errorf("parse local kube version %q: %w", opts.LocalKubeVersion, err)
		}

		helmActionConfig.Capabilities.KubeVersion = *kubeVersion
	} else {
		clientFactory, err = kubeclnt.NewClientFactory()
		if err != nil {
			return fmt.Errorf("construct kube client factory: %w", err)
		}
	}

	helmReleaseStorage := helmActionConfig.Releases

	helmChartPathOptions := action.ChartPathOptions{
		InsecureSkipTLSverify: opts.ChartRepositorySkipTLSVerify,
		PlainHTTP:             opts.ChartRepositoryInsecure,
	}
	helmChartPathOptions.SetRegistryClient(helmRegistryClient)

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

	var historyOptions rlshistor.HistoryOptions
	if !opts.Local {
		historyOptions.Mapper = clientFactory.Mapper()
		historyOptions.DiscoveryClient = clientFactory.Discovery()
	}

	history, err := rlshistor.NewHistory(
		opts.ReleaseName,
		opts.ReleaseNamespace,
		helmReleaseStorage,
		historyOptions,
	)
	if err != nil {
		return fmt.Errorf("construct release history: %w", err)
	}

	prevRelease, prevReleaseFound, err := history.LastRelease()
	if err != nil {
		return fmt.Errorf("get last release: %w", err)
	}

	_, prevDeployedReleaseFound, err := history.LastDeployedRelease()
	if err != nil {
		return fmt.Errorf("get last deployed release: %w", err)
	}

	var newRevision int
	if prevReleaseFound {
		newRevision = prevRelease.Revision() + 1
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

	chartTreeOptions := chrttree.ChartTreeOptions{
		StringSetValues: opts.ValuesStringSets,
		SetValues:       opts.ValuesSets,
		FileValues:      opts.ValuesFileSets,
		ValuesFiles:     opts.ValuesFilesPaths,
	}
	if !opts.Local {
		chartTreeOptions.Mapper = clientFactory.Mapper()
		chartTreeOptions.DiscoveryClient = clientFactory.Discovery()
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

	chartTree, err := chrttree.NewChartTree(
		ctx,
		opts.ChartDirPath,
		opts.ReleaseName,
		opts.ReleaseNamespace,
		newRevision,
		deployType,
		helmActionConfig,
		chartTreeOptions,
	)
	if err != nil {
		return fmt.Errorf("construct chart tree: %w", err)
	}

	var prevRelGeneralResources []*resrc.GeneralResource
	if prevReleaseFound {
		prevRelGeneralResources = prevRelease.GeneralResources()
	}

	resProcessorOptions := resrcprocssr.DeployableResourcesProcessorOptions{
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
	}
	if !opts.Local {
		resProcessorOptions.KubeClient = clientFactory.KubeClient()
		resProcessorOptions.Mapper = clientFactory.Mapper()
		resProcessorOptions.DiscoveryClient = clientFactory.Discovery()
		resProcessorOptions.AllowClusterAccess = true
	}

	resProcessor := resrcprocssr.NewDeployableResourcesProcessor(
		deployType,
		opts.ReleaseName,
		opts.ReleaseNamespace,
		chartTree.StandaloneCRDs(),
		chartTree.HookResources(),
		chartTree.GeneralResources(),
		prevRelGeneralResources,
		resProcessorOptions,
	)

	if err := resProcessor.Process(ctx); err != nil {
		return fmt.Errorf("process resources: %w", err)
	}

	return nil
}

func applyChartLintOptionsDefaults(opts ChartLintOptions, currentDir string, currentUser *user.User) (ChartLintOptions, error) {
	if opts.ChartDirPath == "" {
		opts.ChartDirPath = currentDir
	}

	if opts.ReleaseName == "" {
		opts.ReleaseName = StubReleaseName
	}

	if opts.ReleaseNamespace == "" {
		opts.ReleaseNamespace = StubReleaseNamespace
	}

	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ChartLintOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
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

	if opts.ReleaseName == "" {
		return ChartLintOptions{}, fmt.Errorf("release name not specified")
	}

	if opts.ReleaseStorageDriver == ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = ReleaseStorageDriverSecrets
	}

	if opts.SecretWorkDir == "" {
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return ChartLintOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	if opts.LocalKubeVersion == "" {
		// TODO(v3): update default local version
		opts.LocalKubeVersion = DefaultLocalKubeVersion
	}

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = DefaultRegistryCredentialsPath
	}

	return opts, nil
}
