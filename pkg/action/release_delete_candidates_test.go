package action

import (
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	helmreleasestatus "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/v2/pkg/plan"
)

func TestChartLint_AnalyzesDeletionOfLastDeployedRevisionResources(t *testing.T) {
	api := newProgressPhasesAPI(t)
	cmA := addReleaseConfigMap(t, api, "cm-a", nil)
	cmB := addReleaseConfigMap(t, api, "cm-b", nil)
	addProgressPhaseRevisionWithStatus(t, api, 1, cmA, helmreleasestatus.StatusDeployed)
	addProgressPhaseRevisionWithStatus(t, api, 2, cmB, helmreleasestatus.StatusPendingUpgrade)

	require.NoError(t, ChartLint(newChartRenderInstallContext(t), ChartLintOptions{
		KubeConnectionOptions:     api.kubeOpts,
		Chart:                     newDeleteCandidatesChart(t, cmB),
		ReleaseName:               "progress-release",
		ReleaseNamespace:          "progress-ns",
		ReleaseStorageDriver:      "configmap",
		Remote:                    true,
		ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
		TempDirPath:               t.TempDir(),
	}))

	api.state.RTransaction(func(s *progressPhasesAPIState) {
		assert.Contains(t, s.requests, "GET /api/v1/namespaces/progress-ns/configmaps/cm-a")
	})
	assert.Empty(t, deletedResourceConfigMaps(api))
}

func TestReleaseInstall_DeletesResourcesOfLastDeployedRevision(t *testing.T) {
	tests := []struct {
		name                  string
		lastRevisionStatus    helmreleasestatus.Status
		cmAAnnotations        map[string]string
		cmAAdoptedByOther     bool
		wantDeletedConfigMaps []string
	}{
		{name: "after pending-upgrade revision", lastRevisionStatus: helmreleasestatus.StatusPendingUpgrade, wantDeletedConfigMaps: []string{"cm-a"}},
		{name: "after failed revision", lastRevisionStatus: helmreleasestatus.StatusFailed, wantDeletedConfigMaps: []string{"cm-a"}},
		{name: "invalid for installation", lastRevisionStatus: helmreleasestatus.StatusFailed, cmAAnnotations: map[string]string{"werf.io/weight": "invalid"}, wantDeletedConfigMaps: []string{"cm-a"}},
		{name: "keep resource policy", lastRevisionStatus: helmreleasestatus.StatusFailed, cmAAnnotations: map[string]string{"helm.sh/resource-policy": "keep"}},
		{name: "ownership anyone", lastRevisionStatus: helmreleasestatus.StatusFailed, cmAAnnotations: map[string]string{"werf.io/ownership": "anyone"}},
		{name: "adopted by another release", lastRevisionStatus: helmreleasestatus.StatusFailed, cmAAdoptedByOther: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			cmA := addReleaseConfigMap(t, api, "cm-a", tt.cmAAnnotations)
			cmB := addReleaseConfigMap(t, api, "cm-b", nil)
			addProgressPhaseRevisionWithStatus(t, api, 1, cmA, helmreleasestatus.StatusDeployed)
			addProgressPhaseRevisionWithStatus(t, api, 2, cmB, tt.lastRevisionStatus)

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
	addProgressPhaseRevisionWithStatus(t, api, 1, cmOld, helmreleasestatus.StatusSuperseded)
	addProgressPhaseRevisionWithStatus(t, api, 2, cmA, helmreleasestatus.StatusDeployed)

	_, err := ReleaseInstall(newChartRenderInstallContext(t), "progress-release", "progress-ns", newDeleteCandidatesInstallOptions(t, api, cmB))
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"cm-a"}, deletedResourceConfigMaps(api))
}

func TestReleaseInstall_LastRevisionSpecWinsOverLastDeployedOne(t *testing.T) {
	tests := []struct {
		name                  string
		lastDeployedOwnership string
		lastOwnership         string
		wantDeletedConfigMaps []string
	}{
		{name: "last revision owns the resource", lastDeployedOwnership: "anyone", lastOwnership: "release", wantDeletedConfigMaps: []string{"cm-a"}},
		{name: "last revision shares the resource", lastDeployedOwnership: "release", lastOwnership: "anyone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := newProgressPhasesAPI(t)
			cmALastDeployed := addReleaseConfigMap(t, api, "cm-a", map[string]string{"werf.io/ownership": tt.lastDeployedOwnership})
			cmALast := addReleaseConfigMap(t, api, "cm-a", map[string]string{"werf.io/ownership": tt.lastOwnership})
			cmB := addReleaseConfigMap(t, api, "cm-b", nil)
			addProgressPhaseRevisionWithStatus(t, api, 1, cmALastDeployed, helmreleasestatus.StatusDeployed)
			addProgressPhaseRevisionWithStatus(t, api, 2, cmALast+"\n---\n"+cmB, helmreleasestatus.StatusFailed)

			_, err := ReleaseInstall(newChartRenderInstallContext(t), "progress-release", "progress-ns", newDeleteCandidatesInstallOptions(t, api, cmB))
			require.NoError(t, err)

			assert.ElementsMatch(t, tt.wantDeletedConfigMaps, deletedResourceConfigMaps(api))
		})
	}
}

func TestReleasePlanInstall_ShowsDeletionOfLastDeployedRevisionResources(t *testing.T) {
	api := newProgressPhasesAPI(t)
	cmA := addReleaseConfigMap(t, api, "cm-a", nil)
	cmB := addReleaseConfigMap(t, api, "cm-b", nil)
	addProgressPhaseRevisionWithStatus(t, api, 1, cmA, helmreleasestatus.StatusDeployed)
	addProgressPhaseRevisionWithStatus(t, api, 2, cmB, helmreleasestatus.StatusPendingUpgrade)

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
	addProgressPhaseRevisionWithStatus(t, api, 1, cmA, helmreleasestatus.StatusSuperseded)
	addProgressPhaseRevisionWithStatus(t, api, 2, cmB, helmreleasestatus.StatusDeployed)
	addProgressPhaseRevisionWithStatus(t, api, 3, cmC, helmreleasestatus.StatusFailed)

	require.NoError(t, ReleaseRollback(newChartRenderInstallContext(t), "progress-release", "progress-ns", ReleaseRollbackOptions{
		KubeConnectionOptions:     api.kubeOpts,
		ResourceValidationOptions: common.ResourceValidationOptions{NoResourceValidation: true},
		TrackingOptions:           common.TrackingOptions{NoProgressTablePrint: true},
		LegacyNoReleaseLock:       true, NoShowNotes: true,
		ReleaseStorageDriver: "configmap", Revision: 1, TempDirPath: t.TempDir(),
	}))

	assert.ElementsMatch(t, []string{"cm-b", "cm-c"}, deletedResourceConfigMaps(api))
}

func TestReleaseUninstall_DeletesResourcesOfLastDeployedRevisionButRunsOnlyLastRevisionHooks(t *testing.T) {
	api := newProgressPhasesAPI(t)
	cmA := addReleaseConfigMap(t, api, "cm-a", nil)
	cmB := addReleaseConfigMap(t, api, "cm-b", nil)
	hookA := addReleaseConfigMap(t, api, "hook-a", map[string]string{"helm.sh/hook": "pre-delete"})
	hookB := addReleaseConfigMap(t, api, "hook-b", map[string]string{"helm.sh/hook": "pre-delete"})
	api.state.RWTransaction(func(s *progressPhasesAPIState) {
		s.allowApply = true
		delete(s.configMaps, "hook-a")
		delete(s.configMaps, "hook-b")
	})
	addProgressPhaseRevisionWithStatus(t, api, 1, cmA+"\n---\n"+hookA, helmreleasestatus.StatusDeployed)
	addProgressPhaseRevisionWithStatus(t, api, 2, cmB+"\n---\n"+hookB, helmreleasestatus.StatusFailed)

	require.NoError(t, ReleaseUninstall(newChartRenderInstallContext(t), "progress-release", "progress-ns", ReleaseUninstallOptions{
		KubeConnectionOptions: api.kubeOpts,
		TrackingOptions:       common.TrackingOptions{NoProgressTablePrint: true},
		LegacyNoReleaseLock:   true,
		ReleaseStorageDriver:  "configmap",
		TempDirPath:           t.TempDir(),
	}))

	assert.ElementsMatch(t, []string{"cm-a", "cm-b"}, deletedResourceConfigMaps(api))
	api.state.RTransaction(func(s *progressPhasesAPIState) {
		assert.Contains(t, s.configMaps, "hook-b")
		assert.NotContains(t, s.configMaps, "hook-a")
	})
}
