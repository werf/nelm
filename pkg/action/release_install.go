package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gookit/color"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/lock"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleaseInstallLogLevel = log.InfoLevel
)

type ReleaseInstallOptions struct {
	common.KubeConnectionOptions
	common.ChartRepoConnectionOptions
	common.ResourceValidationOptions
	common.ValuesOptions
	common.SecretValuesOptions
	common.TrackingOptions

	// AutoRollback, when true, automatically rolls back to the previous deployed release on installation failure.
	// Only works if there is a previously successfully deployed release.
	AutoRollback bool
	// Chart specifies the chart to install. Can be a local directory path, chart archive,
	// OCI registry URL (oci://registry/chart), or chart repository reference (repo/chart).
	// Defaults to current directory if not specified.
	Chart string
	// ChartAppVersion overrides the appVersion field in Chart.yaml.
	// Used to set application version metadata without modifying the chart file.
	ChartAppVersion string
	// ChartDirPath is deprecated
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
	// ChartVersion specifies the version of the chart to install (e.g., "1.2.3").
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
	// ExtraAnnotations are additional Kubernetes annotations to add to all chart resources.
	// These are added during chart rendering, before resources are stored in the release.
	ExtraAnnotations map[string]string
	// ExtraLabels are additional Kubernetes labels to add to all chart resources.
	// These are added during chart rendering, before resources are stored in the release.
	ExtraLabels map[string]string
	// ExtraRuntimeAnnotations are additional annotations to add to resources at runtime.
	// These are added during resource creation/update but not stored in the release.
	ExtraRuntimeAnnotations map[string]string
	// ExtraRuntimeLabels are additional labels to add to resources at runtime.
	// These are added during resource creation/update but not stored in the release.
	ExtraRuntimeLabels map[string]string
	// ForceAdoption, when true, allows adopting resources that belong to a different Helm release.
	// WARNING: This can lead to conflicts if resources are managed by multiple releases.
	ForceAdoption bool
	// InstallGraphPath, if specified, saves the Graphviz representation of the install plan to this file path.
	// Useful for debugging and visualizing the dependency graph of resource operations.
	InstallGraphPath string
	// InstallReportPath, if specified, saves a JSON report of the installation results to this file path.
	// The report includes the release status and lists of completed, canceled, and failed operations.
	InstallReportPath string
	// LegacyChartType specifies the chart type for legacy compatibility.
	// Used internally for backward compatibility with werf integration.
	LegacyChartType helmopts.ChartType
	// LegacyExtraValues provides additional values programmatically.
	// Used internally for backward compatibility with werf integration.
	LegacyExtraValues map[string]interface{}
	// LegacyLogRegistryStreamOut is the output writer for Helm registry client logs.
	// Defaults to io.Discard if not set. Used for debugging registry operations.
	LegacyLogRegistryStreamOut io.Writer
	// NetworkParallelism limits the number of concurrent network-related operations (API calls, resource fetches).
	// Defaults to DefaultNetworkParallelism if not set or <= 0.
	NetworkParallelism int
	// NoInstallStandaloneCRDs, when true, skips installation of CustomResourceDefinitions from the "crds/" directory.
	// By default, CRDs are installed first before other chart resources.
	NoInstallStandaloneCRDs bool
	// NoRemoveManualChanges, when true, preserves fields manually added to resources in the cluster
	// that are not present in the chart manifests. By default, such fields are removed during updates.
	NoRemoveManualChanges bool
	// NoShowNotes, when true, suppresses printing of NOTES.txt after successful installation.
	// NOTES.txt typically contains usage instructions and next steps.
	NoShowNotes bool
	// RegistryCredentialsPath is the path to Docker config.json file with registry credentials.
	// Defaults to DefaultRegistryCredentialsPath (~/.docker/config.json) if not set.
	// Used for authenticating to OCI registries when pulling charts.
	RegistryCredentialsPath string
	// ReleaseHistoryLimit sets the maximum number of release revisions to keep in storage.
	// When exceeded, the oldest revisions are deleted. Defaults to DefaultReleaseHistoryLimit if not set or <= 0.
	// Note: Only release metadata is deleted; actual Kubernetes resources are not affected.
	ReleaseHistoryLimit int
	// ReleaseInfoAnnotations are custom annotations to add to the release metadata (stored in Secret/ConfigMap).
	// These do not affect resources but can be used for tagging releases.
	ReleaseInfoAnnotations map[string]string
	// ReleaseLabels are labels to add to the release storage object (Secret/ConfigMap).
	// Used for filtering and organizing releases in storage.
	ReleaseLabels map[string]string
	// ReleaseStorageDriver specifies how release metadata is stored in Kubernetes.
	// Valid values: "secret" (default), "configmap", "sql".
	// Defaults to "secret" if not specified or set to "default".
	ReleaseStorageDriver string
	// ReleaseStorageSQLConnection is the SQL connection string when using SQL storage driver.
	// Only used when ReleaseStorageDriver is "sql".
	ReleaseStorageSQLConnection string
	// RollbackGraphPath, if specified, saves the Graphviz representation of the rollback plan (if auto-rollback occurs)
	// to this file path. Only used when AutoRollback is true and rollback is triggered.
	RollbackGraphPath string
	// ShowSubchartNotes, when true, shows NOTES.txt from subcharts in addition to the main chart's notes.
	// By default, only the parent chart's NOTES.txt is displayed.
	ShowSubchartNotes bool
	// TempDirPath is the directory for temporary files during the operation.
	// A temporary directory is created automatically if not specified.
	TempDirPath string
	// TemplatesAllowDNS, when true, enables DNS lookups in chart templates using template functions.
	// WARNING: This can make template rendering non-deterministic and slower.
	TemplatesAllowDNS bool
	// Timeout is the maximum duration for the entire release installation operation.
	// If 0, no timeout is applied and the operation runs until completion or error.
	Timeout time.Duration
}

func ReleaseInstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleaseInstallOptions) error {
	ctx, ctxCancelFn := context.WithCancelCause(ctx)

	if opts.Timeout == 0 {
		return releaseInstall(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
	}

	ctx, _ = context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("context timed out: action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn(fmt.Errorf("context canceled: action finished"))

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- releaseInstall(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
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

func releaseInstall(ctx context.Context, ctxCancelFn context.CancelCauseFunc, releaseName, releaseNamespace string, opts ReleaseInstallOptions) error {
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get current working directory: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	opts, err = applyReleaseInstallOptionsDefaults(opts, currentDir, homeDir)
	if err != nil {
		return fmt.Errorf("build release install options: %w", err)
	}

	if opts.SecretKey != "" {
		lo.Must0(os.Setenv("WERF_SECRET_KEY", opts.SecretKey))
	}

	if len(opts.KubeConfigPaths) > 0 {
		var splitPaths []string
		for _, path := range opts.KubeConfigPaths {
			splitPaths = append(splitPaths, filepath.SplitList(path)...)
		}

		opts.KubeConfigPaths = lo.Compact(splitPaths)
	}

	kubeConfig, err := kube.NewKubeConfig(ctx, opts.KubeConfigPaths, kube.KubeConfigOptions{
		KubeConnectionOptions: opts.KubeConnectionOptions,
		KubeContextNamespace:  releaseNamespace, // TODO: unset it everywhere
	})
	if err != nil {
		return fmt.Errorf("construct kube config: %w", err)
	}

	clientFactory, err := kube.NewClientFactory(ctx, kubeConfig)
	if err != nil {
		return fmt.Errorf("construct kube client factory: %w", err)
	}

	helmRegistryClientOpts := []registry.ClientOption{
		registry.ClientOptDebug(log.Default.AcceptLevel(ctx, log.DebugLevel)),
		// TODO(log):
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

	releaseStorage, err := release.NewReleaseStorage(ctx, releaseNamespace, opts.ReleaseStorageDriver, clientFactory, release.ReleaseStorageOptions{
		HistoryLimit:  opts.ReleaseHistoryLimit,
		SQLConnection: opts.ReleaseStorageSQLConnection,
	})
	if err != nil {
		return fmt.Errorf("construct release storage: %w", err)
	}

	var lockManager *lock.LockManager
	if m, err := lock.NewLockManager(ctx, releaseNamespace, false, clientFactory); err != nil {
		return fmt.Errorf("construct lock manager: %w", err)
	} else {
		lockManager = m
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

	if err := createReleaseNamespace(ctx, clientFactory, releaseNamespace); err != nil {
		return fmt.Errorf("create release namespace: %w", err)
	}

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Start release")+" %q (namespace: %q)", releaseName, releaseNamespace)

	if lock, err := lockManager.LockRelease(ctx, releaseName); err != nil {
		return fmt.Errorf("lock release: %w", err)
	} else {
		defer func() {
			_ = lockManager.Unlock(lock)
		}()
	}

	log.Default.Debug(ctx, "Build release history")

	history, err := release.BuildHistory(releaseName, releaseStorage, release.HistoryOptions{})
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

	log.Default.Debug(ctx, "Render chart")

	renderChartResult, err := chart.RenderChart(ctx, opts.Chart, releaseName, releaseNamespace, newRevision, deployType, helmRegistryClient, clientFactory, chart.RenderChartOptions{
		ChartRepoConnectionOptions: opts.ChartRepoConnectionOptions,
		ValuesOptions:              opts.ValuesOptions,
		ChartProvenanceKeyring:     opts.ChartProvenanceKeyring,
		ChartProvenanceStrategy:    opts.ChartProvenanceStrategy,
		ChartRepoNoUpdate:          opts.ChartRepoSkipUpdate,
		ChartVersion:               opts.ChartVersion,
		HelmOptions:                helmOptions,
		NoStandaloneCRDs:           opts.NoInstallStandaloneCRDs,
		Remote:                     true,
		SubchartNotes:              opts.ShowSubchartNotes,
		TemplatesAllowDNS:          opts.TemplatesAllowDNS,
	})
	if err != nil {
		return fmt.Errorf("render chart: %w", err)
	}

	log.Default.Debug(ctx, "Build transformed resource specs")

	transformedResSpecs, err := spec.BuildTransformedResourceSpecs(ctx, releaseNamespace, renderChartResult.ResourceSpecs, []spec.ResourceTransformer{
		spec.NewResourceListsTransformer(),
		spec.NewDropInvalidAnnotationsAndLabelsTransformer(),
	})
	if err != nil {
		return fmt.Errorf("build transformed resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build releasable resource specs")

	patchers := []spec.ResourcePatcher{
		spec.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
		spec.NewSecretStringDataPatcher(),
	}

	if opts.LegacyHelmCompatibleTracking {
		patchers = append(patchers, spec.NewLegacyOnlyTrackJobsPatcher())
	}

	releasableResSpecs, err := spec.BuildReleasableResourceSpecs(ctx, releaseNamespace, transformedResSpecs, patchers)
	if err != nil {
		return fmt.Errorf("build releasable resource specs: %w", err)
	}

	newRelease, err := release.NewRelease(releaseName, releaseNamespace, newRevision, deployType, releasableResSpecs, renderChartResult.Chart, renderChartResult.ReleaseConfig, release.ReleaseOptions{
		InfoAnnotations: opts.ReleaseInfoAnnotations,
		Labels:          opts.ReleaseLabels,
		Notes:           renderChartResult.Notes,
	})
	if err != nil {
		return fmt.Errorf("construct new release: %w", err)
	}

	log.Default.Debug(ctx, "Convert previous release to resource specs")

	var prevRelResSpecs []*spec.ResourceSpec
	if prevRelease != nil {
		prevRelResSpecs, err = release.ReleaseToResourceSpecs(prevRelease, releaseNamespace, false)
		if err != nil {
			return fmt.Errorf("convert previous release to resource specs: %w", err)
		}
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, releaseNamespace, false)
	if err != nil {
		return fmt.Errorf("convert new release to resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build resources")

	instResources, delResources, err := resource.BuildResources(ctx, deployType, releaseNamespace, prevRelResSpecs, newRelResSpecs, []spec.ResourcePatcher{
		spec.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
		spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
	}, clientFactory, resource.BuildResourcesOptions{
		Remote:                   true,
		DefaultDeletePropagation: metav1.DeletionPropagation(opts.DefaultDeletePropagation),
	})
	if err != nil {
		return fmt.Errorf("build resources: %w", err)
	}

	log.Default.Debug(ctx, "Locally validate resources")

	if err := resource.ValidateLocal(ctx, releaseNamespace, instResources, opts.ResourceValidationOptions); err != nil {
		return fmt.Errorf("locally validate resources: %w", err)
	}

	log.Default.Debug(ctx, "Build resource infos")

	instResInfos, delResInfos, err := plan.BuildResourceInfos(ctx, deployType, releaseName, releaseNamespace, instResources, delResources, prevReleaseFailed, clientFactory, plan.BuildResourceInfosOptions{
		NetworkParallelism:    opts.NetworkParallelism,
		NoRemoveManualChanges: opts.NoRemoveManualChanges,
	})
	if err != nil {
		return fmt.Errorf("build resource infos: %w", err)
	}

	log.Default.Debug(ctx, "Remotely validate resources")

	if err := plan.ValidateRemote(releaseName, releaseNamespace, instResInfos, opts.ForceAdoption); err != nil {
		return fmt.Errorf("remotely validate resources: %w", err)
	}

	log.Default.Debug(ctx, "Build release infos")

	relInfos, err := plan.BuildReleaseInfos(ctx, deployType, releases, newRelease)
	if err != nil {
		return fmt.Errorf("build release infos: %w", err)
	}

	log.Default.Debug(ctx, "Build install plan")

	installPlan, err := plan.BuildPlan(instResInfos, delResInfos, relInfos, plan.BuildPlanOptions{
		NoFinalTracking: opts.NoFinalTracking,
	})
	if err != nil {
		handleBuildPlanErr(ctx, installPlan, err, opts.InstallGraphPath, opts.TempDirPath, "release-install-graph.dot")
		return fmt.Errorf("build install plan: %w", err)
	}

	return runInstallPlan(ctx, ctxCancelFn, releaseName, releaseNamespace, installPlan, prevRelease, newRelease, clientFactory, history, instResInfos, relInfos, prevDeployedRelease, runInstallPlanOptions{
		TrackingOptions: opts.TrackingOptions,
		runRollbackPlanOptions: runRollbackPlanOptions{
			DefaultDeletePropagation: opts.DefaultDeletePropagation,
			ExtraAnnotations:         opts.ExtraAnnotations,
			ExtraLabels:              opts.ExtraLabels,
			ExtraRuntimeAnnotations:  opts.ExtraRuntimeAnnotations,
			ExtraRuntimeLabels:       opts.ExtraRuntimeLabels,
			ForceAdoption:            opts.ForceAdoption,
			NetworkParallelism:       opts.NetworkParallelism,
			NoRemoveManualChanges:    opts.NoRemoveManualChanges,
			ReleaseInfoAnnotations:   opts.ReleaseInfoAnnotations,
			ReleaseLabels:            opts.ReleaseLabels,
			RollbackGraphPath:        opts.RollbackGraphPath,
		},
		AutoRollback:               opts.AutoRollback,
		InstallGraphPath:           opts.InstallGraphPath,
		InstallReportPath:          opts.InstallReportPath,
		NoShowNotes:                opts.NoShowNotes,
		NoProgressTablePrint:       opts.NoProgressTablePrint,
		ProgressTablePrintInterval: opts.ProgressTablePrintInterval,
		NetworkParallelism:         opts.NetworkParallelism,
	})
}

func applyReleaseInstallOptionsDefaults(opts ReleaseInstallOptions, currentDir, homeDir string) (ReleaseInstallOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleaseInstallOptions{}, fmt.Errorf("create temp dir: %w", err)
		}
	}

	opts.KubeConnectionOptions.ApplyDefaults(homeDir)
	opts.ChartRepoConnectionOptions.ApplyDefaults()
	opts.ValuesOptions.ApplyDefaults()
	opts.SecretValuesOptions.ApplyDefaults(currentDir)
	opts.TrackingOptions.ApplyDefaults()

	if opts.Chart == "" && opts.ChartDirPath != "" {
		opts.Chart = opts.ChartDirPath
	} else if opts.ChartDirPath == "" && opts.Chart == "" {
		opts.Chart = currentDir
	}

	if opts.LegacyLogRegistryStreamOut == nil {
		opts.LegacyLogRegistryStreamOut = io.Discard
	}

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = common.DefaultNetworkParallelism
	}

	if opts.ReleaseHistoryLimit <= 0 {
		opts.ReleaseHistoryLimit = common.DefaultReleaseHistoryLimit
	}

	switch opts.ReleaseStorageDriver {
	case common.ReleaseStorageDriverDefault:
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
	case common.ReleaseStorageDriverMemory:
		return ReleaseInstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
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
