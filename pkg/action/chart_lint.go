package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/samber/lo"
	"k8s.io/cli-runtime/pkg/genericclioptions"

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
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/log"
	"github.com/werf/nelm/internal/plan/resourceinfo"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
)

const (
	DefaultChartLintLogLevel = InfoLogLevel
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
	Remote                       bool
	LocalKubeVersion             string
	LogColorMode                 string
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	RegistryCredentialsPath      string
	ReleaseName                  string
	ReleaseNamespace             string
	ReleaseStorageDriver         string
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

	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyChartLintOptionsDefaults(opts, currentDir, homeDir)
	if err != nil {
		return fmt.Errorf("build chart lint options: %w", err)
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
	}

	var clientFactory *kube.ClientFactory
	var restClientGetter genericclioptions.RESTClientGetter
	if opts.Remote {
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
			Namespace:             opts.ReleaseNamespace,
			QPSLimit:              opts.KubeQPSLimit,
			Server:                opts.KubeAPIServerName,
			TLSServerName:         opts.KubeTLSServerName,
			Token:                 opts.KubeToken,
		})
		if err != nil {
			return fmt.Errorf("construct kube config: %w", err)
		}

		clientFactory, err = kube.NewClientFactory(ctx, kubeConfig)
		if err != nil {
			return fmt.Errorf("construct kube client factory: %w", err)
		}

		restClientGetter = clientFactory.LegacyClientGetter()
	}

	helmSettings := helm_v3.Settings
	helmSettings.Debug = log.Default.AcceptLevel(ctx, log.Level(DebugLogLevel))

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

	helmActionConfig := &action.Configuration{}
	if err := helmActionConfig.Init(
		restClientGetter,
		opts.ReleaseNamespace,
		string(opts.ReleaseStorageDriver),
		func(format string, a ...interface{}) {
			log.Default.Debug(ctx, format, a...)
		},
	); err != nil {
		return fmt.Errorf("helm action config init: %w", err)
	}

	if !opts.Remote {
		helmReleaseStorageDriver := driver.NewMemory()
		helmReleaseStorageDriver.SetNamespace(opts.ReleaseNamespace)
		helmActionConfig.Releases = storage.Init(helmReleaseStorageDriver)
		helmActionConfig.Capabilities = chartutil.DefaultCapabilities.Copy()

		kubeVersion, err := chartutil.ParseKubeVersion(opts.LocalKubeVersion)
		if err != nil {
			return fmt.Errorf("parse local kube version %q: %w", opts.LocalKubeVersion, err)
		}

		helmActionConfig.Capabilities.KubeVersion = *kubeVersion
	}

	helmReleaseStorage := helmActionConfig.Releases

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

	var historyOptions release.HistoryOptions
	if opts.Remote {
		historyOptions.Mapper = clientFactory.Mapper()
		historyOptions.DiscoveryClient = clientFactory.Discovery()
	}

	history, err := release.NewHistory(
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

	var deployType common.DeployType
	if prevReleaseFound && prevDeployedReleaseFound {
		deployType = common.DeployTypeUpgrade
	} else if prevReleaseFound {
		deployType = common.DeployTypeInstall
	} else {
		deployType = common.DeployTypeInitial
	}

	chartTreeOptions := chart.ChartTreeOptions{
		StringSetValues: opts.ValuesStringSets,
		SetValues:       opts.ValuesSets,
		FileValues:      opts.ValuesFileSets,
		ValuesFiles:     opts.ValuesFilesPaths,
	}
	if opts.Remote {
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

	chartTree, err := chart.NewChartTree(
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

	var prevRelGeneralResources []*resource.GeneralResource
	if prevReleaseFound {
		prevRelGeneralResources = prevRelease.GeneralResources()
	}

	resProcessorOptions := resourceinfo.DeployableResourcesProcessorOptions{
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
	}
	if opts.Remote {
		resProcessorOptions.KubeClient = clientFactory.KubeClient()
		resProcessorOptions.Mapper = clientFactory.Mapper()
		resProcessorOptions.DiscoveryClient = clientFactory.Discovery()
		resProcessorOptions.AllowClusterAccess = true
	}

	resProcessor := resourceinfo.NewDeployableResourcesProcessor(
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

func applyChartLintOptionsDefaults(opts ChartLintOptions, currentDir, homeDir string) (ChartLintOptions, error) {
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

	if opts.KubeConfigBase64 == "" && len(lo.Compact(opts.KubeConfigPaths)) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(homeDir, ".kube", "config")}
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
