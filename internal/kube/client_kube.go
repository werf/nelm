package kube

import (
	"context"
	"fmt"
	"sync"

	"github.com/jellydator/ttlcache/v3"
	"github.com/samber/lo"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/log"
)

var _ KubeClienter = (*KubeClient)(nil)

type KubeClienter interface {
	Get(ctx context.Context, meta *spec.ResourceMeta, opts KubeClientGetOptions) (*unstructured.Unstructured, error)
	Create(ctx context.Context, spec *spec.ResourceSpec, opts KubeClientCreateOptions) (*unstructured.Unstructured, error)
	Apply(ctx context.Context, spec *spec.ResourceSpec, opts KubeClientApplyOptions) (*unstructured.Unstructured, error)
	MergePatch(ctx context.Context, meta *spec.ResourceMeta, patch []byte, opts KubeClientMergePatchOptions) (*unstructured.Unstructured, error)
	Delete(ctx context.Context, meta *spec.ResourceMeta, opts KubeClientDeleteOptions) error
}

type KubeClient struct {
	staticClient    kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	mapper          apimeta.ResettableRESTMapper
	clusterCache    *ttlcache.Cache[string, *clusterCacheEntry]
	resourceLocks   *sync.Map
}

func NewKubeClient(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper apimeta.ResettableRESTMapper) *KubeClient {
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

type KubeClientGetOptions struct {
	DefaultNamespace string
	TryCache         bool
}

func (c *KubeClient) Get(ctx context.Context, resMeta *spec.ResourceMeta, opts KubeClientGetOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resMeta)

	lock.Lock()
	defer lock.Unlock()

	if opts.TryCache {
		if res := c.clusterCache.Get(resMeta.IDWithVersion()); res != nil {
			if res.Value().err != nil {
				return nil, fmt.Errorf("get resource %q from client cache: %w", resMeta.IDHuman(), res.Value().err)
			}

			resultObj := res.Value().obj

			log.Default.TraceStruct(ctx, resultObj, "Got resource %q from cache:", resMeta.IDHuman())

			return resultObj, nil
		}
	}

	gvr, namespaced, err := spec.GVKtoGVR(resMeta.GroupVersionKind, c.mapper)
	if err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, resMeta.Namespace, opts.DefaultNamespace, namespaced)

	log.Default.Debug(ctx, "Getting resource %q", resMeta.IDHuman())

	resultObj, err := clientResource.Get(ctx, resMeta.Name, metav1.GetOptions{})
	if err != nil {
		c.clusterCache.Set(resMeta.IDWithVersion(), &clusterCacheEntry{err: err}, 0)
		return nil, fmt.Errorf("get resource %q: %w", resMeta.IDHuman(), err)
	}

	c.clusterCache.Set(resMeta.IDWithVersion(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)

	log.Default.TraceStruct(ctx, resultObj, "Got resource %q via Kubernetes API:", resMeta.IDHuman())

	return resultObj, nil
}

type KubeClientCreateOptions struct {
	DefaultNamespace string
	ForceReplicas    *int
}

func (c *KubeClient) Create(ctx context.Context, resSpec *spec.ResourceSpec, opts KubeClientCreateOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resSpec.ResourceMeta)

	lock.Lock()
	defer lock.Unlock()

	gvr, namespaced, err := spec.GVKtoGVR(resSpec.GroupVersionKind, c.mapper)
	if err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, resSpec.Namespace, opts.DefaultNamespace, namespaced)

	if opts.ForceReplicas != nil {
		if err := unstructured.SetNestedField(resSpec.Unstruct.UnstructuredContent(), int64(*opts.ForceReplicas), "spec", "replicas"); err != nil {
			return nil, fmt.Errorf("set spec.replicas for resource %q: %w", resSpec.IDHuman(), err)
		}
	}

	log.Default.Debug(ctx, "Server-side applying resource %q", resSpec.IDHuman())

	resultObj, err := clientResource.Apply(ctx, resSpec.Name, resSpec.Unstruct, metav1.ApplyOptions{
		Force:        true,
		FieldManager: common.DefaultFieldManager,
	})
	if err != nil {
		c.clusterCache.Set(resSpec.IDWithVersion(), &clusterCacheEntry{err: err}, 0)
		return nil, fmt.Errorf("server-side apply resource %q: %w", resSpec.IDHuman(), err)
	}

	c.clusterCache.Set(resSpec.IDWithVersion(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)

	if spec.IsCRDFromGR(gvr.GroupResource()) {
		c.mapper.Reset()
	}

	log.Default.TraceStruct(ctx, resultObj, "Created resource %q via Kubernetes API:", resSpec.IDHuman())

	return resultObj, nil
}

type KubeClientApplyOptions struct {
	DefaultNamespace string
	DryRun           bool
}

func (c *KubeClient) Apply(ctx context.Context, resSpec *spec.ResourceSpec, opts KubeClientApplyOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resSpec.ResourceMeta)

	lock.Lock()
	defer lock.Unlock()

	gvr, namespaced, err := spec.GVKtoGVR(resSpec.GroupVersionKind, c.mapper)
	if err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, resSpec.Namespace, opts.DefaultNamespace, namespaced)

	var dryRun []string
	if opts.DryRun {
		dryRun = []string{metav1.DryRunAll}
	}

	log.Default.Debug(ctx, "Server-side %sapplying resource %q", lo.Ternary(opts.DryRun, "dry-run ", ""), resSpec.IDHuman())

	resultObj, err := clientResource.Apply(ctx, resSpec.Name, resSpec.Unstruct, metav1.ApplyOptions{
		DryRun:       dryRun,
		Force:        true,
		FieldManager: common.DefaultFieldManager,
	})
	if err != nil {
		if !opts.DryRun {
			c.clusterCache.Set(resSpec.IDWithVersion(), &clusterCacheEntry{err: err}, 0)
		}

		return nil, fmt.Errorf("server-side %sapply resource %q: %w", lo.Ternary(opts.DryRun, "dry-run ", ""), resSpec.IDHuman(), err)
	}

	if !opts.DryRun {
		c.clusterCache.Set(resSpec.IDWithVersion(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)
	}

	if spec.IsCRDFromGR(gvr.GroupResource()) && !opts.DryRun {
		c.mapper.Reset()
	}

	log.Default.TraceStruct(ctx, resultObj, "Server-side %sapplied resource %q via Kubernetes API:", lo.Ternary(opts.DryRun, "dry-run ", ""), resSpec.IDHuman())

	return resultObj, nil
}

type KubeClientMergePatchOptions struct {
	DefaultNamespace string
}

func (c *KubeClient) MergePatch(ctx context.Context, resMeta *spec.ResourceMeta, patch []byte, opts KubeClientMergePatchOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resMeta)

	lock.Lock()
	defer lock.Unlock()

	gvr, namespaced, err := spec.GVKtoGVR(resMeta.GroupVersionKind, c.mapper)
	if err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, resMeta.Namespace, opts.DefaultNamespace, namespaced)

	log.Default.Debug(ctx, "Merge patching resource %q", resMeta.IDHuman())

	resultObj, err := clientResource.Patch(ctx, resMeta.Name, types.MergePatchType, patch, metav1.PatchOptions{
		FieldManager: common.DefaultFieldManager,
	})
	if err != nil {
		if !IsNotFoundErr(err) {
			c.clusterCache.Set(resMeta.IDWithVersion(), &clusterCacheEntry{err: err}, 0)
		}

		return nil, fmt.Errorf("merge patch resource %q: %w", resMeta.IDHuman(), err)
	}

	c.clusterCache.Set(resMeta.IDWithVersion(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)

	log.Default.TraceStruct(ctx, resultObj, "Merge patched resource %q via Kubernetes API:", resMeta.IDHuman())

	return resultObj, nil
}

type KubeClientDeleteOptions struct {
	DefaultNamespace  string
	PropagationPolicy *metav1.DeletionPropagation
}

func (c *KubeClient) Delete(ctx context.Context, resMeta *spec.ResourceMeta, opts KubeClientDeleteOptions) error {
	lock := c.resourceLock(resMeta)

	lock.Lock()
	defer lock.Unlock()

	gvr, namespaced, err := spec.GVKtoGVR(resMeta.GroupVersionKind, c.mapper)
	if err != nil {
		return fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, resMeta.Namespace, opts.DefaultNamespace, namespaced)

	var propagationPolicy *metav1.DeletionPropagation
	if opts.PropagationPolicy != nil {
		propagationPolicy = opts.PropagationPolicy
	} else {
		propagationPolicy = lo.ToPtr(metav1.DeletePropagationForeground)
	}

	log.Default.Debug(ctx, "Deleting resource %q", resMeta.IDHuman())

	if err := clientResource.Delete(ctx, resMeta.Name, metav1.DeleteOptions{
		PropagationPolicy: propagationPolicy,
	}); err != nil {
		if IsNotFoundErr(err) {
			log.Default.Debug(ctx, "Skipping deletion, not found resource %q", resMeta.IDHuman())
			return nil
		}

		return fmt.Errorf("delete resource %q: %w", resMeta.IDHuman(), err)
	}

	c.clusterCache.Delete(resMeta.IDWithVersion())

	return nil
}

func (c *KubeClient) resourceLock(meta *spec.ResourceMeta) *sync.Mutex {
	lock, _ := c.resourceLocks.LoadOrStore(meta.IDWithVersion(), &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (c *KubeClient) clientResource(gvr schema.GroupVersionResource, namespace, defaultNamespace string, namespaced bool) dynamic.ResourceInterface {
	if namespaced {
		if namespace == "" {
			namespace = defaultNamespace
		}

		return c.dynamicClient.Resource(gvr).Namespace(namespace)
	}

	return c.dynamicClient.Resource(gvr)
}

type clusterCacheEntry struct {
	obj *unstructured.Unstructured
	err error
}
