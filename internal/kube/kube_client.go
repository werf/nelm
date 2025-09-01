package kube

import (
	"context"
	"fmt"
	"sync"

	"github.com/jellydator/ttlcache/v3"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/pkg/log"
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

type KubeClientGetOptions struct {
	TryCache bool
}

func (c *KubeClient) Get(ctx context.Context, meta *id.ResourceMeta, opts KubeClientGetOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(meta)
	lock.Lock()
	defer lock.Unlock()

	if opts.TryCache {
		if res := c.clusterCache.Get(meta.IDWithVersion()); res != nil {
			if res.Value().err != nil {
				return nil, fmt.Errorf("get resource %q from client cache: %w", meta.IDHuman(), res.Value().err)
			}

			resultObj := res.Value().obj

			log.Default.TraceStruct(ctx, resultObj, "Got resource %q from cache:", meta.IDHuman())

			return resultObj, nil
		}
	}

	gvr, namespaced, err := resource.GVKtoGVR(meta.GroupVersionKind, c.mapper)
	if err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, meta.Namespace, namespaced)

	log.Default.Debug(ctx, "Getting resource %q", meta.IDHuman())
	resultObj, err := clientResource.Get(ctx, meta.Name, metav1.GetOptions{})
	if err != nil {
		c.clusterCache.Set(meta.IDWithVersion(), &clusterCacheEntry{err: err}, 0)
		return nil, fmt.Errorf("get resource %q: %w", meta.IDHuman(), err)
	}
	c.clusterCache.Set(meta.IDWithVersion(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)

	log.Default.TraceStruct(ctx, resultObj, "Got resource %q via Kubernetes API:", meta.IDHuman())

	return resultObj, nil
}

type KubeClientCreateOptions struct {
	ForceReplicas *int
}

func (c *KubeClient) Create(ctx context.Context, spec *id.ResourceSpec, opts KubeClientCreateOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(spec.ResourceMeta)
	lock.Lock()
	defer lock.Unlock()

	gvr, namespaced, err := resource.GVKtoGVR(spec.GroupVersionKind, c.mapper)
	if err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, spec.Namespace, namespaced)

	if opts.ForceReplicas != nil {
		unstructured.SetNestedField(spec.Unstruct.UnstructuredContent(), int64(*opts.ForceReplicas), "spec", "replicas")
	}

	log.Default.Debug(ctx, "Server-side applying resource %q", spec.IDHuman())
	resultObj, err := clientResource.Apply(ctx, spec.Name, spec.Unstruct, metav1.ApplyOptions{
		Force:        true,
		FieldManager: common.DefaultFieldManager,
	})
	if err != nil {
		c.clusterCache.Set(spec.IDWithVersion(), &clusterCacheEntry{err: err}, 0)
		return nil, fmt.Errorf("server-side apply resource %q: %w", spec.IDHuman(), err)
	}
	c.clusterCache.Set(spec.IDWithVersion(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)

	if resource.IsCRDFromGR(gvr.GroupResource()) {
		c.mapper.Reset()
	}

	log.Default.TraceStruct(ctx, resultObj, "Created resource %q via Kubernetes API:", spec.IDHuman())

	return resultObj, nil
}

type KubeClientApplyOptions struct {
	DryRun bool
}

func (c *KubeClient) Apply(ctx context.Context, spec *id.ResourceSpec, opts KubeClientApplyOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(spec.ResourceMeta)
	lock.Lock()
	defer lock.Unlock()

	gvr, namespaced, err := resource.GVKtoGVR(spec.GroupVersionKind, c.mapper)
	if err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, spec.Namespace, namespaced)

	var dryRun []string
	if opts.DryRun {
		dryRun = []string{metav1.DryRunAll}
	}

	log.Default.Debug(ctx, "Server-side %sapplying resource %q", lo.Ternary(opts.DryRun, "dry-run ", ""), spec.IDHuman())
	resultObj, err := clientResource.Apply(ctx, spec.Name, spec.Unstruct, metav1.ApplyOptions{
		DryRun:       dryRun,
		Force:        true,
		FieldManager: common.DefaultFieldManager,
	})
	if err != nil {
		if !opts.DryRun {
			c.clusterCache.Set(spec.IDWithVersion(), &clusterCacheEntry{err: err}, 0)
		}
		return nil, fmt.Errorf("server-side %sapply resource %q: %w", lo.Ternary(opts.DryRun, "dry-run ", ""), spec.IDHuman(), err)
	}
	if !opts.DryRun {
		c.clusterCache.Set(spec.IDWithVersion(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)
	}

	if resource.IsCRDFromGR(gvr.GroupResource()) && !opts.DryRun {
		c.mapper.Reset()
	}

	log.Default.TraceStruct(ctx, resultObj, "Server-side %sapplied resource %q via Kubernetes API:", lo.Ternary(opts.DryRun, "dry-run ", ""), spec.IDHuman())

	return resultObj, nil
}

func (c *KubeClient) MergePatch(ctx context.Context, meta *id.ResourceMeta, patch []byte) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(meta)
	lock.Lock()
	defer lock.Unlock()

	gvr, namespaced, err := resource.GVKtoGVR(meta.GroupVersionKind, c.mapper)
	if err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, meta.Namespace, namespaced)

	log.Default.Debug(ctx, "Merge patching resource %q", meta.IDHuman())
	resultObj, err := clientResource.Patch(ctx, meta.Name, types.MergePatchType, patch, metav1.PatchOptions{
		FieldManager: common.DefaultFieldManager,
	})
	if err != nil {
		if IsNotFoundErr(err) {
			log.Default.Debug(ctx, "Skipping merge patching, not found resource %q", meta.IDHuman())
			return nil, nil
		}

		c.clusterCache.Set(meta.IDWithVersion(), &clusterCacheEntry{err: err}, 0)
		return nil, fmt.Errorf("merge patch resource %q: %w", meta.IDHuman(), err)
	}
	c.clusterCache.Set(meta.IDWithVersion(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)

	log.Default.TraceStruct(ctx, resultObj, "Merge patched resource %q via Kubernetes API:", meta.IDHuman())

	return resultObj, nil
}

type KubeClientDeleteOptions struct {
	PropagationPolicy *metav1.DeletionPropagation
}

func (c *KubeClient) Delete(ctx context.Context, meta *id.ResourceMeta, opts KubeClientDeleteOptions) error {
	lock := c.resourceLock(meta)
	lock.Lock()
	defer lock.Unlock()

	gvr, namespaced, err := resource.GVKtoGVR(meta.GroupVersionKind, c.mapper)
	if err != nil {
		return fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, meta.Namespace, namespaced)

	var propagationPolicy *metav1.DeletionPropagation
	if opts.PropagationPolicy != nil {
		propagationPolicy = opts.PropagationPolicy
	} else {
		propagationPolicy = lo.ToPtr(metav1.DeletePropagationForeground)
	}

	log.Default.Debug(ctx, "Deleting resource %q", meta.IDHuman())
	if err := clientResource.Delete(ctx, meta.Name, metav1.DeleteOptions{
		PropagationPolicy: propagationPolicy,
	}); err != nil {
		if IsNotFoundErr(err) {
			log.Default.Debug(ctx, "Skipping deletion, not found resource %q", meta.IDHuman())
			return nil
		}

		return fmt.Errorf("delete resource %q: %w", meta.IDHuman(), err)
	}
	c.clusterCache.Delete(meta.IDWithVersion())

	return nil
}

func (c *KubeClient) resourceLock(meta *id.ResourceMeta) *sync.Mutex {
	lock, _ := c.resourceLocks.LoadOrStore(meta.IDWithVersion(), &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (c *KubeClient) clientResource(gvr schema.GroupVersionResource, namespace string, namespaced bool) dynamic.ResourceInterface {
	if namespaced {
		return c.dynamicClient.Resource(gvr).Namespace(namespace)
	}

	return c.dynamicClient.Resource(gvr)
}

type clusterCacheEntry struct {
	obj *unstructured.Unstructured
	err error
}
