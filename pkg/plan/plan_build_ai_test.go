//go:build ai_tests

package plan

import (
	"sort"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release"
)

func TestAI_BuildFailurePlan(t *testing.T) {
	failedRelease := &helmrelease.Release{
		Name:      "test-release",
		Namespace: "test-namespace",
		Version:   1,
		Info:      &helmrelease.Info{Status: helmrelease.StatusPendingInstall},
	}

	for _, tt := range []struct {
		name               string
		failedPlanOps      []*Operation
		installableInfos   []*InstallableResourceInfo
		releaseInfos       []*ReleaseInfo
		expectedOperations []string
	}{
		{
			name:             "deletes a resource whose readiness tracking failed",
			failedPlanOps:    []*Operation{trackReadinessTestOp("a", OperationStatusFailed)},
			installableInfos: []*InstallableResourceInfo{installableInfoToDeleteOnFailedInstall("a")},
			expectedOperations: []string{
				OperationID(OperationTypeDelete, OperationVersionDelete, 0, "test-namespace::ConfigMap:a"),
				OperationID(OperationTypeTrackAbsence, OperationVersionTrackAbsence, 0, "test-namespace::ConfigMap:a"),
			},
		},
		{
			name:               "keeps a resource whose readiness tracking succeeded",
			failedPlanOps:      []*Operation{trackReadinessTestOp("a", OperationStatusCompleted)},
			installableInfos:   []*InstallableResourceInfo{installableInfoToDeleteOnFailedInstall("a")},
			expectedOperations: []string{},
		},
		{
			name:               "keeps a resource that must not be deleted on failed install",
			failedPlanOps:      []*Operation{trackReadinessTestOp("a", OperationStatusFailed)},
			installableInfos:   []*InstallableResourceInfo{installableInfoNamed("a", ResourceInstallTypeApply)},
			expectedOperations: []string{},
		},
		{
			name: "keeps a resource that was already deleted on successful install",
			failedPlanOps: []*Operation{
				trackReadinessTestOp("a", OperationStatusFailed),
				deleteTestOp("a", OperationStatusCompleted),
			},
			installableInfos:   []*InstallableResourceInfo{installableInfoToDeleteOnAnyOutcome("a")},
			expectedOperations: []string{},
		},
		{
			name: "deletes a resource whose delete on successful install never ran",
			failedPlanOps: []*Operation{
				trackReadinessTestOp("a", OperationStatusFailed),
				deleteTestOp("a", OperationStatusPending),
			},
			installableInfos: []*InstallableResourceInfo{installableInfoToDeleteOnAnyOutcome("a")},
			expectedOperations: []string{
				OperationID(OperationTypeDelete, OperationVersionDelete, 0, "test-namespace::ConfigMap:a"),
				OperationID(OperationTypeTrackAbsence, OperationVersionTrackAbsence, 0, "test-namespace::ConfigMap:a"),
			},
		},
		{
			name: "deletes only the resources that failed",
			failedPlanOps: []*Operation{
				trackReadinessTestOp("a", OperationStatusFailed),
				trackReadinessTestOp("b", OperationStatusCompleted),
			},
			installableInfos: []*InstallableResourceInfo{
				installableInfoToDeleteOnFailedInstall("a"),
				installableInfoToDeleteOnFailedInstall("b"),
			},
			expectedOperations: []string{
				OperationID(OperationTypeDelete, OperationVersionDelete, 0, "test-namespace::ConfigMap:a"),
				OperationID(OperationTypeTrackAbsence, OperationVersionTrackAbsence, 0, "test-namespace::ConfigMap:a"),
			},
		},
		{
			name:          "fails a release created by the failed plan",
			failedPlanOps: []*Operation{createReleaseTestOp(failedRelease, OperationStatusCompleted)},
			releaseInfos:  []*ReleaseInfo{{Release: failedRelease, MustFailOnFailedDeploy: true}},
			expectedOperations: []string{
				OperationID(OperationTypeUpdateRelease, OperationVersionUpdateRelease, 0, failedRelease.ID()),
			},
		},
		{
			name:          "fails a release updated by the failed plan",
			failedPlanOps: []*Operation{updateReleaseTestOp(failedRelease, OperationStatusCompleted)},
			releaseInfos:  []*ReleaseInfo{{Release: failedRelease, MustFailOnFailedDeploy: true}},
			expectedOperations: []string{
				OperationID(OperationTypeUpdateRelease, OperationVersionUpdateRelease, 0, failedRelease.ID()),
			},
		},
		{
			name:               "keeps a release that the failed plan never created",
			failedPlanOps:      []*Operation{createReleaseTestOp(failedRelease, OperationStatusPending)},
			releaseInfos:       []*ReleaseInfo{{Release: failedRelease, MustFailOnFailedDeploy: true}},
			expectedOperations: []string{},
		},
		{
			name:               "keeps a release that must not fail on a failed deploy",
			failedPlanOps:      []*Operation{createReleaseTestOp(failedRelease, OperationStatusCompleted)},
			releaseInfos:       []*ReleaseInfo{{Release: failedRelease}},
			expectedOperations: []string{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			failurePlan, err := BuildFailurePlan(buildTestPlan(tt.failedPlanOps, nil), tt.installableInfos, tt.releaseInfos, BuildFailurePlanOptions{})
			require.NoError(t, err)

			assert.Equal(t, tt.expectedOperations, meaningfulOperationIDs(failurePlan))
		})
	}
}

func createReleaseTestOp(rel *helmrelease.Release, status OperationStatus) *Operation {
	return &Operation{
		Type:     OperationTypeCreateRelease,
		Version:  OperationVersionCreateRelease,
		Category: OperationCategoryRelease,
		Status:   status,
		Config:   &OperationConfigCreateRelease{Release: rel},
	}
}

func deleteTestOp(name string, status OperationStatus) *Operation {
	return &Operation{
		Type:     OperationTypeDelete,
		Version:  OperationVersionDelete,
		Category: OperationCategoryResource,
		Status:   status,
		Config:   &OperationConfigDelete{ResourceMeta: makeResourceMeta(name, "test-namespace", gvkConfigMap)},
	}
}

func meaningfulOperationIDs(p *Plan) []string {
	ids := lo.FilterMap(p.Operations(), func(op *Operation, _ int) (string, bool) {
		return op.ID(), op.Category != OperationCategoryMeta
	})

	sort.Strings(ids)

	return ids
}

func trackReadinessTestOp(name string, status OperationStatus) *Operation {
	return &Operation{
		Type:     OperationTypeTrackReadiness,
		Version:  OperationVersionTrackReadiness,
		Category: OperationCategoryTrack,
		Status:   status,
		Config:   &OperationConfigTrackReadiness{ResourceMeta: makeResourceMeta(name, "test-namespace", gvkConfigMap)},
	}
}

func updateReleaseTestOp(rel *helmrelease.Release, status OperationStatus) *Operation {
	return &Operation{
		Type:     OperationTypeUpdateRelease,
		Version:  OperationVersionUpdateRelease,
		Category: OperationCategoryRelease,
		Status:   status,
		Config:   &OperationConfigUpdateRelease{Release: rel},
	}
}
