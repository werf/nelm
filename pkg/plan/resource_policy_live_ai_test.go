//go:build ai_tests

package plan_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/kube/fake"
	"github.com/werf/nelm/pkg/plan"
)

type ResourcePolicyLiveAISuite struct {
	suite.Suite

	clientFactory    *fake.ClientFactory
	releaseName      string
	releaseNamespace string
}

func (s *ResourcePolicyLiveAISuite) SetupSubTest() {
	var err error

	s.clientFactory, err = fake.NewClientFactory(context.Background())
	s.Require().NoError(err)
}

func (s *ResourcePolicyLiveAISuite) SetupSuite() {
	s.releaseName = "test-release"
	s.releaseNamespace = "test-namespace"
}

func (s *ResourcePolicyLiveAISuite) TestAI_ChartPolicyStillProtectsChartRemovedResource() {
	s.Run("chart skip-delete keeps resource", func() {
		s.createLiveResource(nil)

		localRes := defaultDeletableResource(s.releaseName, s.releaseNamespace)
		localRes.ResourcePolicies = []common.ResourcePolicy{common.ResourcePolicySkipDelete}

		resInfo, err := plan.BuildDeletableResourceInfo(context.Background(), localRes, common.DeployTypeUninstall, s.releaseName, s.releaseNamespace, s.clientFactory)
		s.Require().NoError(err)
		s.Require().False(resInfo.MustDelete, "chart-side skip-delete must keep protecting the resource")
	})
}

func (s *ResourcePolicyLiveAISuite) TestAI_ChartPolicyStillSuppressesDeleteOnSucceeded() {
	s.Run("chart skip-delete suppresses delete-on-succeeded", func() {
		s.createLiveResource(nil)

		localRes := defaultInstallableResource(s.releaseName, s.releaseNamespace)
		localRes.DeleteOnSucceeded = true
		localRes.ResourcePolicies = []common.ResourcePolicy{common.ResourcePolicySkipDelete}

		resInfos, err := plan.BuildInstallableResourceInfo(context.Background(), localRes, common.DeployTypeInitial, s.releaseNamespace, false, true, s.clientFactory, plan.BuildResourceInfosOptions{}, nil)
		s.Require().NoError(err)
		s.Require().NotEmpty(resInfos)
		s.Require().False(resInfos[0].MustDeleteOnSuccessfulInstall, "chart-side skip-delete must still suppress delete-on-succeeded")
	})
}

func (s *ResourcePolicyLiveAISuite) TestAI_LiveOnlyPolicyDoesNotProtectChartRemovedResource() {
	livePolicies := []map[string]string{
		{"helm.sh/resource-policy": "keep"},
		{"werf.io/resource-policy": "keep"},
		{"werf.io/resource-policy": "skip-delete"},
		{"werf.io/resource-policy": "bogus"},
	}

	for _, policy := range livePolicies {
		s.Run(policyName(policy), func() {
			s.createLiveResource(policy)

			localRes := defaultDeletableResource(s.releaseName, s.releaseNamespace)

			resInfo, err := plan.BuildDeletableResourceInfo(context.Background(), localRes, common.DeployTypeUninstall, s.releaseName, s.releaseNamespace, s.clientFactory)
			s.Require().NoError(err)
			s.Require().True(resInfo.MustDelete, "chart-removed resource must be deleted despite live-only policy %v", policy)
		})
	}
}

func (s *ResourcePolicyLiveAISuite) TestAI_LiveOnlyPolicyDoesNotSuppressDeleteOnFailed() {
	livePolicies := []map[string]string{
		{"werf.io/resource-policy": "skip-delete"},
		{"werf.io/resource-policy": "bogus"},
	}

	for _, policy := range livePolicies {
		s.Run(policyName(policy), func() {
			s.createLiveResource(policy)

			localRes := updatedInstallableResource(&s.Suite, s.releaseName, s.releaseNamespace)
			localRes.DeleteOnFailed = true

			resInfos, err := plan.BuildInstallableResourceInfo(context.Background(), localRes, common.DeployTypeInitial, s.releaseNamespace, false, true, s.clientFactory, plan.BuildResourceInfosOptions{}, nil)
			s.Require().NoError(err)
			s.Require().NotEmpty(resInfos)
			s.Require().Equal(plan.ResourceInstallTypeUpdate, resInfos[0].MustInstall)
			s.Require().True(resInfos[0].MustDeleteOnFailedInstall, "delete-on-failed must not be suppressed by live-only policy %v", policy)
		})
	}
}

func (s *ResourcePolicyLiveAISuite) TestAI_LiveOnlyPolicyDoesNotSuppressDeleteOnSucceeded() {
	livePolicies := []map[string]string{
		{"werf.io/resource-policy": "skip-delete"},
		{"werf.io/resource-policy": "bogus"},
	}

	for _, policy := range livePolicies {
		s.Run(policyName(policy), func() {
			s.createLiveResource(policy)

			localRes := defaultInstallableResource(s.releaseName, s.releaseNamespace)
			localRes.DeleteOnSucceeded = true

			resInfos, err := plan.BuildInstallableResourceInfo(context.Background(), localRes, common.DeployTypeInitial, s.releaseNamespace, false, true, s.clientFactory, plan.BuildResourceInfosOptions{}, nil)
			s.Require().NoError(err)
			s.Require().NotEmpty(resInfos)
			s.Require().Equal(plan.ResourceInstallTypeNone, resInfos[0].MustInstall)
			s.Require().True(resInfos[0].MustDeleteOnSuccessfulInstall, "delete-on-succeeded must not be suppressed by live-only policy %v", policy)
		})
	}
}

func (s *ResourcePolicyLiveAISuite) createLiveResource(policyAnnotations map[string]string) {
	resSpec := defaultResourceSpec(s.releaseName, s.releaseNamespace)

	annotations := resSpec.Unstruct.GetAnnotations()
	for k, v := range policyAnnotations {
		annotations[k] = v
	}

	resSpec.SetAnnotations(annotations)

	_, err := s.clientFactory.KubeClient().Create(context.Background(), resSpec, kube.KubeClientCreateOptions{
		DefaultNamespace: s.releaseNamespace,
	})
	s.Require().NoError(err)
}

func TestAI_ResourcePolicyLiveSuite(t *testing.T) {
	suite.Run(t, new(ResourcePolicyLiveAISuite))
}

func policyName(policy map[string]string) string {
	if len(policy) == 0 {
		return "no policy"
	}

	var name string
	for k, v := range policy {
		name += k + "=" + v
	}

	return name
}
