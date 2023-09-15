package kubeclnt

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/resrc"
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var _ KubeClienter = (*KubeClient)(nil)

func NewKubeClient(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper meta.ResettableRESTMapper) *KubeClient {
	return &KubeClient{
		fieldManager:    common.DefaultFieldManager,
		staticClient:    staticClient,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		mapper:          mapper,
	}
}

type KubeClient struct {
	fieldManager    string
	staticClient    kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	mapper          meta.ResettableRESTMapper
}

func (c *KubeClient) Get(ctx context.Context, resource *resrcid.ResourceID) (*unstructured.Unstructured, error) {
	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return nil, fmt.Errorf("error getting GroupVersionResource: %w", err)
	}

	namespaced, err := resource.Namespaced()
	if err != nil {
		return nil, fmt.Errorf("error checking if resource is namespaced: %w", err)
	}

	clientResource := c.clientResource(gvr, resource.Namespace(), namespaced)

	log.Default.Debug(ctx, "Getting resource %q ...", resource.HumanID())
	r, err := clientResource.Get(ctx, resource.Name(), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting resource %q: %w", resource.HumanID(), err)
	}

	return r, nil
}

func (c *KubeClient) Create(ctx context.Context, resource *resrcid.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientCreateOptions) (*unstructured.Unstructured, error) {
	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return nil, fmt.Errorf("error getting GroupVersionResource: %w", err)
	}

	namespaced, err := resource.Namespaced()
	if err != nil {
		return nil, fmt.Errorf("error checking if resource is namespaced: %w", err)
	}

	clientResource := c.clientResource(gvr, resource.Namespace(), namespaced)

	if opts.ForceReplicas != nil {
		unstructured.SetNestedField(unstruct.UnstructuredContent(), *opts.ForceReplicas, "spec", "replicas")
	}

	log.Default.Debug(ctx, "Creating resource %q ...", resource.HumanID())
	resultObj, err := clientResource.Create(ctx, unstruct, metav1.CreateOptions{
		FieldManager: c.fieldManager,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating resource %q: %w", resource.HumanID(), err)
	}

	if resrc.IsCRDFromGR(gvr.GroupResource()) {
		c.mapper.Reset()
	}

	return resultObj, nil
}

func (c *KubeClient) Apply(ctx context.Context, resource *resrcid.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientApplyOptions) (*unstructured.Unstructured, error) {
	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return nil, fmt.Errorf("error getting GroupVersionResource: %w", err)
	}

	namespaced, err := resource.Namespaced()
	if err != nil {
		return nil, fmt.Errorf("error checking if resource is namespaced: %w", err)
	}

	clientResource := c.clientResource(gvr, resource.Namespace(), namespaced)

	var dryRun []string
	if opts.DryRun {
		dryRun = []string{metav1.DryRunAll}
	}

	log.Default.Debug(ctx, "Server-side applying resource %q ...", resource.HumanID())
	resultObj, err := clientResource.Apply(ctx, resource.Name(), unstruct, metav1.ApplyOptions{
		DryRun:       dryRun,
		Force:        true,
		FieldManager: c.fieldManager,
	})
	if err != nil {
		return nil, fmt.Errorf("error server-side applying resource %q: %w", resource.HumanID(), err)
	}

	if resrc.IsCRDFromGR(gvr.GroupResource()) && !opts.DryRun {
		c.mapper.Reset()
	}

	return resultObj, nil
}

func (c *KubeClient) StrategicPatch(ctx context.Context, resource *resrcid.ResourceID, patch []byte) (*unstructured.Unstructured, error) {
	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return nil, fmt.Errorf("error getting GroupVersionResource: %w", err)
	}

	namespaced, err := resource.Namespaced()
	if err != nil {
		return nil, fmt.Errorf("error checking if resource is namespaced: %w", err)
	}

	clientResource := c.clientResource(gvr, resource.Namespace(), namespaced)

	log.Default.Debug(ctx, "Strategic patching resource %q ...", resource.HumanID())
	resultObj, err := clientResource.Patch(ctx, resource.Name(), types.StrategicMergePatchType, patch, metav1.PatchOptions{
		FieldManager: c.fieldManager,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			log.Default.Debug(ctx, "Skipping strategic patching, not found resource %q", resource.HumanID())
			return nil, nil
		}

		return nil, fmt.Errorf("error strategic patching resource %q: %w", resource.HumanID(), err)
	}

	return resultObj, nil
}

func (c *KubeClient) Delete(ctx context.Context, resource *resrcid.ResourceID) error {
	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return fmt.Errorf("error getting GroupVersionResource: %w", err)
	}

	namespaced, err := resource.Namespaced()
	if err != nil {
		return fmt.Errorf("error checking if resource is namespaced: %w", err)
	}

	clientResource := c.clientResource(gvr, resource.Namespace(), namespaced)

	log.Default.Debug(ctx, "Deleting resource %q ...", resource.HumanID())
	if err := clientResource.Delete(ctx, resource.Name(), metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			log.Default.Debug(ctx, "Skipping deletion, not found resource %q", resource.HumanID())
			return nil
		}

		return fmt.Errorf("error deleting resource %q: %w", resource.HumanID(), err)
	}

	return nil
}

func (c *KubeClient) clientResource(gvr schema.GroupVersionResource, namespace string, namespaced bool) dynamic.ResourceInterface {
	if namespaced {
		return c.dynamicClient.Resource(gvr).Namespace(namespace)
	}

	return c.dynamicClient.Resource(gvr)
}

type KubeClientCreateOptions struct {
	ForceReplicas *int
}

type KubeClientApplyOptions struct {
	DryRun bool
}

type KubeClienter interface {
	Get(ctx context.Context, resource *resrcid.ResourceID) (*unstructured.Unstructured, error)
	Create(ctx context.Context, resource *resrcid.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientCreateOptions) (*unstructured.Unstructured, error)
	Apply(ctx context.Context, resource *resrcid.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientApplyOptions) (*unstructured.Unstructured, error)
	StrategicPatch(ctx context.Context, resource *resrcid.ResourceID, patch []byte) (*unstructured.Unstructured, error)
	Delete(ctx context.Context, resource *resrcid.ResourceID) error
}
