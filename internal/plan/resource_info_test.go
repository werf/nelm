package plan_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/kube/fake"
	"github.com/werf/nelm/internal/plan"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/test"
	"github.com/werf/nelm/pkg/common"
)

type ResourceInfoSuite struct {
	suite.Suite

	releaseName      string
	releaseNamespace string
	clientFactory    *fake.ClientFactory
	cmpOpts          cmp.Options
}

func (s *ResourceInfoSuite) SetupSuite() {
	s.releaseName = "test-release"
	s.releaseNamespace = "test-namespace"

	s.cmpOpts = cmp.Options{
		cmpopts.EquateEmpty(),
		test.CompareRegexpOption(),
		test.CompareInternalDependencyOption(),
		test.CompareResourceMetadataOption(s.releaseNamespace),
	}
}

func (s *ResourceInfoSuite) SetupSubTest() {
	var err error

	s.clientFactory, err = fake.NewClientFactory(context.Background())
	s.Require().NoError(err)
}

type buildInstallableResourceInfoTestCase struct {
	name    string
	skip    bool
	prepare func()
	input   func() (localRes *resource.InstallableResource, deployType common.DeployType, prevRelFailed bool)
	expect  func(*resource.InstallableResource) []*plan.InstallableResourceInfo
}

func (s *ResourceInfoSuite) TestBuildInstallableResourceInfo() {
	testCases := []buildInstallableResourceInfoTestCase{
		{
			name: `for non-existing resource, initial deploy`,
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				return defaultInstallableResource(s.releaseName, s.releaseNamespace), common.DeployTypeInitial, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				return []*plan.InstallableResourceInfo{defaultInstallableResourceInfo(localRes)}
			},
		},
		{
			name: `for outdated resource, initial deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				return updatedInstallableResource(&s.Suite, s.releaseName, s.releaseNamespace), common.DeployTypeInitial, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				info := defaultInstallableResourceInfo(localRes)
				info.GetResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.DryApplyResult = updatedResourceSpec(&s.Suite, s.releaseName, s.releaseNamespace).Unstruct
				info.MustInstall = plan.ResourceInstallTypeUpdate

				return []*plan.InstallableResourceInfo{info}
			},
		},
		{
			name: `for up-to-date resource, initial deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				return defaultInstallableResource(s.releaseName, s.releaseNamespace), common.DeployTypeInitial, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				info := defaultInstallableResourceInfo(localRes)
				info.GetResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.DryApplyResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.MustInstall = plan.ResourceInstallTypeNone
				info.MustTrackReadiness = false

				return []*plan.InstallableResourceInfo{info}
			},
		},
		{
			name: `for outdated resource, recreate instead of apply, initial deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				localRes := updatedInstallableResource(&s.Suite, s.releaseName, s.releaseNamespace)
				localRes.Recreate = true

				return localRes, common.DeployTypeInitial, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				info := defaultInstallableResourceInfo(localRes)
				info.GetResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.DryApplyResult = updatedResourceSpec(&s.Suite, s.releaseName, s.releaseNamespace).Unstruct
				info.MustInstall = plan.ResourceInstallTypeRecreate

				return []*plan.InstallableResourceInfo{info}
			},
		},
		{
			name: `for up-to-date resource, delete after, initial deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				localRes := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				localRes.DeleteOnSucceeded = true

				return localRes, common.DeployTypeInitial, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				info := defaultInstallableResourceInfo(localRes)
				info.GetResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.DryApplyResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.MustInstall = plan.ResourceInstallTypeNone
				info.MustDeleteOnSuccessfulInstall = true
				info.StageDeleteOnSuccessfulInstall = common.StageUninstall

				return []*plan.InstallableResourceInfo{info}
			},
		},
		{
			name: `for up-to-date resource, delete on failure, initial deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				localRes := defaultInstallableResource(s.releaseName, s.releaseNamespace)
				localRes.DeleteOnFailed = true

				return localRes, common.DeployTypeInitial, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				info := defaultInstallableResourceInfo(localRes)
				info.GetResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.DryApplyResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.MustTrackReadiness = false
				info.MustInstall = plan.ResourceInstallTypeNone
				info.MustDeleteOnFailedInstall = false

				return []*plan.InstallableResourceInfo{info}
			},
		},
		{
			name: `for non-existing resource, upgrade deploy`,
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				return defaultInstallableResource(s.releaseName, s.releaseNamespace), common.DeployTypeUpgrade, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				return []*plan.InstallableResourceInfo{defaultInstallableResourceInfo(localRes)}
			},
		},
		{
			name: `for outdated resource, upgrade deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				return updatedInstallableResource(&s.Suite, s.releaseName, s.releaseNamespace), common.DeployTypeUpgrade, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				info := defaultInstallableResourceInfo(localRes)
				info.GetResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.DryApplyResult = updatedResourceSpec(&s.Suite, s.releaseName, s.releaseNamespace).Unstruct
				info.MustInstall = plan.ResourceInstallTypeUpdate

				return []*plan.InstallableResourceInfo{info}
			},
		},
		{
			name: `for up-to-date resource, upgrade deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				return defaultInstallableResource(s.releaseName, s.releaseNamespace), common.DeployTypeUpgrade, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				info := defaultInstallableResourceInfo(localRes)
				info.GetResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.DryApplyResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.MustInstall = plan.ResourceInstallTypeNone
				info.MustTrackReadiness = false

				return []*plan.InstallableResourceInfo{info}
			},
		},
		{
			name: `for outdated resource, recreate instead of apply, upgrade deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				localRes := updatedInstallableResource(&s.Suite, s.releaseName, s.releaseNamespace)
				localRes.Recreate = true

				return localRes, common.DeployTypeUpgrade, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				info := defaultInstallableResourceInfo(localRes)
				info.GetResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.DryApplyResult = updatedResourceSpec(&s.Suite, s.releaseName, s.releaseNamespace).Unstruct
				info.MustInstall = plan.ResourceInstallTypeRecreate

				return []*plan.InstallableResourceInfo{info}
			},
		},
		{
			name: `for up-to-date resource, previous release failed, upgrade deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				return defaultInstallableResource(s.releaseName, s.releaseNamespace), common.DeployTypeUpgrade, true
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				info := defaultInstallableResourceInfo(localRes)
				info.GetResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.DryApplyResult = defaultResourceSpec(s.releaseName, s.releaseNamespace).Unstruct
				info.MustInstall = plan.ResourceInstallTypeNone
				info.MustTrackReadiness = true

				return []*plan.InstallableResourceInfo{info}
			},
		},
		{
			name: `for non-existing resource, uninstall deploy`,
			input: func() (*resource.InstallableResource, common.DeployType, bool) {
				return defaultInstallableResource(s.releaseName, s.releaseNamespace), common.DeployTypeUninstall, false
			},
			expect: func(localRes *resource.InstallableResource) []*plan.InstallableResourceInfo {
				return []*plan.InstallableResourceInfo{}
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runBuildInstallableResourceInfoTest(tc, s))
	}
}

type buildDeletableResourceInfoTestCase struct {
	name    string
	skip    bool
	prepare func()
	input   func() (localRes *resource.DeletableResource, deployType common.DeployType)
	expect  func(*resource.DeletableResource) *plan.DeletableResourceInfo
}

func (s *ResourceInfoSuite) TestBuildDeletableResourceInfo() {
	testCases := []buildDeletableResourceInfoTestCase{
		{
			name: `for existing resource, uninstall deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.DeletableResource, common.DeployType) {
				return defaultDeletableResource(s.releaseName, s.releaseNamespace), common.DeployTypeUninstall
			},
			expect: func(localRes *resource.DeletableResource) *plan.DeletableResourceInfo {
				return defaultDeletableResourceInfo(localRes, s.releaseName, s.releaseNamespace)
			},
		},
		{
			name: `for non-existing resource, uninstall deploy`,
			input: func() (*resource.DeletableResource, common.DeployType) {
				return defaultDeletableResource(s.releaseName, s.releaseNamespace), common.DeployTypeUninstall
			},
			expect: func(localRes *resource.DeletableResource) *plan.DeletableResourceInfo {
				info := defaultDeletableResourceInfo(localRes, s.releaseName, s.releaseNamespace)
				info.GetResult = nil
				info.MustDelete = false
				info.MustTrackAbsence = false

				return info
			},
		},
		{
			name: `for existing resource, keep resource, uninstall deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.DeletableResource, common.DeployType) {
				localRes := defaultDeletableResource(s.releaseName, s.releaseNamespace)
				localRes.KeepOnDelete = true

				return localRes, common.DeployTypeUninstall
			},
			expect: func(localRes *resource.DeletableResource) *plan.DeletableResourceInfo {
				info := defaultDeletableResourceInfo(localRes, s.releaseName, s.releaseNamespace)
				info.MustDelete = false
				info.MustTrackAbsence = false
				info.GetResult = nil

				return info
			},
		},
		{
			name: `for existing resource, owned by anyone, uninstall deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.DeletableResource, common.DeployType) {
				localRes := defaultDeletableResource(s.releaseName, s.releaseNamespace)
				localRes.Ownership = common.OwnershipAnyone

				return localRes, common.DeployTypeUninstall
			},
			expect: func(localRes *resource.DeletableResource) *plan.DeletableResourceInfo {
				info := defaultDeletableResourceInfo(localRes, s.releaseName, s.releaseNamespace)
				info.GetResult = nil
				info.MustDelete = false
				info.MustTrackAbsence = false

				return info
			},
		},
		{
			name: `for existing resource, with invalid release annotation, uninstall deploy`,
			prepare: func() {
				resSpec := defaultResourceSpec(s.releaseName, s.releaseNamespace)
				annos := resSpec.Unstruct.GetAnnotations()
				annos["meta.helm.sh/release-name"] = "another-release"
				resSpec.Unstruct.SetAnnotations(annos)

				_, err := s.clientFactory.KubeClient().Create(context.Background(), resSpec, kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.DeletableResource, common.DeployType) {
				return defaultDeletableResource(s.releaseName, s.releaseNamespace), common.DeployTypeUninstall
			},
			expect: func(localRes *resource.DeletableResource) *plan.DeletableResourceInfo {
				info := defaultDeletableResourceInfo(localRes, s.releaseName, s.releaseNamespace)
				annos := info.GetResult.GetAnnotations()
				annos["meta.helm.sh/release-name"] = "another-release"
				info.GetResult.SetAnnotations(annos)
				info.MustDelete = false
				info.MustTrackAbsence = false

				return info
			},
		},
		{
			name: `for existing resource, with non-present release annotation, uninstall deploy`,
			prepare: func() {
				resSpec := defaultResourceSpec(s.releaseName, s.releaseNamespace)
				resSpec.Unstruct.SetAnnotations(lo.OmitByKeys(resSpec.Unstruct.GetAnnotations(), []string{"meta.helm.sh/release-name"}))

				_, err := s.clientFactory.KubeClient().Create(context.Background(), resSpec, kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.DeletableResource, common.DeployType) {
				return defaultDeletableResource(s.releaseName, s.releaseNamespace), common.DeployTypeUninstall
			},
			expect: func(localRes *resource.DeletableResource) *plan.DeletableResourceInfo {
				info := defaultDeletableResourceInfo(localRes, s.releaseName, s.releaseNamespace)
				info.GetResult.SetAnnotations(lo.OmitByKeys(info.GetResult.GetAnnotations(), []string{"meta.helm.sh/release-name"}))
				info.MustDelete = false
				info.MustTrackAbsence = false

				return info
			},
		},
		{
			name: `for existing resource, initial deploy`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() (*resource.DeletableResource, common.DeployType) {
				return defaultDeletableResource(s.releaseName, s.releaseNamespace), common.DeployTypeInstall
			},
			expect: func(localRes *resource.DeletableResource) *plan.DeletableResourceInfo {
				info := defaultDeletableResourceInfo(localRes, s.releaseName, s.releaseNamespace)
				info.Stage = common.StagePrePreUninstall

				return info
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runBuildDeletableResourceInfoTest(tc, s))
	}
}

type buildResourceInfosTestCase struct {
	name    string
	skip    bool
	prepare func()
	input   func() (instResources []*resource.InstallableResource, delResources []*resource.DeletableResource, deployType common.DeployType, prevRelFailed bool)
	expect  func([]*resource.InstallableResource, []*resource.DeletableResource) ([]*plan.InstallableResourceInfo, []*plan.DeletableResourceInfo)
}

func (s *ResourceInfoSuite) TestBuildResourceInfos() {
	testCases := []buildResourceInfosTestCase{
		{
			name: `for installable resource`,
			input: func() ([]*resource.InstallableResource, []*resource.DeletableResource, common.DeployType, bool) {
				return []*resource.InstallableResource{defaultInstallableResource(s.releaseName, s.releaseNamespace)}, []*resource.DeletableResource{}, common.DeployTypeInitial, false
			},
			expect: func(instResources []*resource.InstallableResource, delResources []*resource.DeletableResource) ([]*plan.InstallableResourceInfo, []*plan.DeletableResourceInfo) {
				return []*plan.InstallableResourceInfo{defaultInstallableResourceInfo(instResources[0])}, []*plan.DeletableResourceInfo{}
			},
		},
		{
			name: `for duplicated installable resource`,
			input: func() ([]*resource.InstallableResource, []*resource.DeletableResource, common.DeployType, bool) {
				return []*resource.InstallableResource{defaultInstallableResource(s.releaseName, s.releaseNamespace), defaultInstallableResource(s.releaseName, s.releaseNamespace)}, []*resource.DeletableResource{}, common.DeployTypeInitial, false
			},
			expect: func(instResources []*resource.InstallableResource, delResources []*resource.DeletableResource) ([]*plan.InstallableResourceInfo, []*plan.DeletableResourceInfo) {
				info0 := defaultInstallableResourceInfo(instResources[0])
				info1 := defaultInstallableResourceInfo(instResources[1])
				info1.Iteration = 1

				return []*plan.InstallableResourceInfo{info0, info1}, []*plan.DeletableResourceInfo{}
			},
		},
		{
			name: `for deletable resource`,
			prepare: func() {
				_, err := s.clientFactory.KubeClient().Create(context.Background(), defaultResourceSpec(s.releaseName, s.releaseNamespace), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				})
				s.Require().NoError(err)
			},
			input: func() ([]*resource.InstallableResource, []*resource.DeletableResource, common.DeployType, bool) {
				return []*resource.InstallableResource{}, []*resource.DeletableResource{defaultDeletableResource(s.releaseName, s.releaseNamespace)}, common.DeployTypeUninstall, false
			},
			expect: func(instResources []*resource.InstallableResource, delResources []*resource.DeletableResource) ([]*plan.InstallableResourceInfo, []*plan.DeletableResourceInfo) {
				return []*plan.InstallableResourceInfo{}, []*plan.DeletableResourceInfo{defaultDeletableResourceInfo(delResources[0], s.releaseName, s.releaseNamespace)}
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runBuildResourceInfosTest(tc, s))
	}
}

func TestResourceSuites(t *testing.T) {
	suite.Run(t, new(ResourceInfoSuite))
}

func defaultResourceSpec(releaseName, releaseNamespace string) *spec.ResourceSpec {
	resSpec := spec.NewResourceSpec(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "test-configmap",
				"annotations": map[string]interface{}{
					"meta.helm.sh/release-name":      releaseName,
					"meta.helm.sh/release-namespace": releaseNamespace,
				},
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "Helm",
				},
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}, releaseNamespace, spec.ResourceSpecOptions{})
	resSpec.Unstruct.SetNamespace(releaseNamespace)

	return resSpec
}

func updatedResourceSpec(s *suite.Suite, releaseName, releaseNamespace string) *spec.ResourceSpec {
	resSpec := defaultResourceSpec(releaseName, releaseNamespace)

	err := unstructured.SetNestedField(resSpec.Unstruct.UnstructuredContent(), "value2", "data", "key2")
	s.Require().NoError(err)

	return resSpec
}

func defaultInstallableResource(releaseName, releaseNamespace string) *resource.InstallableResource {
	return &resource.InstallableResource{
		ResourceSpec:                    defaultResourceSpec(releaseName, releaseNamespace),
		Ownership:                       common.OwnershipRelease,
		FailMode:                        multitrack.FailWholeDeployProcessImmediately,
		NoActivityTimeout:               4 * time.Minute,
		ShowLogsOnlyForNumberOfReplicas: 1,
		TrackTerminationMode:            multitrack.WaitUntilResourceReady,
		Weight:                          lo.ToPtr(0),
		DeployConditions: map[common.On][]common.Stage{
			common.InstallOnInstall:  {common.StageInstall},
			common.InstallOnUpgrade:  {common.StageInstall},
			common.InstallOnRollback: {common.StageInstall},
		},
	}
}

func updatedInstallableResource(s *suite.Suite, releaseName, releaseNamespace string) *resource.InstallableResource {
	res := defaultInstallableResource(releaseName, releaseNamespace)
	res.ResourceSpec = updatedResourceSpec(s, releaseName, releaseNamespace)

	return res
}

func defaultDeletableResource(releaseName, releaseNamespace string) *resource.DeletableResource {
	return &resource.DeletableResource{
		ResourceMeta: defaultResourceSpec(releaseName, releaseNamespace).ResourceMeta,
		Ownership:    common.OwnershipRelease,
	}
}

func defaultInstallableResourceInfo(localRes *resource.InstallableResource) *plan.InstallableResourceInfo {
	return &plan.InstallableResourceInfo{
		ResourceMeta:       localRes.ResourceMeta,
		LocalResource:      localRes,
		MustInstall:        plan.ResourceInstallTypeCreate,
		MustTrackReadiness: true,
		Stage:              common.StageInstall,
	}
}

func defaultDeletableResourceInfo(localRes *resource.DeletableResource, releaseName, releaseNamespace string) *plan.DeletableResourceInfo {
	return &plan.DeletableResourceInfo{
		ResourceMeta:     localRes.ResourceMeta,
		GetResult:        defaultResourceSpec(releaseName, releaseNamespace).Unstruct,
		LocalResource:    localRes,
		MustDelete:       true,
		MustTrackAbsence: true,
		Stage:            common.StageUninstall,
	}
}

func runBuildInstallableResourceInfoTest(tc buildInstallableResourceInfoTestCase, s *ResourceInfoSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		if tc.prepare != nil {
			tc.prepare()
		}

		localRes, deployType, prevRelFailed := tc.input()

		resInfos, err := plan.BuildInstallableResourceInfo(context.Background(), localRes, deployType, s.releaseNamespace, prevRelFailed, true, s.clientFactory)
		s.Require().NoError(err)

		expectResInfos := tc.expect(localRes)

		if !cmp.Equal(expectResInfos, resInfos, s.cmpOpts) {
			s.T().Fatalf("unexpected installable resource infos (-want +got):\n%s", cmp.Diff(expectResInfos, resInfos, s.cmpOpts...))
		}
	}
}

func runBuildDeletableResourceInfoTest(tc buildDeletableResourceInfoTestCase, s *ResourceInfoSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		if tc.prepare != nil {
			tc.prepare()
		}

		localRes, deployType := tc.input()

		resInfo, err := plan.BuildDeletableResourceInfo(context.Background(), localRes, deployType, s.releaseName, s.releaseNamespace, s.clientFactory)
		s.Require().NoError(err)

		expectResInfo := tc.expect(localRes)

		if !cmp.Equal(expectResInfo, resInfo, s.cmpOpts) {
			s.T().Fatalf("unexpected deletable resource info (-want +got):\n%s", cmp.Diff(expectResInfo, resInfo, s.cmpOpts...))
		}
	}
}

func runBuildResourceInfosTest(tc buildResourceInfosTestCase, s *ResourceInfoSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		if tc.prepare != nil {
			tc.prepare()
		}

		instResources, delResources, deployType, prevRelFailed := tc.input()

		instResInfos, delResInfos, err := plan.BuildResourceInfos(context.Background(), deployType, s.releaseName, s.releaseNamespace, instResources, delResources, prevRelFailed, s.clientFactory, plan.BuildResourceInfosOptions{
			NetworkParallelism: 10,
		})
		s.Require().NoError(err)

		expectInstResInfos, expectDelResInfos := tc.expect(instResources, delResources)

		if !cmp.Equal(expectInstResInfos, instResInfos, s.cmpOpts) {
			s.T().Fatalf("unexpected installable resource infos (-want +got):\n%s", cmp.Diff(expectInstResInfos, instResInfos, s.cmpOpts...))
		}

		if !cmp.Equal(expectDelResInfos, delResInfos, s.cmpOpts) {
			s.T().Fatalf("unexpected deletable resource infos (-want +got):\n%s", cmp.Diff(expectDelResInfos, delResInfos, s.cmpOpts...))
		}
	}
}
