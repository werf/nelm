package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gookit/color"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan/resourceinfo"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	log2 "github.com/werf/nelm/pkg/log"
)

const (
	DefaultChartRenderLogLevel = ErrorLogLevel
)

type ChartRenderOptions struct {
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
	LegacyChartType              helmopts.ChartType
	LegacyExtraValues            map[string]interface{}
	LocalKubeVersion             string
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	OutputFilePath               string
	OutputNoPrint                bool
	RegistryCredentialsPath      string
	ReleaseName                  string
	ReleaseNamespace             string
	ReleaseStorageDriver         string
	Remote                       bool
	SQLConnectionString          string
	SecretKey                    string
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	SecretWorkDir                string
	ShowCRDs                     bool
	ShowOnlyFiles                []string
	TempDirPath                  string
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
}

func ChartRender(ctx context.Context, opts ChartRenderOptions) (*ChartRenderResultV1, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get current working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyChartRenderOptionsDefaults(opts, currentDir, homeDir)
	if err != nil {
		return nil, fmt.Errorf("build chart render options: %w", err)
	}

	if opts.SecretKey != "" {
		os.Setenv("WERF_SECRET_KEY", opts.SecretKey)
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
			return nil, fmt.Errorf("construct kube config: %w", err)
		}

		clientFactory, err = kube.NewClientFactory(ctx, kubeConfig)
		if err != nil {
			return nil, fmt.Errorf("construct kube client factory: %w", err)
		}
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
		return nil, fmt.Errorf("construct registry client: %w", err)
	}

	releaseStorageOptions := release.ReleaseStorageOptions{
		SQLConnectionString: opts.SQLConnectionString,
	}

	if opts.Remote {
		releaseStorageOptions.StaticClient = clientFactory.Static().(*kubernetes.Clientset)
	}

	releaseStorage, err := release.NewReleaseStorage(ctx, opts.ReleaseNamespace, opts.ReleaseStorageDriver, releaseStorageOptions)
	if err != nil {
		return nil, fmt.Errorf("construct release storage: %w", err)
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

	var historyOptions release.HistoryOptions
	if opts.Remote {
		historyOptions.Mapper = clientFactory.Mapper()
		historyOptions.DiscoveryClient = clientFactory.Discovery()
	}

	history, err := release.NewHistory(
		opts.ReleaseName,
		opts.ReleaseNamespace,
		releaseStorage,
		historyOptions,
	)
	if err != nil {
		return nil, fmt.Errorf("construct release history: %w", err)
	}

	prevRelease, prevReleaseFound, err := history.LastRelease()
	if err != nil {
		return nil, fmt.Errorf("get last release: %w", err)
	}

	_, prevDeployedReleaseFound, err := history.LastDeployedRelease()
	if err != nil {
		return nil, fmt.Errorf("get last deployed release: %w", err)
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
		ChartRepoInsecure:      opts.ChartRepositoryInsecure,
		ChartRepoSkipTLSVerify: opts.ChartRepositorySkipTLSVerify,
		ChartVersion:           opts.ChartVersion,
		FileValues:             opts.ValuesFileSets,
		KubeCAPath:             opts.KubeCAPath,
		HelmOptions:            helmOptions,
		RegistryClient:         helmRegistryClient,
		SetValues:              opts.ValuesSets,
		StringSetValues:        opts.ValuesStringSets,
		ValuesFiles:            opts.ValuesFilesPaths,
	}

	if opts.Remote {
		chartTreeOptions.Mapper = clientFactory.Mapper()
		chartTreeOptions.DiscoveryClient = clientFactory.Discovery()
		chartTreeOptions.KubeConfig = clientFactory.KubeConfig()
	} else {
		ver, err := chartutil.ParseKubeVersion(opts.LocalKubeVersion)
		if err != nil {
			return nil, fmt.Errorf("parse local kube version %q: %w", opts.LocalKubeVersion, err)
		}

		chartTreeOptions.KubeVersion = ver
	}

	chartTree, err := chart.NewChartTree(
		ctx,
		opts.Chart,
		opts.ReleaseName,
		opts.ReleaseNamespace,
		newRevision,
		deployType,
		chartTreeOptions,
	)
	if err != nil {
		return nil, fmt.Errorf("construct chart tree: %w", err)
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
		nil,
		prevRelGeneralResources,
		resProcessorOptions,
	)

	if err := resProcessor.Process(ctx); err != nil {
		return nil, fmt.Errorf("process resources: %w", err)
	}

	var showFiles []string
	for _, file := range opts.ShowOnlyFiles {
		absFile, err := filepath.Abs(file)
		if err != nil {
			return nil, fmt.Errorf("get absolute path for %q: %w", file, err)
		}

		if strings.HasPrefix(absFile, opts.Chart) {
			f, err := filepath.Rel(opts.Chart, absFile)
			if err != nil {
				return nil, fmt.Errorf("get relative path for %q: %w", absFile, err)
			}

			if !strings.HasPrefix(f, chartTree.Name()) {
				f = filepath.Join(chartTree.Name(), f)
			}

			showFiles = append(showFiles, f)
		} else {
			if !strings.HasPrefix(file, chartTree.Name()) {
				file = filepath.Join(chartTree.Name(), file)
			}

			showFiles = append(showFiles, file)
		}
	}

	var (
		renderOutStream  io.Writer
		renderColorLevel color.Level
	)
	if opts.OutputFilePath != "" {
		file, err := os.Create(opts.OutputFilePath)
		if err != nil {
			return nil, fmt.Errorf("create chart render output file %q: %w", opts.OutputFilePath, err)
		}
		defer file.Close()

		renderOutStream = file
		renderColorLevel = color.LevelNo
	} else {
		renderOutStream = os.Stdout
		if color.Enable {
			renderColorLevel = color.TermColorLevel()
		}
	}

	result := &ChartRenderResultV1{
		APIVersion: ChartRenderResultApiVersionV1,
	}

	for _, resource := range resProcessor.DeployableStandaloneCRDs() {
		if len(showFiles) > 0 && !lo.Contains(showFiles, resource.FilePath()) {
			continue
		}

		if opts.ShowCRDs {
			if err := renderResource(resource.Unstructured(), resource.FilePath(), renderOutStream, renderColorLevel); err != nil {
				return nil, fmt.Errorf("render CRD %q: %w", resource.HumanID(), err)
			}
		}

		result.CRDs = append(result.CRDs, resource.Unstructured().Object)
	}

	for _, resource := range resProcessor.DeployableHookResources() {
		if len(showFiles) > 0 && !lo.Contains(showFiles, resource.FilePath()) {
			continue
		}

		if err := renderResource(resource.Unstructured(), resource.FilePath(), renderOutStream, renderColorLevel); err != nil {
			return nil, fmt.Errorf("render hook resource %q: %w", resource.HumanID(), err)
		}

		result.Hooks = append(result.Hooks, resource.Unstructured().Object)
	}

	for _, resource := range resProcessor.DeployableGeneralResources() {
		if len(showFiles) > 0 && !lo.Contains(showFiles, resource.FilePath()) {
			continue
		}

		if err := renderResource(resource.Unstructured(), resource.FilePath(), renderOutStream, renderColorLevel); err != nil {
			return nil, fmt.Errorf("render general resource %q: %w", resource.HumanID(), err)
		}

		result.Resources = append(result.Resources, resource.Unstructured().Object)
	}

	return result, nil
}

func applyChartRenderOptionsDefaults(opts ChartRenderOptions, currentDir, homeDir string) (ChartRenderOptions, error) {
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
			return ChartRenderOptions{}, fmt.Errorf("create temp dir: %w", err)
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

	if opts.ReleaseName == "" {
		return ChartRenderOptions{}, fmt.Errorf("release name not specified")
	}

	if opts.ReleaseStorageDriver == ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = ReleaseStorageDriverSecrets
	}

	if opts.SecretWorkDir == "" {
		opts.SecretWorkDir, err = os.Getwd()
		if err != nil {
			return ChartRenderOptions{}, fmt.Errorf("get current working directory: %w", err)
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

func renderResource(unstruct *unstructured.Unstructured, path string, outStream io.Writer, colorLevel color.Level) error {
	resourceJsonBytes, err := runtime.Encode(unstructured.UnstructuredJSONScheme, unstruct)
	if err != nil {
		return fmt.Errorf("encode to JSON: %w", err)
	}

	resourceYamlBytes, err := yaml.JSONToYAML(resourceJsonBytes)
	if err != nil {
		return fmt.Errorf("marshal JSON to YAML: %w", err)
	}

	prefix := fmt.Sprintf("---\n# Source: %s\n", path)
	manifest := prefix + string(resourceYamlBytes)

	if err := writeWithSyntaxHighlight(outStream, manifest, "yaml", colorLevel); err != nil {
		return fmt.Errorf("write resource to output: %w", err)
	}

	return nil
}

const ChartRenderResultApiVersionV1 = "v1"

type ChartRenderResultV1 struct {
	APIVersion string                   `json:"apiVersion"`
	CRDs       []map[string]interface{} `json:"crds,omitempty"`
	Hooks      []map[string]interface{} `json:"hooks,omitempty"`
	Resources  []map[string]interface{} `json:"resources,omitempty"`
}
