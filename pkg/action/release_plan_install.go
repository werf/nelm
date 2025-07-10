package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gookit/color"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/plan/resourceinfo"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	log2 "github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleasePlanInstallLogLevel = InfoLogLevel
)

var ErrChangesPlanned = errors.New("changes planned")

type ReleasePlanInstallOptions struct {
	Chart                        string
	ChartAppVersion              string
	ChartDirPath                 string // TODO(v2): get rid
	ChartRepositoryInsecure      bool
	ChartRepositorySkipTLSVerify bool
	ChartRepositorySkipUpdate    bool
	ChartVersion                 string
	DefaultChartAPIVersion       string
	DefaultChartName             string
	DefaultChartVersion          string
	DefaultSecretValuesDisable   bool
	DefaultValuesDisable         bool
	ErrorIfChangesPlanned        bool
	ExtraAnnotations             map[string]string
	ExtraLabels                  map[string]string
	ExtraRuntimeAnnotations      map[string]string
	ForceAdoption                bool
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
	LegacyChartType              helmopts.ChartType
	LegacyExtraValues            map[string]interface{}
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	NoInstallCRDs                bool
	RegistryCredentialsPath      string
	ReleaseStorageDriver         string
	SQLConnectionString          string
	SecretKey                    string
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	SecretWorkDir                string
	TempDirPath                  string
	Timeout                      time.Duration
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
}

func ReleasePlanInstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleasePlanInstallOptions) error {
	if opts.Timeout == 0 {
		return releasePlanInstall(ctx, releaseName, releaseNamespace, opts)
	}

	ctx, ctxCancelFn := context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("context timed out: action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn()

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- releasePlanInstall(ctx, releaseName, releaseNamespace, opts)
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

func releasePlanInstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleasePlanInstallOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleasePlanInstallOptionsDefaults(opts, currentDir, homeDir)
	if err != nil {
		return fmt.Errorf("build release plan install options: %w", err)
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
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

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(log2.Default.AcceptLevel(ctx, log2.Level(DebugLogLevel))),
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

	releaseStorage, err := release.NewReleaseStorage(
		ctx,
		releaseNamespace,
		opts.ReleaseStorageDriver,
		release.ReleaseStorageOptions{
			StaticClient:        clientFactory.Static().(*kubernetes.Clientset),
			SQLConnectionString: opts.SQLConnectionString,
		},
	)
	if err != nil {
		return fmt.Errorf("construct release storage: %w", err)
	}

	helmOptions := helmopts.HelmOptions{
		ChartLoadOpts: helmopts.ChartLoadOptions{
			ChartAppVersion:        opts.ChartAppVersion,
			ChartType:              opts.LegacyChartType,
			DefaultChartAPIVersion: opts.DefaultChartAPIVersion,
			DefaultChartName:       opts.DefaultChartName,
			DefaultChartVersion:    opts.DefaultChartVersion,
			ExtraValues:            opts.LegacyExtraValues,
			NoDecryptSecrets:       opts.SecretKeyIgnore,
			NoDefaultSecretValues:  opts.DefaultSecretValuesDisable,
			NoDefaultValues:        opts.DefaultValuesDisable,
			SecretValuesFiles:      opts.SecretValuesPaths,
			SecretsWorkingDir:      opts.SecretWorkDir,
		},
	}

	log2.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Planning release install")+" %q (namespace: %q)", releaseName, releaseNamespace)

	log2.Default.Debug(ctx, "Constructing release history")
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

	_, prevDeployedReleaseFound, err := history.LastDeployedRelease()
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

	var deployType common.DeployType
	if prevReleaseFound && prevDeployedReleaseFound {
		deployType = common.DeployTypeUpgrade
	} else if prevReleaseFound {
		deployType = common.DeployTypeInstall
	} else {
		deployType = common.DeployTypeInitial
	}

	log2.Default.Debug(ctx, "Constructing chart tree")
	chartTree, err := chart.NewChartTree(
		ctx,
		opts.Chart,
		releaseName,
		releaseNamespace,
		newRevision,
		deployType,
		chart.ChartTreeOptions{
			ChartRepoInsecure:      opts.ChartRepositoryInsecure,
			ChartRepoSkipTLSVerify: opts.ChartRepositorySkipTLSVerify,
			ChartRepoSkipUpdate:    opts.ChartRepositorySkipUpdate,
			ChartVersion:           opts.ChartVersion,
			DiscoveryClient:        clientFactory.Discovery(),
			FileValues:             opts.ValuesFileSets,
			KubeCAPath:             opts.KubeCAPath,
			KubeConfig:             clientFactory.KubeConfig(),
			HelmOptions:            helmOptions,
			Mapper:                 clientFactory.Mapper(),
			NoStandaloneCRDs:       opts.NoInstallCRDs,
			RegistryClient:         helmRegistryClient,
			SetValues:              opts.ValuesSets,
			StringSetValues:        opts.ValuesStringSets,
			ValuesFiles:            opts.ValuesFilesPaths,
		},
	)
	if err != nil {
		return fmt.Errorf("construct chart tree: %w", err)
	}

	notes := chartTree.Notes()

	var prevRelGeneralResources []*resource.GeneralResource
	var prevRelFailed bool
	if prevReleaseFound {
		prevRelGeneralResources = prevRelease.GeneralResources()
		prevRelFailed = prevRelease.Failed()
	}

	log2.Default.Debug(ctx, "Processing resources")
	resProcessor := resourceinfo.NewDeployableResourcesProcessor(
		deployType,
		releaseName,
		releaseNamespace,
		chartTree.StandaloneCRDs(),
		chartTree.HookResources(),
		chartTree.GeneralResources(),
		nil,
		prevRelGeneralResources,
		resourceinfo.DeployableResourcesProcessorOptions{
			NetworkParallelism: opts.NetworkParallelism,
			ForceAdoption:      opts.ForceAdoption,
			ReleasableHookResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
			},
			ReleasableGeneralResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
			},
			DeployableStandaloneCRDsPatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(
					lo.Assign(opts.ExtraAnnotations, opts.ExtraRuntimeAnnotations),
					opts.ExtraLabels,
				),
			},
			DeployableHookResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(
					lo.Assign(opts.ExtraAnnotations, opts.ExtraRuntimeAnnotations),
					opts.ExtraLabels,
				),
			},
			DeployableGeneralResourcePatchers: []resource.ResourcePatcher{
				resource.NewExtraMetadataPatcher(
					lo.Assign(opts.ExtraAnnotations, opts.ExtraRuntimeAnnotations),
					opts.ExtraLabels,
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

	log2.Default.Debug(ctx, "Constructing new release")
	newRel, err := release.NewRelease(
		releaseName,
		releaseNamespace,
		newRevision,
		chartTree.OverrideValues(),
		chartTree.LegacyChart(),
		resProcessor.ReleasableHookResources(),
		resProcessor.ReleasableGeneralResources(),
		notes,
		release.ReleaseOptions{
			FirstDeployed: firstDeployed,
			Mapper:        clientFactory.Mapper(),
		},
	)
	if err != nil {
		return fmt.Errorf("construct new release: %w", err)
	}

	log2.Default.Debug(ctx, "Calculating planned changes")
	createdChanges, recreatedChanges, updatedChanges, appliedChanges, deletedChanges, planChangesPlanned := plan.CalculatePlannedChanges(
		deployType,
		releaseName,
		releaseNamespace,
		resProcessor.DeployableStandaloneCRDsInfos(),
		resProcessor.DeployableHookResourcesInfos(),
		resProcessor.DeployableGeneralResourcesInfos(),
		resProcessor.DeployablePrevReleaseGeneralResourcesInfos(),
		prevRelFailed,
	)

	var releaseUpToDate bool
	if prevReleaseFound {
		releaseUpToDate, err = release.ReleaseUpToDate(prevRelease, newRel)
		if err != nil {
			return fmt.Errorf("check if release is up to date: %w", err)
		}
	}

	plan.LogPlannedChanges(
		ctx,
		releaseName,
		releaseNamespace,
		!releaseUpToDate,
		createdChanges,
		recreatedChanges,
		updatedChanges,
		appliedChanges,
		deletedChanges,
	)

	if opts.ErrorIfChangesPlanned && (planChangesPlanned || !releaseUpToDate) {
		return ErrChangesPlanned
	}

	return nil
}

func applyReleasePlanInstallOptionsDefaults(opts ReleasePlanInstallOptions, currentDir, homeDir string) (ReleasePlanInstallOptions, error) {
	if opts.Chart == "" && opts.ChartDirPath != "" {
		opts.Chart = opts.ChartDirPath
	} else if opts.ChartDirPath == "" && opts.Chart == "" {
		opts.Chart = currentDir
	}

	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleasePlanInstallOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.KubeConfigBase64 == "" && len(lo.Compact(opts.KubeConfigPaths)) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(homeDir, ".kube", "config")}
	}

	if opts.LogRegistryStreamOut == nil {
		opts.LogRegistryStreamOut = io.Discard
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
	} else if opts.ReleaseStorageDriver == ReleaseStorageDriverMemory {
		return ReleasePlanInstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	if opts.SecretWorkDir == "" {
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return ReleasePlanInstallOptions{}, fmt.Errorf("get current working directory: %w", err)
		}
	}

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = DefaultRegistryCredentialsPath
	}

	return opts, nil
}
