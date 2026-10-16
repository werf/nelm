package action

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/legacy/progrep"
)

func TestReleaseInstallProgressPhasesOuterTimeout(t *testing.T) {
	api := newProgressPhasesAPI(t)
	requestStarted := make(chan struct{}, 1)
	api.onRequest = func(r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/progress-ns/configmaps" {
			select {
			case requestStarted <- struct{}{}:
			default:
			}

			<-r.Context().Done()
		}
	}
	ch := make(chan progrep.ProgressReport, 1)
	reports := collectProgressPhaseReports(ch)

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 5*time.Second)
	defer cancel()

	started := time.Now()
	result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", ReleaseInstallOptions{
		KubeConnectionOptions: api.kubeOpts,
		Chart:                 newProgressPhasesChart(t),
		TrackingOptions:       common.TrackingOptions{NoProgressTablePrint: true},
		Timeout:               300 * time.Millisecond,
		NoCreateNamespace:     true, LegacyNoReleaseLock: true,
		LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{ReleaseStorageDriver: "configmap"},
		TempDirPath:                  t.TempDir(),
	})
	require.ErrorContains(t, err, "action timed out")
	assert.Nil(t, result)
	assert.Less(t, time.Since(started), 2*time.Second)
	require.NoError(t, ctx.Err())
	require.Len(t, requestStarted, 1)

	select {
	case snapshots := <-reports:
		require.NotEmpty(t, snapshots)

		for _, snapshot := range snapshots {
			assert.Equal(t, progrep.PhaseKindInitializing, snapshot.CurrentPhase)
			assert.Empty(t, snapshot.Operations)
		}
	case <-ctx.Done():
		t.Fatal("report channel was not closed on return")
	}
}

func TestReleaseUninstallProgressPhasesOuterTimeout(t *testing.T) {
	for _, capacity := range []int{1, 64} {
		for attempt := 0; attempt < 3; attempt++ {
			t.Run(fmt.Sprintf("capacity-%d-attempt-%d", capacity, attempt), func(t *testing.T) {
				api := newProgressPhasesAPI(t)
				api.state.RWTransaction(func(s *progressPhasesAPIState) { s.retainNamespace = true })

				ch := make(chan progrep.ProgressReport, capacity)
				reports := collectProgressPhaseReports(ch)

				ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 5*time.Second)
				defer cancel()

				started := time.Now()
				err := ReleaseUninstall(ctx, "progress-release", "progress-ns", ReleaseUninstallOptions{
					KubeConnectionOptions:  api.kubeOpts,
					TrackingOptions:        common.TrackingOptions{NoProgressTablePrint: true, TrackDeletionTimeout: 3 * time.Second},
					Timeout:                300 * time.Millisecond,
					DeleteReleaseNamespace: true,
					LegacyNoReleaseLock:    true,
					LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
					ReleaseStorageDriver: "configmap", TempDirPath: t.TempDir(),
				})
				require.ErrorContains(t, err, "action timed out")
				assert.Less(t, time.Since(started), 2*time.Second)
				require.NoError(t, ctx.Err())

				var snapshots []progrep.ProgressReport
				select {
				case snapshots = <-reports:
				case <-ctx.Done():
					t.Fatal("report channel was not closed on return")
				}

				var tracking bool
				for _, report := range snapshots {
					if report.CurrentPhase == progrep.PhaseKindDeletingNamespace && report.NamespaceDeleteDetails != nil {
						tracking = tracking || (report.NamespaceDeleteDetails.DeleteStatus == progrep.OperationStatusCompleted && report.NamespaceDeleteDetails.TrackAbsenceStatus == progrep.OperationStatusProgressing)
					}
				}

				assert.True(t, tracking, "timeout must occur during namespace absence tracking")
				api.state.RTransaction(func(s *progressPhasesAPIState) {
					assert.Contains(t, s.requests, "DELETE /api/v1/namespaces/progress-ns")
				})
			})
		}
	}
}
