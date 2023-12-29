package kubeclnt

import (
	"context"
	"fmt"
	"sync"

	"github.com/jellydator/ttlcache/v3"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcid"
)

var _ KubeClienter = (*KubeClient)(nil)

func NewKubeClient(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper meta.ResettableRESTMapper) *KubeClient {
	clusterCache := ttlcache.New[string, *clusterCacheEntry](
		ttlcache.WithDisableTouchOnHit[string, *clusterCacheEntry](),
	)

	return &KubeClient{
		staticClient:    staticClient,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		mapper:          mapper,
		clusterCache:    clusterCache,
		resourceLocks:   &sync.Map{},
	}
}

type KubeClient struct {
	staticClient    kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	mapper          meta.ResettableRESTMapper
	clusterCache    *ttlcache.Cache[string, *clusterCacheEntry]
	resourceLocks   *sync.Map
}

func (c *KubeClient) Get(ctx context.Context, resource *resrcid.ResourceID, opts KubeClientGetOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resource)
	lock.Lock()
	defer lock.Unlock()

	if opts.TryCache {
		if res := c.clusterCache.Get(resource.VersionID()); res != nil {
			if res.Value().err != nil {
				return nil, fmt.Errorf("error getting resource %q from client cache: %w", resource.HumanID(), res.Value().err)
			}
			return res.Value().obj, nil
		}
	}

	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return nil, fmt.Errorf("error getting GroupVersionResource: %w", err)
	}

	namespaced, err := resource.Namespaced()
	if err != nil {
		return nil, fmt.Errorf("error checking if resource is namespaced: %w", err)
	}

	clientResource := c.clientResource(gvr, resource.Namespace(), namespaced)

	log.Default.Debug(ctx, "Getting resource %q", resource.HumanID())
	r, err := clientResource.Get(ctx, resource.Name(), metav1.GetOptions{})
	if err != nil {
		c.clusterCache.Set(resource.VersionID(), &clusterCacheEntry{err: err}, 0)
		return nil, fmt.Errorf("error getting resource %q: %w", resource.HumanID(), err)
	}
	c.clusterCache.Set(resource.VersionID(), &clusterCacheEntry{obj: r.DeepCopy()}, 0)

	return r, nil
}

func (c *KubeClient) Create(ctx context.Context, resource *resrcid.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientCreateOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resource)
	lock.Lock()
	defer lock.Unlock()

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

	log.Default.Debug(ctx, "Server-side applying resource %q", resource.HumanID())
	resultObj, err := clientResource.Apply(ctx, resource.Name(), unstruct, metav1.ApplyOptions{
		Force:        true,
		FieldManager: common.DefaultFieldManager,
	})
	if err != nil {
		c.clusterCache.Set(resource.VersionID(), &clusterCacheEntry{err: err}, 0)
		return nil, fmt.Errorf("error server-side applying resource %q: %w", resource.HumanID(), err)
	}
	c.clusterCache.Set(resource.VersionID(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)

	if resrc.IsCRDFromGR(gvr.GroupResource()) {
		c.mapper.Reset()
	}

	return resultObj, nil
}

func (c *KubeClient) Apply(ctx context.Context, resource *resrcid.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientApplyOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resource)
	lock.Lock()
	defer lock.Unlock()

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

	log.Default.Debug(ctx, "Server-side %sapplying resource %q", lo.Ternary(opts.DryRun, "dry-run ", ""), resource.HumanID())
	resultObj, err := clientResource.Apply(ctx, resource.Name(), unstruct, metav1.ApplyOptions{
		DryRun:       dryRun,
		Force:        true,
		FieldManager: common.DefaultFieldManager,
	})
	if err != nil {
		if !opts.DryRun {
			c.clusterCache.Set(resource.VersionID(), &clusterCacheEntry{err: err}, 0)
		}
		return nil, fmt.Errorf("error server-side %sapplying resource %q: %w", lo.Ternary(opts.DryRun, "dry-run ", ""), resource.HumanID(), err)
	}
	if !opts.DryRun {
		c.clusterCache.Set(resource.VersionID(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)
	}

	if resrc.IsCRDFromGR(gvr.GroupResource()) && !opts.DryRun {
		c.mapper.Reset()
	}

	return resultObj, nil
}

func (c *KubeClient) MergePatch(ctx context.Context, resource *resrcid.ResourceID, patch []byte) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resource)
	lock.Lock()
	defer lock.Unlock()

	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return nil, fmt.Errorf("error getting GroupVersionResource: %w", err)
	}

	namespaced, err := resource.Namespaced()
	if err != nil {
		return nil, fmt.Errorf("error checking if resource is namespaced: %w", err)
	}

	clientResource := c.clientResource(gvr, resource.Namespace(), namespaced)

	log.Default.Debug(ctx, "Merge patching resource %q", resource.HumanID())
	resultObj, err := clientResource.Patch(ctx, resource.Name(), types.MergePatchType, patch, metav1.PatchOptions{
		FieldManager: common.DefaultFieldManager,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			log.Default.Debug(ctx, "Skipping merge patching, not found resource %q", resource.HumanID())
			return nil, nil
		}

		c.clusterCache.Set(resource.VersionID(), &clusterCacheEntry{err: err}, 0)
		return nil, fmt.Errorf("error merge patching resource %q: %w", resource.HumanID(), err)
	}
	c.clusterCache.Set(resource.VersionID(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)

	return resultObj, nil
}

func (c *KubeClient) Delete(ctx context.Context, resource *resrcid.ResourceID) error {
	lock := c.resourceLock(resource)
	lock.Lock()
	defer lock.Unlock()

	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return fmt.Errorf("error getting GroupVersionResource: %w", err)
	}

	namespaced, err := resource.Namespaced()
	if err != nil {
		return fmt.Errorf("error checking if resource is namespaced: %w", err)
	}

	clientResource := c.clientResource(gvr, resource.Namespace(), namespaced)

	log.Default.Debug(ctx, "Deleting resource %q", resource.HumanID())
	if err := clientResource.Delete(ctx, resource.Name(), metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			log.Default.Debug(ctx, "Skipping deletion, not found resource %q", resource.HumanID())
			return nil
		}

		return fmt.Errorf("error deleting resource %q: %w", resource.HumanID(), err)
	}
	c.clusterCache.Delete(resource.VersionID())

	return nil
}

func (c *KubeClient) resourceLock(resource *resrcid.ResourceID) *sync.Mutex {
	lock, _ := c.resourceLocks.LoadOrStore(resource.VersionID(), &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (c *KubeClient) clientResource(gvr schema.GroupVersionResource, namespace string, namespaced bool) dynamic.ResourceInterface {
	if namespaced {
		return c.dynamicClient.Resource(gvr).Namespace(namespace)
	}

	return c.dynamicClient.Resource(gvr)
}

type KubeClientGetOptions struct {
	TryCache bool
}

type KubeClientCreateOptions struct {
	ForceReplicas *int
}

type KubeClientApplyOptions struct {
	DryRun bool
}

type KubeClienter interface {
	Get(ctx context.Context, resource *resrcid.ResourceID, opts KubeClientGetOptions) (*unstructured.Unstructured, error)
	Create(ctx context.Context, resource *resrcid.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientCreateOptions) (*unstructured.Unstructured, error)
	Apply(ctx context.Context, resource *resrcid.ResourceID, unstruct *unstructured.Unstructured, opts KubeClientApplyOptions) (*unstructured.Unstructured, error)
	MergePatch(ctx context.Context, resource *resrcid.ResourceID, patch []byte) (*unstructured.Unstructured, error)
	Delete(ctx context.Context, resource *resrcid.ResourceID) error
}

type clusterCacheEntry struct {
	obj *unstructured.Unstructured
	err error
}
