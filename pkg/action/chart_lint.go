package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultChartLintLogLevel = log.InfoLevel
)

type ChartLintOptions struct {
	common.KubeConnectionOptions
	common.ChartRepoConnectionOptions
	common.ResourceValidationOptions
	common.ValuesOptions
	common.SecretValuesOptions

	// Chart specifies the chart to lint. Can be a local directory path, chart archive,
	// OCI registry URL (oci://registry/chart), or chart repository reference (repo/chart).
	// Defaults to current directory if not specified.
	Chart string
	// ChartAppVersion overrides the appVersion field in Chart.yaml.
	// Used to set application version metadata without modifying the chart file.
	ChartAppVersion string
	// ChartDirPath is deprecated (TODO v2: remove). Use Chart instead.
	ChartDirPath string // TODO(v2): get rid
	// ChartProvenanceKeyring is the path to a keyring file containing public keys
	// used to verify chart provenance signatures. Used with signed charts for security.
	ChartProvenanceKeyring string
	// ChartProvenanceStrategy defines how to verify chart provenance.
	// Defaults to DefaultChartProvenanceStrategy if not set.
	ChartProvenanceStrategy string
	// ChartRepoSkipUpdate, when true, skips updating the chart repository cache before fetching the chart.
	// Useful for offline operations or when repository is known to be up-to-date.
	ChartRepoSkipUpdate bool
	// ChartVersion specifies the version of the chart to lint (e.g., "1.2.3").
	// If not specified, the latest version is used.
	ChartVersion string
	// DefaultChartAPIVersion sets the default Chart API version when Chart.yaml doesn't specify one.
	DefaultChartAPIVersion string
	// DefaultChartName sets the default chart name when Chart.yaml doesn't specify one.
	DefaultChartName string
	// DefaultChartVersion sets the default chart version when Chart.yaml doesn't specify one.
	DefaultChartVersion string
	// DefaultDeletePropagation sets the deletion propagation policy for resource deletions.
	DefaultDeletePropagation string
	// ExtraAPIVersions is a list of additional Kubernetes API versions to include during linting.
	// Used by Capabilities.APIVersions in templates to check for API availability.
	ExtraAPIVersions []string
	// ExtraAnnotations are additional Kubernetes annotations to add to all chart resources during validation.
	// These are used for the validation dry-run.
	ExtraAnnotations map[string]string
	// ExtraLabels are additional Kubernetes labels to add to all chart resources during validation.
	// These are used for the validation dry-run.
	ExtraLabels map[string]string
	// ExtraRuntimeAnnotations are additional annotations to add to resources during validation.
	// These are used for the validation dry-run but not stored.
	ExtraRuntimeAnnotations map[string]string
	// ExtraRuntimeLabels are additional labels to add to resources during validation.
	// These are used for the validation dry-run but not stored.
	ExtraRuntimeLabels map[string]string
	// ForceAdoption, when true, allows adopting resources during validation that belong to a different Helm release.
	// Used during the validation phase to check if resources could be adopted.
	ForceAdoption bool
	// LegacyChartType specifies the chart type for legacy compatibility.
	// Used internally for backward compatibility with werf integration.
	LegacyChartType helmopts.ChartType
	// LegacyExtraValues provides additional values programmatically.
	// Used internally for backward compatibility with werf integration.
	LegacyExtraValues map[string]interface{}
	// LegacyLogRegistryStreamOut is the output writer for Helm registry client logs.
	// Defaults to io.Discard if not set. Used for debugging registry operations.
	LegacyLogRegistryStreamOut io.Writer
	// LocalKubeVersion specifies the Kubernetes version to use for linting when not connected to a cluster.
	// Format: "major.minor.patch" (e.g., "1.28.0"). Defaults to DefaultLocalKubeVersion if not set.
	LocalKubeVersion string
	// NetworkParallelism limits the number of concurrent network-related operations (API calls, resource fetches).
	// Defaults to DefaultNetworkParallelism if not set or <= 0.
	NetworkParallelism int
	// NoFinalTracking, when true, disables final tracking operations during validation to speed up linting.
	NoFinalTracking bool
	// NoRemoveManualChanges, when true, preserves fields during validation that would be manually added.
	// Used in the validation dry-run to check resource compatibility.
	NoRemoveManualChanges bool
	// RegistryCredentialsPath is the path to Docker config.json file with registry credentials.
	// Defaults to DefaultRegistryCredentialsPath (~/.docker/config.json) if not set.
	// Used for authenticating to OCI registries when pulling charts.
	RegistryCredentialsPath string
	// ReleaseName is the name of the release to use for linting.
	// Available as .Release.Name in chart templates. Defaults to a stub value if not specified.
	ReleaseName string
	// ReleaseNamespace is the namespace where the release would be installed for linting purposes.
	// Available as .Release.Namespace in chart templates. Defaults to a stub value if not specified.
	ReleaseNamespace string
	// ReleaseStorageDriver specifies how release metadata would be stored (affects validation).
	// Valid values: "secret" (default), "configmap", "sql", "memory".
	// Set to "memory" automatically when Remote is false.
	ReleaseStorageDriver string
	// ReleaseStorageSQLConnection is the SQL connection string when using SQL storage driver.
	// Only used when ReleaseStorageDriver is "sql".
	ReleaseStorageSQLConnection string
	// Remote, when true, connects to a real Kubernetes cluster for validation.
	// When false, performs only local validation without cluster connectivity.
	Remote bool
	// TempDirPath is the directory for temporary files during the operation.
	// A temporary directory is created automatically if not specified.
	TempDirPath string
	// TemplatesAllowDNS, when true, enables DNS lookups in chart templates using template functions.
	// WARNING: This can make template rendering non-deterministic and slower.
	TemplatesAllowDNS bool
}

// Lint the Helm chart.
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
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverMemory
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
			KubeConnectionOptions: opts.KubeConnectionOptions,
			KubeContextNamespace:  opts.ReleaseNamespace, // TODO: unset it everywhere
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
			SecretKeyIgnore:            opts.SecretKeyIgnore,
			SecretValuesFiles:          opts.SecretValuesFiles,
			SecretWorkDir:              opts.SecretWorkDir,
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
		ChartRepoConnectionOptions: opts.ChartRepoConnectionOptions,
		ValuesOptions:              opts.ValuesOptions,
		ChartProvenanceKeyring:     opts.ChartProvenanceKeyring,
		ChartProvenanceStrategy:    opts.ChartProvenanceStrategy,
		ChartRepoNoUpdate:          opts.ChartRepoSkipUpdate,
		ChartVersion:               opts.ChartVersion,
		ExtraAPIVersions:           opts.ExtraAPIVersions,
		HelmOptions:                helmOptions,
		LocalKubeVersion:           opts.LocalKubeVersion,
		Remote:                     opts.Remote,
		TemplatesAllowDNS:          opts.TemplatesAllowDNS,
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
		spec.NewSecretStringDataPatcher(),
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
		prevRelResSpecs, err = release.ReleaseToResourceSpecs(prevRelease, opts.ReleaseNamespace, false)
		if err != nil {
			return fmt.Errorf("convert previous release to resource specs: %w", err)
		}
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, opts.ReleaseNamespace, false)
	if err != nil {
		return fmt.Errorf("convert new release to resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build resources")

	instResources, delResources, err := resource.BuildResources(ctx, deployType, opts.ReleaseNamespace, prevRelResSpecs, newRelResSpecs, []spec.ResourcePatcher{
		spec.NewReleaseMetadataPatcher(opts.ReleaseName, opts.ReleaseNamespace),
		spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
	}, clientFactory, resource.BuildResourcesOptions{
		Remote:                   opts.Remote,
		DefaultDeletePropagation: metav1.DeletionPropagation(opts.DefaultDeletePropagation),
	})
	if err != nil {
		return fmt.Errorf("build resources: %w", err)
	}

	log.Default.Debug(ctx, "Locally validate resources")

	if err := resource.ValidateLocal(ctx, opts.ReleaseNamespace, instResources, opts.ResourceValidationOptions); err != nil {
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
		NoFinalTracking: opts.NoFinalTracking,
	}); err != nil {
		return fmt.Errorf("build install plan: %w", err)
	}

	return nil
}

func applyChartLintOptionsDefaults(opts ChartLintOptions, currentDir, homeDir string) (ChartLintOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ChartLintOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)
	opts.ChartRepoConnectionOptions.ApplyDefaults()
	opts.ValuesOptions.ApplyDefaults()
	opts.SecretValuesOptions.ApplyDefaults(currentDir)

	if opts.Chart == "" && opts.ChartDirPath != "" {
		opts.Chart = opts.ChartDirPath
	} else if opts.ChartDirPath == "" && opts.Chart == "" {
		opts.Chart = currentDir
	}

	if opts.ReleaseName == "" {
		opts.ReleaseName = common.StubReleaseName
	}

	if opts.ReleaseNamespace == "" {
		opts.ReleaseNamespace = common.StubReleaseNamespace
	}

	if opts.LegacyLogRegistryStreamOut == nil {
		opts.LegacyLogRegistryStreamOut = io.Discard
	}

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = common.DefaultNetworkParallelism
	}

	if opts.ReleaseStorageDriver == common.ReleaseStorageDriverDefault {
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
	}

	if opts.LocalKubeVersion == "" {
		// TODO(v3): update default local version
		opts.LocalKubeVersion = common.DefaultLocalKubeVersion
	}

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = common.DefaultRegistryCredentialsPath
	}

	if opts.ChartProvenanceStrategy == "" {
		opts.ChartProvenanceStrategy = common.DefaultChartProvenanceStrategy
	}

	if opts.DefaultDeletePropagation == "" {
		opts.DefaultDeletePropagation = string(common.DefaultDeletePropagation)
	}

	return opts, nil
}
