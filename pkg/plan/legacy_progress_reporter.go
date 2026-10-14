package plan

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dominikbraun/graph"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"

	kdutil "github.com/werf/kubedog/pkg/dyntracker/util"
	"github.com/werf/nelm/v2/pkg/legacy/progrep"
)

type LegacyProgressReporter struct {
	reportCh chan<- progrep.ProgressReport
	state    *kdutil.Concurrent[*progressReporterState]
}

func NewLegacyProgressReporter(reportCh chan<- progrep.ProgressReport) *LegacyProgressReporter {
	if cap(reportCh) < 1 {
		panic(fmt.Sprintf("LegacyProgressReportCh must be a buffered channel with capacity >= 1, got capacity %d", cap(reportCh)))
	}

	return &LegacyProgressReporter{
		reportCh: reportCh,
		state: kdutil.NewConcurrent(&progressReporterState{
			opIndex: make(map[string]int),
		}),
	}
}

// Close stops reporting and closes the channel after in-flight sends finish. Its context does
// not interrupt those sends: cancel any blocking send's context before calling Close.
// Close does not send a final snapshot.
func (r *LegacyProgressReporter) Close(ctx context.Context) {
	r.state.RWTransaction(func(s *progressReporterState) {
		if s.closed {
			return
		}

		s.closed = true
		s.stopped = true

		close(r.reportCh)
	})
}

func (r *LegacyProgressReporter) ReportAnalyzedResource(ctx context.Context) {
	r.state.RWTransaction(func(s *progressReporterState) {
		if s.stopped || s.analyzeDetails == nil {
			return
		}

		s.analyzeDetails.Processed++

		if len(r.reportCh) == cap(r.reportCh) {
			return
		}

		sendNonBlocking(r.reportCh, buildProgressReport(s))
	})
}

// ReportCheckpoint sends the current snapshot with a blocking send without stopping reporting
// or changing pending statuses. Cancellation can interrupt the send.
func (r *LegacyProgressReporter) ReportCheckpoint(ctx context.Context) {
	r.state.RWTransaction(func(s *progressReporterState) {
		if s.stopped || (s.plansCount == 0 && !s.phaseReported) {
			return
		}

		sendBlocking(ctx, r.reportCh, buildProgressReport(s))
	})
}

func (r *LegacyProgressReporter) ReportPhase(ctx context.Context, phase progrep.PhaseKind, opts ReportPhaseOptions) {
	r.state.RWTransaction(func(s *progressReporterState) {
		if s.stopped {
			return
		}

		s.currentPhase = phase

		s.phaseReported = true
		if opts.RenderDetails != nil {
			s.renderDetails = copyRenderDetails(opts.RenderDetails)
		}

		if opts.AnalyzeDetails != nil {
			details := *opts.AnalyzeDetails
			s.analyzeDetails = &details
		}

		if opts.NamespaceDeleteDetails != nil {
			details := *opts.NamespaceDeleteDetails
			s.namespaceDeleteDetails = &details
		}

		if len(r.reportCh) == cap(r.reportCh) {
			return
		}

		sendNonBlocking(r.reportCh, buildProgressReport(s))
	})
}

func (r *LegacyProgressReporter) ReportStatus(opID string, status progrep.OperationStatus) {
	r.state.RWTransaction(func(s *progressReporterState) {
		if s.stopped {
			return
		}

		idx, ok := s.opIndex[opID]
		if !ok {
			return
		}

		s.ops[idx].Status = status

		if len(r.reportCh) == cap(r.reportCh) {
			return
		}

		sendNonBlocking(r.reportCh, buildProgressReport(s))
	})
}

// StartPlan appends the operations of the plan that is about to be executed to the report and
// makes them addressable by ReportStatus. Operations of the previously started plans stay in the
// report as they are. The plans are chained: the root operations of the new plan depend on the
// final operations of the previous one, so the whole run reads as a single graph. Operations of
// the previous plan that never started are marked Canceled. Untouched resources are reported for
// the first plan only: later plans act upon a release the first plan has already described in
// full.
func (r *LegacyProgressReporter) StartPlan(p *Plan, releaseNamespace string, installableResourceInfos []*InstallableResourceInfo, mapper meta.RESTMapper, opts StartPlanOptions) {
	var stopped bool
	r.state.RTransaction(func(s *progressReporterState) { stopped = s.stopped })

	if stopped {
		return
	}

	resolvedNamespaces := buildResolvedNamespaces(p, releaseNamespace, mapper)

	untouchedResolvedNamespaces := make(map[string]string, len(installableResourceInfos))
	for _, info := range installableResourceInfos {
		untouchedResolvedNamespaces[info.ID()] = resolveNamespace(info.GroupVersionKind, info.Namespace, releaseNamespace, mapper)
	}

	r.state.RWTransaction(func(s *progressReporterState) {
		if s.stopped {
			return
		}

		cancelPendingOperations(s.ops)

		s.plansCount++
		idPrefix := planIDPrefix(s.plansCount)

		var planOps []progrep.Operation
		if !opts.UntouchedResourcesOnly {
			predMap := lo.Must(p.Graph.PredecessorMap())

			planOps = buildPlanOperations(p, predMap, resolvedNamespaces, idPrefix, s.lastPlanSinkIDs)
			s.lastPlanSinkIDs = planSinkOperationIDs(predMap, idPrefix)
		}

		seenRefs := make(map[progrep.ObjectRef]struct{}, len(planOps))
		for _, op := range planOps {
			if op.Category == progrep.OperationCategoryResource || op.Category == progrep.OperationCategoryTrack {
				seenRefs[op.ObjectRef] = struct{}{}
			}
		}

		if s.plansCount == 1 {
			s.ops = append(s.ops, buildUntouchedOperations(installableResourceInfos, untouchedResolvedNamespaces, seenRefs)...)
		}

		s.opIndex = make(map[string]int, len(planOps))
		for _, op := range planOps {
			s.opIndex[strings.TrimPrefix(op.ID, idPrefix)] = len(s.ops)
			s.ops = append(s.ops, op)
		}

		sendNonBlocking(r.reportCh, buildProgressReport(s))
	})
}

// Stop sends the final report with a blocking send. Operations that never started are marked
// Canceled first: nothing is going to run them anymore.
func (r *LegacyProgressReporter) Stop(ctx context.Context) {
	r.state.RWTransaction(func(s *progressReporterState) {
		if s.stopped {
			return
		}

		s.stopped = true
		if s.plansCount == 0 && !s.phaseReported {
			return
		}

		cancelPendingOperations(s.ops)

		if s.namespaceDeleteDetails != nil {
			if s.namespaceDeleteDetails.DeleteStatus == progrep.OperationStatusPending {
				s.namespaceDeleteDetails.DeleteStatus = progrep.OperationStatusCanceled
			}

			if s.namespaceDeleteDetails.TrackAbsenceStatus == progrep.OperationStatusPending {
				s.namespaceDeleteDetails.TrackAbsenceStatus = progrep.OperationStatusCanceled
			}
		}

		sendBlocking(ctx, r.reportCh, buildProgressReport(s))
	})
}

// ReportPhaseOptions replaces each nonnil detail with an owned copy; nil retains the existing
// detail. Emitted reports own their data independently of the options and other snapshots.
type ReportPhaseOptions struct {
	AnalyzeDetails         *progrep.AnalyzeDetails
	NamespaceDeleteDetails *progrep.NamespaceDeleteDetails
	RenderDetails          *progrep.RenderDetails
}

type StartPlanOptions struct {
	// UntouchedResourcesOnly, when true, omits the plan's own operations and reports the untouched
	// resources alone. Set it for a plan that is not going to be executed: its operations would
	// otherwise stay Pending forever.
	UntouchedResourcesOnly bool
}

type progressReporterState struct {
	analyzeDetails *progrep.AnalyzeDetails
	closed         bool
	currentPhase   progrep.PhaseKind
	// lastPlanSinkIDs are the report IDs of the operations without successors in the most recently
	// started plan that had operations. The root operations of the next plan depend on them.
	lastPlanSinkIDs        []string
	namespaceDeleteDetails *progrep.NamespaceDeleteDetails
	// opIndex maps the raw operation IDs of the most recently started plan to their positions in
	// ops. Operations of earlier plans are no longer addressable: by the time the next plan starts
	// they are either done or canceled.
	opIndex       map[string]int
	ops           []progrep.Operation
	phaseReported bool
	plansCount    int
	renderDetails *progrep.RenderDetails
	stopped       bool
}

func buildProgressReport(s *progressReporterState) progrep.ProgressReport {
	operations := make([]progrep.Operation, len(s.ops))
	copy(operations, s.ops)

	for i := range operations {
		operations[i].DependsOn = append([]string{}, operations[i].DependsOn...)
	}

	report := progrep.ProgressReport{
		CurrentPhase:  s.currentPhase,
		RenderDetails: copyRenderDetails(s.renderDetails),
		Operations:    operations,
	}
	if s.analyzeDetails != nil {
		details := *s.analyzeDetails
		report.AnalyzeDetails = &details
	}

	if s.namespaceDeleteDetails != nil {
		details := *s.namespaceDeleteDetails
		report.NamespaceDeleteDetails = &details
	}

	return report
}

func sendNonBlocking(ch chan<- progrep.ProgressReport, report progrep.ProgressReport) {
	safeSend(ch, report)
}

func buildPlanOperations(p *Plan, predMap map[string]map[string]graph.Edge[string], resolvedNamespaces map[string]string, idPrefix string, rootDependsOn []string) []progrep.Operation {
	opIDs := lo.Must(graph.StableTopologicalSort(p.Graph, func(a, b string) bool {
		return a < b
	}))

	result := make([]progrep.Operation, 0, len(opIDs))

	for _, opID := range opIDs {
		op := lo.Must(p.Operation(opID))

		dependsOn := lo.Keys(predMap[opID])
		sort.Strings(dependsOn)

		for i := range dependsOn {
			dependsOn[i] = idPrefix + dependsOn[i]
		}

		if len(dependsOn) == 0 {
			dependsOn = append(dependsOn, rootDependsOn...)
		}

		var ref progrep.ObjectRef
		if op.Category == OperationCategoryResource || op.Category == OperationCategoryTrack {
			ref = extractObjectRef(op, resolvedNamespaces)
		}

		result = append(result, progrep.Operation{
			OperationRef: progrep.OperationRef{
				ObjectRef: ref,
				Type:      mapOperationType(op),
				Iteration: int(op.Iteration),
			},
			ID:        idPrefix + opID,
			Category:  mapOperationCategory(op.Category),
			Status:    progrep.OperationStatusPending,
			DependsOn: dependsOn,
		})
	}

	return result
}

// Untouched resources have no operation in the plan and thus no position in its graph, so they
// are reported without edges, before the plan operations, ordered by ID. NoOp is a neutral label
// for a resource the plan leaves as is, shown as Completed.
func buildUntouchedOperations(untouched []*InstallableResourceInfo, untouchedResolvedNamespaces map[string]string, seenRefs map[progrep.ObjectRef]struct{}) []progrep.Operation {
	var result []progrep.Operation

	for _, info := range untouched {
		ref := progrep.ObjectRef{
			GroupVersionKind: info.GroupVersionKind,
			Name:             info.Name,
			Namespace:        untouchedResolvedNamespaces[info.ID()],
		}

		if _, ok := seenRefs[ref]; ok {
			continue
		}

		seenRefs[ref] = struct{}{}

		result = append(result, progrep.Operation{
			OperationRef: progrep.OperationRef{
				ObjectRef: ref,
				Type:      progrep.OperationTypeNoOp,
				Iteration: info.Iteration,
			},
			ID:        OperationID(OperationTypeNoop, OperationVersionNoop, OperationIteration(info.Iteration), info.ID()),
			Category:  progrep.OperationCategoryResource,
			Status:    progrep.OperationStatusCompleted,
			DependsOn: []string{},
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})

	return result
}

// ExecutePlan waits for every started operation before returning, so once a plan is over, what
// is still Pending was never scheduled and never will be.
func cancelPendingOperations(ops []progrep.Operation) {
	for i := range ops {
		if ops[i].Status == progrep.OperationStatusPending {
			ops[i].Status = progrep.OperationStatusCanceled
		}
	}
}

func copyRenderDetails(details *progrep.RenderDetails) *progrep.RenderDetails {
	if details == nil {
		return nil
	}

	result := *details
	if details.ResourceCount != nil {
		count := *details.ResourceCount
		result.ResourceCount = &count
	}

	return &result
}

func planIDPrefix(planNumber int) string {
	if planNumber == 1 {
		return ""
	}

	return fmt.Sprintf("%d/", planNumber)
}

func planSinkOperationIDs(predMap map[string]map[string]graph.Edge[string], idPrefix string) []string {
	hasSuccessors := make(map[string]struct{}, len(predMap))
	for _, preds := range predMap {
		for predID := range preds {
			hasSuccessors[predID] = struct{}{}
		}
	}

	var sinkIDs []string
	for opID := range predMap {
		if _, ok := hasSuccessors[opID]; !ok {
			sinkIDs = append(sinkIDs, idPrefix+opID)
		}
	}

	sort.Strings(sinkIDs)

	return sinkIDs
}

func safeSend(ch chan<- progrep.ProgressReport, report progrep.ProgressReport) (sent bool) {
	defer func() {
		_ = recover()
	}()

	select {
	case ch <- report:
		return true
	default:
		return false
	}
}

func sendBlocking(ctx context.Context, ch chan<- progrep.ProgressReport, report progrep.ProgressReport) {
	defer func() { _ = recover() }()

	select {
	case ch <- report:
	case <-ctx.Done():
	}
}
