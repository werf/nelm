package resource_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
)

type InstallableResourceSuite struct {
	suite.Suite

	releaseNamespace string
	clientFactory    kube.ClientFactorier
	cmpOpts          cmp.Options
}

func (s *InstallableResourceSuite) SetupTest() {
	ctx := context.Background()
	s.releaseNamespace = "test-namespace"
	s.clientFactory = kube.NewFakeClientFactory(ctx)
	s.cmpOpts = cmp.Options{
		cmpopts.EquateEmpty(),
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResource() {
	simplestResSpec, err := spec.NewResourceSpecFromManifest(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
data:
  key: value
`, s.releaseNamespace, spec.ResourceSpecOptions{})
	s.Require().NoError(err)

	simplestControllerResSpec, err := spec.NewResourceSpecFromManifest(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  selector:
`, s.releaseNamespace, spec.ResourceSpecOptions{})
	s.Require().NoError(err)

	simplestClusteredResSpec, err := spec.NewResourceSpecFromManifest(`
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: test-clusterrole
rules:
- verbs: ["get"]
`, s.releaseNamespace, spec.ResourceSpecOptions{})
	s.Require().NoError(err)

	simplestCRDResSpec, err := spec.NewResourceSpecFromManifest(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test-crd.example.com
spec:
  group: example.com
`, s.releaseNamespace, spec.ResourceSpecOptions{})

	simplestHookResSpec, err := spec.NewResourceSpecFromManifest(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
  annotations:
    helm.sh/hook: pre-install
data:
  key: value
`, s.releaseNamespace, spec.ResourceSpecOptions{})
	s.Require().NoError(err)

	simplestHookCRDResSpec, err := spec.NewResourceSpecFromManifest(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test-crd.example.com
  annotations:
    helm.sh/hook: pre-install
spec:
  group: example.com
`, s.releaseNamespace, spec.ResourceSpecOptions{})
	s.Require().NoError(err)

	simplestStandaloneCRDResSpec, err := spec.NewResourceSpecFromManifest(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: test-crd.example.com
spec:
  group: example.com
`, s.releaseNamespace, spec.ResourceSpecOptions{
		StoreAs: common.StoreAsNone,
	})
	s.Require().NoError(err)

	testCases := []struct {
		name    string
		resSpec *spec.ResourceSpec
		expect  *resource.InstallableResource
	}{
		{
			name:    "for simplest resource",
			resSpec: simplestResSpec,
			expect: &resource.InstallableResource{
				ResourceSpec:                    simplestResSpec,
				Ownership:                       common.OwnershipRelease,
				FailMode:                        multitrack.FailWholeDeployProcessImmediately,
				FailuresAllowed:                 0,
				NoActivityTimeout:               4 * time.Minute,
				ShowLogsOnlyForNumberOfReplicas: 1,
				TrackTerminationMode:            multitrack.WaitUntilResourceReady,
				Weight:                          lo.ToPtr(0),
				DeployConditions: map[common.On][]common.Stage{
					common.InstallOnInstall:  {common.StageInstall},
					common.InstallOnUpgrade:  {common.StageInstall},
					common.InstallOnRollback: {common.StageInstall},
				},
			},
		},
		{
			name:    "for simplest controller resource",
			resSpec: simplestControllerResSpec,
			expect: &resource.InstallableResource{
				ResourceSpec:                    simplestControllerResSpec,
				Ownership:                       common.OwnershipRelease,
				FailMode:                        multitrack.FailWholeDeployProcessImmediately,
				FailuresAllowed:                 1,
				NoActivityTimeout:               4 * time.Minute,
				ShowLogsOnlyForNumberOfReplicas: 1,
				TrackTerminationMode:            multitrack.WaitUntilResourceReady,
				Weight:                          lo.ToPtr(0),
				DeployConditions: map[common.On][]common.Stage{
					common.InstallOnInstall:  {common.StageInstall},
					common.InstallOnUpgrade:  {common.StageInstall},
					common.InstallOnRollback: {common.StageInstall},
				},
			},
		},
		{
			name:    "for simplest clustered resource",
			resSpec: simplestClusteredResSpec,
			expect: &resource.InstallableResource{
				ResourceSpec:                    simplestClusteredResSpec,
				Ownership:                       common.OwnershipRelease,
				FailMode:                        multitrack.FailWholeDeployProcessImmediately,
				FailuresAllowed:                 0,
				NoActivityTimeout:               4 * time.Minute,
				ShowLogsOnlyForNumberOfReplicas: 1,
				TrackTerminationMode:            multitrack.WaitUntilResourceReady,
				Weight:                          lo.ToPtr(0),
				DeployConditions: map[common.On][]common.Stage{
					common.InstallOnInstall:  {common.StageInstall},
					common.InstallOnUpgrade:  {common.StageInstall},
					common.InstallOnRollback: {common.StageInstall},
				},
			},
		},
		{
			name:    "for simplest CRD resource",
			resSpec: simplestCRDResSpec,
			expect: &resource.InstallableResource{
				ResourceSpec:                    simplestCRDResSpec,
				Ownership:                       common.OwnershipRelease,
				FailMode:                        multitrack.FailWholeDeployProcessImmediately,
				FailuresAllowed:                 0,
				NoActivityTimeout:               4 * time.Minute,
				ShowLogsOnlyForNumberOfReplicas: 1,
				TrackTerminationMode:            multitrack.WaitUntilResourceReady,
				Weight:                          lo.ToPtr(0),
				DeployConditions: map[common.On][]common.Stage{
					common.InstallOnInstall:  {common.StagePrePreInstall},
					common.InstallOnUpgrade:  {common.StagePrePreInstall},
					common.InstallOnRollback: {common.StagePrePreInstall},
				},
			},
		},
		{
			name:    "for simplest hook resource",
			resSpec: simplestHookResSpec,
			expect: &resource.InstallableResource{
				ResourceSpec:                    simplestHookResSpec,
				Ownership:                       common.OwnershipEveryone,
				Recreate:                        true,
				FailMode:                        multitrack.FailWholeDeployProcessImmediately,
				FailuresAllowed:                 0,
				NoActivityTimeout:               4 * time.Minute,
				ShowLogsOnlyForNumberOfReplicas: 1,
				TrackTerminationMode:            multitrack.WaitUntilResourceReady,
				Weight:                          lo.ToPtr(0),
				DeployConditions: map[common.On][]common.Stage{
					common.InstallOnInstall: {common.StagePreInstall},
				},
			},
		},
		{
			name:    "for simplest hook CRD resource",
			resSpec: simplestHookCRDResSpec,
			expect: &resource.InstallableResource{
				ResourceSpec:                    simplestHookCRDResSpec,
				Ownership:                       common.OwnershipEveryone,
				Recreate:                        true,
				FailMode:                        multitrack.FailWholeDeployProcessImmediately,
				FailuresAllowed:                 0,
				NoActivityTimeout:               4 * time.Minute,
				ShowLogsOnlyForNumberOfReplicas: 1,
				TrackTerminationMode:            multitrack.WaitUntilResourceReady,
				Weight:                          lo.ToPtr(0),
				DeployConditions: map[common.On][]common.Stage{
					common.InstallOnInstall: {common.StagePreInstall},
				},
			},
		},
		{
			name:    "for simplest standalone CRD resource",
			resSpec: simplestStandaloneCRDResSpec,
			expect: &resource.InstallableResource{
				ResourceSpec:                    simplestHookCRDResSpec,
				Ownership:                       common.OwnershipEveryone,
				FailMode:                        multitrack.FailWholeDeployProcessImmediately,
				FailuresAllowed:                 0,
				NoActivityTimeout:               4 * time.Minute,
				ShowLogsOnlyForNumberOfReplicas: 1,
				TrackTerminationMode:            multitrack.WaitUntilResourceReady,
				Weight:                          lo.ToPtr(0),
				DeployConditions: map[common.On][]common.Stage{
					common.InstallOnInstall:  {common.StagePrePreInstall},
					common.InstallOnUpgrade:  {common.StagePrePreInstall},
					common.InstallOnRollback: {common.StagePrePreInstall},
				},
			},
		},
	}

	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			res, err := resource.NewInstallableResource(tc.resSpec, s.releaseNamespace, s.clientFactory, resource.InstallableResourceOptions{})
			s.Require().NoError(err)

			if !cmp.Equal(res, tc.expect, s.cmpOpts) {
				t.Errorf("unexpected installable resource (-want +got):\n%s", cmp.Diff(tc.expect, res, s.cmpOpts))
			}
		})
	}
}

func TestInstallableResourceSuite(t *testing.T) {
	suite.Run(t, new(InstallableResourceSuite))
}
