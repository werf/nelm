package kube

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discfake "k8s.io/client-go/discovery/fake"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
	ktesting "k8s.io/client-go/testing"

	"github.com/werf/nelm/v2/pkg/resource/spec"
)

type testCachedDiscoveryClient struct {
	*discfake.FakeDiscovery

	invalidateCount int
}

func newTestCachedDiscoveryClient() *testCachedDiscoveryClient {
	return &testCachedDiscoveryClient{
		FakeDiscovery: &discfake.FakeDiscovery{Fake: &ktesting.Fake{}},
	}
}

func (c *testCachedDiscoveryClient) Fresh() bool {
	return true
}

func (c *testCachedDiscoveryClient) Invalidate() {
	c.invalidateCount++
}

type testResettableMapper struct {
	err                error
	mappingCalls       int
	resetCount         int
	succeedAfterResets int
}

func (m *testResettableMapper) KindFor(resource schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	panic("not implemented")
}

func (m *testResettableMapper) KindsFor(resource schema.GroupVersionResource) ([]schema.GroupVersionKind, error) {
	panic("not implemented")
}

func (m *testResettableMapper) RESTMapping(gk schema.GroupKind, versions ...string) (*apimeta.RESTMapping, error) {
	m.mappingCalls++
	if m.err != nil {
		return nil, m.err
	}

	version := "v1"
	if len(versions) > 0 {
		version = versions[0]
	}

	if m.resetCount < m.succeedAfterResets {
		return nil, &apimeta.NoKindMatchError{GroupKind: gk, SearchedVersions: []string{version}}
	}

	scope := apimeta.RESTScopeNamespace
	if gk.Group == "" && gk.Kind == "Namespace" {
		scope = apimeta.RESTScopeRoot
	}

	return &apimeta.RESTMapping{
		Resource:         schema.GroupVersionResource{Group: gk.Group, Version: version, Resource: strings.ToLower(gk.Kind) + "s"},
		GroupVersionKind: schema.GroupVersionKind{Group: gk.Group, Version: version, Kind: gk.Kind},
		Scope:            scope,
	}, nil
}

func (m *testResettableMapper) RESTMappings(gk schema.GroupKind, versions ...string) ([]*apimeta.RESTMapping, error) {
	mapping, err := m.RESTMapping(gk, versions...)
	if err != nil {
		return nil, err
	}

	return []*apimeta.RESTMapping{mapping}, nil
}

func (m *testResettableMapper) Reset() {
	m.resetCount++
}

func (m *testResettableMapper) ResourceFor(input schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	panic("not implemented")
}

func (m *testResettableMapper) ResourceSingularizer(resource string) (string, error) {
	panic("not implemented")
}

func (m *testResettableMapper) ResourcesFor(input schema.GroupVersionResource) ([]schema.GroupVersionResource, error) {
	panic("not implemented")
}

func TestKubeClientDelete_UsesRetryingGVKResolution(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(1)
	client.dynamicClient = dynfake.NewSimpleDynamicClient(scheme.Scheme)
	meta := spec.NewResourceMeta("widget", "default", "default", "", schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}, nil, nil)

	err := client.Delete(context.Background(), meta, KubeClientDeleteOptions{DefaultNamespace: "default"})
	require.NoError(t, err)

	assert.Equal(t, 2, mapper.mappingCalls)
	assert.Equal(t, 1, discoveryClient.invalidateCount)
}

func TestKubeClientGVKToGVR_DoesNotRetryNonNoMatch(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(0)
	boom := errors.New("discovery forbidden")
	mapper.err = boom

	err := client.ResetAndRetryOnUnknownGVR(context.Background(), func() error {
		_, _, err := client.GVKToGVR(context.Background(), schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"})

		return err
	})
	require.ErrorIs(t, err, boom)

	assert.Equal(t, 1, mapper.mappingCalls)
	assert.Equal(t, 0, mapper.resetCount)
	assert.Equal(t, 0, discoveryClient.invalidateCount)
}

func TestKubeClientGVKToGVR_RetriesNoMatchAfterRefresh(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(1)

	var (
		gvr        schema.GroupVersionResource
		namespaced bool
	)

	err := client.ResetAndRetryOnUnknownGVR(context.Background(), func() error {
		var err error

		gvr, namespaced, err = client.GVKToGVR(context.Background(), schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"})

		return err
	})
	require.NoError(t, err)

	assert.Equal(t, schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}, gvr)
	assert.True(t, namespaced)
	assert.Equal(t, 2, mapper.mappingCalls)
	assert.Equal(t, 1, mapper.resetCount)
	assert.Equal(t, 1, discoveryClient.invalidateCount)
}

func TestKubeClientGVKToGVR_StopsOnContextCancellation(t *testing.T) {
	client, _, _ := newTestKubeClient(1000)
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(fmt.Errorf("mapping canceled"))

	err := client.ResetAndRetryOnUnknownGVR(ctx, func() error {
		_, _, err := client.GVKToGVR(ctx, schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"})

		return err
	})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "mapping canceled")
}

func TestKubeClientGVKToGVR_SucceedsWithoutRetry(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(0)

	gvr, namespaced, err := client.GVKToGVR(context.Background(), schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
	require.NoError(t, err)

	assert.Equal(t, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, gvr)
	assert.True(t, namespaced)
	assert.Equal(t, 1, mapper.mappingCalls)
	assert.Equal(t, 0, mapper.resetCount)
	assert.Equal(t, 0, discoveryClient.invalidateCount)
}

func TestKubeClientGVKToGVR_TimesOutOnRepeatedNoMatch(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(1000)
	client.mapperNoMatchRetryTimeout = 5 * time.Millisecond
	client.mapperNoMatchRetryInterval = time.Millisecond

	err := client.ResetAndRetryOnUnknownGVR(context.Background(), func() error {
		_, _, err := client.GVKToGVR(context.Background(), schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"})

		return err
	})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "retry mapper NoMatch timed out after")
	assert.Positive(t, mapper.resetCount)
	assert.Equal(t, mapper.resetCount, discoveryClient.invalidateCount)
}

func TestKubeClientGet_DoesNotCacheFailedGet(t *testing.T) {
	client, dynamicClient, resSpec := newTestKubeClientWithConfigMap()
	requestErr := errors.New("connection reset")
	failNext := true
	dynamicClient.PrependReactor("get", "configmaps", func(ktesting.Action) (bool, runtime.Object, error) {
		if !failNext {
			return false, nil, nil
		}

		failNext = false

		return true, nil, requestErr
	})

	_, err := client.Get(context.Background(), resSpec.ResourceMeta, KubeClientGetOptions{DefaultNamespace: "default"})
	require.ErrorIs(t, err, requestErr)

	obj, err := client.Get(context.Background(), resSpec.ResourceMeta, KubeClientGetOptions{DefaultNamespace: "default", TryCache: true})
	require.NoError(t, err)
	assert.Equal(t, "value", obj.Object["data"].(map[string]interface{})["key"])
}

func TestKubeClientGet_TryCacheReadsAPIAfterFailedWrite(t *testing.T) {
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	requestErr := errors.New("context deadline exceeded")
	notFoundErr := &apierrors.StatusError{ErrStatus: metav1.Status{Status: metav1.StatusFailure, Code: 404, Reason: metav1.StatusReasonNotFound}}
	changeObj := func(tracker ktesting.ObjectTracker) error {
		return tracker.Update(gvr, &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": "cm", "namespace": "default"},
			"data":       map[string]interface{}{"key": "changed"},
		}}, "default")
	}
	deleteObj := func(tracker ktesting.ObjectTracker) error {
		return tracker.Delete(gvr, "default", "cm")
	}

	cases := []struct {
		name        string
		verb        string
		changeFn    func(tracker ktesting.ObjectTracker) error
		reactErr    error
		callFn      func(client *KubeClient, resSpec *spec.ResourceSpec) error
		wantCallErr bool
		wantData    string
	}{
		{
			name:     "apply",
			verb:     "patch",
			changeFn: changeObj,
			reactErr: requestErr,
			callFn: func(client *KubeClient, resSpec *spec.ResourceSpec) error {
				_, err := client.Apply(context.Background(), resSpec, KubeClientApplyOptions{DefaultNamespace: "default"})

				return err
			},
			wantCallErr: true,
			wantData:    "changed",
		},
		{
			name:     "create",
			verb:     "patch",
			changeFn: changeObj,
			reactErr: requestErr,
			callFn: func(client *KubeClient, resSpec *spec.ResourceSpec) error {
				_, err := client.Create(context.Background(), resSpec, KubeClientCreateOptions{DefaultNamespace: "default"})

				return err
			},
			wantCallErr: true,
			wantData:    "changed",
		},
		{
			name:     "merge patch",
			verb:     "patch",
			changeFn: changeObj,
			reactErr: requestErr,
			callFn: func(client *KubeClient, resSpec *spec.ResourceSpec) error {
				_, err := client.MergePatch(context.Background(), resSpec.ResourceMeta, []byte(`{}`), KubeClientMergePatchOptions{DefaultNamespace: "default"})

				return err
			},
			wantCallErr: true,
			wantData:    "changed",
		},
		{
			name:     "merge patch of a missing resource",
			verb:     "patch",
			changeFn: deleteObj,
			reactErr: notFoundErr,
			callFn: func(client *KubeClient, resSpec *spec.ResourceSpec) error {
				_, err := client.MergePatch(context.Background(), resSpec.ResourceMeta, []byte(`{}`), KubeClientMergePatchOptions{DefaultNamespace: "default"})

				return err
			},
			wantCallErr: true,
		},
		{
			name:     "delete",
			verb:     "delete",
			changeFn: deleteObj,
			reactErr: requestErr,
			callFn: func(client *KubeClient, resSpec *spec.ResourceSpec) error {
				return client.Delete(context.Background(), resSpec.ResourceMeta, KubeClientDeleteOptions{DefaultNamespace: "default"})
			},
			wantCallErr: true,
		},
		{
			name:     "delete of a missing resource",
			verb:     "delete",
			changeFn: deleteObj,
			reactErr: notFoundErr,
			callFn: func(client *KubeClient, resSpec *spec.ResourceSpec) error {
				return client.Delete(context.Background(), resSpec.ResourceMeta, KubeClientDeleteOptions{DefaultNamespace: "default"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, dynamicClient, resSpec := newTestKubeClientWithConfigMap()
			failNext := false
			dynamicClient.PrependReactor(tc.verb, "configmaps", func(ktesting.Action) (bool, runtime.Object, error) {
				if !failNext {
					return false, nil, nil
				}

				failNext = false

				if err := tc.changeFn(dynamicClient.Tracker()); err != nil {
					return true, nil, err
				}

				return true, nil, tc.reactErr
			})

			_, err := client.Get(context.Background(), resSpec.ResourceMeta, KubeClientGetOptions{DefaultNamespace: "default"})
			require.NoError(t, err)

			failNext = true

			if err := tc.callFn(client, resSpec); tc.wantCallErr {
				require.ErrorIs(t, err, tc.reactErr)
			} else {
				require.NoError(t, err)
			}

			dynamicClient.ClearActions()

			obj, err := client.Get(context.Background(), resSpec.ResourceMeta, KubeClientGetOptions{DefaultNamespace: "default", TryCache: true})

			require.Len(t, dynamicClient.Actions(), 1)
			assert.Equal(t, "get", dynamicClient.Actions()[0].GetVerb())

			if tc.wantData == "" {
				require.True(t, IsNotFoundErr(err), "want NotFound, got %v", err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantData, obj.Object["data"].(map[string]interface{})["key"])
		})
	}
}

func TestKubeClientGet_TryCacheReturnsCachedNotFound(t *testing.T) {
	client, _, _ := newTestKubeClient(0)
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme.Scheme)
	client.dynamicClient = dynamicClient
	meta := spec.NewResourceMeta("cm", "default", "default", "", schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, nil, nil)

	_, err := client.Get(context.Background(), meta, KubeClientGetOptions{DefaultNamespace: "default"})
	require.True(t, IsNotFoundErr(err))

	dynamicClient.ClearActions()

	_, err = client.Get(context.Background(), meta, KubeClientGetOptions{DefaultNamespace: "default", TryCache: true})
	require.True(t, IsNotFoundErr(err))
	assert.Empty(t, dynamicClient.Actions())
}

func TestKubeClientNamespaced_RetriesNoMatchAfterRefresh(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(1)

	var namespaced bool

	err := client.ResetAndRetryOnUnknownGVR(context.Background(), func() error {
		var err error

		namespaced, err = client.Namespaced(context.Background(), schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})

		return err
	})
	require.NoError(t, err)

	assert.True(t, namespaced)
	assert.Equal(t, 2, mapper.mappingCalls)
	assert.Equal(t, 1, mapper.resetCount)
	assert.Equal(t, 1, discoveryClient.invalidateCount)
}

func TestKubeClientRefreshDiscovery_InvalidatesDiscoveryAndResetsMapper(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(0)

	require.NoError(t, client.ResetDiscoveryCache(context.Background()))

	assert.Equal(t, 1, discoveryClient.invalidateCount)
	assert.Equal(t, 1, mapper.resetCount)
}

func TestKubeClientWaitForCRDDiscoverability_ResolvesServedGVKs(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(2)
	crd := newTestCRD(map[string]any{
		"group": "example.com",
		"names": map[string]any{
			"kind":   "Widget",
			"plural": "widgets",
		},
		"scope": "Namespaced",
		"versions": []any{
			map[string]any{"name": "v1", "served": true},
			map[string]any{"name": "v2", "served": false},
		},
	})

	err := client.waitForCRDDiscoverability(context.Background(), crd)
	require.NoError(t, err)

	assert.Equal(t, 2, discoveryClient.invalidateCount)
	assert.Equal(t, 2, mapper.resetCount)
	assert.Equal(t, 2, mapper.mappingCalls)
}

func TestServedCRDGVKs_SupportsV1Beta1VersionFieldWithOmittedScope(t *testing.T) {
	crd := newTestCRD(map[string]any{
		"group": "example.com",
		"names": map[string]any{
			"kind": "Widget",
		},
		"version": "v1beta1",
	})

	gvks, err := servedCRDGVKs(crd.Unstruct)
	require.NoError(t, err)

	assert.Equal(t, []schema.GroupVersionKind{{Group: "example.com", Version: "v1beta1", Kind: "Widget"}}, gvks)
}

func newTestCRD(crdSpec map[string]any) *spec.ResourceSpec {
	return &spec.ResourceSpec{
		Unstruct: &unstructured.Unstructured{
			Object: map[string]any{
				"spec": crdSpec,
			},
		},
	}
}

func newTestKubeClient(succeedAfterResets int) (*KubeClient, *testCachedDiscoveryClient, *testResettableMapper) {
	discoveryClient := newTestCachedDiscoveryClient()
	mapper := &testResettableMapper{succeedAfterResets: succeedAfterResets}
	client := NewKubeClient(nil, nil, discoveryClient, mapper)
	client.mapperNoMatchRetryTimeout = time.Second
	client.mapperNoMatchRetryInterval = time.Millisecond

	return client, discoveryClient, mapper
}
