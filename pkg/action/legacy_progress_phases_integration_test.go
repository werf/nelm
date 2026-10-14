package action

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/v2/pkg/legacy/progrep"
)

func TestReleaseInstallProgressPhasesEmptyChartThenSkipped(t *testing.T) {
	api := newProgressPhasesAPI(t)
	chartPath := newProgressPhasesChart(t)

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	for _, status := range []helmreleasestatus.Status{helmreleasestatus.StatusDeployed, helmreleasestatus.Status("skipped")} {
		ch := make(chan progrep.ProgressReport, 4096)
		result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", ReleaseInstallOptions{
			KubeConnectionOptions: api.kubeOpts, Chart: chartPath,
			TrackingOptions:   common.TrackingOptions{NoProgressTablePrint: true},
			NoCreateNamespace: true, NoShowNotes: true, LegacyNoReleaseLock: true,
			LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
			ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{ReleaseStorageDriver: "configmap"}, TempDirPath: t.TempDir(),
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, status, result.Release.Status)
		reports := progressPhaseReports(t, ch)
		assert.Equal(t, []progrep.PhaseKind{progrep.PhaseKindInitializing, progrep.PhaseKindRendering, progrep.PhaseKindAnalyzing, progrep.PhaseKindPlanning, progrep.PhaseKindExecuting}, progressPhaseSequence(reports))
		final := reports[len(reports)-1]
		require.NotNil(t, final.RenderDetails)
		require.NotNil(t, final.RenderDetails.ResourceCount)
		assert.Zero(t, *final.RenderDetails.ResourceCount)
		assert.Equal(t, &progrep.AnalyzeDetails{Processed: 0, Total: 0}, final.AnalyzeDetails)
		assert.Nil(t, final.NamespaceDeleteDetails)

		if status == helmreleasestatus.StatusDeployed {
			require.NotEmpty(t, final.Operations)

			for _, op := range final.Operations {
				assert.Equal(t, progrep.OperationStatusCompleted, op.Status)
			}
		} else {
			assert.Empty(t, final.Operations)
		}
	}
}

func TestReleaseInstallProgressPhasesPlanningBeforeDeployedBodyRead(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhaseRevision(t, api, 1, "")
	addProgressPhaseRevision(t, api, 2, "")

	ch := make(chan progrep.ProgressReport, 4096)

	bodyReadReports := make(chan []progrep.ProgressReport, 8)

	api.onRequest = func(r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/progress-ns/configmaps/sh.helm.release.v1.progress-release.v1" {
			return
		}

		var observed []progrep.ProgressReport
		for {
			select {
			case report := <-ch:
				observed = append(observed, report)
			default:
				bodyReadReports <- observed
				return
			}
		}
	}

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", ReleaseInstallOptions{
		KubeConnectionOptions: api.kubeOpts, Chart: newProgressPhasesChart(t),
		NoCreateNamespace: true, LegacyNoReleaseLock: true,
		LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{ReleaseStorageDriver: "configmap"}, TempDirPath: t.TempDir(),
	})
	require.NoError(t, err)
	assert.Equal(t, helmreleasestatus.Status("skipped"), result.Release.Status)

	var observed []progrep.ProgressReport
	select {
	case observed = <-bodyReadReports:
	default:
		t.Fatal("the deployed release body was not requested")
	}

	initialReports := observed
	require.NotEmpty(t, initialReports)
	assert.Equal(t, progrep.PhaseKindPlanning, initialReports[len(initialReports)-1].CurrentPhase)
	observed = append(observed, progressPhaseReports(t, ch)...)
	assert.Equal(t, []progrep.PhaseKind{progrep.PhaseKindInitializing, progrep.PhaseKindRendering, progrep.PhaseKindAnalyzing, progrep.PhaseKindPlanning, progrep.PhaseKindExecuting}, progressPhaseSequence(observed))
}

func TestReleaseInstallProgressPhasesRenderError(t *testing.T) {
	api := newProgressPhasesAPI(t)
	chartPath := newProgressPhasesChart(t)
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "invalid.yaml"), []byte(`{{ fail "intentional render error" }}`), 0o600))

	ch := make(chan progrep.ProgressReport, 4096)

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", ReleaseInstallOptions{
		KubeConnectionOptions: api.kubeOpts, Chart: chartPath,
		NoCreateNamespace: true, LegacyNoReleaseLock: true,
		LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{ReleaseStorageDriver: "configmap"}, TempDirPath: t.TempDir(),
	})
	require.ErrorContains(t, err, "intentional render error")
	assert.Nil(t, result)
	reports := progressPhaseReports(t, ch)
	assert.Equal(t, []progrep.PhaseKind{progrep.PhaseKindInitializing, progrep.PhaseKindRendering}, progressPhaseSequence(reports))
	final := reports[len(reports)-1]
	require.NotNil(t, final.RenderDetails)
	assert.Nil(t, final.RenderDetails.ResourceCount)
	assert.Nil(t, final.AnalyzeDetails)
	assert.Empty(t, final.Operations)
}

func TestReleaseInstallProgressPhasesSkippedConfigMapNoOp(t *testing.T) {
	api := newProgressPhasesAPI(t)
	manifest := addProgressPhasesConfigMap(t, api)
	chartPath := newProgressPhasesChart(t)
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "cm.yaml"), []byte(manifest), 0o600))

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	for i, status := range []helmreleasestatus.Status{helmreleasestatus.StatusDeployed, helmreleasestatus.Status("skipped")} {
		ch := make(chan progrep.ProgressReport, 4096)
		result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", ReleaseInstallOptions{
			KubeConnectionOptions: api.kubeOpts, Chart: chartPath,
			TrackingOptions:   common.TrackingOptions{NoProgressTablePrint: true},
			NoCreateNamespace: true, LegacyNoReleaseLock: true,
			LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
			ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{
				ReleaseStorageDriver: "configmap", ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
			}, TempDirPath: t.TempDir(),
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, status, result.Release.Status)
		reports := progressPhaseReports(t, ch)
		assert.Equal(t, []progrep.PhaseKind{progrep.PhaseKindInitializing, progrep.PhaseKindRendering, progrep.PhaseKindAnalyzing, progrep.PhaseKindPlanning, progrep.PhaseKindExecuting}, progressPhaseSequence(reports))
		final := reports[len(reports)-1]
		assert.Equal(t, progrep.PhaseKindExecuting, final.CurrentPhase)
		assert.Equal(t, &progrep.AnalyzeDetails{Processed: i + 1, Total: i + 1}, final.AnalyzeDetails)
		require.NotNil(t, final.RenderDetails)
		require.NotNil(t, final.RenderDetails.ResourceCount)
		assert.Equal(t, 1, *final.RenderDetails.ResourceCount)
		require.NotEmpty(t, final.Operations)
		assert.Equal(t, progrep.OperationTypeNoOp, final.Operations[0].Type)
		assert.Equal(t, progrep.OperationStatusCompleted, final.Operations[0].Status)
		assert.Equal(t, "progress-resource", final.Operations[0].Name)
		assert.Equal(t, "progress-ns", final.Operations[0].Namespace)

		if status == helmreleasestatus.Status("skipped") {
			assert.Len(t, final.Operations, 1)
		}
	}
}

func TestReleaseUninstallProgressPhasesNoReleaseNamespaceDeletion(t *testing.T) {
	for _, test := range []struct {
		name         string
		deleteStatus int
		trackStatus  int
		retain       bool
		wantErr      string
		wantDelete   progrep.OperationStatus
		wantTrack    progrep.OperationStatus
	}{
		{name: "success", wantDelete: progrep.OperationStatusCompleted, wantTrack: progrep.OperationStatusCompleted},
		{name: "delete forbidden", deleteStatus: http.StatusForbidden, wantErr: "delete release namespace", wantDelete: progrep.OperationStatusFailed, wantTrack: progrep.OperationStatusCanceled},
		{name: "tracking forbidden", trackStatus: http.StatusForbidden, wantErr: "track release namespace absence", wantDelete: progrep.OperationStatusCompleted, wantTrack: progrep.OperationStatusFailed},
		{name: "tracking timeout", retain: true, wantErr: "track release namespace absence", wantDelete: progrep.OperationStatusCompleted, wantTrack: progrep.OperationStatusFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			api.state.RWTransaction(func(s *progressPhasesAPIState) {
				s.deleteNamespaceStatus, s.trackNamespaceStatus = test.deleteStatus, test.trackStatus
				s.retainNamespace = test.retain
			})

			ch := make(chan progrep.ProgressReport, 4096)

			ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
			defer cancel()

			err := ReleaseUninstall(ctx, "progress-release", "progress-ns", ReleaseUninstallOptions{
				KubeConnectionOptions:  api.kubeOpts,
				TrackingOptions:        common.TrackingOptions{NoProgressTablePrint: true, TrackDeletionTimeout: 100 * time.Millisecond},
				DeleteReleaseNamespace: true, LegacyNoReleaseLock: true,
				LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
				ReleaseStorageDriver: "configmap", TempDirPath: t.TempDir(),
			})
			if test.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantErr)
			}

			reports := progressPhaseReports(t, ch)
			assert.Equal(t, []progrep.PhaseKind{progrep.PhaseKindInitializing, progrep.PhaseKindDeletingNamespace}, progressPhaseSequence(reports))

			var details []progrep.NamespaceDeleteDetails
			for _, report := range reports {
				assert.Empty(t, report.Operations)
				assert.Nil(t, report.AnalyzeDetails)
				assert.Nil(t, report.RenderDetails)

				if report.NamespaceDeleteDetails != nil {
					assert.Equal(t, "progress-ns", report.NamespaceDeleteDetails.Name)

					if len(details) == 0 || details[len(details)-1] != *report.NamespaceDeleteDetails {
						details = append(details, *report.NamespaceDeleteDetails)
					}
				}
			}

			wantDetails := []progrep.NamespaceDeleteDetails{{Name: "progress-ns", DeleteStatus: progrep.OperationStatusProgressing, TrackAbsenceStatus: progrep.OperationStatusPending}}
			if test.deleteStatus == 0 {
				wantDetails = append(wantDetails, progrep.NamespaceDeleteDetails{Name: "progress-ns", DeleteStatus: progrep.OperationStatusCompleted, TrackAbsenceStatus: progrep.OperationStatusProgressing})
			}

			wantDetails = append(wantDetails, progrep.NamespaceDeleteDetails{Name: "progress-ns", DeleteStatus: test.wantDelete, TrackAbsenceStatus: test.wantTrack})
			assert.Equal(t, wantDetails, details)
			api.state.RTransaction(func(s *progressPhasesAPIState) {
				assert.Contains(t, s.requests, "DELETE /api/v1/namespaces/progress-ns")
			})
		})
	}
}

func TestReleaseUninstallProgressPhasesNoReleaseWithoutNamespaceDeletion(t *testing.T) {
	api := newProgressPhasesAPI(t)
	ch := make(chan progrep.ProgressReport, 4096)

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	require.NoError(t, ReleaseUninstall(ctx, "progress-release", "progress-ns", ReleaseUninstallOptions{
		KubeConnectionOptions: api.kubeOpts, LegacyNoReleaseLock: true,
		LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
		ReleaseStorageDriver: "configmap", TempDirPath: t.TempDir(),
	}))
	reports := progressPhaseReports(t, ch)
	assert.Equal(t, []progrep.PhaseKind{progrep.PhaseKindInitializing}, progressPhaseSequence(reports))

	for _, report := range reports {
		assert.Nil(t, report.NamespaceDeleteDetails)
		assert.Empty(t, report.Operations)
	}

	api.state.RTransaction(func(s *progressPhasesAPIState) {
		assert.NotContains(t, s.requests, "DELETE /api/v1/namespaces/progress-ns")
	})
}

func TestReleaseUninstallProgressPhasesStoredConfigMap(t *testing.T) {
	api := newProgressPhasesAPI(t)
	manifest := addProgressPhasesConfigMap(t, api)
	addProgressPhaseRevision(t, api, 1, manifest)

	ch := make(chan progrep.ProgressReport, 4096)

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	require.NoError(t, ReleaseUninstall(ctx, "progress-release", "progress-ns", ReleaseUninstallOptions{
		KubeConnectionOptions:  api.kubeOpts,
		TrackingOptions:        common.TrackingOptions{NoProgressTablePrint: true, TrackDeletionTimeout: time.Second},
		DeleteReleaseNamespace: true, LegacyNoReleaseLock: true,
		LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
		ReleaseStorageDriver: "configmap", TempDirPath: t.TempDir(),
	}))
	reports := progressPhaseReports(t, ch)
	assert.Equal(t, []progrep.PhaseKind{progrep.PhaseKindInitializing, progrep.PhaseKindAnalyzing, progrep.PhaseKindPlanning, progrep.PhaseKindExecuting, progrep.PhaseKindDeletingNamespace}, progressPhaseSequence(reports))
	final := reports[len(reports)-1]
	assert.Equal(t, &progrep.AnalyzeDetails{Processed: 1, Total: 1}, final.AnalyzeDetails)

	var (
		deletedResource     bool
		executionOperations []progrep.Operation
	)

	for _, report := range reports {
		if report.CurrentPhase == progrep.PhaseKindExecuting {
			executionOperations = report.Operations
		}
	}

	assert.Equal(t, executionOperations, final.Operations)

	for _, op := range final.Operations {
		if op.Type == progrep.OperationTypeDelete && op.Kind == "ConfigMap" && op.Name == "progress-resource" {
			deletedResource = true

			assert.Equal(t, progrep.OperationStatusCompleted, op.Status)
		}

		assert.NotEqual(t, "Namespace", op.Kind)
	}

	assert.True(t, deletedResource)
	api.state.RTransaction(func(s *progressPhasesAPIState) {
		resourceDelete, namespaceDelete := -1, -1
		for i, request := range s.requests {
			if request == "DELETE /api/v1/namespaces/progress-ns/configmaps/progress-resource" {
				resourceDelete = i
			}

			if request == "DELETE /api/v1/namespaces/progress-ns" {
				namespaceDelete = i
			}
		}

		assert.NotEqual(t, -1, resourceDelete)
		assert.Greater(t, namespaceDelete, resourceDelete)
	})
}

func TestReleaseUninstallProgressPhasesStoredEmptyRelease(t *testing.T) {
	api := newProgressPhasesAPI(t)
	addProgressPhaseRevision(t, api, 1, "")

	ch := make(chan progrep.ProgressReport, 4096)

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	require.NoError(t, ReleaseUninstall(ctx, "progress-release", "progress-ns", ReleaseUninstallOptions{
		KubeConnectionOptions: api.kubeOpts, TrackingOptions: common.TrackingOptions{NoProgressTablePrint: true, TrackDeletionTimeout: time.Second},
		DeleteReleaseNamespace: true, LegacyNoReleaseLock: true,
		LegacyProgressReportCh: ch, LegacyProgressReportPhases: true,
		ReleaseStorageDriver: "configmap", TempDirPath: t.TempDir(),
	}))
	reports := progressPhaseReports(t, ch)
	assert.Equal(t, []progrep.PhaseKind{progrep.PhaseKindInitializing, progrep.PhaseKindAnalyzing, progrep.PhaseKindPlanning, progrep.PhaseKindExecuting, progrep.PhaseKindDeletingNamespace}, progressPhaseSequence(reports))
	final := reports[len(reports)-1]
	assert.Equal(t, &progrep.AnalyzeDetails{Processed: 0, Total: 0}, final.AnalyzeDetails)
	assert.Equal(t, &progrep.NamespaceDeleteDetails{Name: "progress-ns", DeleteStatus: progrep.OperationStatusCompleted, TrackAbsenceStatus: progrep.OperationStatusCompleted}, final.NamespaceDeleteDetails)
	require.NotEmpty(t, final.Operations)

	var executionOperations []progrep.Operation
	for _, report := range reports {
		if report.CurrentPhase == progrep.PhaseKindExecuting {
			executionOperations = report.Operations
		}
	}

	assert.Equal(t, executionOperations, final.Operations)

	for _, op := range final.Operations {
		assert.Equal(t, progrep.OperationStatusCompleted, op.Status)
		assert.NotEqual(t, "Namespace", op.Kind)
	}
}
