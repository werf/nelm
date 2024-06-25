package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/mitchellh/copystructure"
	"github.com/samber/lo"
	helm_v3 "helm.sh/helm/v3/cmd/helm"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/nelm/pkg/chrttree"
	helmcommon "github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcpatcher"
	"github.com/werf/nelm/pkg/resrcprocssr"
	"github.com/werf/nelm/pkg/rlshistor"
	"github.com/werf/nelm/pkg/secrets_manager"
)

const (
	DefaultRenderOutputFilename = "render.yaml"
)

type RenderOptions struct {
	ChartDirPath                 string
	ChartRepositoryInsecure      bool
	ChartRepositorySkipTLSVerify bool
	ChartRepositorySkipUpdate    bool
	DefaultSecretValuesDisable   bool
	DefaultValuesDisable         bool
	ExtraAnnotations             map[string]string
	ExtraLabels                  map[string]string
	ExtraRuntimeAnnotations      map[string]string
	KubeConfigBase64             string
	KubeConfigPaths              []string
	KubeContext                  string
	Local                        bool
	LocalKubeVersion             string
	LogDebug                     bool
	LogRegistryStreamOut         io.Writer
	NetworkParallelism           int
	RegistryCredentialsPath      string
	ReleaseName                  string
	ReleaseNamespace             string
	ReleaseStorageDriver         ReleaseStorageDriver
	OutputFilePath               string
	OutputFileSave               bool
	SecretKeyIgnore              bool
	SecretValuesPaths            []string
	ShowCRDs                     bool
	ShowOnlyFiles                []string
	TempDirPath                  string
	ValuesFileSets               []string
	ValuesFilesPaths             []string
	ValuesSets                   []string
	ValuesStringSets             []string
	LegacyPreRenderHook          func(
		ctx context.Context,
		releaseNamespace string,
		helmRegistryClient *registry.Client,
		secretsManager *secrets_manager.SecretsManager,
		registryCredentialsPath string,
		chartRepositorySkipUpdate bool,
		secretValuesPaths []string,
		extraAnnotations map[string]string,
		extraLabels map[string]string,
		defaultValuesDisable bool,
		defaultSecretValuesDisable bool,
		helmSettings *cli.EnvSettings,
	) error
}

func Render(ctx context.Context, userOpts RenderOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	opts, err := buildRenderOptions(&userOpts, currentDir, currentUser)
	if err != nil {
		return fmt.Errorf("build render options: %w", err)
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
			Namespace: opts.ReleaseNamespace,
		},
	)
	if err != nil {
		return fmt.Errorf("construct kube config getter: %w", err)
	}

	helmSettings := helm_v3.Settings
	*helmSettings.GetConfigP() = kubeConfigGetter
	*helmSettings.GetNamespaceP() = opts.ReleaseNamespace
	opts.ReleaseNamespace = helmSettings.Namespace()
	helmSettings.Debug = opts.LogDebug

	if opts.KubeContext != "" {
		helmSettings.KubeContext = opts.KubeContext
	}

	if kubeConfigPath != "" {
		helmSettings.KubeConfig = kubeConfigPath
	}

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(opts.LogDebug),
		registry.ClientOptWriter(opts.LogRegistryStreamOut),
	}

	if opts.ChartRepositoryInsecure {
		helmRegistryClientOpts = append(
			helmRegistryClientOpts,
			registry.ClientOptPlainHTTP(),
		)
	}

	if opts.RegistryCredentialsPath != "" {
		helmRegistryClientOpts = append(
			helmRegistryClientOpts,
			registry.ClientOptCredentialsFile(opts.RegistryCredentialsPath),
		)
	}

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
			log.Default.Info(ctx, format, a...)
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

		if opts.LocalKubeVersion != "" {
			kubeVersion, err := chartutil.ParseKubeVersion(opts.LocalKubeVersion)
			if err != nil {
				return fmt.Errorf("parse local kube version %q: %w", opts.LocalKubeVersion, err)
			}

			helmActionConfig.Capabilities.KubeVersion = *kubeVersion
		}
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

	secretsManager := secrets_manager.NewSecretsManager(
		secrets_manager.SecretsManagerOptions{
			DisableSecretsDecryption: opts.SecretKeyIgnore,
		},
	)

	if opts.LegacyPreRenderHook != nil {
		if err := opts.LegacyPreRenderHook(
			ctx,
			opts.ReleaseNamespace,
			helmRegistryClient,
			secretsManager,
			opts.RegistryCredentialsPath,
			opts.ChartRepositorySkipUpdate,
			opts.SecretValuesPaths,
			opts.ExtraAnnotations,
			opts.ExtraLabels,
			opts.DefaultValuesDisable,
			opts.DefaultSecretValuesDisable,
			helmSettings,
		); err != nil {
			return fmt.Errorf("legacy pre render hook: %w", err)
		}
	}

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

	var showFiles []string
	for _, file := range opts.ShowOnlyFiles {
		absFile, err := filepath.Abs(file)
		if err != nil {
			return fmt.Errorf("get absolute path for %q: %w", file, err)
		}

		if strings.HasPrefix(absFile, opts.ChartDirPath) {
			f, err := filepath.Rel(opts.ChartDirPath, absFile)
			if err != nil {
				return fmt.Errorf("get relative path for %q: %w", absFile, err)
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

	var renderOutStream io.Writer
	if opts.OutputFileSave {
		file, err := os.Create(opts.OutputFilePath)
		if err != nil {
			return fmt.Errorf("create render output file %q: %w", opts.OutputFilePath, err)
		}
		defer file.Close()

		renderOutStream = file
	} else {
		renderOutStream = os.Stdout
	}

	if opts.ShowCRDs {
		for _, resource := range resProcessor.DeployableStandaloneCRDs() {
			if len(showFiles) > 0 && !lo.Contains(showFiles, resource.FilePath()) {
				continue
			}

			if err := renderResource(resource.Unstructured(), resource.FilePath(), renderOutStream); err != nil {
				return fmt.Errorf("render CRD %q: %w", resource.HumanID(), err)
			}
		}
	}

	for _, resource := range resProcessor.DeployableHookResources() {
		if len(showFiles) > 0 && !lo.Contains(showFiles, resource.FilePath()) {
			continue
		}

		if err := renderResource(resource.Unstructured(), resource.FilePath(), renderOutStream); err != nil {
			return fmt.Errorf("render hook resource %q: %w", resource.HumanID(), err)
		}
	}

	for _, resource := range resProcessor.DeployableGeneralResources() {
		if len(showFiles) > 0 && !lo.Contains(showFiles, resource.FilePath()) {
			continue
		}

		if err := renderResource(resource.Unstructured(), resource.FilePath(), renderOutStream); err != nil {
			return fmt.Errorf("render general resource %q: %w", resource.HumanID(), err)
		}
	}

	return nil
}

func buildRenderOptions(
	originalOpts *RenderOptions,
	currentDir string,
	currentUser *user.User,
) (*RenderOptions, error) {
	var opts *RenderOptions
	if o, err := copystructure.Copy(originalOpts); err != nil {
		return nil, fmt.Errorf("deep copy options: %w", err)
	} else {
		opts = o.(*RenderOptions)
	}

	if opts.ChartDirPath == "" {
		opts.ChartDirPath = currentDir
	}

	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
	}

	if opts.OutputFileSave {
		if opts.OutputFilePath == "" {
			opts.OutputFilePath = filepath.Join(opts.TempDirPath, DefaultRenderOutputFilename)
		}
	}

	if opts.KubeConfigBase64 == "" && len(opts.KubeConfigPaths) == 0 {
		opts.KubeConfigPaths = []string{filepath.Join(currentUser.HomeDir, ".kube", "config")}
	}

	if opts.LogRegistryStreamOut == nil {
		opts.LogRegistryStreamOut = os.Stdout
	}

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = 30
	}

	if opts.ReleaseName == "" {
		return nil, fmt.Errorf("release name not specified")
	}

	if opts.ReleaseStorageDriver == ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = ReleaseStorageDriverSecrets
	}

	return opts, nil
}

func renderResource(unstruct *unstructured.Unstructured, path string, outStream io.Writer) error {
	resourceJsonBytes, err := runtime.Encode(unstructured.UnstructuredJSONScheme, unstruct)
	if err != nil {
		return fmt.Errorf("encode to JSON: %w", err)
	}

	resourceYamlBytes, err := yaml.JSONToYAML(resourceJsonBytes)
	if err != nil {
		return fmt.Errorf("marshal JSON to YAML: %w", err)
	}

	prefixBytes := []byte(fmt.Sprintf("---\n# Source: %s\n", path))

	if _, err := outStream.Write(append(prefixBytes, resourceYamlBytes...)); err != nil {
		return fmt.Errorf("write to output: %w", err)
	}

	return nil
}
