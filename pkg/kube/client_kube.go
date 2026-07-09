package kube

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"github.com/samber/lo"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kvwait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
	"github.com/werf/nelm/pkg/resource/spec"
)

var _ KubeClienter = (*KubeClient)(nil)

type KubeClienter interface {
	Get(ctx context.Context, meta *spec.ResourceMeta, opts KubeClientGetOptions) (*unstructured.Unstructured, error)
	Create(ctx context.Context, spec *spec.ResourceSpec, opts KubeClientCreateOptions) (*unstructured.Unstructured, error)
	Apply(ctx context.Context, spec *spec.ResourceSpec, opts KubeClientApplyOptions) (*unstructured.Unstructured, error)
	MergePatch(ctx context.Context, meta *spec.ResourceMeta, patch []byte, opts KubeClientMergePatchOptions) (*unstructured.Unstructured, error)
	Delete(ctx context.Context, meta *spec.ResourceMeta, opts KubeClientDeleteOptions) error
	GVKToGVR(ctx context.Context, gvk schema.GroupVersionKind) (schema.GroupVersionResource, bool, error)
	Namespaced(ctx context.Context, gvk schema.GroupVersionKind) (bool, error)
	ResetDiscoveryCache(ctx context.Context) error
	ServerVersion(ctx context.Context) (*version.Info, error)
	ResetAndRetryOnUnknownGVR(ctx context.Context, fn func() error) error
}

// High-level Kubernetes Client. Always prefer using it instead of static/dynamic Kubernetes
// go-client directly. Provides caching, which works as long as there is no other client or other
// program modifying Kubernetes resources that we work with through this client.
type KubeClient struct {
	clusterCache               *ttlcache.Cache[string, *clusterCacheEntry]
	discoveryClient            discovery.CachedDiscoveryInterface
	dynamicClient              dynamic.Interface
	mapper                     apimeta.ResettableRESTMapper
	mapperNoMatchRetryInterval time.Duration
	mapperNoMatchRetryTimeout  time.Duration
	mapperRefreshLock          *sync.RWMutex
	resourceLocks              *sync.Map
	staticClient               kubernetes.Interface
}

func NewKubeClient(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper apimeta.ResettableRESTMapper) *KubeClient {
	clusterCache := ttlcache.New(
		ttlcache.WithDisableTouchOnHit[string, *clusterCacheEntry](),
	)

	return &KubeClient{
		clusterCache:               clusterCache,
		discoveryClient:            discoveryClient,
		dynamicClient:              dynamicClient,
		mapper:                     mapper,
		mapperNoMatchRetryInterval: common.DefaultMapperNoMatchRetryInterval,
		mapperNoMatchRetryTimeout:  common.DefaultMapperNoMatchRetryTimeout,
		mapperRefreshLock:          &sync.RWMutex{},
		resourceLocks:              &sync.Map{},
		staticClient:               staticClient,
	}
}

func (c *KubeClient) Apply(ctx context.Context, resSpec *spec.ResourceSpec, opts KubeClientApplyOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resSpec.ResourceMeta)

	lock.Lock()
	defer lock.Unlock()

	var (
		gvr        schema.GroupVersionResource
		namespaced bool
		err        error
	)
	if err := c.ResetAndRetryOnUnknownGVR(ctx, func() error {
		var err error

		gvr, namespaced, err = c.GVKToGVR(ctx, resSpec.GroupVersionKind)

		return err
	}); err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, resSpec.Namespace, opts.DefaultNamespace, namespaced)

	var dryRun []string
	if opts.DryRun {
		dryRun = []string{metav1.DryRunAll}
	}

	log.Default.Debug(ctx, "Server-side %sapplying resource %q", lo.Ternary(opts.DryRun, "dry-run ", ""), resSpec.IDHuman())

	var resultObj *unstructured.Unstructured

	applyFn := func() error {
		var applyErr error

		resultObj, applyErr = clientResource.Apply(ctx, resSpec.Name, resSpec.Unstruct, metav1.ApplyOptions{
			DryRun:       dryRun,
			Force:        true,
			FieldManager: common.DefaultFieldManager,
		})
		if applyErr != nil {
			return fmt.Errorf("server-side apply: %w", applyErr)
		}

		return nil
	}

	if opts.RetryOnWebhookError {
		err = retryOnWebhookErr(ctx, applyFn)
	} else {
		err = applyFn()
	}

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
		if err := c.waitForCRDDiscoverability(ctx, resSpec); err != nil {
			return nil, fmt.Errorf("wait for CRD discoverability: %w", err)
		}
	}

	log.Default.TraceStruct(ctx, resultObj, "Server-side %sapplied resource %q via Kubernetes API:", lo.Ternary(opts.DryRun, "dry-run ", ""), resSpec.IDHuman())

	return resultObj, nil
}

func (c *KubeClient) Create(ctx context.Context, resSpec *spec.ResourceSpec, opts KubeClientCreateOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resSpec.ResourceMeta)

	lock.Lock()
	defer lock.Unlock()

	var (
		gvr        schema.GroupVersionResource
		namespaced bool
		err        error
	)
	if err := c.ResetAndRetryOnUnknownGVR(ctx, func() error {
		var err error

		gvr, namespaced, err = c.GVKToGVR(ctx, resSpec.GroupVersionKind)

		return err
	}); err != nil {
		return nil, fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, resSpec.Namespace, opts.DefaultNamespace, namespaced)

	if opts.ForceReplicas != nil {
		if err := unstructured.SetNestedField(resSpec.Unstruct.UnstructuredContent(), int64(*opts.ForceReplicas), "spec", "replicas"); err != nil {
			return nil, fmt.Errorf("set spec.replicas for resource %q: %w", resSpec.IDHuman(), err)
		}
	}

	log.Default.Debug(ctx, "Server-side applying resource %q", resSpec.IDHuman())

	var resultObj *unstructured.Unstructured

	createFn := func() error {
		var createErr error

		resultObj, createErr = clientResource.Apply(ctx, resSpec.Name, resSpec.Unstruct, metav1.ApplyOptions{
			Force:        true,
			FieldManager: common.DefaultFieldManager,
		})
		if createErr != nil {
			return fmt.Errorf("server-side apply: %w", createErr)
		}

		return nil
	}

	if opts.RetryOnWebhookError {
		err = retryOnWebhookErr(ctx, createFn)
	} else {
		err = createFn()
	}

	if err != nil {
		c.clusterCache.Set(resSpec.IDWithVersion(), &clusterCacheEntry{err: err}, 0)

		return nil, fmt.Errorf("server-side apply resource %q: %w", resSpec.IDHuman(), err)
	}

	c.clusterCache.Set(resSpec.IDWithVersion(), &clusterCacheEntry{obj: resultObj.DeepCopy()}, 0)

	if spec.IsCRDFromGR(gvr.GroupResource()) {
		if err := c.waitForCRDDiscoverability(ctx, resSpec); err != nil {
			return nil, fmt.Errorf("wait for CRD discoverability: %w", err)
		}
	}

	log.Default.TraceStruct(ctx, resultObj, "Created resource %q via Kubernetes API:", resSpec.IDHuman())

	return resultObj, nil
}

func (c *KubeClient) Delete(ctx context.Context, resMeta *spec.ResourceMeta, opts KubeClientDeleteOptions) error {
	lock := c.resourceLock(resMeta)

	lock.Lock()
	defer lock.Unlock()

	var (
		gvr        schema.GroupVersionResource
		namespaced bool
	)

	if err := c.ResetAndRetryOnUnknownGVR(ctx, func() error {
		var err error

		gvr, namespaced, err = c.GVKToGVR(ctx, resMeta.GroupVersionKind)

		return err
	}); err != nil {
		return fmt.Errorf("convert GVK to GVR: %w", err)
	}

	clientResource := c.clientResource(gvr, resMeta.Namespace, opts.DefaultNamespace, namespaced)

	var propagationPolicy metav1.DeletionPropagation
	if opts.PropagationPolicy != "" {
		propagationPolicy = opts.PropagationPolicy
	} else {
		propagationPolicy = common.DefaultDeletePropagation
	}

	log.Default.Debug(ctx, "Deleting resource %q", resMeta.IDHuman())

	if err := clientResource.Delete(ctx, resMeta.Name, metav1.DeleteOptions{
		PropagationPolicy: lo.ToPtr(propagationPolicy),
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

func (c *KubeClient) GVKToGVR(_ context.Context, gvk schema.GroupVersionKind) (schema.GroupVersionResource, bool, error) {
	c.mapperRefreshLock.RLock()
	defer c.mapperRefreshLock.RUnlock()

	gvr, namespaced, err := spec.GVKtoGVR(gvk, c.mapper)
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("get GVK to GVR mapping: %w", err)
	}

	return gvr, namespaced, nil
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

	var (
		gvr        schema.GroupVersionResource
		namespaced bool
	)

	if err := c.ResetAndRetryOnUnknownGVR(ctx, func() error {
		var err error

		gvr, namespaced, err = c.GVKToGVR(ctx, resMeta.GroupVersionKind)

		return err
	}); err != nil {
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

func (c *KubeClient) MergePatch(ctx context.Context, resMeta *spec.ResourceMeta, patch []byte, opts KubeClientMergePatchOptions) (*unstructured.Unstructured, error) {
	lock := c.resourceLock(resMeta)

	lock.Lock()
	defer lock.Unlock()

	var (
		gvr        schema.GroupVersionResource
		namespaced bool
	)

	if err := c.ResetAndRetryOnUnknownGVR(ctx, func() error {
		var err error

		gvr, namespaced, err = c.GVKToGVR(ctx, resMeta.GroupVersionKind)

		return err
	}); err != nil {
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

func (c *KubeClient) Namespaced(_ context.Context, gvk schema.GroupVersionKind) (bool, error) {
	c.mapperRefreshLock.RLock()
	defer c.mapperRefreshLock.RUnlock()

	namespaced, err := spec.Namespaced(gvk, c.mapper)
	if err != nil {
		return false, fmt.Errorf("check resource scope: %w", err)
	}

	return namespaced, nil
}

func (c *KubeClient) ResetAndRetryOnUnknownGVR(ctx context.Context, fn func() error) error {
	firstErr := fn()
	if firstErr == nil {
		return nil
	}

	if !apimeta.IsNoMatchError(firstErr) {
		return firstErr
	}

	var resultErr error

	func() {
		retryCtx, cancel := context.WithTimeout(ctx, c.mapperNoMatchRetryTimeout)
		defer cancel()

		lastErr := firstErr

		if err := c.ResetDiscoveryCache(retryCtx); err != nil {
			resultErr = fmt.Errorf("reset discovery cache after mapper NoMatch: %w", err)

			return
		}

		if err := kvwait.PollUntilContextCancel(retryCtx, c.mapperNoMatchRetryInterval, false, func(ctx context.Context) (bool, error) {
			if err := fn(); err != nil {
				lastErr = err
				if apimeta.IsNoMatchError(err) {
					if err := c.ResetDiscoveryCache(ctx); err != nil {
						return false, fmt.Errorf("reset discovery cache after mapper NoMatch: %w", err)
					}

					return false, nil
				}

				return false, err
			}

			return true, nil
		}); err != nil {
			if retryCtx.Err() != nil {
				if ctx.Err() != nil {
					resultErr = fmt.Errorf("retry mapper NoMatch: %w", context.Cause(ctx))

					return
				}

				resultErr = fmt.Errorf("retry mapper NoMatch timed out after %s: %w", c.mapperNoMatchRetryTimeout.String(), lastErr)

				return
			}

			resultErr = err
		}
	}()

	return resultErr
}

func (c *KubeClient) ResetDiscoveryCache(_ context.Context) error {
	c.mapperRefreshLock.Lock()
	defer c.mapperRefreshLock.Unlock()

	c.resetDiscoveryCacheNoLock()

	return nil
}

func (c *KubeClient) ServerVersion(_ context.Context) (*version.Info, error) {
	versionInfo, err := c.discoveryClient.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("get kubernetes server version: %w", err)
	}

	return versionInfo, nil
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

func (c *KubeClient) resetDiscoveryCacheNoLock() {
	c.discoveryClient.Invalidate()
	c.mapper.Reset()
}

func (c *KubeClient) resourceLock(meta *spec.ResourceMeta) *sync.Mutex {
	lock, _ := c.resourceLocks.LoadOrStore(meta.IDWithVersion(), &sync.Mutex{})

	return lock.(*sync.Mutex)
}

func (c *KubeClient) waitForCRDDiscoverability(ctx context.Context, resSpec *spec.ResourceSpec) error {
	servedGVKs, err := servedCRDGVKs(resSpec.Unstruct)
	if err != nil {
		return fmt.Errorf("extract served GVKs: %w", err)
	}

	if err := c.ResetDiscoveryCache(ctx); err != nil {
		return fmt.Errorf("refresh discovery: %w", err)
	}

	for _, gvk := range servedGVKs {
		if err := c.ResetAndRetryOnUnknownGVR(ctx, func() error {
			_, _, err := c.GVKToGVR(ctx, gvk)

			return err
		}); err != nil {
			return fmt.Errorf("resolve served GVK %q: %w", gvk.String(), err)
		}
	}

	return nil
}

type KubeClientGetOptions struct {
	DefaultNamespace string
	TryCache         bool
}

type KubeClientCreateOptions struct {
	DefaultNamespace    string
	ForceReplicas       *int
	RetryOnWebhookError bool
}

type KubeClientApplyOptions struct {
	DefaultNamespace    string
	DryRun              bool
	RetryOnWebhookError bool
}

type KubeClientMergePatchOptions struct {
	DefaultNamespace string
}

type KubeClientDeleteOptions struct {
	DefaultNamespace  string
	PropagationPolicy metav1.DeletionPropagation
}

type clusterCacheEntry struct {
	err error
	obj *unstructured.Unstructured
}

func retryOnWebhookErr(ctx context.Context, fn func() error) error {
	retryCtx, cancel := context.WithTimeoutCause(ctx, common.DefaultWebhookRetryTimeout, fmt.Errorf("context timed out: webhook retry timed out after %s", common.DefaultWebhookRetryTimeout.String()))
	defer cancel()

	var lastErr error
	if err := kvwait.PollUntilContextCancel(retryCtx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		lastErr = fn()
		if lastErr != nil {
			if IsWebhookErr(lastErr) {
				log.Default.Debug(ctx, "Retrying due to webhook error: %s", lastErr)

				return false, nil
			}

			return false, lastErr
		}

		return true, nil
	}); err != nil {
		if retryCtx.Err() != nil {
			return fmt.Errorf("retryable on webhook error: %w due to: %w", context.Cause(retryCtx), lastErr)
		}

		return fmt.Errorf("retryable on webhook error: %w", err)
	}

	return nil
}

func servedCRDGVKs(obj *unstructured.Unstructured) ([]schema.GroupVersionKind, error) {
	group, found, err := unstructured.NestedString(obj.UnstructuredContent(), "spec", "group")
	if err != nil {
		return nil, fmt.Errorf("get spec.group: %w", err)
	}

	if !found || group == "" {
		return nil, fmt.Errorf("get spec.group: value not found")
	}

	kind, found, err := unstructured.NestedString(obj.UnstructuredContent(), "spec", "names", "kind")
	if err != nil {
		return nil, fmt.Errorf("get spec.names.kind: %w", err)
	}

	if !found || kind == "" {
		return nil, fmt.Errorf("get spec.names.kind: value not found")
	}

	var result []schema.GroupVersionKind

	versions, found, err := unstructured.NestedSlice(obj.UnstructuredContent(), "spec", "versions")
	if err != nil {
		return nil, fmt.Errorf("get spec.versions: %w", err)
	}

	if found {
		for i, rawVersion := range versions {
			versionMap, ok := rawVersion.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("get spec.versions[%d]: expected object", i)
			}

			served, _, err := unstructured.NestedBool(versionMap, "served")
			if err != nil {
				return nil, fmt.Errorf("get spec.versions[%d].served: %w", i, err)
			}

			if !served {
				continue
			}

			version, found, err := unstructured.NestedString(versionMap, "name")
			if err != nil {
				return nil, fmt.Errorf("get spec.versions[%d].name: %w", i, err)
			}

			if !found || version == "" {
				return nil, fmt.Errorf("get spec.versions[%d].name: value not found", i)
			}

			result = append(result, schema.GroupVersionKind{Group: group, Version: version, Kind: kind})
		}
	} else {
		version, found, err := unstructured.NestedString(obj.UnstructuredContent(), "spec", "version")
		if err != nil {
			return nil, fmt.Errorf("get spec.version: %w", err)
		}

		if found && version != "" {
			result = append(result, schema.GroupVersionKind{Group: group, Version: version, Kind: kind})
		}
	}

	return result, nil
}
