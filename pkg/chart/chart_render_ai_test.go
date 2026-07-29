//go:build ai_tests

package chart

import (
	"context"
	"testing"
	"unicode/utf8"

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

func (f *chartCapabilitiesClientFactory) LegacyClientGetter() *kube.LegacyClientGetter {
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

func TestAI_IsBinaryManifest(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "clean yaml",
			data: []byte("apiVersion: v1\nkind: Secret\ndata:\n  cni: Y2lsaXVt\n"),
			want: false,
		},
		{
			name: "multibyte utf-8 is allowed",
			data: []byte("metadata:\n  name: тест\n"),
			want: false,
		},
		{
			name: "invalid leading utf-8 octet",
			data: []byte{'d', 'a', 't', 'a', ':', '\n', ' ', ' ', 0x68, 0x1d, 0xf1, 0x63, 0xdc, 0xca},
			want: true,
		},
		{
			name: "disallowed control character",
			data: []byte("data:\n  value\x00here\n"),
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, isBinaryManifest(test.data))
		})
	}
}

func TestAI_RenderedTemplatesToResourceSpecsBinaryManifest(t *testing.T) {
	binaryManifest := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: test\ndata:\n  key: value\x01\x02\x03\n"
	renderedTemplates := map[string]string{"chart/templates/secret.yaml": binaryManifest}

	t.Run("fails without the option", func(t *testing.T) {
		_, err := renderedTemplatesToResourceSpecs(context.Background(), renderedTemplates, "ns", RenderChartOptions{})
		require.ErrorContains(t, err, "control characters are not allowed")
	})

	t.Run("sanitizes with the option", func(t *testing.T) {
		resources, err := renderedTemplatesToResourceSpecs(context.Background(), renderedTemplates, "ns", RenderChartOptions{
			LegacySanitizeBinaryManifest: true,
		})
		require.NoError(t, err)
		require.Len(t, resources, 1)

		assert.Equal(t, "Secret", resources[0].GroupVersionKind.Kind)
		assert.Equal(t, "value___", resources[0].Unstruct.Object["data"].(map[string]interface{})["key"])
	})

	t.Run("skips manifest that stays unparseable after sanitizing", func(t *testing.T) {
		resources, err := renderedTemplatesToResourceSpecs(context.Background(), map[string]string{
			"chart/templates/broken.yaml": "apiVersion: v1\nkind: Secret\n\x00data: [unclosed\n",
		}, "ns", RenderChartOptions{LegacySanitizeBinaryManifest: true})
		require.NoError(t, err)
		assert.Empty(t, resources)
	})
}

func TestAI_SanitizeBinaryManifest(t *testing.T) {
	t.Run("preserves clean content", func(t *testing.T) {
		in := []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: тест\n")
		assert.Equal(t, in, sanitizeBinaryManifest(in))
	})

	t.Run("replaces invalid utf-8 and keeps layout", func(t *testing.T) {
		in := []byte("apiVersion: v1\nkind: Secret\nmetadata:\n  name: test\ndata:\n  key: ")
		in = append(in, 0x68, 0x1d, 0xf1, 0x63, 0xdc, 0xca)
		in = append(in, '\n')

		out := sanitizeBinaryManifest(in)

		require.True(t, utf8.Valid(out))
		assert.Equal(t, "apiVersion: v1\nkind: Secret\nmetadata:\n  name: test\ndata:\n  key: h__c__\n", string(out))
	})

	t.Run("replaces control characters", func(t *testing.T) {
		out := sanitizeBinaryManifest([]byte("data:\n  value\x01\x02\x03\n"))

		require.True(t, utf8.Valid(out))
		assert.Equal(t, "data:\n  value___\n", string(out))
	})
}
