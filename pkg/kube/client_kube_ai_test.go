//go:build ai_tests

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
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discfake "k8s.io/client-go/discovery/fake"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
	ktesting "k8s.io/client-go/testing"

	"github.com/werf/nelm/pkg/resource/spec"
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

func TestAI_KubeClientDelete_UsesRetryingGVKResolution(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(1)
	client.dynamicClient = dynfake.NewSimpleDynamicClient(scheme.Scheme)
	meta := spec.NewResourceMeta("widget", "default", "default", "", schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"}, nil, nil)

	err := client.Delete(context.Background(), meta, KubeClientDeleteOptions{DefaultNamespace: "default"})
	require.NoError(t, err)

	assert.Equal(t, 2, mapper.mappingCalls)
	assert.Equal(t, 1, discoveryClient.invalidateCount)
}

func TestAI_KubeClientGVKToGVR_DoesNotRetryNonNoMatch(t *testing.T) {
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

func TestAI_KubeClientGVKToGVR_RetriesNoMatchAfterRefresh(t *testing.T) {
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

func TestAI_KubeClientGVKToGVR_StopsOnContextCancellation(t *testing.T) {
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

func TestAI_KubeClientGVKToGVR_SucceedsWithoutRetry(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(0)

	gvr, namespaced, err := client.GVKToGVR(context.Background(), schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
	require.NoError(t, err)

	assert.Equal(t, schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, gvr)
	assert.True(t, namespaced)
	assert.Equal(t, 1, mapper.mappingCalls)
	assert.Equal(t, 0, mapper.resetCount)
	assert.Equal(t, 0, discoveryClient.invalidateCount)
}

func TestAI_KubeClientGVKToGVR_TimesOutOnRepeatedNoMatch(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(1000)
	client.mapperNoMatchRetryTimeout = 5 * time.Millisecond
	client.mapperNoMatchRetryInterval = time.Millisecond

	err := client.ResetAndRetryOnUnknownGVR(context.Background(), func() error {
		_, _, err := client.GVKToGVR(context.Background(), schema.GroupVersionKind{Group: "example.com", Version: "v1", Kind: "Widget"})

		return err
	})
	require.Error(t, err)

	assert.Contains(t, err.Error(), "retry mapper NoMatch timed out after")
	assert.True(t, mapper.resetCount > 0)
	assert.Equal(t, mapper.resetCount, discoveryClient.invalidateCount)
}

func TestAI_KubeClientNamespaced_RetriesNoMatchAfterRefresh(t *testing.T) {
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

func TestAI_KubeClientRefreshDiscovery_InvalidatesDiscoveryAndResetsMapper(t *testing.T) {
	client, discoveryClient, mapper := newTestKubeClient(0)

	require.NoError(t, client.ResetDiscoveryCache(context.Background()))

	assert.Equal(t, 1, discoveryClient.invalidateCount)
	assert.Equal(t, 1, mapper.resetCount)
}

func TestAI_KubeClientWaitForCRDDiscoverability_ResolvesServedGVKs(t *testing.T) {
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

func TestAI_ServedCRDGVKs_SupportsV1Beta1VersionFieldWithOmittedScope(t *testing.T) {
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
