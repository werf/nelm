//go:build ai_tests

package chart

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	discfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ktesting "k8s.io/client-go/testing"

	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/resource/spec"
)

var (
	_ kube.ClientFactorier = (*chartCapabilitiesClientFactory)(nil)
	_ kube.KubeClienter    = (*chartCapabilitiesKubeClient)(nil)
)

type chartCapabilitiesClientFactory struct {
	discoveryClient discovery.CachedDiscoveryInterface
	kubeClient      kube.KubeClienter
}

func (f *chartCapabilitiesClientFactory) Discovery() discovery.CachedDiscoveryInterface {
	return f.discoveryClient
}

func (f *chartCapabilitiesClientFactory) Dynamic() dynamic.Interface {
	panic("not implemented")
}

func (f *chartCapabilitiesClientFactory) KubeClient() kube.KubeClienter {
	return f.kubeClient
}

func (f *chartCapabilitiesClientFactory) KubeConfig() *kube.KubeConfig {
	panic("not implemented")
}

func (f *chartCapabilitiesClientFactory) Mapper() apimeta.ResettableRESTMapper {
	panic("not implemented")
}

func (f *chartCapabilitiesClientFactory) Static() kubernetes.Interface {
	panic("not implemented")
}

type chartCapabilitiesKubeClient struct {
	refreshCount            int
	serverVersionSawRefresh bool
}

func (c *chartCapabilitiesKubeClient) Apply(ctx context.Context, spec *spec.ResourceSpec, opts kube.KubeClientApplyOptions) (*unstructured.Unstructured, error) {
	panic("not implemented")
}

func (c *chartCapabilitiesKubeClient) Create(ctx context.Context, spec *spec.ResourceSpec, opts kube.KubeClientCreateOptions) (*unstructured.Unstructured, error) {
	panic("not implemented")
}

func (c *chartCapabilitiesKubeClient) Delete(ctx context.Context, meta *spec.ResourceMeta, opts kube.KubeClientDeleteOptions) error {
	panic("not implemented")
}

func (c *chartCapabilitiesKubeClient) GVKToGVR(ctx context.Context, gvk schema.GroupVersionKind) (schema.GroupVersionResource, bool, error) {
	panic("not implemented")
}

func (c *chartCapabilitiesKubeClient) Get(ctx context.Context, meta *spec.ResourceMeta, opts kube.KubeClientGetOptions) (*unstructured.Unstructured, error) {
	panic("not implemented")
}

func (c *chartCapabilitiesKubeClient) MergePatch(ctx context.Context, meta *spec.ResourceMeta, patch []byte, opts kube.KubeClientMergePatchOptions) (*unstructured.Unstructured, error) {
	panic("not implemented")
}

func (c *chartCapabilitiesKubeClient) Namespaced(ctx context.Context, gvk schema.GroupVersionKind) (bool, error) {
	panic("not implemented")
}

func (c *chartCapabilitiesKubeClient) ResetAndRetryOnUnknownGVR(ctx context.Context, fn func() error) error {
	return fn()
}

func (c *chartCapabilitiesKubeClient) ResetDiscoveryCache(ctx context.Context) error {
	c.refreshCount++

	return nil
}

func (c *chartCapabilitiesKubeClient) ServerVersion(ctx context.Context) (*version.Info, error) {
	if c.refreshCount > 0 {
		c.serverVersionSawRefresh = true
	}

	return &version.Info{GitVersion: "v1.34.0", Major: "1", Minor: "34"}, nil
}

type chartCapabilitiesDiscovery struct {
	*discfake.FakeDiscovery

	kubeClient *chartCapabilitiesKubeClient
}

func newChartCapabilitiesDiscovery(kubeClient *chartCapabilitiesKubeClient) *chartCapabilitiesDiscovery {
	fakeDiscovery := &discfake.FakeDiscovery{
		Fake: &ktesting.Fake{},
		FakedServerVersion: &version.Info{
			GitVersion: "v1.34.0",
			Major:      "1",
			Minor:      "34",
		},
	}
	fakeDiscovery.Resources = []*metav1.APIResourceList{}

	return &chartCapabilitiesDiscovery{
		FakeDiscovery: fakeDiscovery,
		kubeClient:    kubeClient,
	}
}

func (d *chartCapabilitiesDiscovery) Fresh() bool {
	return true
}

func (d *chartCapabilitiesDiscovery) Invalidate() {}

func TestAI_BuildChartCapabilitiesRefreshesKubeClientDiscovery(t *testing.T) {
	fakeKubeClient := &chartCapabilitiesKubeClient{}
	fakeDiscovery := newChartCapabilitiesDiscovery(fakeKubeClient)
	clientFactory := &chartCapabilitiesClientFactory{
		discoveryClient: fakeDiscovery,
		kubeClient:      fakeKubeClient,
	}

	capabilities, err := buildChartCapabilities(context.Background(), clientFactory, buildChartCapabilitiesOptions{Remote: true})
	require.NoError(t, err)

	assert.Equal(t, 1, fakeKubeClient.refreshCount)
	assert.Equal(t, "v1.34.0", capabilities.KubeVersion.Version)
	assert.True(t, fakeKubeClient.serverVersionSawRefresh)
}
