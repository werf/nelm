package action

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/gookit/color"
	"github.com/samber/lo"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/track"
	"github.com/xo/terminfo"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

func init() {
	style := lo.Must(chroma.NewXMLStyle(strings.NewReader(syntaxHighlightTheme)))
	styles.Register(style)
}

const (
	syntaxHighlightThemeName = "solarized-dark-customized"
)

var syntaxHighlightTheme = fmt.Sprintf(`
<style name=%q>
  <entry type="Other" style="#9c4c2a"/>
  <entry type="Keyword" style="#719e07"/>
  <entry type="KeywordConstant" style="#9c4c2a"/>
  <entry type="KeywordDeclaration" style="#3f7541"/>
  <entry type="KeywordReserved" style="#3f7541"/>
  <entry type="KeywordType" style="#a14240"/>
  <entry type="NameBuiltin" style="#b58900"/>
  <entry type="NameBuiltinPseudo" style="#018727"/>
  <entry type="NameClass" style="#3f7541"/>
  <entry type="NameConstant" style="#9c4c2a"/>
  <entry type="NameDecorator" style="#3f7541"/>
  <entry type="NameEntity" style="#9c4c2a"/>
  <entry type="NameException" style="#9c4c2a"/>
  <entry type="NameFunction" style="#3f7541"/>
  <entry type="NameTag" style="#3f7541"/>
  <entry type="NameVariable" style="#3f7541"/>
  <entry type="LiteralStringBacktick" style="#586e75"/>
  <entry type="LiteralStringChar" style="#328a82"/>
  <entry type="LiteralStringEscape" style="#9c4c2a"/>
  <entry type="LiteralStringRegex" style="#a14240"/>
  <entry type="LiteralNumber" style="#328a82"/>
  <entry type="Operator" style="#719e07"/>
  <entry type="Comment" style="#586e75"/>
  <entry type="CommentSpecial" style="#719e07"/>
  <entry type="CommentPreproc" style="#719e07"/>
  <entry type="GenericDeleted" style="#a14240"/>
  <entry type="GenericEmph" style="italic"/>
  <entry type="GenericError" style="bold #a14240"/>
  <entry type="GenericHeading" style="#9c4c2a"/>
  <entry type="GenericInserted" style="#719e07"/>
  <entry type="GenericStrong" style="bold"/>
  <entry type="GenericSubheading" style="#3f7541"/>
</style>
`, syntaxHighlightThemeName)

// TODO(v2): Version > APIVersion as string "v3"
type releaseReportV3 struct {
	Version             int                `json:"version,omitempty"`
	Release             string             `json:"release,omitempty"`
	Namespace           string             `json:"namespace,omitempty"`
	Revision            int                `json:"revision,omitempty"`
	Status              helmrelease.Status `json:"status,omitempty"`
	CompletedOperations []string           `json:"completedOperations,omitempty"`
	CanceledOperations  []string           `json:"canceledOperations,omitempty"`
	FailedOperations    []string           `json:"failedOperations,omitempty"`
}

func writeWithSyntaxHighlight(outStream io.Writer, text, lang string, colorLevel terminfo.ColorLevel) error {
	if colorLevel == color.LevelNo {
		if _, err := outStream.Write([]byte(text)); err != nil {
			return fmt.Errorf("write to output: %w", err)
		}

		return nil
	}

	var formatterName string
	switch colorLevel {
	case color.Level16:
		formatterName = "terminal16"
	case color.Level256:
		formatterName = "terminal256"
	case color.LevelRgb:
		formatterName = "terminal16m"
	default:
		panic(fmt.Sprintf("unexpected color level %d", colorLevel))
	}

	if err := quick.Highlight(outStream, text, lang, formatterName, syntaxHighlightThemeName); err != nil {
		return fmt.Errorf("highlight and write to output: %w", err)
	}

	return nil
}

func printNotes(ctx context.Context, notes string) {
	if notes == "" {
		return
	}

	log.Default.InfoBlock(ctx, log.BlockOptions{
		BlockTitle: color.Style{color.Bold, color.Blue}.Render("Release notes"),
	}, func() {
		log.Default.Info(ctx, notes)
	})
}

func saveReport(reportPath string, report *releaseReportV3) error {
	reportByte, err := json.MarshalIndent(report, "", "\t")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	if err := os.WriteFile(reportPath, reportByte, 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	return nil
}

func printReport(ctx context.Context, report *releaseReportV3) {
	if totalOpsLen := len(report.CompletedOperations) + len(report.CanceledOperations) + len(report.FailedOperations); totalOpsLen == 0 {
		return
	}

	if len(report.CompletedOperations) > 0 {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: color.Style{color.Bold, color.Green}.Render("Completed operations"),
		}, func() {
			for _, op := range report.CompletedOperations {
				log.Default.Info(ctx, util.Capitalize(op))
			}
		})
	}

	if len(report.CanceledOperations) > 0 {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: color.Style{color.Bold, color.Yellow}.Render("Canceled operations"),
		}, func() {
			for _, op := range report.CanceledOperations {
				log.Default.Info(ctx, util.Capitalize(op))
			}
		})
	}

	if len(report.FailedOperations) > 0 {
		log.Default.InfoBlock(ctx, log.BlockOptions{
			BlockTitle: color.Style{color.Bold, color.Red}.Render("Failed operations"),
		}, func() {
			for _, op := range report.FailedOperations {
				log.Default.Info(ctx, util.Capitalize(op))
			}
		})
	}
}

type runFailureInstallPlanOptions struct {
	common.TrackingOptions

	NetworkParallelism int
}

type runFailurePlanResult struct {
	CompletedResourceOps []*plan.Operation
	CanceledResourceOps  []*plan.Operation
	FailedResourceOps    []*plan.Operation
}

func runFailurePlan(
	ctx context.Context,
	releaseNamespace string,
	failedPlan *plan.Plan,
	installableInfos []*plan.InstallableResourceInfo,
	releaseInfos []*plan.ReleaseInfo,
	taskStore *kdutil.Concurrent[*statestore.TaskStore],
	logStore *kdutil.Concurrent[*logstore.LogStore],
	informerFactory *kdutil.Concurrent[*informer.InformerFactory],
	history *release.History,
	clientFactory *kube.ClientFactory,
	opts runFailureInstallPlanOptions,
) (result *runFailurePlanResult, nonCritErrs, critErrs *util.MultiError) {
	critErrs = &util.MultiError{}
	nonCritErrs = &util.MultiError{}

	log.Default.Debug(ctx, "Build failure plan")

	failurePlan, err := plan.BuildFailurePlan(failedPlan, installableInfos, releaseInfos, plan.BuildFailurePlanOptions{
		NoFinalTracking: opts.NoFinalTracking,
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build failure plan: %w", err))
	}

	if _, planIsUseless := lo.Find(failurePlan.Operations(), func(op *plan.Operation) bool {
		switch op.Category {
		case plan.OperationCategoryResource, plan.OperationCategoryTrack, plan.OperationCategoryRelease:
			return true
		default:
			return false
		}
	}); planIsUseless {
		return &runFailurePlanResult{}, nonCritErrs, critErrs
	}

	log.Default.Debug(ctx, "Execute failure plan")

	if err := plan.ExecutePlan(ctx, releaseNamespace, failurePlan, taskStore, logStore, informerFactory, history, clientFactory, plan.ExecutePlanOptions{
		TrackingOptions:    opts.TrackingOptions,
		NetworkParallelism: opts.NetworkParallelism,
	}); err != nil {
		critErrs.Add(fmt.Errorf("execute failure plan: %w", err))
	}

	resourceOps := lo.Filter(failurePlan.Operations(), func(op *plan.Operation, _ int) bool {
		return op.Category == plan.OperationCategoryResource
	})

	completedResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusCompleted
	})

	canceledResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusPending || op.Status == plan.OperationStatusUnknown
	})

	failedResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusFailed
	})

	return &runFailurePlanResult{
		CompletedResourceOps: completedResourceOps,
		CanceledResourceOps:  canceledResourceOps,
		FailedResourceOps:    failedResourceOps,
	}, nonCritErrs, critErrs
}

func savePlanAsDot(plan *plan.Plan, path string) error {
	dotByte, err := plan.ToDOT()
	if err != nil {
		return fmt.Errorf("convert plan to DOT file: %w", err)
	}

	if err := os.WriteFile(path, dotByte, 0o600); err != nil {
		return fmt.Errorf("write DOT graph file at %q: %w", path, err)
	}

	return nil
}

func saveInstallPlan(ctx context.Context, p *plan.Plan, changes []*plan.ResourceChange, releaseName, releaseNamespace string, releaseVersion int, path string, deployType common.DeployType, defaultDeletePropagation, secretKey, secretWorkDir string) error {
	artifact, err := plan.BuildInstallPlanArtifact(p, changes, plan.InstallPlanArtifactRelease{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Version:   releaseVersion,
	}, plan.BuildInstallPlanArtifactOptions{
		DeployType:               deployType,
		DefaultDeletePropagation: defaultDeletePropagation,
	})
	if err != nil {
		return fmt.Errorf("build install plan artifact: %w", err)
	}

	jsonData, err := plan.MarshalInstallPlanArtifact(ctx, artifact, secretKey, secretWorkDir)
	if err != nil {
		return fmt.Errorf("marshal install plan artifact: %w", err)
	}

	if err := os.WriteFile(path, jsonData, 0o644); err != nil {
		return fmt.Errorf("write install plan at %q: %w", path, err)
	}

	return nil
}

func handleBuildPlanErr(ctx context.Context, installPlan *plan.Plan, planErr error, installGraphPath, tempDirPath, fallbackGraphFilename string) {
	var graphPath string
	if installGraphPath != "" {
		graphPath = installGraphPath
	} else {
		graphPath = filepath.Join(tempDirPath, fallbackGraphFilename)
	}

	if err := savePlanAsDot(installPlan, graphPath); err != nil {
		log.Default.Error(ctx, "Error: save plan graph: %s", err)
		return
	}

	log.Default.Warn(ctx, "Plan graph saved to %q for debugging", graphPath)
}

type runRollbackPlanOptions struct {
	common.TrackingOptions
	common.ResourceValidationOptions

	DefaultDeletePropagation string
	ExtraAnnotations         map[string]string
	ExtraLabels              map[string]string
	ExtraRuntimeAnnotations  map[string]string
	ExtraRuntimeLabels       map[string]string
	ForceAdoption            bool
	NetworkParallelism       int
	NoRemoveManualChanges    bool
	ReleaseInfoAnnotations   map[string]string
	ReleaseLabels            map[string]string
	RollbackGraphPath        string
}

type runRollbackPlanResult struct {
	CompletedResourceOps []*plan.Operation
	CanceledResourceOps  []*plan.Operation
	FailedResourceOps    []*plan.Operation
}

func runRollbackPlan(ctx context.Context, releaseName, releaseNamespace string, failedRelease, prevDeployedRelease *helmrelease.Release, taskStore *kdutil.Concurrent[*statestore.TaskStore], logStore *kdutil.Concurrent[*logstore.LogStore], informerFactory *kdutil.Concurrent[*informer.InformerFactory], history *release.History, clientFactory *kube.ClientFactory, opts runRollbackPlanOptions) (result *runRollbackPlanResult, nonCritErrs, critErrs *util.MultiError) {
	critErrs = &util.MultiError{}
	nonCritErrs = &util.MultiError{}

	log.Default.Debug(ctx, "Convert prev deployed release to resource specs")

	resSpecs, err := release.ReleaseToResourceSpecs(prevDeployedRelease, releaseNamespace, false)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("convert previous deployed release to resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Build transformed resource specs")

	transformedResSpecs, err := spec.BuildTransformedResourceSpecs(ctx, releaseNamespace, resSpecs, []spec.ResourceTransformer{
		spec.NewResourceListsTransformer(),
		spec.NewDropInvalidAnnotationsAndLabelsTransformer(),
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build transformed resource specs: %w", err))
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
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build releasable resource specs: %w", err))
	}

	newRelease, err := release.NewRelease(releaseName, releaseNamespace, failedRelease.Version+1, common.DeployTypeRollback, releasableResSpecs, prevDeployedRelease.Chart, prevDeployedRelease.Config, release.ReleaseOptions{
		InfoAnnotations: opts.ReleaseInfoAnnotations,
		Labels:          opts.ReleaseLabels,
		Notes:           prevDeployedRelease.Info.Notes,
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("construct new release: %w", err))
	}

	log.Default.Debug(ctx, "Convert failed release to resource specs")

	failedRelResSpecs, err := release.ReleaseToResourceSpecs(failedRelease, releaseNamespace, false)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("convert previous release to resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Convert new release to resource specs")

	newRelResSpecs, err := release.ReleaseToResourceSpecs(newRelease, releaseNamespace, false)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("convert new release to resource specs: %w", err))
	}

	log.Default.Debug(ctx, "Build resources")

	instResources, delResources, err := resource.BuildResources(ctx, common.DeployTypeRollback, releaseNamespace, failedRelResSpecs, newRelResSpecs, []spec.ResourcePatcher{
		spec.NewReleaseMetadataPatcher(releaseName, releaseNamespace),
		spec.NewExtraMetadataPatcher(opts.ExtraRuntimeAnnotations, opts.ExtraRuntimeLabels),
	}, clientFactory, resource.BuildResourcesOptions{
		Remote:                   true,
		DefaultDeletePropagation: metav1.DeletionPropagation(opts.DefaultDeletePropagation),
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build resources: %w", err))
	}

	log.Default.Debug(ctx, "Locally validate resources")

	if err := resource.ValidateLocal(ctx, releaseNamespace, instResources, opts.ResourceValidationOptions); err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("locally validate resources: %w", err))
	}

	log.Default.Debug(ctx, "Build resource infos")

	instResInfos, delResInfos, err := plan.BuildResourceInfos(ctx, common.DeployTypeRollback, releaseName, releaseNamespace, instResources, delResources, true, clientFactory, plan.BuildResourceInfosOptions{
		NetworkParallelism:    opts.NetworkParallelism,
		NoRemoveManualChanges: opts.NoRemoveManualChanges,
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build resource infos: %w", err))
	}

	log.Default.Debug(ctx, "Remotely validate resources")

	if err := plan.ValidateRemote(releaseName, releaseNamespace, instResInfos, opts.ForceAdoption); err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("remotely validate resources: %w", err))
	}

	releases := history.Releases()

	log.Default.Debug(ctx, "Build release infos")

	relInfos, err := plan.BuildReleaseInfos(ctx, common.DeployTypeRollback, releases, newRelease)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build release infos: %w", err))
	}

	log.Default.Debug(ctx, "Build rollback plan")

	rollbackPlan, err := plan.BuildPlan(instResInfos, delResInfos, relInfos, plan.BuildPlanOptions{
		NoFinalTracking: opts.NoFinalTracking,
	})
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("build rollback plan: %w", err))
	}

	if opts.RollbackGraphPath != "" {
		if err := savePlanAsDot(rollbackPlan, opts.RollbackGraphPath); err != nil {
			return nil, nonCritErrs, critErrs.Add(fmt.Errorf("save rollback graph: %w", err))
		}
	}

	releaseIsUpToDate, err := release.IsReleaseUpToDate(failedRelease, newRelease)
	if err != nil {
		return nil, nonCritErrs, critErrs.Add(fmt.Errorf("check if release is up to date: %w", err))
	}

	planIsUseless := lo.NoneBy(rollbackPlan.Operations(), func(op *plan.Operation) bool {
		switch op.Category {
		case plan.OperationCategoryResource, plan.OperationCategoryTrack:
			return true
		default:
			return false
		}
	})

	if releaseIsUpToDate && planIsUseless {
		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render("Skipped rollback release")+" %q (namespace: %q): cluster resources already as desired", releaseName, releaseNamespace)

		return &runRollbackPlanResult{}, nonCritErrs, critErrs
	}

	log.Default.Debug(ctx, "Execute rollback plan")

	executePlanErr := plan.ExecutePlan(ctx, releaseNamespace, rollbackPlan, taskStore, logStore, informerFactory, history, clientFactory, plan.ExecutePlanOptions{
		TrackingOptions:    opts.TrackingOptions,
		NetworkParallelism: opts.NetworkParallelism,
	})
	if executePlanErr != nil {
		critErrs.Add(fmt.Errorf("execute rollback plan: %w", executePlanErr))
	}

	resourceOps := lo.Filter(rollbackPlan.Operations(), func(op *plan.Operation, _ int) bool {
		return op.Category == plan.OperationCategoryResource
	})

	completedResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusCompleted
	})

	canceledResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusPending || op.Status == plan.OperationStatusUnknown
	})

	failedResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusFailed
	})

	if executePlanErr != nil {
		runFailurePlanResult, nonCrErrs, crErrs := runFailurePlan(ctx, releaseNamespace, rollbackPlan, instResInfos, relInfos, taskStore, logStore, informerFactory, history, clientFactory, runFailureInstallPlanOptions{
			TrackingOptions:    opts.TrackingOptions,
			NetworkParallelism: opts.NetworkParallelism,
		})

		critErrs.Add(crErrs)
		nonCritErrs.Add(nonCrErrs)

		if runFailurePlanResult != nil {
			completedResourceOps = append(completedResourceOps, runFailurePlanResult.CompletedResourceOps...)
			canceledResourceOps = append(canceledResourceOps, runFailurePlanResult.CanceledResourceOps...)
			failedResourceOps = append(failedResourceOps, runFailurePlanResult.FailedResourceOps...)
		}
	}

	return &runRollbackPlanResult{
		CompletedResourceOps: completedResourceOps,
		CanceledResourceOps:  canceledResourceOps,
		FailedResourceOps:    failedResourceOps,
	}, nonCritErrs, critErrs
}

func createReleaseNamespace(ctx context.Context, clientFactory *kube.ClientFactory, releaseNamespace string) error {
	unstruct := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": releaseNamespace,
			},
		},
	}

	resSpec := spec.NewResourceSpec(unstruct, releaseNamespace, spec.ResourceSpecOptions{})

	if _, err := clientFactory.KubeClient().Get(ctx, resSpec.ResourceMeta, kube.KubeClientGetOptions{
		TryCache: true,
	}); err != nil {
		if kube.IsNotFoundErr(err) {
			log.Default.Debug(ctx, "Create release namespace %q", releaseNamespace)

			if _, err := clientFactory.KubeClient().Create(ctx, resSpec, kube.KubeClientCreateOptions{}); err != nil {
				return fmt.Errorf("create release namespace: %w", err)
			}
		} else if errors.IsForbidden(err) {
		} else {
			return fmt.Errorf("get release namespace: %w", err)
		}
	}

	return nil
}

type runInstallPlanOptions struct {
	common.TrackingOptions
	runRollbackPlanOptions

	AutoRollback               bool
	InstallGraphPath           string
	InstallReportPath          string
	NoShowNotes                bool
	NoProgressTablePrint       bool
	ProgressTablePrintInterval time.Duration
	NetworkParallelism         int
}

func runInstallPlan(ctx context.Context, ctxCancelFn context.CancelCauseFunc, releaseName string, releaseNamespace string, installPlan *plan.Plan, prevRelease *helmrelease.Release, newRelease *helmrelease.Release, clientFactory *kube.ClientFactory, history *release.History, instResInfos []*plan.InstallableResourceInfo, relInfos []*plan.ReleaseInfo, prevDeployedRelease *helmrelease.Release, opts runInstallPlanOptions) error {
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
		if opts.InstallReportPath != "" {
			if err := saveReport(opts.InstallReportPath, &releaseReportV3{
				Version:   3,
				Release:   releaseName,
				Namespace: releaseNamespace,
				Revision:  newRelease.Version,
				Status:    helmrelease.StatusSkipped,
			}); err != nil {
				return fmt.Errorf("save release install report: %w", err)
			}
		}

		if !opts.NoShowNotes {
			printNotes(ctx, newRelease.Info.Notes)
		}

		log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Skipped release %q (namespace: %q): cluster resources already as desired", releaseName, releaseNamespace)))

		return nil
	}

	taskStore := kdutil.NewConcurrent(statestore.NewTaskStore())
	logStore := kdutil.NewConcurrent(logstore.NewLogStore())
	watchErrCh := make(chan error, 1)
	informerFactory := informer.NewConcurrentInformerFactory(ctx.Done(), watchErrCh, clientFactory.Dynamic(), informer.ConcurrentInformerFactoryOptions{})

	log.Default.Debug(ctx, "Start tracking")

	go func() {
		if err := <-watchErrCh; err != nil {
			ctxCancelFn(fmt.Errorf("context canceled: watch error: %w", err))
		}
	}()

	var progressPrinter *track.ProgressTablesPrinter
	if !opts.NoProgressTablePrint {
		progressPrinter = track.NewProgressTablesPrinter(taskStore, logStore, track.ProgressTablesPrinterOptions{
			DefaultNamespace: releaseNamespace,
		})
		progressPrinter.Start(ctx, opts.ProgressTablePrintInterval)
	}

	criticalErrs := &util.MultiError{}
	nonCriticalErrs := &util.MultiError{}

	log.Default.Debug(ctx, "Execute release install plan")

	executePlanErr := plan.ExecutePlan(ctx, releaseNamespace, installPlan, taskStore, logStore, informerFactory, history, clientFactory, plan.ExecutePlanOptions{
		TrackingOptions:    opts.TrackingOptions,
		NetworkParallelism: opts.NetworkParallelism,
	})
	if executePlanErr != nil {
		criticalErrs.Add(fmt.Errorf("execute release install plan: %w", executePlanErr))
	}

	resourceOps := lo.Filter(installPlan.Operations(), func(op *plan.Operation, _ int) bool {
		return op.Category == plan.OperationCategoryResource
	})

	completedResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusCompleted
	})

	canceledResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusPending || op.Status == plan.OperationStatusUnknown
	})

	failedResourceOps := lo.Filter(resourceOps, func(op *plan.Operation, _ int) bool {
		return op.Status == plan.OperationStatusFailed
	})

	if executePlanErr != nil {
		runFailurePlanResult, nonCritErrs, critErrs := runFailurePlan(ctx, releaseNamespace, installPlan, instResInfos, relInfos, taskStore, logStore, informerFactory, history, clientFactory, runFailureInstallPlanOptions{
			TrackingOptions:    opts.TrackingOptions,
			NetworkParallelism: opts.NetworkParallelism,
		})

		criticalErrs.Add(critErrs)
		nonCriticalErrs.Add(nonCritErrs)

		if runFailurePlanResult != nil {
			completedResourceOps = append(completedResourceOps, runFailurePlanResult.CompletedResourceOps...)
			canceledResourceOps = append(canceledResourceOps, runFailurePlanResult.CanceledResourceOps...)
			failedResourceOps = append(failedResourceOps, runFailurePlanResult.FailedResourceOps...)
		}

		if opts.AutoRollback && prevDeployedRelease != nil {
			runRollbackPlanResult, nonCritErrs, critErrs := runRollbackPlan(ctx, releaseName, releaseNamespace, newRelease, prevDeployedRelease, taskStore, logStore, informerFactory, history, clientFactory, runRollbackPlanOptions{
				DefaultDeletePropagation:  opts.DefaultDeletePropagation,
				TrackingOptions:           opts.TrackingOptions,
				ExtraAnnotations:          opts.ExtraAnnotations,
				ExtraLabels:               opts.ExtraLabels,
				ExtraRuntimeAnnotations:   opts.ExtraRuntimeAnnotations,
				ExtraRuntimeLabels:        opts.ExtraRuntimeLabels,
				ForceAdoption:             opts.ForceAdoption,
				NetworkParallelism:        opts.NetworkParallelism,
				NoRemoveManualChanges:     opts.NoRemoveManualChanges,
				ReleaseInfoAnnotations:    opts.ReleaseInfoAnnotations,
				ReleaseLabels:             opts.ReleaseLabels,
				RollbackGraphPath:         opts.RollbackGraphPath,
				ResourceValidationOptions: opts.ResourceValidationOptions,
			})

			criticalErrs.Add(critErrs)
			nonCriticalErrs.Add(nonCritErrs)

			if runRollbackPlanResult != nil {
				completedResourceOps = append(completedResourceOps, runRollbackPlanResult.CompletedResourceOps...)
				canceledResourceOps = append(canceledResourceOps, runRollbackPlanResult.CanceledResourceOps...)
				failedResourceOps = append(failedResourceOps, runRollbackPlanResult.FailedResourceOps...)
			}
		}
	}

	if !opts.NoProgressTablePrint {
		progressPrinter.Stop()
		progressPrinter.Wait()
	}

	reportCompletedOps := lo.Map(completedResourceOps, func(op *plan.Operation, _ int) string {
		return op.IDHuman()
	})

	reportCanceledOps := lo.Map(canceledResourceOps, func(op *plan.Operation, _ int) string {
		return op.IDHuman()
	})

	reportFailedOps := lo.Map(failedResourceOps, func(op *plan.Operation, _ int) string {
		return op.IDHuman()
	})

	sort.Strings(reportCompletedOps)
	sort.Strings(reportCanceledOps)
	sort.Strings(reportFailedOps)

	report := &releaseReportV3{
		Version:             3,
		Release:             releaseName,
		Namespace:           releaseNamespace,
		Revision:            newRelease.Version,
		Status:              lo.Ternary(executePlanErr == nil, helmrelease.StatusDeployed, helmrelease.StatusFailed),
		CompletedOperations: reportCompletedOps,
		CanceledOperations:  reportCanceledOps,
		FailedOperations:    reportFailedOps,
	}

	printReport(ctx, report)

	if opts.InstallReportPath != "" {
		if err := saveReport(opts.InstallReportPath, report); err != nil {
			nonCriticalErrs.Add(fmt.Errorf("save release install report: %w", err))
		}
	}

	if !criticalErrs.HasErrors() && !opts.NoShowNotes {
		printNotes(ctx, newRelease.Info.Notes)
	}

	if criticalErrs.HasErrors() {
		allErrs := &util.MultiError{}
		allErrs.Add(criticalErrs, nonCriticalErrs)

		return fmt.Errorf("failed release %q (namespace: %q): %w", releaseName, releaseNamespace, allErrs)
	} else if nonCriticalErrs.HasErrors() {
		return fmt.Errorf("succeeded release %q (namespace: %q), but non-critical errors encountered: %w", releaseName, releaseNamespace, nonCriticalErrs)
	}

	log.Default.Info(ctx, color.Style{color.Bold, color.Green}.Render(fmt.Sprintf("Succeeded release %q (namespace: %q)", releaseName, releaseNamespace)))

	return nil
}

func logPlannedChanges(
	ctx context.Context,
	releaseName string,
	releaseNamespace string,
	changes []*plan.ResourceChange,
	opts plan.CalculatePlannedChangesOptions,
) error {
	if len(changes) == 0 {
		return nil
	}

	log.Default.Info(ctx, "")

	for _, change := range changes {
		if err := log.Default.InfoBlockErr(ctx, log.BlockOptions{
			BlockTitle: buildDiffHeader(change),
		}, func() error {
			uDiff, err := change.GetUDiff(opts)
			if err != nil {
				return fmt.Errorf("calculate diff for resource %s: %w", change.ResourceMeta.IDHuman(), err)
			}

			log.Default.Info(ctx, "%s", uDiff)

			if change.Reason != "" {
				log.Default.Info(ctx, "<%s reason: %s>", change.Type, change.Reason)
			}

			return nil
		}); err != nil {
			return err
		}
	}

	log.Default.Info(ctx, color.Bold.Render("Planned changes summary")+" for release %q (namespace: %q):", releaseName, releaseNamespace)

	for _, changeType := range []string{"create", "recreate", "update", "blind apply", "delete"} {
		logSummaryLine(ctx, changes, changeType)
	}

	log.Default.Info(ctx, "")

	return nil
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
