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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	clientFactory    kube.ClientFactorier
	cmpOpts          cmp.Options
	releaseNamespace string
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

func (s *InstallableResourceSuite) TestNewInstallableResourceForDefaults() {
	testCases := []installableResourceTestCase{
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				return defaultResourceSpec(s.releaseNamespace)
			},
			name: "for simplest resource",
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultDeploymentInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				return defaultDeploymentResourceSpec(s.releaseNamespace)
			},
			name: "for simplest Deployment resource",
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultJobInstallableResource(resSpec)
				res.RecreateOnImmutable = true

				return res
			},
			input: func() *spec.ResourceSpec {
				return defaultJobResourceSpec(s.releaseNamespace)
			},
			name: `for simplest Job resource`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultCRDInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				return defaultCRDResourceSpec(s.releaseNamespace)
			},
			name: "for simplest CRD resource",
		},
		{
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
			input: func() *spec.ResourceSpec {
				resSpec := defaultCRDResourceSpec(s.releaseNamespace)
				resSpec.StoreAs = common.StoreAsNone

				return resSpec
			},
			name: "for standalone CRD resource",
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultReleaseNamespaceInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				return defaultReleaseNamespaceResourceSpec(s.releaseNamespace)
			},
			name: `for release namespace`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForDeletePolicies() {
	testCases := []installableResourceTestCase{
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Recreate = true

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/delete-policy": "before-creation",
				}))

				return resSpec
			},
			name: `for resource with werf.io/delete-policy="before-creation"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Recreate = true
				res.DeleteOnSucceeded = true
				res.DeleteOnFailed = true

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/delete-policy": "before-creation,succeeded,failed",
				}))

				return resSpec
			},
			name: `for resource with werf.io/delete-policy="before-creation,succeeded,failed"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Recreate = true

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/delete-policy": "before-creation",
				}))

				return resSpec
			},
			name: `for hook resource with werf.io/delete-policy="before-creation"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-delete-policy": "before-hook-creation",
				}))

				return resSpec
			},
			name: `for resource with helm.sh/hook-delete-policy="before-hook-creation"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Recreate = true

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-delete-policy": "before-hook-creation",
				}))

				return resSpec
			},
			name: `for hook resource with helm.sh/hook-delete-policy="before-hook-creation"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Recreate = true
				res.DeleteOnSucceeded = true
				res.DeleteOnFailed = true

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-delete-policy": "before-hook-creation,hook-succeeded,hook-failed",
				}))

				return resSpec
			},
			name: `for hook resource with helm.sh/hook-delete-policy="<all possible values>"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Recreate = true

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/delete-policy":      "before-creation",
					"helm.sh/hook-delete-policy": "hook-succeeded",
				}))

				return resSpec
			},
			name: `for hook resource with werf.io/delete-policy="before-creation" and helm.sh/hook-delete-policy="hook-succeeded"`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForDependencies() {
	testCases := []installableResourceTestCase{
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight": "10",
				}))

				return resSpec
			},
			name: `for resource with werf.io/weight="10"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight": "10",
				}))

				return resSpec
			},
			name: `for hook resource with werf.io/weight="10"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-weight": "10",
				}))

				return resSpec
			},
			name: `for resource with helm.sh/hook-weight="10"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook-weight": "10",
				}))

				return resSpec
			},
			name: `for hook resource with helm.sh/hook-weight="10"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight":      "10",
					"helm.sh/hook-weight": "20",
				}))

				return resSpec
			},
			name: `for resource with werf.io/weight="10" and helm.sh/hook-weight="20"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Weight = lo.ToPtr(10)

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight":      "10",
					"helm.sh/hook-weight": "20",
				}))

				return resSpec
			},
			name: `for hook resource with werf.io/weight="10" and helm.sh/hook-weight="20"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultCRDInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultCRDResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/weight":      "10",
					"helm.sh/hook-weight": "20",
				}))

				return resSpec
			},
			name: `for CRD resource with werf.io/weight="10" and helm.sh/hook-weight="20"`,
		},
		{
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
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-dependency-backend": "state=present,name=backend",
				},
				))

				return resSpec
			},
			name: `for resource with werf.io/deploy-dependency-backend="state=present,name=backend"`,
		},
		{
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
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-dependency-backend":  "state=ready,kind=Deployment,group=apps,version=v1,name=backend,namespace=app",
					"werf.io/deploy-dependency-frontend": "state=ready,kind=StatefulSet,group=apps,version=v1,name=frontend,namespace=app",
				}))

				return resSpec
			},
			name: `for resource with werf.io/deploy-dependency-(backend|frontend)="<all possible options>"`,
		},
		{
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
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-dependency-backend": "state=ready,name=backend",
					"werf.io/weight":                    "10",
					"helm.sh/hook-weight":               "20",
				}))

				return resSpec
			},
			name: `for hook resource with werf.io/deploy-dependency-backend="state=ready,name=backend" and werf.io/weight="10" and helm.sh/hook-weight="20"`,
		},
		{
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
			name: `for Deployment resource with auto internal dependency on configmap`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForHooksAndDeployConditions() {
	testCases := []installableResourceTestCase{
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall: {common.StageInstall},
				}

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-on": "install",
				}))

				return resSpec
			},
			name: `for resource with werf.io/deploy-on="install"`,
		},
		{
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
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-on": "pre-install,install,post-install,upgrade,rollback,delete,test",
				}))

				return resSpec
			},
			name: `for resource with werf.io/deploy-on="<all possible values>"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultHookInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				return defaultHookResourceSpec(s.releaseNamespace)
			},
			name: `for resource with helm.sh/hook="pre-install"`,
		},
		{
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
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook": "pre-install,post-install,pre-upgrade,post-upgrade,pre-rollback,post-rollback,pre-delete,post-delete",
				}))

				return resSpec
			},
			name: `for resource with helm.sh/hook="<all possible values>"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall: {common.StagePrePreInstall},
				}

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultCRDResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/hook": "pre-install,post-install",
				}))

				return resSpec
			},
			name: `for CRD resource with helm.sh/hook="pre-install,post-install"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.DeployConditions = map[common.On][]common.Stage{
					common.InstallOnInstall: {common.StageInstall},
				}

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/deploy-on": "install",
				}))

				return resSpec
			},
			name: `for resource with werf.io/deploy-on="install" and helm.sh/hook="pre-install"`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForLogFilters() {
	testCases := []installableResourceTestCase{
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.LogRegex = regexp.MustCompile("^Error:.*")

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/log-regex": "^Error:.*",
				}))

				return resSpec
			},
			name: `for resource with werf.io/log-regex="^Error:.*"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.LogRegexesForContainers = map[string]*regexp.Regexp{
					"backend": regexp.MustCompile("^Error:.*"),
				}

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/log-regex-for-backend": "^Error:.*",
				}))

				return resSpec
			},
			name: `for resource with werf.io/log-regex-for-backend="^Error:.*"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.ShowLogsOnlyForContainers = []string{"backend", "worker"}

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/show-logs-only-for-containers": "backend,worker",
				}))

				return resSpec
			},
			name: `for resource with werf.io/show-logs-only-for-containers="backend,worker"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.ShowServiceMessages = true

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/show-service-messages": "true",
				}))

				return resSpec
			},
			name: `for resource with werf.io/show-service-messages="true"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.ShowLogsOnlyForNumberOfReplicas = 2

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/show-logs-only-for-number-of-replicas": "2",
				}))

				return resSpec
			},
			name: `for resource with werf.io/show-logs-only-for-number-of-replicas="2"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.SkipLogs = true

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/skip-logs": "true",
				}))

				return resSpec
			},
			name: `for resource with werf.io/skip-logs="true"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.SkipLogsForContainers = []string{"backend", "worker"}

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/skip-logs-for-containers": "backend,worker",
				}))

				return resSpec
			},
			name: `for resource with werf.io/skip-logs-for-containers="backend,worker"`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForOwnership() {
	testCases := []installableResourceTestCase{
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.Ownership = common.OwnershipAnyone

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "anyone",
				}))

				return resSpec
			},
			name: `for resource with werf.io/ownership="anyone"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultHookInstallableResource(resSpec)
				res.Ownership = common.OwnershipRelease

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "release",
				}))

				return resSpec
			},
			name: `for hook resource with werf.io/ownership="release"`,
		},
		{
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
			input: func() *spec.ResourceSpec {
				resSpec := defaultCRDResourceSpec(s.releaseNamespace)
				resSpec.StoreAs = common.StoreAsNone
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "release",
				}))

				return resSpec
			},
			name: `for standalone CRD with werf.io/ownership="release"`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForResourcePolicies() {
	testCases := []installableResourceTestCase{
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.KeepOnDelete = true

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"helm.sh/resource-policy": "keep",
				}))

				return resSpec
			},
			name: `for resource with helm.sh/resource-policy="keep"`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

func (s *InstallableResourceSuite) TestNewInstallableResourceForTracking() {
	testCases := []installableResourceTestCase{
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/fail-mode": "FailWholeDeployProcessImmediately",
				}))

				return resSpec
			},
			name: `for resource with werf.io/fail-mode="FailWholeDeployProcessImmediately"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.FailMode = multitrack.IgnoreAndContinueDeployProcess

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/fail-mode": "IgnoreAndContinueDeployProcess",
				}))

				return resSpec
			},
			name: `for resource with werf.io/fail-mode="IgnoreAndContinueDeployProcess"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultDeploymentInstallableResource(resSpec)
				res.FailuresAllowed = 0

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultDeploymentResourceSpec(s.releaseNamespace)
				err := unstructured.SetNestedField(resSpec.Unstruct.UnstructuredContent(), string(corev1.RestartPolicyNever), "spec", "template", "spec", "restartPolicy")
				s.Require().NoError(err)

				return resSpec
			},
			name: `for Deployment resource with Pod restartPolicy: "Never"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.FailuresAllowed = 100

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/failures-allowed-per-replica": "100",
				}))

				return resSpec
			},
			name: `for resource with werf.io/failures-allowed="100"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultDeploymentInstallableResource(resSpec)
				res.FailuresAllowed = 100

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultDeploymentResourceSpec(s.releaseNamespace)
				err := unstructured.SetNestedField(resSpec.Unstruct.UnstructuredContent(), int64(10), "spec", "replicas")
				s.Require().NoError(err)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/failures-allowed-per-replica": "10",
				}))

				return resSpec
			},
			name: `for Deployment resource with 10 replicas and werf.io/failures-allowed-per-replica="10"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.NoActivityTimeout = 100 * time.Minute

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/no-activity-timeout": "100m",
				}))

				return resSpec
			},
			name: `for resource with werf.io/no-activity-timeout="100m"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				return defaultInstallableResource(resSpec)
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/track-termination-mode": "WaitUntilResourceReady",
				}))

				return resSpec
			},
			name: `for resource with werf.io/track-termination-mode="WaitUntilResourceReady"`,
		},
		{
			expect: func(resSpec *spec.ResourceSpec) *resource.InstallableResource {
				res := defaultInstallableResource(resSpec)
				res.TrackTerminationMode = multitrack.NonBlocking

				return res
			},
			input: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/track-termination-mode": "NonBlocking",
				}))

				return resSpec
			},
			name: `for resource with werf.io/track-termination-mode="NonBlocking"`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runInstallableResourceTest(tc, s))
	}
}

type installableResourceTestCase struct {
	expect func(resSpec *spec.ResourceSpec) *resource.InstallableResource
	input  func() *spec.ResourceSpec
	name   string
	skip   bool
}

type DeletableResourceSuite struct {
	suite.Suite

	cmpOpts          cmp.Options
	releaseNamespace string
}

func (s *DeletableResourceSuite) SetupSuite() {
	s.releaseNamespace = "test-namespace"
	s.cmpOpts = cmp.Options{
		cmpopts.EquateEmpty(),
	}
}

func (s *DeletableResourceSuite) TestNewDeletableResourceForDefaults() {
	testCases := []deletableResourceTestCase{
		{
			expectFunc: func(resSpec *spec.ResourceSpec) *resource.DeletableResource {
				return defaultDeletableResource(resSpec.ResourceMeta)
			},
			inputFunc: func() *spec.ResourceSpec {
				return defaultResourceSpec(s.releaseNamespace)
			},
			name: "for simplest resource",
		},
		{
			expectFunc: func(resSpec *spec.ResourceSpec) *resource.DeletableResource {
				return defaultReleaseNamespaceDeletableResource(resSpec)
			},
			inputFunc: func() *spec.ResourceSpec {
				return defaultReleaseNamespaceResourceSpec(s.releaseNamespace)
			},
			name: `for release namespace`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runDeletableResourceTest(tc, s))
	}
}

func (s *DeletableResourceSuite) TestNewDeletableResourceForOwnership() {
	testCases := []deletableResourceTestCase{
		{
			expectFunc: func(resSpec *spec.ResourceSpec) *resource.DeletableResource {
				res := defaultDeletableResource(resSpec.ResourceMeta)
				res.Ownership = common.OwnershipAnyone

				return res
			},
			inputFunc: func() *spec.ResourceSpec {
				resSpec := defaultResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "anyone",
				}))

				return resSpec
			},
			name: `for resource with werf.io/ownership="anyone"`,
		},
		{
			expectFunc: func(resSpec *spec.ResourceSpec) *resource.DeletableResource {
				res := defaultHookDeletableResource(resSpec)
				res.Ownership = common.OwnershipRelease

				return res
			},
			inputFunc: func() *spec.ResourceSpec {
				resSpec := defaultHookResourceSpec(s.releaseNamespace)
				resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
					"werf.io/ownership": "release",
				}))

				return resSpec
			},
			name: `for hook resource with werf.io/ownership="release"`,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runDeletableResourceTest(tc, s))
	}
}

type deletableResourceTestCase struct {
	expectFunc func(resSpec *spec.ResourceSpec) *resource.DeletableResource
	inputFunc  func() *spec.ResourceSpec
	name       string
	skip       bool
}

func TestResourceSuites(t *testing.T) {
	suite.Run(t, new(InstallableResourceSuite))
	suite.Run(t, new(DeletableResourceSuite))
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

func defaultDeploymentInstallableResource(resSpec *spec.ResourceSpec) *resource.InstallableResource {
	res := defaultInstallableResource(resSpec)
	res.FailuresAllowed = 1

	return res
}

func defaultHookDeletableResource(resSpec *spec.ResourceSpec) *resource.DeletableResource {
	res := defaultDeletableResource(resSpec.ResourceMeta)
	res.Ownership = common.OwnershipAnyone

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

func defaultHookResourceSpec(releaseNamespace string) *spec.ResourceSpec {
	resSpec := defaultResourceSpec(releaseNamespace)
	resSpec.SetAnnotations(lo.Assign(resSpec.Annotations, map[string]string{
		"helm.sh/hook": "pre-install",
	}))

	return resSpec
}

func defaultJobInstallableResource(resSpec *spec.ResourceSpec) *resource.InstallableResource {
	res := defaultInstallableResource(resSpec)

	return res
}

func defaultReleaseNamespaceDeletableResource(resSpec *spec.ResourceSpec) *resource.DeletableResource {
	res := defaultDeletableResource(resSpec.ResourceMeta)
	res.Ownership = common.OwnershipAnyone
	res.KeepOnDelete = true

	return res
}

func defaultReleaseNamespaceInstallableResource(resSpec *spec.ResourceSpec) *resource.InstallableResource {
	res := defaultInstallableResource(resSpec)
	res.Ownership = common.OwnershipAnyone
	res.KeepOnDelete = true

	return res
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

func defaultDeletableResource(resMeta *spec.ResourceMeta) *resource.DeletableResource {
	return &resource.DeletableResource{
		ResourceMeta:      resMeta,
		Ownership:         common.OwnershipRelease,
		DeletePropagation: metav1.DeletePropagationForeground,
	}
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
		DeletePropagation: metav1.DeletePropagationForeground,
	}
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

func runDeletableResourceTest(tc deletableResourceTestCase, s *DeletableResourceSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		resSpec := tc.inputFunc()

		res := resource.NewDeletableResource(resSpec, []*spec.ResourceSpec{}, s.releaseNamespace, resource.DeletableResourceOptions{})

		expectRes := tc.expectFunc(resSpec)

		if !cmp.Equal(expectRes, res, s.cmpOpts) {
			s.T().Fatalf("unexpected deletable resource (-want +got):\n%s", cmp.Diff(expectRes, res, s.cmpOpts...))
		}
	}
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
