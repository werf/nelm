package plan_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/kube"
	"github.com/werf/nelm/v2/pkg/kube/fake"
	"github.com/werf/nelm/v2/pkg/plan"
)

type ResourcePolicyLiveSuite struct {
	suite.Suite

	clientFactory    *fake.ClientFactory
	releaseName      string
	releaseNamespace string
}

func (s *ResourcePolicyLiveSuite) SetupSubTest() {
	var err error

	s.clientFactory, err = fake.NewClientFactory(context.Background())
	s.Require().NoError(err)
}

func (s *ResourcePolicyLiveSuite) SetupSuite() {
	s.releaseName = "test-release"
	s.releaseNamespace = "test-namespace"
}

func (s *ResourcePolicyLiveSuite) TestChartPolicyStillProtectsChartRemovedResource() {
	s.Run("chart skip-delete keeps resource", func() {
		s.createLiveResource(nil)

		localRes := defaultDeletableResource(s.releaseName, s.releaseNamespace)
		localRes.ResourcePolicies = []common.ResourcePolicy{common.ResourcePolicySkipDelete}

		resInfo, err := plan.BuildDeletableResourceInfo(context.Background(), localRes, common.DeployTypeUninstall, s.releaseName, s.releaseNamespace, s.clientFactory)
		s.Require().NoError(err)
		s.Require().False(resInfo.MustDelete, "chart-side skip-delete must keep protecting the resource")
	})
}

func (s *ResourcePolicyLiveSuite) TestChartPolicyStillSuppressesDeleteOnSucceeded() {
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

func (s *ResourcePolicyLiveSuite) TestLiveOnlyPolicyProtectsChartRemovedResource() {
	livePolicies := []map[string]string{
		{"helm.sh/resource-policy": "keep"},
		{"werf.io/resource-policy": "keep"},
		{"werf.io/resource-policy": "skip-delete"},
		{"werf.io/resource-policy": "bogus"},
	}

	for _, deployType := range []common.DeployType{common.DeployTypeUpgrade, common.DeployTypeUninstall} {
		for _, policy := range livePolicies {
			s.Run(string(deployType)+"/"+policyName(policy), func() {
				s.createLiveResource(policy)

				localRes := defaultDeletableResource(s.releaseName, s.releaseNamespace)

				resInfo, err := plan.BuildDeletableResourceInfo(context.Background(), localRes, deployType, s.releaseName, s.releaseNamespace, s.clientFactory)
				s.Require().NoError(err)
				s.Require().False(resInfo.MustDelete, "chart-removed resource must be kept by live-only policy %v", policy)
			})
		}
	}
}

func (s *ResourcePolicyLiveSuite) TestLiveOnlyPolicySuppressesDeleteOnFailed() {
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
			s.Require().False(resInfos[0].MustDeleteOnFailedInstall, "delete-on-failed must be suppressed by live-only policy %v", policy)
		})
	}
}

func (s *ResourcePolicyLiveSuite) TestLiveOnlyPolicySuppressesDeleteOnSucceeded() {
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
			s.Require().False(resInfos[0].MustDeleteOnSuccessfulInstall, "delete-on-succeeded must be suppressed by live-only policy %v", policy)
		})
	}
}

func (s *ResourcePolicyLiveSuite) TestNoLivePolicyStillDeletesChartRemovedResource() {
	s.Run("unannotated live resource is deleted", func() {
		s.createLiveResource(nil)

		localRes := defaultDeletableResource(s.releaseName, s.releaseNamespace)

		resInfo, err := plan.BuildDeletableResourceInfo(context.Background(), localRes, common.DeployTypeUninstall, s.releaseName, s.releaseNamespace, s.clientFactory)
		s.Require().NoError(err)
		s.Require().True(resInfo.MustDelete, "chart-removed resource without any policy must be deleted")
	})
}

func (s *ResourcePolicyLiveSuite) createLiveResource(policyAnnotations map[string]string) {
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

func TestResourcePolicyLiveSuite(t *testing.T) {
	suite.Run(t, new(ResourcePolicyLiveSuite))
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
