package action

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/quick"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/pkg/homedir"
	"github.com/gookit/color"
	"github.com/samber/lo"
	"github.com/xo/terminfo"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	kdutil "github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/log"
)

func init() {
	style := lo.Must(chroma.NewXMLStyle(strings.NewReader(syntaxHighlightTheme)))
	styles.Register(style)
}

const (
	ReleaseStorageDriverDefault    = ""
	ReleaseStorageDriverSecrets    = "secrets"
	ReleaseStorageDriverSecret     = "secret"
	ReleaseStorageDriverConfigMaps = "configmaps"
	ReleaseStorageDriverConfigMap  = "configmap"
	ReleaseStorageDriverMemory     = "memory"
	ReleaseStorageDriverSQL        = "sql"
)

const (
	YamlOutputFormat  = "yaml"
	JSONOutputFormat  = "json"
	TableOutputFormat = "table"
)

const (
	DefaultQPSLimit              = 30
	DefaultBurstLimit            = 100
	DefaultNetworkParallelism    = 30
	DefaultDiffContextLines      = 3
	DefaultLocalKubeVersion      = "1.20.0"
	DefaultProgressPrintInterval = 5 * time.Second
	DefaultReleaseHistoryLimit   = 10
	DefaultLogColorMode          = log.LogColorModeAuto

	StubReleaseName      = "stub-release"
	StubReleaseNamespace = "stub-namespace"
)

var DefaultRegistryCredentialsPath = filepath.Join(homedir.Get(), ".docker", config.ConfigFileName)

const syntaxHighlightThemeName = "solarized-dark-customized"

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
	NetworkParallelism    int
	TrackReadinessTimeout time.Duration
	TrackCreationTimeout  time.Duration
	TrackDeletionTimeout  time.Duration
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
) (result *runFailurePlanResult, nonCritErrs, critErrs []error) {
	log.Default.Debug(ctx, "Build failure plan")

	failurePlan, err := plan.BuildFailurePlan(failedPlan, installableInfos, releaseInfos)
	if err != nil {
		return nil, nonCritErrs, append(critErrs, fmt.Errorf("build failure plan: %w", err))
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
		NetworkParallelism: opts.NetworkParallelism,
		ReadinessTimeout:   opts.TrackReadinessTimeout,
		PresenceTimeout:    opts.TrackCreationTimeout,
		AbsenceTimeout:     opts.TrackDeletionTimeout,
	}); err != nil {
		critErrs = append(critErrs, fmt.Errorf("execute failure plan: %w", err))
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
