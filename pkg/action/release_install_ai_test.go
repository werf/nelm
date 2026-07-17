//go:build ai_tests

package action

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/nelm/pkg/kube"
	"github.com/werf/nelm/pkg/resource/spec"
)

var (
	_ kube.ClientFactorier = (*createNamespaceClientFactory)(nil)
	_ kube.KubeClienter    = (*createNamespaceKubeClient)(nil)
)

type createNamespaceClientFactory struct {
	kubeClient kube.KubeClienter
}

func (f *createNamespaceClientFactory) Discovery() discovery.CachedDiscoveryInterface {
	panic("not implemented")
}

func (f *createNamespaceClientFactory) Dynamic() dynamic.Interface {
	panic("not implemented")
}

func (f *createNamespaceClientFactory) KubeClient() kube.KubeClienter {
	return f.kubeClient
}

func (f *createNamespaceClientFactory) KubeConfig() *kube.KubeConfig {
	panic("not implemented")
}

func (f *createNamespaceClientFactory) LegacyClientGetter() *kube.LegacyClientGetter {
	panic("not implemented")
}

func (f *createNamespaceClientFactory) Mapper() apimeta.ResettableRESTMapper {
	panic("not implemented")
}

func (f *createNamespaceClientFactory) Static() kubernetes.Interface {
	panic("not implemented")
}

type createNamespaceCall struct {
	dryRun bool
	kind   string
}

type createNamespaceKubeClient struct {
	calls       []createNamespaceCall
	cmApplyErr  error
	nsApplyErr  error
	nsCreateErr error
}

func (c *createNamespaceKubeClient) Apply(ctx context.Context, resSpec *spec.ResourceSpec, opts kube.KubeClientApplyOptions) (*unstructured.Unstructured, error) {
	kind := resSpec.GroupVersionKind.Kind
	c.calls = append(c.calls, createNamespaceCall{dryRun: opts.DryRun, kind: kind})

	switch kind {
	case "ConfigMap":
		return nil, c.cmApplyErr
	case "Namespace":
		return nil, c.nsApplyErr
	default:
		panic("unexpected apply kind: " + kind)
	}
}

func (c *createNamespaceKubeClient) Create(ctx context.Context, resSpec *spec.ResourceSpec, opts kube.KubeClientCreateOptions) (*unstructured.Unstructured, error) {
	c.calls = append(c.calls, createNamespaceCall{dryRun: false, kind: resSpec.GroupVersionKind.Kind})

	return nil, c.nsCreateErr
}

func (c *createNamespaceKubeClient) Delete(ctx context.Context, meta *spec.ResourceMeta, opts kube.KubeClientDeleteOptions) error {
	panic("not implemented")
}

func (c *createNamespaceKubeClient) GVKToGVR(ctx context.Context, gvk schema.GroupVersionKind) (schema.GroupVersionResource, bool, error) {
	panic("not implemented")
}

func (c *createNamespaceKubeClient) Get(ctx context.Context, meta *spec.ResourceMeta, opts kube.KubeClientGetOptions) (*unstructured.Unstructured, error) {
	panic("not implemented")
}

func (c *createNamespaceKubeClient) MergePatch(ctx context.Context, meta *spec.ResourceMeta, patch []byte, opts kube.KubeClientMergePatchOptions) (*unstructured.Unstructured, error) {
	panic("not implemented")
}

func (c *createNamespaceKubeClient) Namespaced(ctx context.Context, gvk schema.GroupVersionKind) (bool, error) {
	panic("not implemented")
}

func (c *createNamespaceKubeClient) ResetAndRetryOnUnknownGVR(ctx context.Context, fn func() error) error {
	panic("not implemented")
}

func (c *createNamespaceKubeClient) ResetDiscoveryCache(ctx context.Context) error {
	panic("not implemented")
}

func (c *createNamespaceKubeClient) ServerVersion(ctx context.Context) (*version.Info, error) {
	panic("not implemented")
}

func TestAI_CreateReleaseNamespaceBothProbesForbiddenAggregates(t *testing.T) {
	cmErr := newForbiddenErr("configmaps", "werf-synchronization")
	nsErr := newForbiddenErr("namespaces", "my-namespace")
	kubeClient := &createNamespaceKubeClient{cmApplyErr: cmErr, nsApplyErr: nsErr}
	clientFactory := &createNamespaceClientFactory{kubeClient: kubeClient}

	err := createReleaseNamespace(context.Background(), clientFactory, "my-namespace")
	require.Error(t, err)
	require.ErrorIs(t, err, cmErr)
	require.ErrorIs(t, err, nsErr)
	assert.Contains(t, err.Error(), "unable to ensure release namespace \"my-namespace\" exists")

	require.Len(t, kubeClient.calls, 2)
	assert.Equal(t, createNamespaceCall{dryRun: true, kind: "ConfigMap"}, kubeClient.calls[0])
	assert.Equal(t, createNamespaceCall{dryRun: true, kind: "Namespace"}, kubeClient.calls[1])
}

func TestAI_CreateReleaseNamespaceConfigMapForbiddenNamespaceNotFoundAggregates(t *testing.T) {
	cmErr := newForbiddenErr("configmaps", "werf-synchronization")
	nsErr := newNotFoundErr("namespaces", "my-namespace")
	kubeClient := &createNamespaceKubeClient{cmApplyErr: cmErr, nsApplyErr: nsErr}
	clientFactory := &createNamespaceClientFactory{kubeClient: kubeClient}

	err := createReleaseNamespace(context.Background(), clientFactory, "my-namespace")
	require.Error(t, err)
	require.ErrorIs(t, err, cmErr)
	require.ErrorIs(t, err, nsErr)

	require.Len(t, kubeClient.calls, 2)
}

func TestAI_CreateReleaseNamespaceConfigMapForbiddenThenNamespaceCreated(t *testing.T) {
	kubeClient := &createNamespaceKubeClient{
		cmApplyErr: newForbiddenErr("configmaps", "werf-synchronization"),
	}
	clientFactory := &createNamespaceClientFactory{kubeClient: kubeClient}

	err := createReleaseNamespace(context.Background(), clientFactory, "my-namespace")
	require.NoError(t, err)

	require.Len(t, kubeClient.calls, 3)
	assert.Equal(t, createNamespaceCall{dryRun: true, kind: "ConfigMap"}, kubeClient.calls[0])
	assert.Equal(t, createNamespaceCall{dryRun: true, kind: "Namespace"}, kubeClient.calls[1])
	assert.Equal(t, createNamespaceCall{dryRun: false, kind: "Namespace"}, kubeClient.calls[2])
}

func TestAI_CreateReleaseNamespaceConfigMapNotFoundThenNamespaceCreated(t *testing.T) {
	kubeClient := &createNamespaceKubeClient{
		cmApplyErr: newNotFoundErr("configmaps", "werf-synchronization"),
	}
	clientFactory := &createNamespaceClientFactory{kubeClient: kubeClient}

	err := createReleaseNamespace(context.Background(), clientFactory, "my-namespace")
	require.NoError(t, err)

	require.Len(t, kubeClient.calls, 3)
	assert.Equal(t, createNamespaceCall{dryRun: true, kind: "ConfigMap"}, kubeClient.calls[0])
	assert.Equal(t, createNamespaceCall{dryRun: true, kind: "Namespace"}, kubeClient.calls[1])
	assert.Equal(t, createNamespaceCall{dryRun: false, kind: "Namespace"}, kubeClient.calls[2])
}

func TestAI_CreateReleaseNamespaceConfigMapOtherErrorPropagates(t *testing.T) {
	cmErr := errors.New("connection refused")
	kubeClient := &createNamespaceKubeClient{cmApplyErr: cmErr}
	clientFactory := &createNamespaceClientFactory{kubeClient: kubeClient}

	err := createReleaseNamespace(context.Background(), clientFactory, "my-namespace")
	require.Error(t, err)
	require.ErrorIs(t, err, cmErr)

	require.Len(t, kubeClient.calls, 1)
	assert.Equal(t, "ConfigMap", kubeClient.calls[0].kind)
}

func TestAI_CreateReleaseNamespaceConfigMapProbeSucceeds(t *testing.T) {
	kubeClient := &createNamespaceKubeClient{}
	clientFactory := &createNamespaceClientFactory{kubeClient: kubeClient}

	err := createReleaseNamespace(context.Background(), clientFactory, "my-namespace")
	require.NoError(t, err)

	require.Len(t, kubeClient.calls, 1)
	assert.Equal(t, "ConfigMap", kubeClient.calls[0].kind)
	assert.True(t, kubeClient.calls[0].dryRun)
}

func TestAI_CreateReleaseNamespaceNamespaceProbeOtherErrorPropagates(t *testing.T) {
	nsErr := errors.New("connection refused")
	kubeClient := &createNamespaceKubeClient{
		cmApplyErr: newForbiddenErr("configmaps", "werf-synchronization"),
		nsApplyErr: nsErr,
	}
	clientFactory := &createNamespaceClientFactory{kubeClient: kubeClient}

	err := createReleaseNamespace(context.Background(), clientFactory, "my-namespace")
	require.Error(t, err)
	require.ErrorIs(t, err, nsErr)
	assert.Contains(t, err.Error(), "dry-run apply release namespace")

	require.Len(t, kubeClient.calls, 2)
}

func TestAI_CreateReleaseNamespaceRealCreateFailurePropagates(t *testing.T) {
	createErr := errors.New("quota exceeded")
	kubeClient := &createNamespaceKubeClient{
		cmApplyErr:  newForbiddenErr("configmaps", "werf-synchronization"),
		nsCreateErr: createErr,
	}
	clientFactory := &createNamespaceClientFactory{kubeClient: kubeClient}

	err := createReleaseNamespace(context.Background(), clientFactory, "my-namespace")
	require.Error(t, err)
	require.ErrorIs(t, err, createErr)
	assert.Contains(t, err.Error(), "create release namespace")

	require.Len(t, kubeClient.calls, 3)
	assert.Equal(t, createNamespaceCall{dryRun: false, kind: "Namespace"}, kubeClient.calls[2])
}

func newForbiddenErr(resource, name string) error {
	return apierrors.NewForbidden(schema.GroupResource{Resource: resource}, name, errors.New("forbidden"))
}

func newNotFoundErr(resource, name string) error {
	return apierrors.NewNotFound(schema.GroupResource{Resource: resource}, name)
}
