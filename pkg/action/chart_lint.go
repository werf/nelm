package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/samber/lo"

	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultChartLintLogLevel = log.InfoLevel
)

type ChartLintOptions struct {
	Chart                       string
	ChartAppVersion             string
	ChartDirPath                string // TODO(v2): get rid
	ChartProvenanceKeyring      string
	ChartProvenanceStrategy     string
	ChartRepoBasicAuthPassword  string
	ChartRepoBasicAuthUsername  string
	ChartRepoCAPath             string
	ChartRepoCertPath           string
	ChartRepoInsecure           bool
	ChartRepoKeyPath            string
	ChartRepoPassCreds          bool
	ChartRepoRequestTimeout     time.Duration
	ChartRepoSkipTLSVerify      bool
	ChartRepoSkipUpdate         bool
	ChartRepoURL                string
	ChartVersion                string
	DefaultChartAPIVersion      string
	DefaultChartName            string
	DefaultChartVersion         string
	DefaultSecretValuesDisable  bool
	DefaultValuesDisable        bool
	ExtraAPIVersions            []string
	ExtraAnnotations            map[string]string
	ExtraLabels                 map[string]string
	ExtraRuntimeAnnotations     map[string]string
	ExtraRuntimeLabels          map[string]string
	ForceAdoption               bool
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
	LegacyChartType             helmopts.ChartType
	LegacyExtraValues           map[string]interface{}
	LegacyLogRegistryStreamOut  io.Writer
	LocalKubeVersion            string
	NetworkParallelism          int
	NoFinalTracking             bool
	NoRemoveManualChanges       bool
	RegistryCredentialsPath     string
	ReleaseName                 string
	ReleaseNamespace            string
	ReleaseStorageDriver        string
	ReleaseStorageSQLConnection string
	Remote                      bool
	RuntimeSetJSON              []string
	SecretKey                   string
	SecretKeyIgnore             bool
	SecretValuesFiles           []string
	SecretWorkDir               string
	TempDirPath                 string
	TemplatesAllowDNS           bool
	ValuesFiles                 []string
	ValuesSet                   []string
	ValuesSetFile               []string
	ValuesSetJSON               []string
	ValuesSetLiteral            []string
	ValuesSetString             []string
}

func ChartLint(ctx context.Context, opts ChartLintOptions) error {
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
		lo.Must0(os.Setenv("WERF_SECRET_KEY", opts.SecretKey))
	}

	if !opts.Remote {
		opts.ReleaseStorageDriver = ReleaseStorageDriverMemory
	}

	var clientFactory *kube.ClientFactory
	if opts.Remote {
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
			ContextNamespace:   opts.ReleaseNamespace, // TODO: unset it everywhere
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

		clientFactory, err = kube.NewClientFactory(ctx, kubeConfig)
		if err != nil {
			return fmt.Errorf("construct kube client factory: %w", err)
		}
	}

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(log.Default.AcceptLevel(ctx, log.DebugLevel)),
		registry.ClientOptWriter(opts.LegacyLogRegistryStreamOut),
		registry.ClientOptCredentialsFile(opts.RegistryCredentialsPath),
	}

	if opts.ChartRepoInsecure {
		helmRegistryClientOpts = append(
			helmRegistryClientOpts,
			registry.ClientOptPlainHTTP(),
		)
	}

	helmRegistryClient, err := registry.NewClient(helmRegistryClientOpts...)
	if err != nil {
		return fmt.Errorf("construct registry client: %w", err)
	}

	releaseStorageOptions := release.ReleaseStorageOptions{
		SQLConnection: opts.ReleaseStorageSQLConnection,
	}

	releaseStorage, err := release.NewReleaseStorage(ctx, opts.ReleaseNamespace, opts.ReleaseStorageDriver, clientFactory, releaseStorageOptions)
	if err != nil {
		return fmt.Errorf("construct release storage: %w", err)
	}

	helmOptions := helmopts.HelmOptions{
		ChartLoadOpts: helmopts.ChartLoadOptions{
			ChartAppVersion:            opts.ChartAppVersion,
			ChartType:                  opts.LegacyChartType,
			DefaultChartAPIVersion:     opts.DefaultChartAPIVersion,
			DefaultChartName:           opts.DefaultChartName,
			DefaultChartVersion:        opts.DefaultChartVersion,
			DefaultSecretValuesDisable: opts.DefaultSecretValuesDisable,
			DefaultValuesDisable:       opts.DefaultValuesDisable,
			ExtraValues:                opts.LegacyExtraValues,
			NoDecryptSecrets:           opts.SecretKeyIgnore,
			SecretValuesFiles:          opts.SecretValuesFiles,
			SecretsWorkingDir:          opts.SecretWorkDir,
		},
	}

	log.Default.Debug(ctx, "Build release history")

	history, err := release.BuildHistory(opts.ReleaseName, releaseStorage, release.HistoryOptions{})
	if err != nil {
		return fmt.Errorf("build release history: %w", err)
	}

	releases := history.Releases()
	deployedReleases := history.FindAllDeployed()
	prevRelease := lo.LastOrEmpty(releases)
	prevDeployedRelease := lo.LastOrEmpty(deployedReleases)

	var (
		newRevision       int
		prevReleaseFailed bool
	)

	if prevRelease != nil {
		newRevision = prevRelease.Version + 1
		prevReleaseFailed = prevRelease.IsStatusFailed()
	} else {
		newRevision = 1
	}

	var deployType common.DeployType
	if prevDeployedRelease != nil {
		deployType = common.DeployTypeUpgrade
	} else if prevRelease != nil {
		deployType = common.DeployTypeInstall
	} else {
		deployType = common.DeployTypeInitial
	}

	chartTreeOptions := chart.RenderChartOptions{
		ChartProvenanceKeyring:     opts.ChartProvenanceKeyring,
		ChartProvenanceStrategy:    opts.ChartProvenanceStrategy,
		ChartRepoBasicAuthPassword: opts.ChartRepoBasicAuthPassword,
		ChartRepoBasicAuthUsername: opts.ChartRepoBasicAuthUsername,
		ChartRepoCAPath:            opts.ChartRepoCAPath,
		ChartRepoCertPath:          opts.ChartRepoCertPath,
		ChartRepoInsecure:          opts.ChartRepoInsecure,
		ChartRepoKeyPath:           opts.ChartRepoKeyPath,
		ChartRepoNoTLSVerify:       opts.ChartRepoSkipTLSVerify,
		ChartRepoNoUpdate:          opts.ChartRepoSkipUpdate,
		ChartRepoPassCreds:         opts.ChartRepoPassCreds,
		ChartRepoRequestTimeout:    opts.ChartRepoRequestTimeout,
		ChartRepoURL:               opts.ChartRepoURL,
		ChartVersion:               opts.ChartVersion,
		ExtraAPIVersions:           opts.ExtraAPIVersions,
		HelmOptions:                helmOptions,
		LocalKubeVersion:           opts.LocalKubeVersion,
		Remote:                     opts.Remote,
		RuntimeSetJSON:             opts.RuntimeSetJSON,
		TemplatesAllowDNS:          opts.TemplatesAllowDNS,
		ValuesFiles:                opts.ValuesFiles,
		ValuesSet:                  opts.ValuesSet,
		ValuesSetFile:              opts.ValuesSetFile,
		ValuesSetJSON:              opts.ValuesSetJSON,
		ValuesSetLiteral:           opts.ValuesSetLiteral,
		ValuesSetString:            opts.ValuesSetString,
	}

	log.Default.Debug(ctx, "Render chart")

	renderChartResult, err := chart.RenderChart(ctx, opts.Chart, opts.ReleaseName, opts.ReleaseNamespace, newRevision, deployType, helmRegistryClient, clientFactory, chartTreeOptions)
	if err != nil {
		return fmt.Errorf("render chart: %w", err)
	}

	log.Default.Debug(ctx, "Build transformed resource specs")

	transformedResSpecs, err := spec.BuildTransformedResourceSpecs(ctx, opts.ReleaseNamespace, renderChartResult.ResourceSpecs, []spec.ResourceTransformer{
		spec.NewResourceListsTransformer(),
		spec.NewDropInvalidAnnotationsAndLabelsTransformer(),
	})
	if err != nil {
		return fmt.Errorf("build transformed resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build releasable resource specs")

	releasableResSpecs, err := spec.BuildReleasableResourceSpecs(ctx, opts.ReleaseNamespace, transformedResSpecs, []spec.ResourcePatcher{
		spec.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
	})
	if err != nil {
		return fmt.Errorf("build releasable resource specs: %w", err)
	}

	newRelease, err := release.NewRelease(opts.ReleaseName, opts.ReleaseNamespace, newRevision, deployType, releasableResSpecs, renderChartResult.Chart, renderChartResult.ReleaseConfig, release.ReleaseOptions{})
	if err != nil {
		return fmt.Errorf("construct new release: %w", err)
	}

	log.Default.Debug(ctx, "Convert previous release to resource specs")

	var prevRelResSpecs []*spec.ResourceSpec
	if prevRelease != nil {
		prevRelResSpecs, err = release.ReleaseToResourceSpecs(prevRelease, opts.ReleaseNamespace)
		if err != nil {
			return fmt.Errorf("convert previous release to resource specs: %w", err)
		}
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, opts.ReleaseNamespace)
	if err != nil {
		return fmt.Errorf("convert new release to resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build resources")

	instResources, delResources, err := resource.BuildResources(ctx, deployType, opts.ReleaseNamespace, prevRelResSpecs, newRelResSpecs, []spec.ResourcePatcher{
		spec.NewReleaseMetadataPatcher(opts.ReleaseName, opts.ReleaseNamespace),
		spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
	}, clientFactory, resource.BuildResourcesOptions{
		Remote: opts.Remote,
	})
	if err != nil {
		return fmt.Errorf("build resources: %w", err)
	}

	log.Default.Debug(ctx, "Locally validate resources")

	if err := resource.ValidateLocal(opts.ReleaseNamespace, instResources); err != nil {
		return fmt.Errorf("locally validate resources: %w", err)
	}

	if !opts.Remote {
		return nil
	}

	log.Default.Debug(ctx, "Build resource infos")

	instResInfos, delResInfos, err := plan.BuildResourceInfos(ctx, deployType, opts.ReleaseName, opts.ReleaseNamespace, instResources, delResources, prevReleaseFailed, clientFactory, plan.BuildResourceInfosOptions{
		NetworkParallelism:    opts.NetworkParallelism,
		NoRemoveManualChanges: opts.NoRemoveManualChanges,
	})
	if err != nil {
		return fmt.Errorf("build resource infos: %w", err)
	}

	log.Default.Debug(ctx, "Remotely validate resources")

	if err := plan.ValidateRemote(opts.ReleaseName, opts.ReleaseNamespace, instResInfos, opts.ForceAdoption); err != nil {
		return fmt.Errorf("remotely validate resources: %w", err)
	}

	log.Default.Debug(ctx, "Build release infos")

	relInfos, err := plan.BuildReleaseInfos(ctx, deployType, releases, newRelease)
	if err != nil {
		return fmt.Errorf("build release infos: %w", err)
	}

	log.Default.Debug(ctx, "Build install plan")

	if _, err := plan.BuildPlan(instResInfos, delResInfos, relInfos, plan.BuildPlanOptions{
		NoFinalTracking: true,
	}); err != nil {
		return fmt.Errorf("build install plan: %w", err)
	}

	return nil
}

func applyChartLintOptionsDefaults(opts ChartLintOptions, currentDir, homeDir string) (ChartLintOptions, error) {
	if opts.Chart == "" && opts.ChartDirPath != "" {
		opts.Chart = opts.ChartDirPath
	} else if opts.ChartDirPath == "" && opts.Chart == "" {
		opts.Chart = currentDir
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

	if opts.LegacyLogRegistryStreamOut == nil {
		opts.LegacyLogRegistryStreamOut = io.Discard
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
