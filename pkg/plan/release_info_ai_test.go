//go:build ai_tests

package plan

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/werf/nelm/pkg/common"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/pkg/release"
)

func TestAI_BuildPlan_DeleteReleaseUsesRevisionWithoutReleaseBody(t *testing.T) {
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

func TestAI_BuildReleaseInfos_FillsRevisionAndKeepsDeployedOnlySupersedes(t *testing.T) {
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

func TestAI_BuildReleaseInfos_RejectsUninstallDeployType(t *testing.T) {
	infos, err := BuildReleaseInfos(context.Background(), common.DeployTypeUninstall, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BuildUninstallReleaseInfos")
	assert.Nil(t, infos)
}

func TestAI_BuildUninstallReleaseInfos_LoadsBodyOnlyForLastRevision(t *testing.T) {
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

func TestAI_BuildUninstallReleaseInfos_NoRevisions(t *testing.T) {
	infos, err := BuildUninstallReleaseInfos(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.Empty(t, infos)
}

func TestAI_BuildUninstallReleaseInfos_RejectsMissingLastReleaseBody(t *testing.T) {
	revisions := []release.Revision{
		{Name: "myrelease", Namespace: "test-ns", Version: 2, Status: helmreleasecommon.StatusDeployed.String()},
	}

	infos, err := BuildUninstallReleaseInfos(context.Background(), revisions, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no release body for the last revision 2")
	assert.Nil(t, infos)
}

func TestAI_BuildUninstallReleaseInfos_RejectsReleaseBodyOfWrongRevision(t *testing.T) {
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
