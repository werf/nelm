package resource_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/resource"
)

type UnstructSuite struct {
	suite.Suite

	cmpOpts cmp.Options
}

func (s *UnstructSuite) SetupSuite() {
	s.cmpOpts = cmp.Options{
		cmpopts.EquateEmpty(),
	}
}

type cleanUnstructTestCase struct {
	name         string
	skip         bool
	inputFunc    func() (*unstructured.Unstructured, resource.CleanUnstructOptions)
	expectedFunc func() *unstructured.Unstructured
}

func (s *UnstructSuite) TestCleanUnstruct() {
	testCases := []cleanUnstructTestCase{
		{
			name: `should not change anything`,
			inputFunc: func() (*unstructured.Unstructured, resource.CleanUnstructOptions) {
				return defaultUncleanUnstruct(), resource.CleanUnstructOptions{}
			},
			expectedFunc: func() *unstructured.Unstructured {
				return defaultUncleanUnstruct()
			},
		},
		{
			name: `should clean helm.sh annotations`,
			inputFunc: func() (*unstructured.Unstructured, resource.CleanUnstructOptions) {
				return defaultUncleanUnstruct(), resource.CleanUnstructOptions{
					CleanHelmShAnnos: true,
				}
			},
			expectedFunc: func() *unstructured.Unstructured {
				unstruct := defaultUncleanUnstruct()
				unstruct.SetAnnotations(lo.OmitByKeys(unstruct.GetAnnotations(), []string{
					"helm.sh/hook",
					"helm.sh/hook-delete-policy",
				}))
				return unstruct
			},
		},
		{
			name: `should clean werf.io annotations`,
			inputFunc: func() (*unstructured.Unstructured, resource.CleanUnstructOptions) {
				return defaultUncleanUnstruct(), resource.CleanUnstructOptions{
					CleanWerfIoAnnos: true,
				}
			},
			expectedFunc: func() *unstructured.Unstructured {
				unstruct := defaultUncleanUnstruct()
				unstruct.SetAnnotations(lo.OmitByKeys(unstruct.GetAnnotations(), []string{
					"werf.io/weight",
					"werf.io/version",
					"project.werf.io/name",
					"ci.werf.io/commit",
				}))
				return unstruct
			},
		},
		{
			name: `should clean werf.io runtime annotations`,
			inputFunc: func() (*unstructured.Unstructured, resource.CleanUnstructOptions) {
				return defaultUncleanUnstruct(), resource.CleanUnstructOptions{
					CleanWerfIoRuntimeAnnos: true,
				}
			},
			expectedFunc: func() *unstructured.Unstructured {
				unstruct := defaultUncleanUnstruct()
				unstruct.SetAnnotations(lo.OmitByKeys(unstruct.GetAnnotations(), []string{
					"werf.io/version",
					"project.werf.io/name",
					"ci.werf.io/commit",
				}))
				return unstruct
			},
		},
		{
			name: `should clean release annotations and labels`,
			inputFunc: func() (*unstructured.Unstructured, resource.CleanUnstructOptions) {
				return defaultUncleanUnstruct(), resource.CleanUnstructOptions{
					CleanReleaseAnnosLabels: true,
				}
			},
			expectedFunc: func() *unstructured.Unstructured {
				unstruct := defaultUncleanUnstruct()
				unstruct.SetAnnotations(lo.OmitByKeys(unstruct.GetAnnotations(), []string{
					"meta.helm.sh/release-name",
					"meta.helm.sh/release-namespace",
				}))
				unstruct.SetLabels(lo.OmitByKeys(unstruct.GetLabels(), []string{
					"app.kubernetes.io/managed-by",
				}))
				return unstruct
			},
		},
		{
			name: `should clean managed fields`,
			inputFunc: func() (*unstructured.Unstructured, resource.CleanUnstructOptions) {
				return defaultUncleanUnstruct(), resource.CleanUnstructOptions{
					CleanManagedFiles: true,
				}
			},
			expectedFunc: func() *unstructured.Unstructured {
				unstruct := defaultUncleanUnstruct()
				unstruct.SetManagedFields(nil)
				return unstruct
			},
		},
		{
			name: `should clean runtime data`,
			inputFunc: func() (*unstructured.Unstructured, resource.CleanUnstructOptions) {
				return defaultUncleanUnstruct(), resource.CleanUnstructOptions{
					CleanRuntimeData: true,
				}
			},
			expectedFunc: func() *unstructured.Unstructured {
				unstruct := defaultUncleanUnstruct()

				unstruct.SetResourceVersion("")
				unstruct.SetGeneration(0)
				unstruct.SetUID("")
				unstruct.SetCreationTimestamp(v1.Time{})
				unstruct.SetSelfLink("")
				unstruct.SetFinalizers(nil)
				unstruct.Object["status"] = nil

				managedFields := unstruct.GetManagedFields()
				for _, entry := range managedFields {
					entry.Time = nil
				}

				unstruct.SetManagedFields(managedFields)

				return unstruct
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runCleanUnstructTest(tc, s))
	}
}

func TestUnstructSuites(t *testing.T) {
	suite.Run(t, new(UnstructSuite))
}

func defaultUncleanUnstruct() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "test-configmap",
				"annotations": map[string]interface{}{
					"werf.io/weight":                 "1",
					"helm.sh/hook":                   "pre-install",
					"helm.sh/hook-delete-policy":     "before-hook-creation",
					"werf.io/version":                "1.1.1",
					"project.werf.io/name":           "project",
					"ci.werf.io/commit":              "commit",
					"meta.helm.sh/release-name":      "release",
					"meta.helm.sh/release-namespace": "namespace",
					"annotation":                     "value",
				},
				"labels": map[string]interface{}{
					"app.kubernetes.io/managed-by": "helm",
					"label":                        "value",
				},
				"managedFields": []interface{}{
					map[string]interface{}{
						"manager":    "test",
						"operation":  "Update",
						"apiVersion": "v1",
						"time":       "ts",
					},
				},
				"uid":               "111",
				"finalizers":        []interface{}{"finalizer"},
				"resourceVersion":   "1",
				"generation":        int64(1),
				"creationTimestamp": "ts",
				"selfLink":          "link",
			},
			"status": map[string]interface{}{
				"state": "Ready",
			},
		},
	}
}

func runCleanUnstructTest(tc cleanUnstructTestCase, s *UnstructSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		inputUnstruct, opts := tc.inputFunc()
		expectedUnstruct := tc.expectedFunc()
		unstruct := resource.CleanUnstruct(inputUnstruct, opts)

		if !cmp.Equal(expectedUnstruct, unstruct, s.cmpOpts) {
			s.T().Fatalf("unexpected unstruct (-want +got):\n%s", cmp.Diff(expectedUnstruct, unstruct, s.cmpOpts))
		}
	}
}
