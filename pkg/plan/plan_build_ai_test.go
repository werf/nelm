//go:build ai_tests

package plan_test

import (
	"fmt"
	"testing"

	"github.com/dominikbraun/graph"
	"github.com/stretchr/testify/suite"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/plan"
	"github.com/werf/nelm/pkg/resource"
	"github.com/werf/nelm/pkg/resource/spec"
)

type BuildPlanAISuite struct {
	BuildPlanSuite
}

func TestAI_BuildPlanSuiteMultiMatchDependencies(t *testing.T) {
	suite.Run(t, new(BuildPlanAISuite))
}

func (s *BuildPlanAISuite) TestAI_BuildPlanConnectsAllMatchingDependencies() {
	testCases := []buildPlanTestCase{
		{
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				createOp1 := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[0].LocalResource.ResourceSpec,
					},
				}

				createOp2 := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[1].LocalResource.ResourceSpec,
					},
				}

				createDependentOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[2].LocalResource.ResourceSpec,
					},
				}

				mainStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInstall, common.StageStartSuffix),
					},
				}

				mainStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInstall, common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					createOp1,
					createOp2,
					createDependentOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						createOp1.ID(): {},
						createOp2.ID(): {},
					},
					createOp1.ID(): {
						createDependentOp.ID(): {},
					},
					createOp2.ID(): {
						createDependentOp.ID(): {},
					},
					createDependentOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				res1 := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				res1.Name = "test-configmap-1"
				res1.Unstruct.SetName("test-configmap-1")
				res1.Weight = nil

				res2 := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				res2.Name = "test-configmap-2"
				res2.Unstruct.SetName("test-configmap-2")
				res2.Weight = nil

				dependentRes := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				dependentRes.Name = "dependent-secret"
				dependentRes.Unstruct.SetName("dependent-secret")
				dependentRes.Unstruct.SetKind("Secret")
				dependentRes.GroupVersionKind.Kind = "Secret"
				dependentRes.Weight = nil
				dependentRes.AutoInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Kinds: []string{"ConfigMap"},
						},
						ResourceState: common.ResourceStatePresent,
					},
				}

				info1 := defaultInstallableResourceInfo(res1)
				info1.MustTrackReadiness = false

				info2 := defaultInstallableResourceInfo(res2)
				info2.MustTrackReadiness = false

				dependentInfo := defaultInstallableResourceInfo(dependentRes)
				dependentInfo.MustTrackReadiness = false

				return []*plan.InstallableResourceInfo{info1, info2, dependentInfo}, nil, nil, plan.BuildPlanOptions{}
			},
			name: "connect deploy dependency to all matching create operations",
		},
		{
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				createOp1 := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[0].LocalResource.ResourceSpec,
					},
				}

				createOp2 := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[1].LocalResource.ResourceSpec,
					},
				}

				createDependentOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[2].LocalResource.ResourceSpec,
					},
				}

				trackReadinessOp1 := &plan.Operation{
					Type:     plan.OperationTypeTrackReadiness,
					Version:  plan.OperationVersionTrackReadiness,
					Category: plan.OperationCategoryTrack,
					Config: &plan.OperationConfigTrackReadiness{
						ResourceMeta:                             installableInfos[0].LocalResource.ResourceMeta,
						FailMode:                                 installableInfos[0].LocalResource.FailMode,
						FailuresAllowed:                          installableInfos[0].LocalResource.FailuresAllowed,
						IgnoreLogs:                               installableInfos[0].LocalResource.SkipLogs,
						IgnoreLogsForContainers:                  installableInfos[0].LocalResource.SkipLogsForContainers,
						IgnoreReadinessProbeFailsByContainerName: installableInfos[0].LocalResource.IgnoreReadinessProbeFailsForContainers,
						NoActivityTimeout:                        installableInfos[0].LocalResource.NoActivityTimeout,
						SaveEvents:                               installableInfos[0].LocalResource.ShowServiceMessages,
						SaveLogsByRegex:                          installableInfos[0].LocalResource.LogRegex,
						SaveLogsByRegexForContainers:             installableInfos[0].LocalResource.LogRegexesForContainers,
						SaveLogsOnlyForContainers:                installableInfos[0].LocalResource.ShowLogsOnlyForContainers,
						SaveLogsOnlyForNumberOfReplicas:          installableInfos[0].LocalResource.ShowLogsOnlyForNumberOfReplicas,
					},
				}

				trackReadinessOp2 := &plan.Operation{
					Type:     plan.OperationTypeTrackReadiness,
					Version:  plan.OperationVersionTrackReadiness,
					Category: plan.OperationCategoryTrack,
					Config: &plan.OperationConfigTrackReadiness{
						ResourceMeta:                             installableInfos[1].LocalResource.ResourceMeta,
						FailMode:                                 installableInfos[1].LocalResource.FailMode,
						FailuresAllowed:                          installableInfos[1].LocalResource.FailuresAllowed,
						IgnoreLogs:                               installableInfos[1].LocalResource.SkipLogs,
						IgnoreLogsForContainers:                  installableInfos[1].LocalResource.SkipLogsForContainers,
						IgnoreReadinessProbeFailsByContainerName: installableInfos[1].LocalResource.IgnoreReadinessProbeFailsForContainers,
						NoActivityTimeout:                        installableInfos[1].LocalResource.NoActivityTimeout,
						SaveEvents:                               installableInfos[1].LocalResource.ShowServiceMessages,
						SaveLogsByRegex:                          installableInfos[1].LocalResource.LogRegex,
						SaveLogsByRegexForContainers:             installableInfos[1].LocalResource.LogRegexesForContainers,
						SaveLogsOnlyForContainers:                installableInfos[1].LocalResource.ShowLogsOnlyForContainers,
						SaveLogsOnlyForNumberOfReplicas:          installableInfos[1].LocalResource.ShowLogsOnlyForNumberOfReplicas,
					},
				}

				mainStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInstall, common.StageStartSuffix),
					},
				}

				mainStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInstall, common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					createOp1,
					trackReadinessOp1,
					createOp2,
					trackReadinessOp2,
					createDependentOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						createOp1.ID(): {},
						createOp2.ID(): {},
					},
					createOp1.ID(): {
						trackReadinessOp1.ID(): {},
					},
					trackReadinessOp1.ID(): {
						createDependentOp.ID(): {},
					},
					createOp2.ID(): {
						trackReadinessOp2.ID(): {},
					},
					trackReadinessOp2.ID(): {
						createDependentOp.ID(): {},
					},
					createDependentOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				res1 := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				res1.Name = "test-configmap-1"
				res1.Unstruct.SetName("test-configmap-1")
				res1.Weight = nil

				res2 := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				res2.Name = "test-configmap-2"
				res2.Unstruct.SetName("test-configmap-2")
				res2.Weight = nil

				dependentRes := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				dependentRes.Name = "dependent-secret"
				dependentRes.Unstruct.SetName("dependent-secret")
				dependentRes.Unstruct.SetKind("Secret")
				dependentRes.GroupVersionKind.Kind = "Secret"
				dependentRes.Weight = nil
				dependentRes.AutoInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Kinds: []string{"ConfigMap"},
						},
						ResourceState: common.ResourceStateReady,
					},
				}

				info1 := defaultInstallableResourceInfo(res1)
				info2 := defaultInstallableResourceInfo(res2)
				dependentInfo := defaultInstallableResourceInfo(dependentRes)
				dependentInfo.MustTrackReadiness = false

				return []*plan.InstallableResourceInfo{info1, info2, dependentInfo}, nil, nil, plan.BuildPlanOptions{}
			},
			name: "connect deploy dependency to all matching track-readiness operations",
		},
		{
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				deleteOp1 := &plan.Operation{
					Type:     plan.OperationTypeDelete,
					Version:  plan.OperationVersionDelete,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigDelete{
						ResourceMeta:      deletableInfos[0].LocalResource.ResourceMeta,
						DeletePropagation: deletableInfos[0].LocalResource.DeletePropagation,
					},
				}

				trackAbsenceOp1 := &plan.Operation{
					Type:     plan.OperationTypeTrackAbsence,
					Version:  plan.OperationVersionTrackAbsence,
					Category: plan.OperationCategoryTrack,
					Config: &plan.OperationConfigTrackAbsence{
						ResourceMeta: deletableInfos[0].LocalResource.ResourceMeta,
					},
				}

				deleteOp2 := &plan.Operation{
					Type:     plan.OperationTypeDelete,
					Version:  plan.OperationVersionDelete,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigDelete{
						ResourceMeta:      deletableInfos[1].LocalResource.ResourceMeta,
						DeletePropagation: deletableInfos[1].LocalResource.DeletePropagation,
					},
				}

				trackAbsenceOp2 := &plan.Operation{
					Type:     plan.OperationTypeTrackAbsence,
					Version:  plan.OperationVersionTrackAbsence,
					Category: plan.OperationCategoryTrack,
					Config: &plan.OperationConfigTrackAbsence{
						ResourceMeta: deletableInfos[1].LocalResource.ResourceMeta,
					},
				}

				createDependentOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[0].LocalResource.ResourceSpec,
					},
				}

				mainStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInstall, common.StageStartSuffix),
					},
				}

				mainStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInstall, common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					deleteOp1,
					trackAbsenceOp1,
					deleteOp2,
					trackAbsenceOp2,
					createDependentOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						deleteOp1.ID(): {},
						deleteOp2.ID(): {},
					},
					deleteOp1.ID(): {
						trackAbsenceOp1.ID(): {},
					},
					trackAbsenceOp1.ID(): {
						createDependentOp.ID(): {},
					},
					deleteOp2.ID(): {
						trackAbsenceOp2.ID(): {},
					},
					trackAbsenceOp2.ID(): {
						createDependentOp.ID(): {},
					},
					createDependentOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				delRes1 := defaultDeletableResource(s.releaseName, s.releaseNamespace)
				delRes1.Name = "test-configmap-1"

				delRes2 := defaultDeletableResource(s.releaseName, s.releaseNamespace)
				delRes2.Name = "test-configmap-2"

				delInfo1 := defaultDeletableResourceInfo(delRes1, s.releaseName, s.releaseNamespace)
				delInfo1.Stage = common.StageInstall

				delInfo2 := defaultDeletableResourceInfo(delRes2, s.releaseName, s.releaseNamespace)
				delInfo2.Stage = common.StageInstall

				dependentRes := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				dependentRes.Name = "dependent-secret"
				dependentRes.Unstruct.SetName("dependent-secret")
				dependentRes.Unstruct.SetKind("Secret")
				dependentRes.GroupVersionKind.Kind = "Secret"
				dependentRes.Weight = nil
				dependentRes.AutoInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Kinds: []string{"ConfigMap"},
						},
						ResourceState: common.ResourceStateAbsent,
					},
				}

				dependentInfo := defaultInstallableResourceInfo(dependentRes)
				dependentInfo.MustTrackReadiness = false

				return []*plan.InstallableResourceInfo{dependentInfo}, []*plan.DeletableResourceInfo{delInfo1, delInfo2}, nil, plan.BuildPlanOptions{}
			},
			name: "connect deploy dependency to all matching track-absence operations",
		},
		{
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				deleteOp1 := &plan.Operation{
					Type:     plan.OperationTypeDelete,
					Version:  plan.OperationVersionDelete,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigDelete{
						ResourceMeta:      deletableInfos[0].LocalResource.ResourceMeta,
						DeletePropagation: deletableInfos[0].LocalResource.DeletePropagation,
					},
				}

				trackAbsenceOp1 := &plan.Operation{
					Type:     plan.OperationTypeTrackAbsence,
					Version:  plan.OperationVersionTrackAbsence,
					Category: plan.OperationCategoryTrack,
					Config: &plan.OperationConfigTrackAbsence{
						ResourceMeta: deletableInfos[0].LocalResource.ResourceMeta,
					},
				}

				deleteOp2 := &plan.Operation{
					Type:     plan.OperationTypeDelete,
					Version:  plan.OperationVersionDelete,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigDelete{
						ResourceMeta:      deletableInfos[1].LocalResource.ResourceMeta,
						DeletePropagation: deletableInfos[1].LocalResource.DeletePropagation,
					},
				}

				trackAbsenceOp2 := &plan.Operation{
					Type:     plan.OperationTypeTrackAbsence,
					Version:  plan.OperationVersionTrackAbsence,
					Category: plan.OperationCategoryTrack,
					Config: &plan.OperationConfigTrackAbsence{
						ResourceMeta: deletableInfos[1].LocalResource.ResourceMeta,
					},
				}

				deleteDependentOp := &plan.Operation{
					Type:     plan.OperationTypeDelete,
					Version:  plan.OperationVersionDelete,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigDelete{
						ResourceMeta:      deletableInfos[2].LocalResource.ResourceMeta,
						DeletePropagation: deletableInfos[2].LocalResource.DeletePropagation,
					},
				}

				trackAbsenceDependentOp := &plan.Operation{
					Type:     plan.OperationTypeTrackAbsence,
					Version:  plan.OperationVersionTrackAbsence,
					Category: plan.OperationCategoryTrack,
					Config: &plan.OperationConfigTrackAbsence{
						ResourceMeta: deletableInfos[2].LocalResource.ResourceMeta,
					},
				}

				mainStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageUninstall, common.StageStartSuffix),
					},
				}

				mainStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageUninstall, common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					deleteOp1,
					trackAbsenceOp1,
					deleteOp2,
					trackAbsenceOp2,
					deleteDependentOp,
					trackAbsenceDependentOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						deleteOp1.ID(): {},
						deleteOp2.ID(): {},
					},
					deleteOp1.ID(): {
						trackAbsenceOp1.ID(): {},
					},
					trackAbsenceOp1.ID(): {
						deleteDependentOp.ID(): {},
					},
					deleteOp2.ID(): {
						trackAbsenceOp2.ID(): {},
					},
					trackAbsenceOp2.ID(): {
						deleteDependentOp.ID(): {},
					},
					deleteDependentOp.ID(): {
						trackAbsenceDependentOp.ID(): {},
					},
					trackAbsenceDependentOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				delRes1 := defaultDeletableResource(s.releaseName, s.releaseNamespace)
				delRes1.Name = "test-configmap-1"

				delRes2 := defaultDeletableResource(s.releaseName, s.releaseNamespace)
				delRes2.Name = "test-configmap-2"

				dependentDelRes := defaultDeletableResource(s.releaseName, s.releaseNamespace)
				dependentDelRes.Name = "dependent-secret"
				dependentDelRes.GroupVersionKind.Kind = "Secret"
				dependentDelRes.AutoInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Kinds: []string{"ConfigMap"},
						},
						ResourceState: common.ResourceStateAbsent,
					},
				}

				delInfo1 := defaultDeletableResourceInfo(delRes1, s.releaseName, s.releaseNamespace)
				delInfo2 := defaultDeletableResourceInfo(delRes2, s.releaseName, s.releaseNamespace)
				dependentDelInfo := defaultDeletableResourceInfo(dependentDelRes, s.releaseName, s.releaseNamespace)

				return nil, []*plan.DeletableResourceInfo{delInfo1, delInfo2, dependentDelInfo}, nil, plan.BuildPlanOptions{}
			},
			name: "connect delete dependency to all matching track-absence operations",
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runBuildPlanTest(tc, &s.BuildPlanSuite))
	}
}
