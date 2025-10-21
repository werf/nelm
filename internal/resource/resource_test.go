package resource_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/kube/fake"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/test"
	"github.com/werf/nelm/pkg/common"
)

type InstallableResourceSuite struct {
	suite.Suite

	releaseNamespace string
	clientFactory    kube.ClientFactorier
	cmpOpts          cmp.Options
}

func (s *InstallableResourceSuite) SetupSuite() {
	ctx := context.Background()

	s.releaseNamespace = "test-namespace"
	s.cmpOpts = cmp.Options{
		cmpopts.EquateEmpty(),
		test.CompareRegexpOption(),
		test.CompareInternalDependencyOption(),
	}

	var err error

	s.clientFactory, err = fake.NewClientFactory(ctx)
	s.Require().NoError(err)
}

type installableResourceTestCase struct {
	name   string
	skip   bool
	input  func() *spec.ResourceSpec
	expect func(resSpec *spec.ResourceSpec) *resource.InstallableResource
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForDefaults() {
	testCases := []installableResourceTestCase{
		{
			name: "for simplest resource",
			input: func() *spec.ResourceSpec {
				return defaultResourceSpec(s.releaseNamespace)
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
		},
		{
			name: "for simplest Deployment resource",
			input: func() *spec.ResourceSpec {
				return defaultDeploymentResourceSpec(s.releaseNamespace)
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultDeploymentInstallableResource(resSpec)
			},
		},
		{
			name: `for simplest Job resource`,
			input: func() *spec.ResourceSpec {
				return defaultJobResourceSpec(s.releaseNamespace)
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultJobInstallableResource(resSpec)
				res.RecreateOnImmutable = true

				return res
			},
		},
		{
			name: "for simplest CRD resource",
			input: func() *spec.ResourceSpec {
				return defaultCRDResourceSpec(s.releaseNamespace)
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultCRDInstallableResource(resSpec)
			},
		},
		{
			name: "for standalone CRD resource",
			input: func() *spec.ResourceSpec {
				resSpec := defaultCRDResourceSpec(s.releaseNamespace)
				resSpec.StoreAs = common.StoreAsNone

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultCRDInstallableResource(resSpec)
				res.Ownership = common.OwnershipAnyone
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall:  {common.StagePrePreInstall},
					common.InstallOnUpgrade:  {common.StagePrePreInstall},
					common.InstallOnRollback: {common.StagePrePreInstall},
				}

				return res
			},
		},
		{
			name: `for release namespace`,
			input: func() *spec.ResourceSpec {
				return defaultReleaseNamespaceResourceSpec(s.releaseNamespace)
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultReleaseNamespaceInstallableResource(resSpec)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForHooksAndDeployConditions() {
	testCases := []installableResourceTestCase{
		{
			name: `for resource with werf.io/deploy-on="install"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-on": "install",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall: {common.StageInstall},
				}

				return res
			},
		},
		{
			name: `for resource with werf.io/deploy-on="<all possible values>"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-on": "pre-install,install,post-install,upgrade,rollback,delete,test",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall:  {common.StagePreInstall, common.StageInstall, common.StagePostInstall},
					common.InstallOnUpgrade:  {common.StageInstall},
					common.InstallOnRollback: {common.StageInstall},
					common.InstallOnDelete:   {common.StageInstall},
					common.InstallOnTest:     {common.StageInstall},
				}

				return res
			},
		},
		{
			name: `for resource with helm.sh/hook="pre-install"`,
			input: func() *spec.ResourceSpec {
				return defaultHookResourceSpec(s.releaseNamespace)
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultHookInstallableResource(resSpec)
			},
		},
		{
			name: `for resource with helm.sh/hook="<all possible values>"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook": "pre-install,post-install,pre-upgrade,post-upgrade,pre-rollback,post-rollback,pre-delete,post-delete",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall:  {common.StagePreInstall, common.StagePostInstall},
					common.InstallOnUpgrade:  {common.StagePreInstall, common.StagePostInstall},
					common.InstallOnRollback: {common.StagePreInstall, common.StagePostInstall},
					common.InstallOnDelete:   {common.StagePreInstall, common.StagePostInstall},
				}

				return res
			},
		},
		{
			name: `for CRD resource with helm.sh/hook="pre-install,post-install"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultCRDResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook": "pre-install,post-install",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall: {common.StagePrePreInstall},
				}

				return res
			},
		},
		{
			name: `for resource with werf.io/deploy-on="install" and helm.sh/hook="pre-install"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-on": "install",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall: {common.StageInstall},
				}

				return res
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForOwnership() {
	testCases := []installableResourceTestCase{
		{
			name: `for resource with werf.io/ownership="anyone"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "anyone",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Ownership = common.OwnershipAnyone

				return res
			},
		},
		{
			name: `for hook resource with werf.io/ownership="release"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "release",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Ownership = common.OwnershipRelease

				return res
			},
		},
		{
			name: `for standalone CRD with werf.io/ownership="release"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultCRDResourceSpec(s.releaseNamespace)
				resSpec.StoreAs = common.StoreAsNone
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "release",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultCRDInstallableResource(resSpec)
				res.Ownership = common.OwnershipAnyone
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall:  {common.StagePrePreInstall},
					common.InstallOnUpgrade:  {common.StagePrePreInstall},
					common.InstallOnRollback: {common.StagePrePreInstall},
				}

				return res
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForDependencies() {
	testCases := []installableResourceTestCase{
		{
			name: `for resource with werf.io/weight="10"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight": "10",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
		},
		{
			name: `for hook resource with werf.io/weight="10"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight": "10",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
		},
		{
			name: `for resource with helm.sh/hook-weight="10"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-weight": "10",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
		},
		{
			name: `for hook resource with helm.sh/hook-weight="10"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-weight": "10",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
		},
		{
			name: `for resource with werf.io/weight="10" and helm.sh/hook-weight="20"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight":      "10",
					"helm.sh/hook-weight": "20",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
		},
		{
			name: `for hook resource with werf.io/weight="10" and helm.sh/hook-weight="20"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight":      "10",
					"helm.sh/hook-weight": "20",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
		},
		{
			name: `for CRD resource with werf.io/weight="10" and helm.sh/hook-weight="20"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultCRDResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight":      "10",
					"helm.sh/hook-weight": "20",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultCRDInstallableResource(resSpec)
			},
		},
		{
			name: `for resource with werf.io/deploy-dependency-backend="state=present,name=backend"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-dependency-backend": "state=present,name=backend",
				},
				))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.ManualInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Names: []string{"backend"},
						},
						ResourceState: common.ResourceStatePresent,
					},
				}
				res.Weight = nil

				return res
			},
		},
		{
			name: `for resource with werf.io/deploy-dependency-(backend|frontend)="<all possible options>"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-dependency-backend":  "state=ready,kind=Deployment,group=apps,version=v1,name=backend,namespace=app",
					"werf.io/deploy-dependency-frontend": "state=ready,kind=StatefulSet,group=apps,version=v1,name=frontend,namespace=app",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.ManualInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Names:      []string{"backend"},
							Kinds:      []string{"Deployment"},
							Groups:     []string{"apps"},
							Versions:   []string{"v1"},
							Namespaces: []string{"app"},
						},
						ResourceState: common.ResourceStateReady,
					},
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Names:      []string{"frontend"},
							Kinds:      []string{"StatefulSet"},
							Groups:     []string{"apps"},
							Versions:   []string{"v1"},
							Namespaces: []string{"app"},
						},
						ResourceState: common.ResourceStateReady,
					},
				}
				res.Weight = nil

				return res
			},
		},
		{
			name: `for hook resource with werf.io/deploy-dependency-backend="state=ready,name=backend" and werf.io/weight="10" and helm.sh/hook-weight="20"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-dependency-backend": "state=ready,name=backend",
					"werf.io/weight":                    "10",
					"helm.sh/hook-weight":               "20",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.ManualInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Names: []string{"backend"},
						},
						ResourceState: common.ResourceStateReady,
					},
				}
				res.Weight = nil

				return res
			},
		},
		{
			name: `for Deployment resource with auto internal dependency on configmap`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultDeploymentResourceSpec(s.releaseNamespace)
				containers := []interface{}{
					map[string]interface{}{
						"env": []interface{}{
							map[string]interface{}{
								"name": "MY_ENV",
								"valueFrom": map[string]interface{}{
									"configMapKeyRef": map[string]interface{}{
										"name": "configmap-envs",
										"key":  "MY_ENV",
									},
								},
							},
						},
					},
				}
				err := unstructured.SetNestedSlice(resSpec.Unstruct.UnstructuredContent(), containers, "spec", "template", "spec", "containers")
				s.Require().NoError(err)

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultDeploymentInstallableResource(resSpec)
				res.AutoInternalDependencies = []*resource.InternalDependency{
					{
						ResourceMatcher: &spec.ResourceMatcher{
							Names:      []string{"configmap-envs"},
							Kinds:      []string{"ConfigMap"},
							Groups:     []string{""},
							Namespaces: []string{""},
						},
						ResourceState: common.ResourceStatePresent,
					},
				}

				return res
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForDeletePolicies() {
	testCases := []installableResourceTestCase{
		{
			name: `for resource with werf.io/delete-policy="before-creation"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/delete-policy": "before-creation",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Recreate = true

				return res
			},
		},
		{
			name: `for resource with werf.io/delete-policy="before-creation,succeeded,failed"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/delete-policy": "before-creation,succeeded,failed",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Recreate = true
				res.DeleteOnSucceeded = true
				res.DeleteOnFailed = true

				return res
			},
		},
		{
			name: `for hook resource with werf.io/delete-policy="before-creation"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/delete-policy": "before-creation",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Recreate = true

				return res
			},
		},
		{
			name: `for resource with helm.sh/hook-delete-policy="before-hook-creation"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-delete-policy": "before-hook-creation",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
		},
		{
			name: `for hook resource with helm.sh/hook-delete-policy="before-hook-creation"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-delete-policy": "before-hook-creation",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Recreate = true

				return res
			},
		},
		{
			name: `for hook resource with helm.sh/hook-delete-policy="<all possible values>"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-delete-policy": "before-hook-creation,hook-succeeded,hook-failed",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Recreate = true
				res.DeleteOnSucceeded = true
				res.DeleteOnFailed = true

				return res
			},
		},
		{
			name: `for hook resource with werf.io/delete-policy="before-creation" and helm.sh/hook-delete-policy="hook-succeeded"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/delete-policy":      "before-creation",
					"helm.sh/hook-delete-policy": "hook-succeeded",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Recreate = true

				return res
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForResourcePolicies() {
	testCases := []installableResourceTestCase{
		{
			name: `for resource with helm.sh/resource-policy="keep"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/resource-policy": "keep",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.KeepOnDelete = true

				return res
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForTracking() {
	testCases := []installableResourceTestCase{
		{
			name: `for resource with werf.io/fail-mode="FailWholeDeployProcessImmediately"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/fail-mode": "FailWholeDeployProcessImmediately",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
		},
		{
			name: `for resource with werf.io/fail-mode="IgnoreAndContinueDeployProcess"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/fail-mode": "IgnoreAndContinueDeployProcess",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.FailMode = multitrack.IgnoreAndContinueDeployProcess

				return res
			},
		},
		{
			name: `for Deployment resource with Pod restartPolicy: "Never"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultDeploymentResourceSpec(s.releaseNamespace)
				err := unstructured.SetNestedField(resSpec.Unstruct.UnstructuredContent(), string(corev1.RestartPolicyNever), "spec", "template", "spec", "restartPolicy")
				s.Require().NoError(err)

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultDeploymentInstallableResource(resSpec)
				res.FailuresAllowed = 0

				return res
			},
		},
		{
			name: `for resource with werf.io/failures-allowed="100"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/failures-allowed-per-replica": "100",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.FailuresAllowed = 100

				return res
			},
		},
		{
			name: `for Deployment resource with 10 replicas and werf.io/failures-allowed-per-replica="10"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultDeploymentResourceSpec(s.releaseNamespace)
				err := unstructured.SetNestedField(resSpec.Unstruct.UnstructuredContent(), int64(10), "spec", "replicas")
				s.Require().NoError(err)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/failures-allowed-per-replica": "10",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultDeploymentInstallableResource(resSpec)
				res.FailuresAllowed = 100

				return res
			},
		},
		{
			name: `for resource with werf.io/no-activity-timeout="100m"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/no-activity-timeout": "100m",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.NoActivityTimeout = 100 * time.Minute

				return res
			},
		},
		{
			name: `for resource with werf.io/track-termination-mode="WaitUntilResourceReady"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/track-termination-mode": "WaitUntilResourceReady",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
		},
		{
			name: `for resource with werf.io/track-termination-mode="NonBlocking"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/track-termination-mode": "NonBlocking",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.TrackTerminationMode = multitrack.NonBlocking

				return res
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForLogFilters() {
	testCases := []installableResourceTestCase{
		{
			name: `for resource with werf.io/log-regex="^Error:.*"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/log-regex": "^Error:.*",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.LogRegex = regexp.MustCompile("^Error:.*")

				return res
			},
		},
		{
			name: `for resource with werf.io/log-regex-for-backend="^Error:.*"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/log-regex-for-backend": "^Error:.*",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.LogRegexesForContainers = map[string]*regexp.Regexp{
					"backend": regexp.MustCompile("^Error:.*"),
				}

				return res
			},
		},
		{
			name: `for resource with werf.io/show-logs-only-for-containers="backend,worker"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/show-logs-only-for-containers": "backend,worker",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.ShowLogsOnlyForContainers = []string{"backend", "worker"}

				return res
			},
		},
		{
			name: `for resource with werf.io/show-service-messages="true"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/show-service-messages": "true",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.ShowServiceMessages = true

				return res
			},
		},
		{
			name: `for resource with werf.io/show-logs-only-for-number-of-replicas="2"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/show-logs-only-for-number-of-replicas": "2",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.ShowLogsOnlyForNumberOfReplicas = 2

				return res
			},
		},
		{
			name: `for resource with werf.io/skip-logs="true"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/skip-logs": "true",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.SkipLogs = true

				return res
			},
		},
		{
			name: `for resource with werf.io/skip-logs-for-containers="backend,worker"`,
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/skip-logs-for-containers": "backend,worker",
				}))

				return resSpec
			},
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.SkipLogsForContainers = []string{"backend", "worker"}

				return res
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

type DeletableResourceSuite struct {
	suite.Suite

	releaseNamespace string
	cmpOpts          cmp.Options
}

func (s *DeletableResourceSuite) SetupSuite() {
	s.releaseNamespace = "test-namespace"
	s.cmpOpts = cmp.Options{
		cmpopts.EquateEmpty(),
	}
}

type deletableResourceTestCase struct {
	name       string
	skip       bool
	inputFunc  func() *spec.ResourceSpec
	expectFunc func(resSpec *spec.ResourceSpec) *resource.DeletableResource
}

func (s *DeletableResourceSuite) TestNewDeletableResourceForDefaults() {
	testCases := []deletableResourceTestCase{
		{
			name: "for simplest resource",
			inputFunc: func() *spec.ResourceSpec {
				return defaultResourceSpec(s.releaseNamespace)
			},
			expectFunc: func(resSpec *spec.ResourceSpec) *resource.DeletableResource {
				return defaultDeletableResource(resSpec.ResourceMeta)
			},
		},
		{
			name: `for release namespace`,
			inputFunc: func() *spec.ResourceSpec {
				return defaultReleaseNamespaceResourceSpec(s.releaseNamespace)
			},
			expectFunc: func(resSpec *spec.ResourceSpec) *resource.DeletableResource {
				return defaultReleaseNamespaceDeletableResource(resSpec)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runDeletableResourceTest(tc, s))
	}
}

func (s *DeletableResourceSuite) TestNewDeletableResourceForOwnership() {
	testCases := []deletableResourceTestCase{
		{
			name: `for resource with werf.io/ownership="anyone"`,
			inputFunc: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "anyone",
				}))

				return resSpec
			},
			expectFunc: func(resSpec *spec.ResourceSpec) *resource.DeletableResource {
				res := defaultDeletableResource(resSpec.ResourceMeta)
				res.Ownership = common.OwnershipAnyone

				return res
			},
		},
		{
			name: `for hook resource with werf.io/ownership="release"`,
			inputFunc: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "release",
				}))

				return resSpec
			},
			expectFunc: func(resSpec *spec.ResourceSpec) *resource.DeletableResource {
				res := defaultHookDeletableResource(resSpec)
				res.Ownership = common.OwnershipRelease

				return res
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runDeletableResourceTest(tc, s))
	}
}

func TestResourceSuites(t *testing.T) {
	suite.Run(t, new(InstallableResourceSuite))
	suite.Run(t, new(DeletableResourceSuite))
}

func defaultResourceSpec(releaseNamespace string) *spec.ResourceSpec {
	return spec.NewResourceSpec(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "test-configmap",
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}, releaseNamespace, spec.ResourceSpecOptions{})
}

func defaultDeploymentResourceSpec(releaseNamespace string) *spec.ResourceSpec {
	return spec.NewResourceSpec(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]interface{}{
				"name": "test-deployment",
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{},
			},
		},
	}, releaseNamespace, spec.ResourceSpecOptions{})
}

func defaultJobResourceSpec(releaseNamespace string) *spec.ResourceSpec {
	return spec.NewResourceSpec(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name": "test-job",
			},
			"spec": map[string]interface{}{},
		},
	}, releaseNamespace, spec.ResourceSpecOptions{})
}

func defaultCRDResourceSpec(releaseNamespace string) *spec.ResourceSpec {
	return spec.NewResourceSpec(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]interface{}{
				"name": "test-crd.example.com",
			},
			"spec": map[string]interface{}{
				"group": "example.com",
			},
		},
	}, releaseNamespace, spec.ResourceSpecOptions{})
}

func defaultReleaseNamespaceResourceSpec(releaseNamespace string) *spec.ResourceSpec {
	return spec.NewResourceSpec(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": releaseNamespace,
			},
		},
	}, releaseNamespace, spec.ResourceSpecOptions{})
}

func defaultHookResourceSpec(releaseNamespace string) *spec.ResourceSpec {
	resSpec := defaultResourceSpec(releaseNamespace)
	resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
		"helm.sh/hook": "pre-install",
	}))

	return resSpec
}

func defaultInstallableResource(resSpec *spec.ResourceSpec) *resource.InstallableResource {
	return &resource.InstallableResource{
		ResourceSpec:                    resSpec,
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

func defaultJobInstallableResource(resSpec *spec.ResourceSpec) *resource.InstallableResource {
	res := defaultInstallableResource(resSpec)
	return res
}

func defaultDeploymentInstallableResource(resSpec *spec.ResourceSpec) *resource.InstallableResource {
	res := defaultInstallableResource(resSpec)
	res.FailuresAllowed = 1

	return res
}

func defaultCRDInstallableResource(resSpec *spec.ResourceSpec) *resource.InstallableResource {
	res := defaultInstallableResource(resSpec)
	res.DeployConditions = map[common.On][]common.Stage{
		common.InstallOnInstall:  {common.StagePrePreInstall},
		common.InstallOnUpgrade:  {common.StagePrePreInstall},
		common.InstallOnRollback: {common.StagePrePreInstall},
	}

	return res
}

func defaultReleaseNamespaceInstallableResource(resSpec *spec.ResourceSpec) *resource.InstallableResource {
	res := defaultInstallableResource(resSpec)
	res.Ownership = common.OwnershipAnyone
	res.KeepOnDelete = true

	return res
}

func defaultHookInstallableResource(resSpec *spec.ResourceSpec) *resource.InstallableResource {
	res := defaultInstallableResource(resSpec)
	res.Ownership = common.OwnershipAnyone
	res.Recreate = true
	res.DeployConditions = map[common.On][]common.Stage{
		common.InstallOnInstall: {common.StagePreInstall},
	}

	return res
}

func defaultDeletableResource(resMeta *spec.ResourceMeta) *resource.DeletableResource {
	return &resource.DeletableResource{
		ResourceMeta: resMeta,
		Ownership:    common.OwnershipRelease,
	}
}

func defaultHookDeletableResource(resSpec *spec.ResourceSpec) *resource.DeletableResource {
	res := defaultDeletableResource(resSpec.ResourceMeta)
	res.Ownership = common.OwnershipAnyone

	return res
}

func defaultReleaseNamespaceDeletableResource(resSpec *spec.ResourceSpec) *resource.DeletableResource {
	res := defaultDeletableResource(resSpec.ResourceMeta)
	res.Ownership = common.OwnershipAnyone
	res.KeepOnDelete = true

	return res
}

func runInstallableResourceTest(tc installableResourceTestCase, s *InstallableResourceSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		resSpec := tc.input()

		res, err := resource.NewInstallableResource(resSpec, s.releaseNamespace, s.clientFactory, resource.InstallableResourceOptions{})
		s.Require().NoError(err)

		expectRes := tc.expect(resSpec)

		if !cmp.Equal(expectRes, res, s.cmpOpts) {
			s.T().Fatalf("unexpected installable resource (-want +got):\n%s", cmp.Diff(expectRes, res, s.cmpOpts...))
		}
	}
}

func runDeletableResourceTest(tc deletableResourceTestCase, s *DeletableResourceSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		resSpec := tc.inputFunc()

		res := resource.NewDeletableResource(resSpec, s.releaseNamespace, resource.DeletableResourceOptions{})

		expectRes := tc.expectFunc(resSpec)

		if !cmp.Equal(expectRes, res, s.cmpOpts) {
			s.T().Fatalf("unexpected deletable resource (-want +got):\n%s", cmp.Diff(expectRes, res, s.cmpOpts...))
		}
	}
}
