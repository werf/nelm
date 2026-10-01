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

func (r *LegacyProgressReporter) ReportStatus(opID string, status progrep.OperationStatus) {
	r.state.RWTransaction(func(s *progressReporterState) {
		idx, ok := s.opIndex[opID]
		if !ok {
			return
		}

		s.ops[idx].Status = status

		if len(r.reportCh) == cap(r.reportCh) {
			return
		}

		sendNonBlocking(r.reportCh, buildProgressReport(s.ops))
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
	resolvedNamespaces := buildResolvedNamespaces(p, releaseNamespace, mapper)

	untouchedResolvedNamespaces := make(map[string]string, len(installableResourceInfos))
	for _, info := range installableResourceInfos {
		untouchedResolvedNamespaces[info.ID()] = resolveNamespace(info.GroupVersionKind, info.Namespace, releaseNamespace, mapper)
	}

	r.state.RWTransaction(func(s *progressReporterState) {
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

		sendNonBlocking(r.reportCh, buildProgressReport(s.ops))
	})
}

// Stop sends the final report with a blocking send. Operations that never started are marked
// Canceled first: nothing is going to run them anymore.
func (r *LegacyProgressReporter) Stop(ctx context.Context) {
	var report progrep.ProgressReport

	r.state.RWTransaction(func(s *progressReporterState) {
		cancelPendingOperations(s.ops)

		report = buildProgressReport(s.ops)
	})

	func() {
		defer func() { _ = recover() }()

		select {
		case r.reportCh <- report:
		case <-ctx.Done():
		}
	}()
}

type StartPlanOptions struct {
	// UntouchedResourcesOnly, when true, omits the plan's own operations and reports the untouched
	// resources alone. Set it for a plan that is not going to be executed: its operations would
	// otherwise stay Pending forever.
	UntouchedResourcesOnly bool
}

type progressReporterState struct {
	// lastPlanSinkIDs are the report IDs of the operations without successors in the most recently
	// started plan that had operations. The root operations of the next plan depend on them.
	lastPlanSinkIDs []string
	// opIndex maps the raw operation IDs of the most recently started plan to their positions in
	// ops. Operations of earlier plans are no longer addressable: by the time the next plan starts
	// they are either done or canceled.
	opIndex    map[string]int
	ops        []progrep.Operation
	plansCount int
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

func buildProgressReport(ops []progrep.Operation) progrep.ProgressReport {
	operations := make([]progrep.Operation, len(ops))
	copy(operations, ops)

	return progrep.ProgressReport{
		Operations: operations,
	}
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
