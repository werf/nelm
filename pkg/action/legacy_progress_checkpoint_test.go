package action

import (
	"context"
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

func TestReleaseInstallPreparationCheckpointsWithSlowConsumer(t *testing.T) {
	api := newProgressPhasesAPI(t)
	ch := make(chan progrep.ProgressReport, 1)
	reports := collectSlowProgressPhaseReports(ch)

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", ReleaseInstallOptions{
		KubeConnectionOptions: api.kubeOpts, Chart: newProgressPhasesChart(t),
		TrackingOptions:   common.TrackingOptions{NoProgressTablePrint: true},
		NoCreateNamespace: true, LegacyNoReleaseLock: true,
		LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{ReleaseStorageDriver: "configmap"}, TempDirPath: t.TempDir(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	var snapshots []progrep.ProgressReport
	select {
	case snapshots = <-reports:
	case <-ctx.Done():
		t.Fatal("progress consumer did not finish")
	}

	var rendered, analyzed bool
	for _, snapshot := range snapshots {
		if snapshot.CurrentPhase == progrep.PhaseKindRendering && snapshot.RenderDetails != nil && snapshot.RenderDetails.ResourceCount != nil {
			rendered = true

			assert.Zero(t, *snapshot.RenderDetails.ResourceCount)
		}

		if snapshot.CurrentPhase == progrep.PhaseKindAnalyzing && snapshot.AnalyzeDetails != nil {
			analyzed = true

			assert.Equal(t, &progrep.AnalyzeDetails{Processed: 0, Total: 0}, snapshot.AnalyzeDetails)
		}
	}

	assert.True(t, rendered, "completed rendering must be delivered before analysis starts")
	assert.True(t, analyzed, "completed analysis must be delivered before planning starts")
}

func TestReleaseUninstallAnalysisCheckpointWithSlowConsumer(t *testing.T) {
	api := newProgressPhasesAPI(t)
	manifest := addProgressPhasesConfigMap(t, api)
	addProgressPhaseRevision(t, api, 1, manifest)

	ch := make(chan progrep.ProgressReport, 1)
	reports := collectSlowProgressPhaseReports(ch)

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	require.NoError(t, ReleaseUninstall(ctx, "progress-release", "progress-ns", ReleaseUninstallOptions{
		KubeConnectionOptions:  api.kubeOpts,
		TrackingOptions:        common.TrackingOptions{NoProgressTablePrint: true, TrackDeletionTimeout: time.Second},
		LegacyNoReleaseLock:    true,
		LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
		ReleaseStorageDriver: "configmap", TempDirPath: t.TempDir(),
	}))

	var snapshots []progrep.ProgressReport
	select {
	case snapshots = <-reports:
	case <-ctx.Done():
		t.Fatal("progress consumer did not finish")
	}

	var analyzed bool
	for _, snapshot := range snapshots {
		if snapshot.CurrentPhase == progrep.PhaseKindAnalyzing && snapshot.AnalyzeDetails != nil && snapshot.AnalyzeDetails.Processed == snapshot.AnalyzeDetails.Total {
			analyzed = true

			assert.Equal(t, &progrep.AnalyzeDetails{Processed: 1, Total: 1}, snapshot.AnalyzeDetails)
		}
	}

	assert.True(t, analyzed, "completed uninstall analysis must be delivered before planning starts")
}

func TestReleaseUninstallExecutionCheckpointSurvivesNamespaceCancellation(t *testing.T) {
	for _, phases := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "phases"}[phases], func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			addProgressPhaseRevision(t, api, 1, "")

			ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
			defer cancel()

			namespaceReached := make(chan struct{}, 1)
			api.onRequest = func(r *http.Request) {
				if r.Method == http.MethodDelete && r.URL.Path == "/api/v1/namespaces/progress-ns" {
					select {
					case namespaceReached <- struct{}{}:
					default:
					}

					cancel()
				}
			}
			ch := make(chan progrep.ProgressReport, 1)
			reports := collectSlowProgressPhaseReports(ch)
			err := ReleaseUninstall(ctx, "progress-release", "progress-ns", ReleaseUninstallOptions{
				KubeConnectionOptions:  api.kubeOpts,
				TrackingOptions:        common.TrackingOptions{NoProgressTablePrint: true, TrackDeletionTimeout: time.Second},
				DeleteReleaseNamespace: true, LegacyNoReleaseLock: true,
				LegacyProgressReportCh: ch, LegacyProgressReportPhases: phases,
				ReleaseStorageDriver: "configmap", TempDirPath: t.TempDir(),
			})
			require.Error(t, err)
			require.Len(t, namespaceReached, 1)

			var snapshots []progrep.ProgressReport
			select {
			case snapshots = <-reports:
			case <-time.After(5 * time.Second):
				t.Fatal("progress consumer did not finish")
			}

			var completed bool
			for _, snapshot := range snapshots {
				if phases {
					if snapshot.CurrentPhase != progrep.PhaseKindExecuting {
						continue
					}
				} else {
					assert.Empty(t, snapshot.CurrentPhase)
					assert.Nil(t, snapshot.AnalyzeDetails)
					assert.Nil(t, snapshot.NamespaceDeleteDetails)
				}

				if len(snapshot.Operations) == 0 {
					continue
				}

				allCompleted := true
				for _, op := range snapshot.Operations {
					allCompleted = allCompleted && op.Status == progrep.OperationStatusCompleted
				}

				completed = completed || allCompleted
			}

			assert.True(t, completed, "plan completion must survive cancellation during namespace deletion")
		})
	}
}
