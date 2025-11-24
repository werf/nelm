package plan_test

import (
	"fmt"
	"testing"

	"github.com/dominikbraun/graph"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/mitchellh/copystructure"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/test"
	"github.com/werf/nelm/pkg/common"
)

type BuildPlanSuite struct {
	suite.Suite

	releaseName      string
	releaseNamespace string
	cmpOpts          cmp.Options
}

func (s *BuildPlanSuite) SetupSuite() {
	s.releaseName = "test-release"
	s.releaseNamespace = "test-namespace"

	s.cmpOpts = cmp.Options{
		cmpopts.EquateEmpty(),
		test.IgnoreEdgeOption(),
		cmpopts.SortSlices(func(a, b *plan.Operation) bool {
			return a.ID() < b.ID()
		}),
	}
}

type buildPlanTestCase struct {
	name   string
	skip   bool
	input  func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions)
	expect func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) (operations []*plan.Operation, adjMap map[string]map[string]graph.Edge[string])
}

func (s *BuildPlanSuite) TestBuildPlan() {
	testCases := []buildPlanTestCase{
		{
			name: `install resource`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				return []*plan.InstallableResourceInfo{
					defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace)),
				}, nil, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				createOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[0].LocalResource.ResourceSpec,
					},
				}

				trackReadinessOp := &plan.Operation{
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

				weightStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					weightStageStartOp,
					createOp,
					trackReadinessOp,
					weightStageEndOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						weightStageStartOp.ID(): {},
					},
					weightStageStartOp.ID(): {
						createOp.ID(): {},
					},
					createOp.ID(): {
						trackReadinessOp.ID(): {},
					},
					trackReadinessOp.ID(): {
						weightStageEndOp.ID(): {},
					},
					weightStageEndOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `delete resource`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				return nil, []*plan.DeletableResourceInfo{
					defaultDeletableResourceInfo(defaultDeletableResource(s.releaseName, s.releaseNamespace), s.releaseName, s.releaseNamespace),
				}, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				deleteOp := &plan.Operation{
					Type:     plan.OperationTypeDelete,
					Version:  plan.OperationVersionDelete,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigDelete{
						ResourceMeta:      deletableInfos[0].LocalResource.ResourceMeta,
						DeletePropagation: deletableInfos[0].LocalResource.DeletePropagation,
					},
				}

				trackDeletionOp := &plan.Operation{
					Type:     plan.OperationTypeTrackAbsence,
					Version:  plan.OperationVersionTrackAbsence,
					Category: plan.OperationCategoryTrack,
					Config: &plan.OperationConfigTrackAbsence{
						ResourceMeta: deletableInfos[0].LocalResource.ResourceMeta,
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
					deleteOp,
					trackDeletionOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						deleteOp.ID(): {},
					},
					deleteOp.ID(): {
						trackDeletionOp.ID(): {},
					},
					trackDeletionOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `create release`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				return nil, nil, []*plan.ReleaseInfo{
					defaultReleaseInfo(s.releaseName, s.releaseNamespace),
				}, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				createReleaseOp := &plan.Operation{
					Type:     plan.OperationTypeCreateRelease,
					Version:  plan.OperationVersionCreateRelease,
					Category: plan.OperationCategoryRelease,
					Config: &plan.OperationConfigCreateRelease{
						Release: releaseInfos[0].Release,
					},
				}

				updatedRelRaw, err := copystructure.Copy(releaseInfos[0].Release)
				s.Require().NoError(err)

				updatedRel := updatedRelRaw.(*helmrelease.Release)
				updatedRel.Info.Status = helmrelease.StatusDeployed

				updateReleaseOp := &plan.Operation{
					Type:     plan.OperationTypeUpdateRelease,
					Version:  plan.OperationVersionUpdateRelease,
					Category: plan.OperationCategoryRelease,
					Config: &plan.OperationConfigUpdateRelease{
						Release: updatedRel,
					},
				}

				initStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInit, common.StageStartSuffix),
					},
				}

				initStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInit, common.StageEndSuffix),
					},
				}

				finalStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageFinal, common.StageStartSuffix),
					},
				}

				finalStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageFinal, common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					initStageStartOp,
					createReleaseOp,
					initStageEndOp,
					finalStageStartOp,
					updateReleaseOp,
					finalStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					initStageStartOp.ID(): {
						createReleaseOp.ID(): {},
					},
					createReleaseOp.ID(): {
						initStageEndOp.ID(): {},
					},
					initStageEndOp.ID(): {
						finalStageStartOp.ID(): {},
					},
					finalStageStartOp.ID(): {
						updateReleaseOp.ID(): {},
					},
					updateReleaseOp.ID(): {
						finalStageEndOp.ID(): {},
					},
					finalStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `delete release`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				relInfo := defaultReleaseInfo(s.releaseName, s.releaseNamespace)
				relInfo.Must = plan.ReleaseTypeDelete
				relInfo.MustFailOnFailedDeploy = false

				return nil, nil, []*plan.ReleaseInfo{
					relInfo,
				}, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				deleteReleaseOp := &plan.Operation{
					Type:     plan.OperationTypeDeleteRelease,
					Version:  plan.OperationVersionDeleteRelease,
					Category: plan.OperationCategoryRelease,
					Config: &plan.OperationConfigDeleteRelease{
						ReleaseName:      s.releaseName,
						ReleaseNamespace: s.releaseNamespace,
						ReleaseRevision:  1,
					},
				}

				mainStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageFinal, common.StageStartSuffix),
					},
				}

				mainStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageFinal, common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					deleteReleaseOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						deleteReleaseOp.ID(): {},
					},
					deleteReleaseOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `do nothing`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				return nil, nil, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				return []*plan.Operation{}, map[string]map[string]graph.Edge[string]{}
			},
		},
		{
			name: `install automatically interdependent resources`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				resInfo := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))
				resInfo.MustTrackReadiness = false

				dependentResSpec := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				dependentResSpec.Name = "dependent-resource"
				dependentResSpec.Unstruct.SetName("dependent-resource")
				dependentResSpec.AutoInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Names:      []string{resInfo.Name},
							Namespaces: []string{resInfo.Namespace},
							Groups:     []string{resInfo.GroupVersionKind.Group},
							Kinds:      []string{resInfo.GroupVersionKind.Kind},
							Versions:   []string{resInfo.GroupVersionKind.Version},
						},
						ResourceState: common.ResourceStatePresent,
					},
				}
				dependentResInfo := defaultInstallableResourceInfo(dependentResSpec)
				dependentResInfo.MustTrackReadiness = false

				return []*plan.InstallableResourceInfo{
					resInfo,
					dependentResInfo,
				}, nil, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				createOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[0].LocalResource.ResourceSpec,
					},
				}

				createDependentOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[1].LocalResource.ResourceSpec,
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

				weightStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					weightStageStartOp,
					createOp,
					createDependentOp,
					weightStageEndOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						weightStageStartOp.ID(): {},
					},
					weightStageStartOp.ID(): {
						createOp.ID(): {},
					},
					createOp.ID(): {
						createDependentOp.ID(): {},
					},
					createDependentOp.ID(): {
						weightStageEndOp.ID(): {},
					},
					weightStageEndOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `install manually interdependent resources`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				resInfo := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))
				resInfo.MustTrackReadiness = false

				dependentResSpec := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				dependentResSpec.Name = "dependent-resource"
				dependentResSpec.Unstruct.SetName("dependent-resource")
				dependentResSpec.Weight = nil
				dependentResSpec.ManualInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Names:      []string{resInfo.Name},
							Namespaces: []string{resInfo.Namespace},
							Groups:     []string{resInfo.GroupVersionKind.Group},
							Kinds:      []string{resInfo.GroupVersionKind.Kind},
							Versions:   []string{resInfo.GroupVersionKind.Version},
						},
						ResourceState: common.ResourceStatePresent,
					},
				}
				dependentResInfo := defaultInstallableResourceInfo(dependentResSpec)
				dependentResInfo.MustTrackReadiness = false

				return []*plan.InstallableResourceInfo{
					resInfo,
					dependentResInfo,
				}, nil, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				createOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[0].LocalResource.ResourceSpec,
					},
				}

				createDependentOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[1].LocalResource.ResourceSpec,
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

				weightStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					weightStageStartOp,
					createOp,
					weightStageEndOp,
					createDependentOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						weightStageStartOp.ID(): {},
					},
					weightStageStartOp.ID(): {
						createOp.ID(): {},
					},
					createOp.ID(): {
						weightStageEndOp.ID():  {},
						createDependentOp.ID(): {},
					},
					weightStageEndOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					createDependentOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `install manually interdependent resources with no final tracking operations`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				resInfo := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))

				dependentResSpec := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				dependentResSpec.Name = "dependent-resource"
				dependentResSpec.Unstruct.SetName("dependent-resource")
				dependentResSpec.Weight = nil
				dependentResSpec.ManualInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Names:      []string{resInfo.Name},
							Namespaces: []string{resInfo.Namespace},
							Groups:     []string{resInfo.GroupVersionKind.Group},
							Kinds:      []string{resInfo.GroupVersionKind.Kind},
							Versions:   []string{resInfo.GroupVersionKind.Version},
						},
						ResourceState: common.ResourceStateReady,
					},
				}
				dependentResInfo := defaultInstallableResourceInfo(dependentResSpec)

				return []*plan.InstallableResourceInfo{
						resInfo,
						dependentResInfo,
					}, nil, nil, plan.BuildPlanOptions{
						NoFinalTracking: true,
					}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				createOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[0].LocalResource.ResourceSpec,
					},
				}

				createDependentOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[1].LocalResource.ResourceSpec,
					},
				}

				trackReadinessOp := &plan.Operation{
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

				weightStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					weightStageStartOp,
					createOp,
					trackReadinessOp,
					weightStageEndOp,
					createDependentOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						weightStageStartOp.ID(): {},
					},
					weightStageStartOp.ID(): {
						createOp.ID(): {},
					},
					createOp.ID(): {
						trackReadinessOp.ID(): {},
					},
					trackReadinessOp.ID(): {
						weightStageEndOp.ID():  {},
						createDependentOp.ID(): {},
					},
					weightStageEndOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					createDependentOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `create resource on pre-install stage`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				info := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))
				info.Stage = common.StagePreInstall
				info.MustTrackReadiness = false

				return []*plan.InstallableResourceInfo{
					info,
				}, nil, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				createOp := &plan.Operation{
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
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StagePreInstall, common.StageStartSuffix),
					},
				}

				mainStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StagePreInstall, common.StageEndSuffix),
					},
				}

				weightStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StagePreInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StagePreInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					weightStageStartOp,
					createOp,
					weightStageEndOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						weightStageStartOp.ID(): {},
					},
					weightStageStartOp.ID(): {
						createOp.ID(): {},
					},
					createOp.ID(): {
						weightStageEndOp.ID(): {},
					},
					weightStageEndOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `create two resource iterations`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				info1 := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))
				info1.MustTrackReadiness = false

				info2 := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))
				info2.Stage = common.StagePostInstall
				info2.Iteration = 1
				info2.MustTrackReadiness = false

				return []*plan.InstallableResourceInfo{
					info1,
					info2,
				}, nil, nil, plan.BuildPlanOptions{}
			},
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
					Iteration: 1,
				}

				mainStageStartOp1 := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInstall, common.StageStartSuffix),
					},
				}

				mainStageEndOp1 := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StageInstall, common.StageEndSuffix),
					},
				}

				weightStageStartOp1 := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp1 := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				mainStageStartOp2 := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StagePostInstall, common.StageStartSuffix),
					},
				}

				mainStageEndOp2 := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.StagePostInstall, common.StageEndSuffix),
					},
				}

				weightStageStartOp2 := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StagePostInstall, *installableInfos[1].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp2 := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StagePostInstall, *installableInfos[1].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp1,
					weightStageStartOp1,
					createOp1,
					weightStageEndOp1,
					mainStageEndOp1,
					mainStageStartOp2,
					weightStageStartOp2,
					createOp2,
					weightStageEndOp2,
					mainStageEndOp2,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp1.ID(): {
						weightStageStartOp1.ID(): {},
					},
					weightStageStartOp1.ID(): {
						createOp1.ID(): {},
					},
					createOp1.ID(): {
						weightStageEndOp1.ID(): {},
					},
					weightStageEndOp1.ID(): {
						mainStageEndOp1.ID(): {},
					},
					mainStageEndOp1.ID(): {
						mainStageStartOp2.ID(): {},
					},
					mainStageStartOp2.ID(): {
						weightStageStartOp2.ID(): {},
					},
					weightStageStartOp2.ID(): {
						createOp2.ID(): {},
					},
					createOp2.ID(): {
						weightStageEndOp2.ID(): {},
					},
					weightStageEndOp2.ID(): {
						mainStageEndOp2.ID(): {},
					},
					mainStageEndOp2.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `create resource and delete it after`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				info := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))
				info.MustTrackReadiness = false
				info.MustDeleteOnSuccessfulInstall = true

				return []*plan.InstallableResourceInfo{
					info,
				}, nil, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				createOp := &plan.Operation{
					Type:     plan.OperationTypeCreate,
					Version:  plan.OperationVersionCreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigCreate{
						ResourceSpec: installableInfos[0].LocalResource.ResourceSpec,
					},
				}

				deleteOp := &plan.Operation{
					Type:     plan.OperationTypeDelete,
					Version:  plan.OperationVersionDelete,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigDelete{
						ResourceMeta:      installableInfos[0].LocalResource.ResourceMeta,
						DeletePropagation: installableInfos[0].LocalResource.DeletePropagation,
					},
				}

				trackDeletionOp := &plan.Operation{
					Type:     plan.OperationTypeTrackAbsence,
					Version:  plan.OperationVersionTrackAbsence,
					Category: plan.OperationCategoryTrack,
					Config: &plan.OperationConfigTrackAbsence{
						ResourceMeta: installableInfos[0].LocalResource.ResourceMeta,
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

				weightStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					weightStageStartOp,
					createOp,
					deleteOp,
					trackDeletionOp,
					weightStageEndOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						weightStageStartOp.ID(): {},
					},
					weightStageStartOp.ID(): {
						createOp.ID(): {},
					},
					createOp.ID(): {
						deleteOp.ID(): {},
					},
					deleteOp.ID(): {
						trackDeletionOp.ID(): {},
					},
					trackDeletionOp.ID(): {
						weightStageEndOp.ID(): {},
					},
					weightStageEndOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `recreate resource`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				info := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))
				info.MustTrackReadiness = false
				info.MustInstall = plan.ResourceInstallTypeRecreate

				return []*plan.InstallableResourceInfo{
					info,
				}, nil, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				recreateOp := &plan.Operation{
					Type:     plan.OperationTypeRecreate,
					Version:  plan.OperationVersionRecreate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigRecreate{
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

				weightStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					weightStageStartOp,
					recreateOp,
					weightStageEndOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						weightStageStartOp.ID(): {},
					},
					weightStageStartOp.ID(): {
						recreateOp.ID(): {},
					},
					recreateOp.ID(): {
						weightStageEndOp.ID(): {},
					},
					weightStageEndOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `apply resource`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				info := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))
				info.MustTrackReadiness = false
				info.MustInstall = plan.ResourceInstallTypeApply

				return []*plan.InstallableResourceInfo{
					info,
				}, nil, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				applyOp := &plan.Operation{
					Type:     plan.OperationTypeApply,
					Version:  plan.OperationVersionApply,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigApply{
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

				weightStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					weightStageStartOp,
					applyOp,
					weightStageEndOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						weightStageStartOp.ID(): {},
					},
					weightStageStartOp.ID(): {
						applyOp.ID(): {},
					},
					applyOp.ID(): {
						weightStageEndOp.ID(): {},
					},
					weightStageEndOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
		{
			name: `update resource`,
			input: func() (installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo, opts plan.BuildPlanOptions) {
				info := defaultInstallableResourceInfo(defaultInstallableResource(s.releaseName, s.releaseNamespace))
				info.MustTrackReadiness = false
				info.MustInstall = plan.ResourceInstallTypeUpdate

				return []*plan.InstallableResourceInfo{
					info,
				}, nil, nil, plan.BuildPlanOptions{}
			},
			expect: func(installableInfos []*plan.InstallableResourceInfo, deletableInfos []*plan.DeletableResourceInfo, releaseInfos []*plan.ReleaseInfo) ([]*plan.Operation, map[string]map[string]graph.Edge[string]) {
				updateOp := &plan.Operation{
					Type:     plan.OperationTypeUpdate,
					Version:  plan.OperationVersionUpdate,
					Category: plan.OperationCategoryResource,
					Config: &plan.OperationConfigUpdate{
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

				weightStageStartOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageStartSuffix),
					},
				}

				weightStageEndOp := &plan.Operation{
					Type:     plan.OperationTypeNoop,
					Version:  plan.OperationVersionNoop,
					Category: plan.OperationCategoryMeta,
					Config: &plan.OperationConfigNoop{
						OpID: fmt.Sprintf("%s/%s/%s", common.StagePrefix, common.SubStageWeighted(common.StageInstall, *installableInfos[0].LocalResource.Weight), common.StageEndSuffix),
					},
				}

				ops := []*plan.Operation{
					mainStageStartOp,
					weightStageStartOp,
					updateOp,
					weightStageEndOp,
					mainStageEndOp,
				}

				adjMap := map[string]map[string]graph.Edge[string]{
					mainStageStartOp.ID(): {
						weightStageStartOp.ID(): {},
					},
					weightStageStartOp.ID(): {
						updateOp.ID(): {},
					},
					updateOp.ID(): {
						weightStageEndOp.ID(): {},
					},
					weightStageEndOp.ID(): {
						mainStageEndOp.ID(): {},
					},
					mainStageEndOp.ID(): {},
				}

				return ops, adjMap
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runBuildPlanTest(tc, s))
	}
}

func TestBuildPlanSuites(t *testing.T) {
	suite.Run(t, new(BuildPlanSuite))
}

func defaultRelease(releaseName, releaseNamespace string) *helmrelease.Release {
	return &helmrelease.Release{
		Name:      releaseName,
		Namespace: releaseNamespace,
		Info: &helmrelease.Info{
			Status: helmrelease.StatusPendingInstall,
		},
		Version: 1,
	}
}

func defaultReleaseInfo(releaseName, releaseNamespace string) *plan.ReleaseInfo {
	return &plan.ReleaseInfo{
		Release:                defaultRelease(releaseName, releaseNamespace),
		Must:                   plan.ReleaseTypeInstall,
		MustFailOnFailedDeploy: true,
	}
}

func runBuildPlanTest(tc buildPlanTestCase, s *BuildPlanSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		instInfos, delInfos, relInfos, opts := tc.input()

		plan, err := plan.BuildPlan(instInfos, delInfos, relInfos, opts)
		s.Require().NoError(err)

		operations := plan.Operations()
		adjMap := lo.Must(plan.Graph.AdjacencyMap())

		expectOperations, expectAdjMap := tc.expect(instInfos, delInfos, relInfos)

		if !cmp.Equal(expectOperations, operations, s.cmpOpts) {
			s.T().Fatalf("unexpected plan operations (-want +got):\n%s", cmp.Diff(expectOperations, plan.Operations(), s.cmpOpts...))
		}

		if !cmp.Equal(expectAdjMap, adjMap, s.cmpOpts) {
			s.T().Fatalf("unexpected plan adjacency map (-want +got):\n%s", cmp.Diff(expectAdjMap, adjMap, s.cmpOpts...))
		}
	}
}
