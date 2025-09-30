package kube_test

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource/spec"
)

type KubeClientSuite struct {
	suite.Suite

	releaseNamespace string
	factory          kube.ClientFactorier
	cmpOpts          cmp.Options
}

func (s *KubeClientSuite) SetupSuite() {
	s.releaseNamespace = "test-namespace"
	s.cmpOpts = cmp.Options{
		cmpopts.EquateEmpty(),
	}
}

func (s *KubeClientSuite) SetupSubTest() {
	ctx := context.Background()
	s.factory = kube.NewFakeClientFactory(ctx)
}

type kubeClientGetTestCase struct {
	name         string
	skip         bool
	setupFunc    func()
	inputFunc    func() (*spec.ResourceMeta, kube.KubeClientGetOptions)
	expectedFunc func() *unstructured.Unstructured
}

func (s *KubeClientSuite) TestKubeClientGet() {
	testCases := []kubeClientGetTestCase{
		{
			name: `Get resource`,
			setupFunc: func() {
				_, err := s.factory.Dynamic().Resource(schema.GroupVersionResource{
					Version:  "v1",
					Resource: "configmaps",
				}).Namespace(s.releaseNamespace).Create(context.Background(), defaultUnstruct(s), v1.CreateOptions{})
				s.Require().NoError(err)
			},
			inputFunc: func() (*spec.ResourceMeta, kube.KubeClientGetOptions) {
				return spec.NewResourceMetaFromUnstructured(defaultUnstruct(s), s.releaseNamespace, ""), kube.KubeClientGetOptions{
					DefaultNamespace: s.releaseNamespace,
				}
			},
			expectedFunc: func() *unstructured.Unstructured {
				return defaultUnstruct(s)
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runKubeClientGetTest(tc, s))
	}
}

type kubeClientCreateTestCase struct {
	name         string
	skip         bool
	setupFunc    func()
	inputFunc    func() (*spec.ResourceSpec, kube.KubeClientCreateOptions)
	expectedFunc func() *unstructured.Unstructured
}

func (s *KubeClientSuite) TestKubeClientCreate() {
	testCases := []kubeClientCreateTestCase{
		{
			name: `Create already present resource`,
			setupFunc: func() {
				_, err := s.factory.Dynamic().Resource(schema.GroupVersionResource{
					Version:  "v1",
					Resource: "configmaps",
				}).Namespace(s.releaseNamespace).Create(context.Background(), defaultUnstruct(s), v1.CreateOptions{})
				s.Require().NoError(err)
			},
			inputFunc: func() (*spec.ResourceSpec, kube.KubeClientCreateOptions) {
				unstruct := defaultUnstruct(s)

				unstruct.SetAnnotations(lo.Assign(unstruct.GetAnnotations(), map[string]string{
					"anno": "value",
				}))

				return spec.NewResourceSpec(unstruct, s.releaseNamespace, spec.ResourceSpecOptions{}), kube.KubeClientCreateOptions{
					DefaultNamespace: s.releaseNamespace,
				}
			},
			expectedFunc: func() *unstructured.Unstructured {
				unstruct := defaultUnstruct(s)

				unstruct.SetAnnotations(lo.Assign(unstruct.GetAnnotations(), map[string]string{
					"anno": "value",
				}))

				return unstruct
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, runKubeClientCreateTest(tc, s))
	}
}

func defaultUnstruct(s *KubeClientSuite) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-configmap",
				"namespace": s.releaseNamespace,
			},
		},
	}
}

func TestKubeClientSuites(t *testing.T) {
	suite.Run(t, new(KubeClientSuite))
}

func runKubeClientGetTest(tc kubeClientGetTestCase, s *KubeClientSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		if tc.setupFunc != nil {
			tc.setupFunc()
		}

		resMeta, opts := tc.inputFunc()

		unstruct, err := s.factory.KubeClient().Get(context.Background(), resMeta, opts)
		s.Require().NoError(err)

		expectedUnstruct := tc.expectedFunc()

		if !cmp.Equal(expectedUnstruct, unstruct, s.cmpOpts) {
			s.T().Fatalf("unexpected unstructured (-want +got):\n%s", cmp.Diff(expectedUnstruct, unstruct, s.cmpOpts))
		}
	}
}

func runKubeClientCreateTest(tc kubeClientCreateTestCase, s *KubeClientSuite) func() {
	return func() {
		if tc.skip {
			s.T().Skip()
		}

		if tc.setupFunc != nil {
			tc.setupFunc()
		}

		resSpec, opts := tc.inputFunc()

		unstruct, err := s.factory.KubeClient().Create(context.Background(), resSpec, opts)
		s.Require().NoError(err)

		expectedUnstruct := tc.expectedFunc()

		if !cmp.Equal(expectedUnstruct, unstruct, s.cmpOpts) {
			s.T().Fatalf("unexpected unstructured (-want +got):\n%s", cmp.Diff(expectedUnstruct, unstruct, s.cmpOpts))
		}
	}
}
