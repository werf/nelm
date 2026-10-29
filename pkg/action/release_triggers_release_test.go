package action

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
)

func TestReleaseInstall_SkipsReleaseWithOnlyNonTriggeringChanges(t *testing.T) {
	api := newProgressPhasesAPI(t)
	chartPath := newTriggersReleaseChart(t, api, map[string]string{"werf.io/triggers-release": "false"})

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	runtimeOpts := common.ReleaseInstallRuntimeOptions{
		ReleaseStorageDriver: "configmap", ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
	}

	installOpts := ReleaseInstallOptions{
		KubeConnectionOptions: api.kubeOpts,
		TrackingOptions:       common.TrackingOptions{NoProgressTablePrint: true},
		NoCreateNamespace:     true, LegacyNoReleaseLock: true, NoShowNotes: true,
		TempDirPath: t.TempDir(),
	}

	chartOpts := installOpts
	chartOpts.Chart = chartPath
	chartOpts.ReleaseInstallRuntimeOptions = runtimeOpts

	result, err := ReleaseInstall(ctx, "progress-release", "progress-ns", chartOpts)
	require.NoError(t, err)
	assert.Equal(t, helmreleasestatus.Status("skipped"), result.Release.Status)

	planArtifact, err := ReleasePlanInstall(ctx, "progress-release", "progress-ns", ReleasePlanInstallOptions{
		KubeConnectionOptions: api.kubeOpts, Chart: chartPath, ReleaseInstallRuntimeOptions: runtimeOpts, TempDirPath: t.TempDir(),
	})
	require.NoError(t, err)

	planOpts := installOpts
	planOpts.LegacyPlanArtifact = planArtifact
	planOpts.PlanArtifactLifetime = time.Hour

	result, err = ReleaseInstall(ctx, "progress-release", "progress-ns", planOpts)
	require.NoError(t, err)
	assert.Equal(t, helmreleasestatus.Status("skipped"), result.Release.Status)

	api.state.RTransaction(func(s *progressPhasesAPIState) {
		assert.NotContains(t, s.requests, "DELETE /api/v1/namespaces/progress-ns/configmaps/progress-resource")
		assert.NotContains(t, s.requests, "POST /api/v1/namespaces/progress-ns/configmaps")
	})
}

func TestReleasePlanInstall_TriggersRelease(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantErr     error
	}{
		{name: "recreated resource triggers release by default", wantErr: ErrResourceChangesPlanned},
		{name: "recreated resource with triggers-release true", annotations: map[string]string{"werf.io/triggers-release": "true"}, wantErr: ErrResourceChangesPlanned},
		{name: "recreated resource with triggers-release false", annotations: map[string]string{"werf.io/triggers-release": "false"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			chartPath := newTriggersReleaseChart(t, api, tt.annotations)

			ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
			defer cancel()

			planArtifact, err := ReleasePlanInstall(ctx, "progress-release", "progress-ns", ReleasePlanInstallOptions{
				KubeConnectionOptions: api.kubeOpts, Chart: chartPath, ErrorIfChangesPlanned: true,
				ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{
					ReleaseStorageDriver: "configmap", ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
				}, TempDirPath: t.TempDir(),
			})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.NotEmpty(t, planArtifact.Data.Changes)
			} else {
				require.NoError(t, err)
				assert.Empty(t, planArtifact.Data.Changes)
			}
		})
	}
}
