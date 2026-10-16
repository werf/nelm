package plan

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/legacy/progrep"
)

func TestLegacyProgressReporter_CanceledFinalSendStillStops(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	r := NewLegacyProgressReporter(ch)
	r.ReportPhase(context.Background(), progrep.PhaseKindInitializing, ReportPhaseOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.Stop(ctx)
	assert.Equal(t, progrep.PhaseKindInitializing, (<-ch).CurrentPhase)
	r.Stop(context.Background())
	r.ReportPhase(context.Background(), progrep.PhaseKindRendering, ReportPhaseOptions{})
	assert.Empty(t, ch)
}

func TestLegacyProgressReporter_ConcurrentAnalysis(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 1)
	r := NewLegacyProgressReporter(ch)
	r.ReportAnalyzedResource(ctx)
	assert.Empty(t, ch)
	r.ReportPhase(ctx, progrep.PhaseKindAnalyzing, ReportPhaseOptions{AnalyzeDetails: &progrep.AnalyzeDetails{Total: 1000}})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				r.ReportAnalyzedResource(ctx)
			}
		}()
	}

	wg.Wait()

	initial := <-ch

	r.Stop(ctx)
	assert.Equal(t, 0, initial.AnalyzeDetails.Processed)
	assert.Equal(t, progrep.AnalyzeDetails{Processed: 1000, Total: 1000}, *(<-ch).AnalyzeDetails)
}

func TestLegacyProgressReporter_ConcurrentStopSendsOnce(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 64)
	r := NewLegacyProgressReporter(ch)
	r.ReportPhase(ctx, progrep.PhaseKindInitializing, ReportPhaseOptions{})
	drainChannel(ch)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			r.Stop(ctx)
		}()
	}

	wg.Wait()
	require.Len(t, drainChannel(ch), 1)
}

func TestLegacyProgressReporter_FullBufferKeepsPhaseAndCount(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 1)
	r := NewLegacyProgressReporter(ch)
	r.ReportPhase(ctx, progrep.PhaseKindInitializing, ReportPhaseOptions{})
	r.ReportPhase(ctx, progrep.PhaseKindAnalyzing, ReportPhaseOptions{AnalyzeDetails: &progrep.AnalyzeDetails{Total: 1}})
	r.ReportAnalyzedResource(ctx)

	stale := <-ch
	assert.Equal(t, progrep.PhaseKindInitializing, stale.CurrentPhase)
	assert.Nil(t, stale.AnalyzeDetails)
	assert.Empty(t, ch)
	r.Stop(ctx)

	final := <-ch
	assert.Equal(t, progrep.PhaseKindAnalyzing, final.CurrentPhase)
	assert.Equal(t, progrep.AnalyzeDetails{Processed: 1, Total: 1}, *final.AnalyzeDetails)
}

func TestLegacyProgressReporter_MetadataSurvivesEveryPlanSnapshot(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 16)
	r := NewLegacyProgressReporter(ch)
	count := 0
	r.ReportPhase(ctx, progrep.PhaseKindExecuting, ReportPhaseOptions{
		RenderDetails:          &progrep.RenderDetails{ResourceCount: &count},
		AnalyzeDetails:         &progrep.AnalyzeDetails{Processed: 1, Total: 1},
		NamespaceDeleteDetails: &progrep.NamespaceDeleteDetails{Name: "ns", DeleteStatus: progrep.OperationStatusCompleted, TrackAbsenceStatus: progrep.OperationStatusCompleted},
	})

	want := <-ch
	op := createConfigMapOp("first")
	startTestPlan(r, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})
	r.ReportStatus(op.ID(), progrep.OperationStatusCompleted)
	r.ReportPhase(ctx, progrep.PhaseKindPlanning, ReportPhaseOptions{})

	next := createConfigMapOp("second")
	startTestPlan(r, buildTestPlan([]*Operation{next}, nil), nil, StartPlanOptions{})
	r.Stop(ctx)

	reports := drainChannel(ch)
	require.Len(t, reports, 5)

	for _, report := range reports {
		assert.Equal(t, want.RenderDetails, report.RenderDetails)
		assert.Equal(t, want.AnalyzeDetails, report.AnalyzeDetails)
		assert.Equal(t, want.NamespaceDeleteDetails, report.NamespaceDeleteDetails)
		*report.RenderDetails.ResourceCount = 100
		report.AnalyzeDetails.Total = 100
		report.NamespaceDeleteDetails.Name = "mutated"
	}

	assert.Equal(t, progrep.PhaseKindExecuting, reports[0].CurrentPhase)
	assert.Equal(t, progrep.PhaseKindPlanning, reports[4].CurrentPhase)
	final := reports[4].Operations
	require.Len(t, final, 2)
	assert.Equal(t, progrep.OperationStatusCompleted, final[0].Status)
	assert.Equal(t, progrep.OperationStatusCanceled, final[1].Status)
	assert.Equal(t, []string{op.ID()}, final[1].DependsOn)
}

func TestLegacyProgressReporter_PhaseJSONAndUnknownCount(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 8)
	r := NewLegacyProgressReporter(ch)

	ctx := context.Background()
	for _, phase := range []progrep.PhaseKind{progrep.PhaseKindInitializing, progrep.PhaseKindRendering, progrep.PhaseKindAnalyzing, progrep.PhaseKindPlanning, progrep.PhaseKindExecuting, progrep.PhaseKindDeletingNamespace} {
		r.ReportPhase(ctx, phase, ReportPhaseOptions{})
		assert.Equal(t, phase, (<-ch).CurrentPhase)
	}

	r.ReportPhase(ctx, progrep.PhaseKindRendering, ReportPhaseOptions{RenderDetails: &progrep.RenderDetails{}})

	unknown := <-ch
	require.NotNil(t, unknown.RenderDetails)
	assert.Nil(t, unknown.RenderDetails.ResourceCount)
	raw, err := json.Marshal(unknown)
	require.NoError(t, err)
	assert.JSONEq(t, `{"currentPhase":"Rendering","renderDetails":{},"operations":[]}`, string(raw))

	zero := 0
	r.ReportPhase(ctx, progrep.PhaseKindAnalyzing, ReportPhaseOptions{
		RenderDetails:          &progrep.RenderDetails{ResourceCount: &zero},
		AnalyzeDetails:         &progrep.AnalyzeDetails{},
		NamespaceDeleteDetails: &progrep.NamespaceDeleteDetails{Name: "ns", DeleteStatus: progrep.OperationStatusPending, TrackAbsenceStatus: progrep.OperationStatusPending},
	})

	raw, err = json.Marshal(<-ch)
	require.NoError(t, err)
	assert.JSONEq(t, `{"currentPhase":"Analyzing","renderDetails":{"resourceCount":0},"analyzeDetails":{"processed":0,"total":0},"namespaceDeleteDetails":{"name":"ns","deleteStatus":"Pending","trackAbsenceStatus":"Pending"},"operations":[]}`, string(raw))
	assert.Nil(t, unknown.RenderDetails.ResourceCount)

	legacyCh := make(chan progrep.ProgressReport, 1)
	legacy := NewLegacyProgressReporter(legacyCh)
	startTestPlan(legacy, buildTestPlan([]*Operation{createConfigMapOp("cm")}, nil), nil, StartPlanOptions{})

	legacyReport := <-legacyCh
	assert.Empty(t, legacyReport.CurrentPhase)
	raw, err = json.Marshal(legacyReport)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Len(t, decoded, 1, "legacy users still get operations only")
}

func TestLegacyProgressReporter_PhaseSnapshotsOwnDetails(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 16)
	r := NewLegacyProgressReporter(ch)
	count := 2
	render := &progrep.RenderDetails{ResourceCount: &count}
	analyze := &progrep.AnalyzeDetails{Total: 2}
	ns := &progrep.NamespaceDeleteDetails{Name: "ns", DeleteStatus: progrep.OperationStatusCompleted, TrackAbsenceStatus: progrep.OperationStatusPending}
	r.ReportPhase(ctx, progrep.PhaseKindRendering, ReportPhaseOptions{RenderDetails: render, AnalyzeDetails: analyze, NamespaceDeleteDetails: ns})

	first := <-ch
	count = 99
	analyze.Total = 99
	ns.Name = "changed"
	render.ResourceCount = nil
	assert.Equal(t, 2, *first.RenderDetails.ResourceCount)
	assert.Equal(t, 2, first.AnalyzeDetails.Total)
	assert.Equal(t, "ns", first.NamespaceDeleteDetails.Name)
	*first.RenderDetails.ResourceCount = 88
	first.AnalyzeDetails.Processed = 88
	first.NamespaceDeleteDetails.Name = "mutated snapshot"

	r.ReportAnalyzedResource(ctx)

	second := <-ch
	assert.Equal(t, 2, *second.RenderDetails.ResourceCount)
	assert.Equal(t, progrep.AnalyzeDetails{Processed: 1, Total: 2}, *second.AnalyzeDetails)
	assert.Equal(t, "ns", second.NamespaceDeleteDetails.Name)
	r.ReportPhase(ctx, progrep.PhaseKindPlanning, ReportPhaseOptions{AnalyzeDetails: &progrep.AnalyzeDetails{Total: 3}})

	third := <-ch
	assert.Equal(t, progrep.PhaseKindPlanning, third.CurrentPhase)
	assert.Equal(t, 0, third.AnalyzeDetails.Processed, "supplied details replace, rather than merge")
	assert.Equal(t, 2, *third.RenderDetails.ResourceCount)
	assert.Equal(t, "ns", third.NamespaceDeleteDetails.Name)
	assert.Equal(t, 1, second.AnalyzeDetails.Processed, "older snapshots remain unchanged")
}

func TestLegacyProgressReporter_PhasesSafeOnClosedChannel(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	r := NewLegacyProgressReporter(ch)
	close(ch)
	assert.NotPanics(t, func() {
		r.ReportPhase(context.Background(), progrep.PhaseKindAnalyzing, ReportPhaseOptions{AnalyzeDetails: &progrep.AnalyzeDetails{Total: 1}})
		r.ReportAnalyzedResource(context.Background())
		r.Stop(context.Background())
	})
}

func TestLegacyProgressReporter_StopCancelsNamespacePendingOnly(t *testing.T) {
	ctx := context.Background()
	for _, status := range []progrep.OperationStatus{progrep.OperationStatusPending, progrep.OperationStatusProgressing, progrep.OperationStatusCompleted, progrep.OperationStatusFailed, progrep.OperationStatusCanceled} {
		t.Run(string(status), func(t *testing.T) {
			ch := make(chan progrep.ProgressReport, 8)
			r := NewLegacyProgressReporter(ch)
			r.ReportPhase(ctx, progrep.PhaseKindDeletingNamespace, ReportPhaseOptions{NamespaceDeleteDetails: &progrep.NamespaceDeleteDetails{Name: "ns", DeleteStatus: status, TrackAbsenceStatus: status}})

			initial := <-ch
			op := createConfigMapOp("cm")
			startTestPlan(r, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})
			drainChannel(ch)
			r.Stop(ctx)

			final := <-ch

			want := status
			if want == progrep.OperationStatusPending {
				want = progrep.OperationStatusCanceled
			}

			assert.Equal(t, status, initial.NamespaceDeleteDetails.DeleteStatus)
			assert.Equal(t, want, final.NamespaceDeleteDetails.DeleteStatus)
			assert.Equal(t, want, final.NamespaceDeleteDetails.TrackAbsenceStatus)
			assert.Equal(t, progrep.OperationStatusCanceled, final.Operations[0].Status)
			r.ReportStatus(op.ID(), progrep.OperationStatusCompleted)
			r.Stop(ctx)
			assert.Empty(t, ch)
			r.state.RTransaction(func(s *progressReporterState) {
				assert.Equal(t, progrep.OperationStatusCanceled, s.ops[0].Status)
			})
		})
	}
}

func TestLegacyProgressReporter_StopIsTerminal(t *testing.T) {
	ctx := context.Background()
	for _, withPhase := range []bool{false, true} {
		t.Run(map[bool]string{false: "early", true: "phase only"}[withPhase], func(t *testing.T) {
			ch := make(chan progrep.ProgressReport, 8)

			r := NewLegacyProgressReporter(ch)
			if withPhase {
				r.ReportPhase(ctx, progrep.PhaseKindAnalyzing, ReportPhaseOptions{AnalyzeDetails: &progrep.AnalyzeDetails{Total: 1}})
				drainChannel(ch)
			}

			r.Stop(ctx)

			reports := drainChannel(ch)
			if withPhase {
				require.Len(t, reports, 1)
				assert.Equal(t, progrep.PhaseKindAnalyzing, reports[0].CurrentPhase)
				assert.Empty(t, reports[0].Operations)
			} else {
				assert.Empty(t, reports)
			}

			r.ReportPhase(ctx, progrep.PhaseKindExecuting, ReportPhaseOptions{})
			r.ReportAnalyzedResource(ctx)
			r.ReportStatus("unknown", progrep.OperationStatusCompleted)
			startTestPlan(r, buildTestPlan([]*Operation{createConfigMapOp("later")}, nil), nil, StartPlanOptions{})
			r.Stop(ctx)
			assert.Empty(t, ch)
			r.state.RTransaction(func(s *progressReporterState) {
				assert.Zero(t, s.plansCount)

				if withPhase {
					assert.Equal(t, progrep.PhaseKindAnalyzing, s.currentPhase)
					assert.Zero(t, s.analyzeDetails.Processed)
				}
			})
		})
	}
}
