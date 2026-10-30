package plan

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/v2/pkg/common"
	helmrel "github.com/werf/nelm/v2/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/v2/pkg/release"
)

func TestBuildPlan_DeleteReleaseUsesRevisionWithoutReleaseBody(t *testing.T) {
	info := &ReleaseInfo{
		Must: ReleaseTypeDelete,
		Revision: release.Revision{
			Name:      "myrelease",
			Namespace: "test-ns",
			Status:    helmreleasecommon.StatusSuperseded.String(),
			Version:   2,
		},
	}
	require.Nil(t, info.Release, "a delete info must plan without a release body")

	builtPlan, err := BuildPlan(context.Background(), nil, nil, []*ReleaseInfo{info}, "test-ns", BuildPlanOptions{})
	require.NoError(t, err)

	deleteConfigs := lo.FilterMap(builtPlan.Operations(), func(op *Operation, _ int) (*OperationConfigDeleteRelease, bool) {
		cfg, ok := op.Config.(*OperationConfigDeleteRelease)

		return cfg, ok
	})
	require.Len(t, deleteConfigs, 1)

	assert.Equal(t, "myrelease", deleteConfigs[0].ReleaseName)
	assert.Equal(t, "test-ns", deleteConfigs[0].ReleaseNamespace)
	assert.Equal(t, 2, deleteConfigs[0].ReleaseRevision)
}

func TestBuildPlan_FailsStuckPendingRevisionOnlyInFinalStage(t *testing.T) {
	newRel := newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 3, helmreleasecommon.StatusPendingUpgrade)

	infos, err := BuildReleaseInfos(context.Background(), common.DeployTypeUpgrade, []helmrel.Accessor{
		newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 1, helmreleasecommon.StatusDeployed),
		newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 2, helmreleasecommon.StatusPendingUpgrade),
	}, newRel)
	require.NoError(t, err)

	builtPlan, err := BuildPlan(context.Background(), nil, nil, infos, "test-ns", BuildPlanOptions{})
	require.NoError(t, err)

	failOpID := OperationID(OperationTypeUpdateRelease, OperationVersionUpdateRelease, 0, releaseID("test-ns", "myrelease", 2))
	failOp, found := builtPlan.Operation(failOpID)
	require.True(t, found)

	cfg, ok := failOp.Config.(*OperationConfigUpdateRelease)
	require.True(t, ok)
	assert.Equal(t, helmreleasecommon.StatusFailed.String(), cfg.Release.Accessor.Status())
	assert.Equal(t, 2, cfg.Release.Accessor.Version())

	predMap, err := builtPlan.Graph.PredecessorMap()
	require.NoError(t, err)
	assert.Equal(t, []string{stageOperationID(common.StageFinal, common.StageStartSuffix)}, lo.Keys(predMap[failOpID]), "the stuck revision is marked failed only after every resource stage completed")

	failurePlan, err := BuildFailurePlan(builtPlan, nil, infos, BuildFailurePlanOptions{})
	require.NoError(t, err)

	_, found = failurePlan.Operation(failOpID)
	assert.False(t, found, "a failed deploy must leave the stuck revision as is")
}

func TestBuildReleaseInfos_FailsStuckPendingRevisions(t *testing.T) {
	for _, tt := range []struct {
		deployType common.DeployType
		newMust    ReleaseType
	}{
		{deployType: common.DeployTypeInstall, newMust: ReleaseTypeInstall},
		{deployType: common.DeployTypeUpgrade, newMust: ReleaseTypeUpgrade},
		{deployType: common.DeployTypeRollback, newMust: ReleaseTypeRollback},
	} {
		t.Run(string(tt.deployType), func(t *testing.T) {
			prevReleases := []helmrel.Accessor{
				newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 1, helmreleasecommon.StatusSuperseded),
				newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 2, helmreleasecommon.StatusPendingInstall),
				newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 3, helmreleasecommon.StatusFailed),
				newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 4, helmreleasecommon.StatusDeployed),
				newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 5, helmreleasecommon.StatusPendingUpgrade),
				newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 6, helmreleasecommon.StatusUninstalled),
				newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 7, helmreleasecommon.StatusPendingRollback),
			}
			newRel := newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 8, helmreleasecommon.StatusPendingUpgrade)

			infos, err := BuildReleaseInfos(context.Background(), tt.deployType, prevReleases, newRel)
			require.NoError(t, err)

			type versionMust struct {
				Must    ReleaseType
				Version int
			}

			assert.Equal(t, []versionMust{
				{Must: tt.newMust, Version: 8},
				{Must: ReleaseTypeFail, Version: 2},
				{Must: ReleaseTypeSupersede, Version: 4},
				{Must: ReleaseTypeFail, Version: 5},
				{Must: ReleaseTypeFail, Version: 7},
			}, lo.Map(infos, func(info *ReleaseInfo, _ int) versionMust {
				return versionMust{Must: info.Must, Version: info.Revision.Version}
			}))

			for _, info := range infos[1:] {
				assert.False(t, info.MustFailOnFailedDeploy, "a failed deploy must not touch revision %d", info.Revision.Version)
			}
		})
	}
}

func TestBuildReleaseInfos_FillsRevisionAndKeepsDeployedOnlySupersedes(t *testing.T) {
	prevReleases := []helmrel.Accessor{
		newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 1, helmreleasecommon.StatusSuperseded),
		newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 2, helmreleasecommon.StatusDeployed),
	}
	newRel := newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 3, helmreleasecommon.StatusPendingUpgrade)

	infos, err := BuildReleaseInfos(context.Background(), common.DeployTypeUpgrade, prevReleases, newRel)
	require.NoError(t, err)
	require.Len(t, infos, 2, "only the deployed previous revision is superseded")

	assert.Equal(t, ReleaseTypeUpgrade, infos[0].Must)
	assert.Equal(t, release.Revision{
		Name:      "myrelease",
		Namespace: "test-ns",
		Status:    helmreleasecommon.StatusPendingUpgrade.String(),
		Version:   3,
	}, infos[0].Revision)
	require.NotNil(t, infos[0].Release)

	assert.Equal(t, ReleaseTypeSupersede, infos[1].Must)
	assert.Equal(t, release.Revision{
		Name:      "myrelease",
		Namespace: "test-ns",
		Status:    helmreleasecommon.StatusDeployed.String(),
		Version:   2,
	}, infos[1].Revision)
	require.NotNil(t, infos[1].Release)
}

func TestBuildUninstallReleaseInfos_LoadsBodyOnlyForLastRevision(t *testing.T) {
	revisions := []release.Revision{
		{Name: "myrelease", Namespace: "test-ns", Version: 1, Status: helmreleasecommon.StatusSuperseded.String()},
		{Name: "myrelease", Namespace: "test-ns", Version: 2, Status: helmreleasecommon.StatusDeployed.String()},
	}

	lastRel := newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 2, helmreleasecommon.StatusDeployed)

	infos, err := BuildUninstallReleaseInfos(context.Background(), revisions, lastRel)
	require.NoError(t, err)
	require.Len(t, infos, 2)

	assert.Equal(t, ReleaseTypeDelete, infos[0].Must)
	assert.Equal(t, 1, infos[0].Revision.Version)
	assert.Nil(t, infos[0].Release, "delete op needs no release body")

	assert.Equal(t, ReleaseTypeUninstall, infos[1].Must)
	assert.Equal(t, 2, infos[1].Revision.Version)
	require.NotNil(t, infos[1].Release, "uninstall op copies and rewrites the release")
}

func TestBuildUninstallReleaseInfos_NoRevisions(t *testing.T) {
	infos, err := BuildUninstallReleaseInfos(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.Empty(t, infos)
}

func TestBuildUninstallReleaseInfos_RejectsMissingLastReleaseBody(t *testing.T) {
	revisions := []release.Revision{
		{Name: "myrelease", Namespace: "test-ns", Version: 2, Status: helmreleasecommon.StatusDeployed.String()},
	}

	infos, err := BuildUninstallReleaseInfos(context.Background(), revisions, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no release body for the last revision 2")
	assert.Nil(t, infos)
}

func TestBuildUninstallReleaseInfos_RejectsReleaseBodyOfWrongRevision(t *testing.T) {
	revisions := []release.Revision{
		{Name: "myrelease", Namespace: "test-ns", Version: 1, Status: helmreleasecommon.StatusSuperseded.String()},
		{Name: "myrelease", Namespace: "test-ns", Version: 2, Status: helmreleasecommon.StatusDeployed.String()},
	}

	staleRel := newTestReleaseAccessorForPlan(t, "myrelease", "test-ns", 1, helmreleasecommon.StatusSuperseded)

	infos, err := BuildUninstallReleaseInfos(context.Background(), revisions, staleRel)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "release body revision 1 does not match the last revision 2")
	assert.Nil(t, infos)
}
