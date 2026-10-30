package action

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/logboek"
	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/plan"
)

func TestReleaseInstall_DeletesResourcesOfLastDeployedRevision(t *testing.T) {
	tests := []struct {
		name                  string
		lastRevisionStatus    string
		cmAAnnotations        map[string]string
		cmAAdoptedByOther     bool
		wantDeletedConfigMaps []string
	}{
		{name: "after pending-upgrade revision", lastRevisionStatus: "pending-upgrade", wantDeletedConfigMaps: []string{"cm-a"}},
		{name: "after failed revision", lastRevisionStatus: "failed", wantDeletedConfigMaps: []string{"cm-a"}},
		{name: "keep resource policy", lastRevisionStatus: "failed", cmAAnnotations: map[string]string{"helm.sh/resource-policy": "keep"}},
		{name: "ownership anyone", lastRevisionStatus: "failed", cmAAnnotations: map[string]string{"werf.io/ownership": "anyone"}},
		{name: "adopted by another release", lastRevisionStatus: "failed", cmAAdoptedByOther: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			cmA := addReleaseConfigMap(t, api, "cm-a", tt.cmAAnnotations)
			cmB := addReleaseConfigMap(t, api, "cm-b", nil)
			addProgressPhaseRevisionWithStatus(t, api, 1, "deployed", cmA)
			addProgressPhaseRevisionWithStatus(t, api, 2, tt.lastRevisionStatus, cmB)

			if tt.cmAAdoptedByOther {
				api.state.RWTransaction(func(s *progressPhasesAPIState) {
					s.configMaps["cm-a"].Annotations["meta.helm.sh/release-name"] = "other-release"
				})
			}

			_, err := ReleaseInstall(newChartRenderInstallContext(t), "progress-release", "progress-ns", newDeleteCandidatesInstallOptions(t, api, cmB))
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.wantDeletedConfigMaps, deletedResourceConfigMaps(api))
		})
	}
}

func TestReleaseInstall_IgnoresRevisionsOlderThanLastDeployed(t *testing.T) {
	api := newProgressPhasesAPI(t)
	cmOld := addReleaseConfigMap(t, api, "cm-old", nil)
	cmA := addReleaseConfigMap(t, api, "cm-a", nil)
	cmB := addReleaseConfigMap(t, api, "cm-b", nil)
	addProgressPhaseRevisionWithStatus(t, api, 1, "superseded", cmOld)
	addProgressPhaseRevisionWithStatus(t, api, 2, "deployed", cmA)

	_, err := ReleaseInstall(newChartRenderInstallContext(t), "progress-release", "progress-ns", newDeleteCandidatesInstallOptions(t, api, cmB))
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"cm-a"}, deletedResourceConfigMaps(api))
}

func TestReleasePlanInstall_ShowsDeletionOfLastDeployedRevisionResources(t *testing.T) {
	api := newProgressPhasesAPI(t)
	cmA := addReleaseConfigMap(t, api, "cm-a", nil)
	cmB := addReleaseConfigMap(t, api, "cm-b", nil)
	addProgressPhaseRevisionWithStatus(t, api, 1, "deployed", cmA)
	addProgressPhaseRevisionWithStatus(t, api, 2, "pending-upgrade", cmB)

	planArtifact, err := ReleasePlanInstall(newChartRenderInstallContext(t), "progress-release", "progress-ns", ReleasePlanInstallOptions{
		KubeConnectionOptions: api.kubeOpts, Chart: newDeleteCandidatesChart(t, cmB),
		ReleaseInstallRuntimeOptions: common.ReleaseInstallRuntimeOptions{
			ReleaseStorageDriver: "configmap", ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
		}, TempDirPath: t.TempDir(),
	})
	require.NoError(t, err)

	changes := lo.Map(planArtifact.Data.Changes, func(change *plan.ResourceChange, _ int) string {
		return change.Type + " " + change.ResourceMeta.IDHuman()
	})
	assert.Equal(t, []string{"delete ConfigMap/cm-a"}, changes)
	assert.Empty(t, deletedResourceConfigMaps(api))
}

func TestReleaseRollback_DeletesResourcesOfLastDeployedRevision(t *testing.T) {
	api := newProgressPhasesAPI(t)
	cmA := addReleaseConfigMap(t, api, "cm-a", nil)
	cmB := addReleaseConfigMap(t, api, "cm-b", nil)
	cmC := addReleaseConfigMap(t, api, "cm-c", nil)
	addProgressPhaseRevisionWithStatus(t, api, 1, "superseded", cmA)
	addProgressPhaseRevisionWithStatus(t, api, 2, "deployed", cmB)
	addProgressPhaseRevisionWithStatus(t, api, 3, "failed", cmC)

	ctx, cancel := context.WithTimeout(logboek.NewContext(context.Background(), logboek.NewLogger(io.Discard, io.Discard)), 10*time.Second)
	defer cancel()

	require.NoError(t, ReleaseRollback(ctx, "progress-release", "progress-ns", ReleaseRollbackOptions{
		KubeConnectionOptions:     api.kubeOpts,
		ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
		TrackingOptions:           common.TrackingOptions{NoProgressTablePrint: true},
		LegacyNoReleaseLock:       true, NoShowNotes: true,
		ReleaseStorageDriver: "configmap", Revision: 1, TempDirPath: t.TempDir(),
	}))

	assert.ElementsMatch(t, []string{"cm-b", "cm-c"}, deletedResourceConfigMaps(api))
}
