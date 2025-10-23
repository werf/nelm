package action

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gookit/color"
	"github.com/pkg/errors"
	"github.com/samber/lo"

	"github.com/werf/3p-helm/pkg/registry"
	"github.com/werf/3p-helm/pkg/werf/helmopts"
	"github.com/werf/nelm/internal/chart"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const (
	DefaultReleasePlanInstallLogLevel = log.InfoLevel
)

var ErrChangesPlanned = errors.New("changes planned")

type ReleasePlanInstallOptions struct {
	common.KubeConnectionOptions
	common.ChartRepoConnectionOptions
	common.ValuesOptions
	common.SecretValuesOptions

	Chart                       string
	ChartAppVersion             string
	ChartDirPath                string // TODO(v2): get rid
	ChartProvenanceKeyring      string
	ChartProvenanceStrategy     string
	ChartRepoSkipUpdate         bool
	ChartVersion                string
	DefaultChartAPIVersion      string
	DefaultChartName            string
	DefaultChartVersion         string
	DiffContextLines            int
	ErrorIfChangesPlanned       bool
	ExtraAnnotations            map[string]string
	ExtraLabels                 map[string]string
	ExtraRuntimeAnnotations     map[string]string
	ExtraRuntimeLabels          map[string]string
	ForceAdoption               bool
	InstallGraphPath            string
	LegacyChartType             helmopts.ChartType
	LegacyExtraValues           map[string]interface{}
	LegacyLogRegistryStreamOut  io.Writer
	NetworkParallelism          int
	NoFinalTracking             bool
	NoInstallStandaloneCRDs     bool
	NoRemoveManualChanges       bool
	RegistryCredentialsPath     string
	ReleaseInfoAnnotations      map[string]string
	ReleaseLabels               map[string]string
	ReleaseStorageDriver        string
	ReleaseStorageSQLConnection string
	ShowInsignificantDiffs      bool
	ShowSensitiveDiffs          bool
	ShowVerboseCRDDiffs         bool
	ShowVerboseDiffs            bool
	TempDirPath                 string
	TemplatesAllowDNS           bool
	Timeout                     time.Duration
}

func ReleasePlanInstall(ctx context.Context, releaseName, releaseNamespace string, opts ReleasePlanInstallOptions) error {
	ctx, ctxCancelFn := context.WithCancelCause(ctx)

	if opts.Timeout == 0 {
		return releasePlanInstall(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
	}

	ctx, _ = context.WithTimeoutCause(ctx, opts.Timeout, fmt.Errorf("context timed out: action timed out after %s", opts.Timeout.String()))
	defer ctxCancelFn(fmt.Errorf("context canceled: action finished"))

	actionCh := make(chan error, 1)
	go func() {
		actionCh <- releasePlanInstall(ctx, ctxCancelFn, releaseName, releaseNamespace, opts)
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

func releasePlanInstall(ctx context.Context, ctxCancelFn context.CancelCauseFunc, releaseName, releaseNamespace string, opts ReleasePlanInstallOptions) error {
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
		SQLConnection: opts.ReleaseStorageSQLConnection,
	})
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

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Planning release install")+" %q (namespace: %q)", releaseName, releaseNamespace)

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

	releasableResSpecs, err := spec.BuildReleasableResourceSpecs(ctx, releaseNamespace, transformedResSpecs, []spec.ResourcePatcher{
		spec.NewExtraMetadataPatcher(opts.ExtraAnnotations, opts.ExtraLabels),
	})
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
		prevRelResSpecs, err = release.ReleaseToResourceSpecs(prevRelease, releaseNamespace)
		if err != nil {
			return fmt.Errorf("convert previous release to resource specs: %w", err)
		}
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, releaseNamespace)
	if err != nil {
		return fmt.Errorf("convert new release to resource specs: %w", err)
	}

	log.Default.Debug(ctx, "Build resources")

	instResources, delResources, err := resource.BuildResources(ctx, deployType, releaseNamespace, prevRelResSpecs, newRelResSpecs, []spec.ResourcePatcher{
		spec.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
		spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
	}, clientFactory, resource.BuildResourcesOptions{
		Remote: true,
	})
	if err != nil {
		return fmt.Errorf("build resources: %w", err)
	}

	log.Default.Debug(ctx, "Locally validate resources")

	if err := resource.ValidateLocal(releaseNamespace, instResources); err != nil {
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

	if opts.InstallGraphPath != "" {
		if err := savePlanAsDot(installPlan, opts.InstallGraphPath); err != nil {
			return fmt.Errorf("save release install graph: %w", err)
		}
	}

	releaseIsUpToDate, err := release.IsReleaseUpToDate(prevRelease, newRelease)
	if err != nil {
		return fmt.Errorf("check if release is up to date: %w", err)
	}

	installPlanIsUseless := lo.NoneBy(installPlan.Operations(), func(op *plan.Operation) bool {
		switch op.Category {
		case plan.OperationCategoryResource, plan.OperationCategoryTrack:
			return true
		default:
			return false
		}
	})

	if releaseIsUpToDate && installPlanIsUseless {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("No changes planned for release %q (namespace: %q)", releaseName, releaseNamespace)))
	} else if installPlanIsUseless {
		log.Default.Info(ctx, color.Style{color.Bold, color.Yellow}.Render(fmt.Sprintf("No resource changes planned, but still must install release %q (namespace: %q)", releaseName, releaseNamespace)))
	}

	log.Default.Debug(ctx, "Calculate planned changes")

	changes, err := plan.CalculatePlannedChanges(instResInfos, delResInfos, plan.CalculatePlannedChangesOptions{
		DiffContextLines:       opts.DiffContextLines,
		ShowVerboseCRDDiffs:    opts.ShowVerboseCRDDiffs,
		ShowVerboseDiffs:       opts.ShowVerboseDiffs,
		ShowSensitiveDiffs:     opts.ShowSensitiveDiffs,
		ShowInsignificantDiffs: opts.ShowInsignificantDiffs,
	})
	if err != nil {
		return fmt.Errorf("calculate planned changes: %w", err)
	}

	logPlannedChanges(ctx, releaseName, releaseNamespace, changes)

	if opts.ErrorIfChangesPlanned && (!releaseIsUpToDate || !installPlanIsUseless) {
		return ErrChangesPlanned
	}

	return nil
}

func applyReleasePlanInstallOptionsDefaults(opts ReleasePlanInstallOptions, currentDir, homeDir string) (ReleasePlanInstallOptions, error) {
	var err error
	if opts.TempDirPath == "" {
		opts.TempDirPath, err = os.MkdirTemp("", "")
		if err != nil {
			return ReleasePlanInstallOptions{}, fmt.Errorf("create temp dir: %w", err)
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

	if opts.LegacyLogRegistryStreamOut == nil {
		opts.LegacyLogRegistryStreamOut = io.Discard
	}

	if opts.NetworkParallelism <= 0 {
		opts.NetworkParallelism = common.DefaultNetworkParallelism
	}

	if opts.DiffContextLines < 0 {
		opts.DiffContextLines = common.DefaultDiffContextLines
	}

	switch opts.ReleaseStorageDriver {
	case common.ReleaseStorageDriverDefault:
		opts.ReleaseStorageDriver = common.ReleaseStorageDriverSecrets
	case common.ReleaseStorageDriverMemory:
		return ReleasePlanInstallOptions{}, fmt.Errorf("memory release storage driver is not supported")
	}

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = common.DefaultRegistryCredentialsPath
	}

	if opts.ChartProvenanceStrategy == "" {
		opts.ChartProvenanceStrategy = common.DefaultChartProvenanceStrategy
	}

	return opts, nil
}

func logPlannedChanges(
	ctx context.Context,
	releaseName string,
	releaseNamespace string,
	changes []*plan.ResourceChange,
) {
	if len(changes) == 0 {
		return
	}

	log.Default.Info(ctx, "")

	for _, change := range changes {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: buildDiffHeader(change),
		}, func() {
			log.Default.Info(ctx, "%s", change.Udiff)

			if change.Reason != "" {
				log.Default.Info(ctx, "<%s reason: %s>", change.Type, change.Reason)
			}
		})
	}

	log.Default.Info(ctx, color.Bold.Render("Planned changes summary")+" for release %q (namespace: %q):", releaseName, releaseNamespace)

	for _, changeType := range []string{"create", "recreate", "update", "blind apply", "delete"} {
		logSummaryLine(ctx, changes, changeType)
	}

	log.Default.Info(ctx, "")
}

func buildDiffHeader(change *plan.ResourceChange) string {
	header := change.TypeStyle.Render(util.Capitalize(change.Type))
	header += " " + color.Style{color.Bold}.Render(change.ResourceMeta.IDHuman())

	var headerOps []string
	for _, op := range change.ExtraOperations {
		headerOps = append(headerOps, color.Style{color.Bold}.Render(op))
	}

	if len(headerOps) > 0 {
		header += ", then " + strings.Join(headerOps, ", ")
	}

	return header
}

func logSummaryLine(ctx context.Context, changes []*plan.ResourceChange, changeType string) {
	filteredChanges := lo.Filter(changes, func(change *plan.ResourceChange, _ int) bool {
		return change.Type == changeType
	})

	if len(filteredChanges) > 0 {
		log.Default.Info(ctx, "- %s: %d resources", filteredChanges[0].TypeStyle.Render(changeType), len(filteredChanges))
	}
}
