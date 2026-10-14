package plan

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/legacy/progrep"
)

func TestLegacyProgressReporterCanceledCheckpointKeepsReporting(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	r := NewLegacyProgressReporter(ch)
	r.ReportPhase(context.Background(), progrep.PhaseKindRendering, ReportPhaseOptions{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.ReportCheckpoint(ctx)
	assert.Equal(t, progrep.PhaseKindRendering, (<-ch).CurrentPhase)
	r.ReportPhase(context.Background(), progrep.PhaseKindAnalyzing, ReportPhaseOptions{})
	assert.Equal(t, progrep.PhaseKindAnalyzing, (<-ch).CurrentPhase)
	r.Close(context.Background())
}

func TestLegacyProgressReporterCheckpointPreservesPendingStatuses(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 8)
	r := NewLegacyProgressReporter(ch)
	r.ReportPhase(ctx, progrep.PhaseKindExecuting, ReportPhaseOptions{
		NamespaceDeleteDetails: &progrep.NamespaceDeleteDetails{Name: "ns", DeleteStatus: progrep.OperationStatusPending, TrackAbsenceStatus: progrep.OperationStatusPending},
	})

	op := createConfigMapOp("cm")
	startTestPlan(r, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})
	drainChannel(ch)
	r.ReportCheckpoint(ctx)

	checkpoint := <-ch
	require.Len(t, checkpoint.Operations, 1)
	assert.Equal(t, progrep.OperationStatusPending, checkpoint.Operations[0].Status)
	assert.Equal(t, progrep.OperationStatusPending, checkpoint.NamespaceDeleteDetails.DeleteStatus)
	assert.Equal(t, progrep.OperationStatusPending, checkpoint.NamespaceDeleteDetails.TrackAbsenceStatus)
	r.ReportStatus(op.ID(), progrep.OperationStatusCompleted)

	next := <-ch
	assert.Equal(t, progrep.OperationStatusCompleted, next.Operations[0].Status)
	assert.Equal(t, progrep.OperationStatusPending, checkpoint.Operations[0].Status)
	r.Close(ctx)
}

func TestLegacyProgressReporterCheckpointSafeOnClosedChannel(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 1)
	r := NewLegacyProgressReporter(ch)
	r.ReportPhase(ctx, progrep.PhaseKindRendering, ReportPhaseOptions{})
	close(ch)
	assert.NotPanics(t, func() { r.ReportCheckpoint(ctx) })
}

func TestLegacyProgressReporterCheckpointWaitsForConsumer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch := make(chan progrep.ProgressReport, 1)
	r := NewLegacyProgressReporter(ch)
	r.ReportPhase(ctx, progrep.PhaseKindAnalyzing, ReportPhaseOptions{AnalyzeDetails: &progrep.AnalyzeDetails{Total: 1}})
	r.ReportAnalyzedResource(ctx)

	done := make(chan struct{})
	go func() {
		r.ReportCheckpoint(ctx)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("checkpoint returned while the channel was full")
	case <-time.After(20 * time.Millisecond):
	}

	initial := <-ch
	assert.Zero(t, initial.AnalyzeDetails.Processed)

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("checkpoint did not finish after the consumer freed space")
	}

	checkpoint := <-ch
	assert.Equal(t, progrep.PhaseKindAnalyzing, checkpoint.CurrentPhase)
	assert.Equal(t, &progrep.AnalyzeDetails{Processed: 1, Total: 1}, checkpoint.AnalyzeDetails)
	checkpoint.AnalyzeDetails.Processed = 99

	r.ReportPhase(ctx, progrep.PhaseKindPlanning, ReportPhaseOptions{})

	next := <-ch
	assert.Equal(t, progrep.PhaseKindPlanning, next.CurrentPhase)
	assert.Equal(t, &progrep.AnalyzeDetails{Processed: 1, Total: 1}, next.AnalyzeDetails)
	r.Stop(ctx)
	assert.Equal(t, next, <-ch)
	r.Close(ctx)
}

func TestLegacyProgressReporterCheckpointWithoutReportsOrAfterStop(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 4)
	r := NewLegacyProgressReporter(ch)
	r.ReportCheckpoint(ctx)
	assert.Empty(t, ch)
	r.ReportPhase(ctx, progrep.PhaseKindInitializing, ReportPhaseOptions{})
	r.Stop(ctx)
	drainChannel(ch)
	r.ReportCheckpoint(ctx)
	assert.Empty(t, ch)
	r.Close(ctx)
	assert.NotPanics(t, func() { r.ReportCheckpoint(ctx) })
}

func TestLegacyProgressReporterCloseAndCanceledCheckpoint(t *testing.T) {
	for attempt := 0; attempt < 32; attempt++ {
		ch := make(chan progrep.ProgressReport, 1)
		r := NewLegacyProgressReporter(ch)
		r.ReportPhase(context.Background(), progrep.PhaseKindInitializing, ReportPhaseOptions{})
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan struct{})
		go func() {
			r.ReportCheckpoint(ctx)
			close(done)
		}()

		cancel()

		closed := make(chan struct{})
		go func() {
			r.Close(ctx)
			close(closed)
		}()

		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("Close did not finish after checkpoint cancellation")
		}

		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("checkpoint did not finish after cancellation")
		}

		var reports []progrep.ProgressReport
		for report := range ch {
			reports = append(reports, report)
		}

		require.Len(t, reports, 1)
		assert.Equal(t, progrep.PhaseKindInitializing, reports[0].CurrentPhase)
	}
}
