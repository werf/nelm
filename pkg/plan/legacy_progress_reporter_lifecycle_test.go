package plan

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/legacy/progrep"
)

func TestLegacyProgressReporterCloseAndCanceledFinalSend(t *testing.T) {
	for attempt := 0; attempt < 32; attempt++ {
		ch := make(chan progrep.ProgressReport, 1)
		r := NewLegacyProgressReporter(ch)
		r.ReportPhase(context.Background(), progrep.PhaseKindInitializing, ReportPhaseOptions{})
		ctx, cancel := context.WithCancel(context.Background())

		stopDone := make(chan struct{})
		go func() {
			r.Stop(ctx)
			close(stopDone)
		}()

		cancel()

		closeDone := make(chan struct{})
		go func() {
			r.Close(ctx)
			close(closeDone)
		}()

		select {
		case <-closeDone:
		case <-time.After(time.Second):
			t.Fatal("Close did not finish after final-send cancellation")
		}

		select {
		case <-stopDone:
		case <-time.After(time.Second):
			t.Fatal("Stop did not finish after cancellation")
		}

		var reports []progrep.ProgressReport
		for report := range ch {
			reports = append(reports, report)
		}

		require.Len(t, reports, 1)
		assert.Equal(t, progrep.PhaseKindInitializing, reports[0].CurrentPhase)
	}
}

func TestLegacyProgressReporterCloseConcurrentReports(t *testing.T) {
	ctx := context.Background()
	for attempt := 0; attempt < 32; attempt++ {
		ch := make(chan progrep.ProgressReport, 1)
		r := NewLegacyProgressReporter(ch)
		r.ReportPhase(ctx, progrep.PhaseKindAnalyzing, ReportPhaseOptions{AnalyzeDetails: &progrep.AnalyzeDetails{Total: 100}})

		observedUpdate := make(chan struct{}, 1)
		consumerReady := make(chan struct{})

		consumerDone := make(chan struct{})
		go func() {
			<-ch
			close(consumerReady)

			for report := range ch {
				if report.AnalyzeDetails.Processed > 0 {
					select {
					case observedUpdate <- struct{}{}:
					default:
					}
				}
			}

			close(consumerDone)
		}()

		<-consumerReady

		start := make(chan struct{})

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)

			go func() {
				defer wg.Done()

				<-start

				for j := 0; j < 10; j++ {
					r.ReportAnalyzedResource(ctx)
					r.ReportStatus("unknown", progrep.OperationStatusCompleted)
				}
			}()
		}

		close(start)

		select {
		case <-observedUpdate:
		case <-time.After(time.Second):
			t.Fatal("no update reached the concurrent consumer")
		}

		r.Close(ctx)
		wg.Wait()
		<-consumerDone
		r.state.RTransaction(func(s *progressReporterState) {
			assert.True(t, s.closed)
			assert.True(t, s.stopped)
		})
	}
}

func TestLegacyProgressReporterCloseDoesNotWaitForRESTMapper(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 1)
	r := NewLegacyProgressReporter(ch)
	mapper := &blockingRESTMapper{
		RESTMapper:    newFakeRESTMapper(),
		lookupStarted: make(chan struct{}),
		releaseLookup: make(chan struct{}),
	}

	startDone := make(chan struct{})
	go func() {
		r.StartPlan(buildTestPlan([]*Operation{createConfigMapOp("cm")}, nil), "default", nil, mapper, StartPlanOptions{})
		close(startDone)
	}()

	select {
	case <-mapper.lookupStarted:
	case <-time.After(time.Second):
		close(mapper.releaseLookup)
		t.Fatal("StartPlan did not reach RESTMapper")
	}

	closeDone := make(chan struct{})
	go func() {
		r.Close(ctx)
		close(closeDone)
	}()

	select {
	case <-closeDone:
	case <-time.After(time.Second):
		close(mapper.releaseLookup)
		t.Fatal("Close waited for RESTMapper discovery")
	}

	close(mapper.releaseLookup)

	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("StartPlan did not finish after discovery")
	}

	_, ok := <-ch
	assert.False(t, ok)
	r.StartPlan(buildTestPlan([]*Operation{createConfigMapOp("late")}, nil), "default", nil, mapper, StartPlanOptions{})
	r.state.RTransaction(func(s *progressReporterState) { assert.Zero(t, s.plansCount) })
}

func TestLegacyProgressReporterCloseRetainsFinalSnapshot(t *testing.T) {
	ctx := context.Background()
	ch := make(chan progrep.ProgressReport, 8)
	r := NewLegacyProgressReporter(ch)
	r.ReportPhase(ctx, progrep.PhaseKindAnalyzing, ReportPhaseOptions{AnalyzeDetails: &progrep.AnalyzeDetails{Total: 1}})
	r.ReportAnalyzedResource(ctx)
	r.Stop(ctx)
	r.Close(ctx)
	r.Close(ctx)
	r.Stop(ctx)
	r.ReportPhase(ctx, progrep.PhaseKindExecuting, ReportPhaseOptions{})
	r.ReportAnalyzedResource(ctx)
	r.ReportStatus("unknown", progrep.OperationStatusCompleted)

	var reports []progrep.ProgressReport
	for report := range ch {
		reports = append(reports, report)
	}

	require.Len(t, reports, 3)
	assert.Equal(t, progrep.PhaseKindAnalyzing, reports[2].CurrentPhase)
	assert.Equal(t, &progrep.AnalyzeDetails{Processed: 1, Total: 1}, reports[2].AnalyzeDetails)
}

func TestLegacyProgressReporterCloseWithoutReports(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	r := NewLegacyProgressReporter(ch)
	r.Close(context.Background())
	r.Stop(context.Background())

	_, ok := <-ch
	assert.False(t, ok)
}
