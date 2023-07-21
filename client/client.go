package client

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/mutator"
	"helm.sh/helm/v3/pkg/werf/resource"
	"helm.sh/helm/v3/pkg/werf/resourcewaiter"
	"helm.sh/helm/v3/pkg/werf/util"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var addToScheme sync.Once

type Client struct {
	staticClient    kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	mapper          meta.ResettableRESTMapper

	resourceWaiter *resourcewaiter.ResourceWaiter
	// FIXME(ilya-lesikov): to duration
	waitDeletionTimeout int

	targetResourceMutators []mutator.RuntimeResourceMutator
}

func NewClient(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper meta.ResettableRESTMapper, waiter *resourcewaiter.ResourceWaiter) (*Client, error) {
	return &Client{
		staticClient:    staticClient,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		mapper:          mapper,
		resourceWaiter:  waiter,
	}, nil
}

func (c *Client) AddTargetResourceMutators(mutators ...mutator.RuntimeResourceMutator) *Client {
	c.targetResourceMutators = append(c.targetResourceMutators, mutators...)
	return c
}

func (c *Client) SetDeletionTimeout(timeout int) *Client {
	c.waitDeletionTimeout = timeout
	return c
}

func (c *Client) Get(ctx context.Context, opts GetOptions, refs ...resource.Referencer) (*GetResult, []error) {
	result := &GetResult{}

	var fallbackNamespace string
	if opts.FallbackNamespace != "" {
		fallbackNamespace = opts.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, ref := range refs {
		gvr, namespaced, err := util.ConvertGVKtoGVR(ref.GroupVersionKind(), c.mapper)
		if err != nil {
			if strings.Contains(err.Error(), "no matches for kind") {
				// FIXME(ilya-lesikov): all the Reset()'s should happen automatically right after
				// CRDs are deployed, otherwise no Reset's needed. Should we reset kubedog mapper?
				// Might be not since it is only needed for elimination?
				c.mapper.Reset()
				gvr, namespaced, err = util.ConvertGVKtoGVR(ref.GroupVersionKind(), c.mapper)
				if strings.Contains(err.Error(), "no matches for kind") {
					result.NotFound = append(result.NotFound, ref)
					continue
				}
			}

			if err != nil {
				return result, []error{fmt.Errorf("error converting kind %q to resource: %w", ref.GroupVersionKind(), err)}
			}
		}

		var namespace string
		if ref.Namespace() != "" {
			namespace = ref.Namespace()
		} else {
			namespace = fallbackNamespace
		}

		var clientResource dynamic.ResourceInterface
		if namespaced {
			clientResource = c.dynamicClient.Resource(gvr).Namespace(namespace)
		} else {
			clientResource = c.dynamicClient.Resource(gvr)
		}

		res, err := clientResource.Get(ctx, ref.Name(), metav1.GetOptions{})
		if err != nil {
			// FIXME(ilya-lesikov): does this match api resource not existing?
			if apierrors.IsNotFound(err) {
				result.NotFound = append(result.NotFound, ref)
				continue
			}

			return result, []error{fmt.Errorf("error getting resource %q: %w", ref, err)}
		}

		foundRes := struct {
			Target resource.Referencer
			Result *resource.GenericResource
		}{
			Target: ref,
			Result: resource.NewGenericResource(res),
		}

		result.Found = append(result.Found, foundRes)
	}

	return result, nil
}

func (c *Client) Create(ctx context.Context, opts CreateOptions, resources ...resource.Resourcer) (*CreateResult, []error) {
	result := &CreateResult{}

	var fallbackNamespace string
	if opts.FallbackNamespace != "" {
		fallbackNamespace = opts.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, targetRes := range resources {
		runtimeRes := targetRes.DeepCopy()

		for _, mutator := range c.targetResourceMutators {
			var err error
			runtimeRes, err = mutator.Mutate(runtimeRes, common.ClientOperationTypeCreate)
			if err != nil {
				return result, []error{fmt.Errorf("error mutating resource %q: %w", runtimeRes, err)}
			}
		}

		gvr, namespaced, err := util.ConvertGVKtoGVR(runtimeRes.GroupVersionKind(), c.mapper)
		if err != nil {
			if strings.Contains(err.Error(), "no matches for kind") {
				c.mapper.Reset()
				gvr, namespaced, err = util.ConvertGVKtoGVR(runtimeRes.GroupVersionKind(), c.mapper)
			}

			if err != nil {
				return result, []error{fmt.Errorf("error converting kind %q to resource: %w", runtimeRes.GroupVersionKind(), err)}
			}
		}

		var namespace string
		if runtimeRes.Namespace() != "" {
			namespace = runtimeRes.Namespace()
		} else {
			namespace = fallbackNamespace
		}

		var clientResource dynamic.ResourceInterface
		if namespaced {
			clientResource = c.dynamicClient.Resource(gvr).Namespace(namespace)
		} else {
			clientResource = c.dynamicClient.Resource(gvr)
		}

		if opts.Recreate {
			if err := clientResource.Delete(ctx, runtimeRes.Name(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return result, []error{fmt.Errorf("error deleting resource %q before recreating: %w", runtimeRes, err)}
			}

			if err := c.resourceWaiter.WaitDeletion(ctx, runtimeRes, resourcewaiter.WaitDeletionOptions{
				FallbackNamespace: fallbackNamespace,
				Timeout:           time.Duration(c.waitDeletionTimeout) * time.Second,
			}); err != nil {
				return result, []error{fmt.Errorf("error waiting for resource %q to be deleted before recreating: %w", runtimeRes, err)}
			}
		}

		resultObj, err := clientResource.Apply(ctx, runtimeRes.Name(), runtimeRes.Unstructured(), metav1.ApplyOptions{
			Force:        true,
			FieldManager: common.ManagedFieldsManager,
		})
		if err != nil {
			return result, []error{fmt.Errorf("error applying resource %q: %w", runtimeRes, err)}
		}

		if opts.Recreate {
			recreatedRes := struct {
				Target resource.Resourcer
				Result *resource.GenericResource
			}{
				Target: targetRes,
				Result: resource.NewGenericResource(resultObj),
			}

			result.Recreated = append(result.Recreated, recreatedRes)
		} else {
			createdRes := struct {
				Target resource.Resourcer
				Result *resource.GenericResource
			}{
				Target: targetRes,
				Result: resource.NewGenericResource(resultObj),
			}

			result.Created = append(result.Created, createdRes)
		}
	}

	return result, nil
}

func (c *Client) Update(ctx context.Context, opts UpdateOptions, resources ...resource.Resourcer) (*UpdateResult, []error) {
	result := &UpdateResult{}

	var fallbackNamespace string
	if opts.FallbackNamespace != "" {
		fallbackNamespace = opts.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, targetRes := range resources {
		runtimeRes := targetRes.DeepCopy()

		for _, mutator := range c.targetResourceMutators {
			var err error
			runtimeRes, err = mutator.Mutate(runtimeRes, common.ClientOperationTypeUpdate)
			if err != nil {
				return result, []error{fmt.Errorf("error mutating resource %q: %w", runtimeRes, err)}
			}
		}

		gvr, namespaced, err := util.ConvertGVKtoGVR(runtimeRes.GroupVersionKind(), c.mapper)
		if err != nil {
			if strings.Contains(err.Error(), "no matches for kind") {
				c.mapper.Reset()
				gvr, namespaced, err = util.ConvertGVKtoGVR(runtimeRes.GroupVersionKind(), c.mapper)
			}

			if err != nil {
				return result, []error{fmt.Errorf("error converting kind %q to resource: %w", runtimeRes.GroupVersionKind(), err)}
			}
		}

		var namespace string
		if runtimeRes.Namespace() != "" {
			namespace = runtimeRes.Namespace()
		} else {
			namespace = fallbackNamespace
		}

		var clientResource dynamic.ResourceInterface
		if namespaced {
			clientResource = c.dynamicClient.Resource(gvr).Namespace(namespace)
		} else {
			clientResource = c.dynamicClient.Resource(gvr)
		}

		var recreatedImmutable bool
		resultObj, err := clientResource.Apply(ctx, runtimeRes.Name(), runtimeRes.Unstructured(), metav1.ApplyOptions{
			Force:        true,
			FieldManager: common.ManagedFieldsManager,
		})
		if err != nil {
			gotImmutableError := apierrors.IsInvalid(err) && strings.Contains(err.Error(), "field is immutable")

			if opts.RecreateOnImmutable && gotImmutableError {
				if err := clientResource.Delete(ctx, runtimeRes.Name(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
					return result, []error{fmt.Errorf("error deleting resource %q before recreating: %w", runtimeRes, err)}
				}

				if err := c.resourceWaiter.WaitDeletion(ctx, runtimeRes, resourcewaiter.WaitDeletionOptions{
					FallbackNamespace: fallbackNamespace,
					Timeout:           time.Duration(c.waitDeletionTimeout) * time.Second,
				}); err != nil {
					return result, []error{fmt.Errorf("error waiting for resource %q to be deleted before recreating: %w", runtimeRes, err)}
				}

				resultObj, err = clientResource.Apply(ctx, runtimeRes.Name(), runtimeRes.Unstructured(), metav1.ApplyOptions{
					Force:        true,
					FieldManager: common.ManagedFieldsManager,
				})
				if err != nil {
					return result, []error{fmt.Errorf("error applying resource %q while recreating it: %w", runtimeRes, err)}
				}

				recreatedImmutable = true
			} else {
				return result, []error{fmt.Errorf("error applying resource %q: %w", runtimeRes, err)}
			}
		}

		if recreatedImmutable {
			recreatedImmutableRes := struct {
				Target resource.Resourcer
			}{
				Target: targetRes,
			}

			result.RecreatedImmutable = append(result.RecreatedImmutable, recreatedImmutableRes)
		} else {
			updatedRes := struct {
				Target resource.Resourcer
				Result *resource.GenericResource
			}{
				Target: targetRes,
				Result: resource.NewGenericResource(resultObj),
			}

			result.Updated = append(result.Updated, updatedRes)
		}
	}

	return result, nil
}

func (c *Client) SmartApply(ctx context.Context, opts SmartApplyOptions, resources ...resource.Resourcer) (*SmartApplyResult, []error) {
	result := &SmartApplyResult{}

	var fallbackNamespace string
	if opts.FallbackNamespace != "" {
		fallbackNamespace = opts.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, targetRes := range resources {
		runtimeRes := targetRes.DeepCopy()

		for _, mutator := range c.targetResourceMutators {
			var err error
			runtimeRes, err = mutator.Mutate(runtimeRes, common.ClientOperationTypeSmartApply)
			if err != nil {
				return result, []error{fmt.Errorf("error mutating resource %q: %w", runtimeRes, err)}
			}
		}

		gvr, namespaced, err := util.ConvertGVKtoGVR(runtimeRes.GroupVersionKind(), c.mapper)
		if err != nil {
			if strings.Contains(err.Error(), "no matches for kind") {
				c.mapper.Reset()
				gvr, namespaced, err = util.ConvertGVKtoGVR(runtimeRes.GroupVersionKind(), c.mapper)
				if strings.Contains(err.Error(), "no matches for kind") && opts.ContinueOnUnsupportedResource {
					unsupported := struct {
						Target resource.Resourcer
					}{
						Target: targetRes,
					}
					result.SkippedUnsupportedResource = append(result.SkippedUnsupportedResource, unsupported)
					continue
				}
			}

			if err != nil {
				return result, []error{fmt.Errorf("error converting kind %q to resource: %w", runtimeRes.GroupVersionKind(), err)}
			}
		}

		var namespace string
		if runtimeRes.Namespace() != "" {
			namespace = runtimeRes.Namespace()
		} else {
			namespace = fallbackNamespace
		}

		var clientResource dynamic.ResourceInterface
		if namespaced {
			clientResource = c.dynamicClient.Resource(gvr).Namespace(namespace)
		} else {
			clientResource = c.dynamicClient.Resource(gvr)
		}

		liveObj, err := clientResource.Get(ctx, runtimeRes.Name(), metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return result, []error{fmt.Errorf("error getting resource %q: %w", runtimeRes, err)}
		}
		found := !apierrors.IsNotFound(err)

		var recreateImmutable bool
		resultObj, err := clientResource.Apply(ctx, runtimeRes.Name(), runtimeRes.Unstructured(), metav1.ApplyOptions{
			DryRun:       []string{metav1.DryRunAll},
			Force:        true,
			FieldManager: common.ManagedFieldsManager,
		})
		if err != nil {
			gotImmutableError := apierrors.IsInvalid(err) && strings.Contains(err.Error(), "field is immutable")
			if opts.RecreateOnImmutable && gotImmutableError {
				recreateImmutable = true
			} else {
				return result, []error{fmt.Errorf("error dry-run applying resource %q: %w", runtimeRes, err)}
			}
		}

		different := compareLiveWithResult(liveObj, resultObj)

		if !opts.DryRun && recreateImmutable {
			if err := clientResource.Delete(ctx, runtimeRes.Name(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
				return result, []error{fmt.Errorf("error deleting resource %q before recreating: %w", runtimeRes, err)}
			}

			if err := c.resourceWaiter.WaitDeletion(ctx, runtimeRes, resourcewaiter.WaitDeletionOptions{
				FallbackNamespace: fallbackNamespace,
				Timeout:           time.Duration(c.waitDeletionTimeout) * time.Second,
			}); err != nil {
				return result, []error{fmt.Errorf("error waiting for resource %q to be deleted before recreating: %w", runtimeRes, err)}
			}

			resultObj, err = clientResource.Apply(ctx, runtimeRes.Name(), runtimeRes.Unstructured(), metav1.ApplyOptions{
				Force:        true,
				FieldManager: common.ManagedFieldsManager,
			})
			if err != nil {
				return result, []error{fmt.Errorf("error applying resource %q while recreating it: %w", runtimeRes, err)}
			}
		} else if !opts.DryRun && ((found && different) || !found) {
			resultObj, err = clientResource.Apply(ctx, runtimeRes.Name(), runtimeRes.Unstructured(), metav1.ApplyOptions{
				Force:        true,
				FieldManager: common.ManagedFieldsManager,
			})
			if err != nil {
				return result, []error{fmt.Errorf("error applying resource %q: %w", runtimeRes, err)}
			}
		}

		if recreateImmutable {
			recreatedImmutableRes := struct {
				Target resource.Resourcer
				Live   *resource.GenericResource
			}{
				Target: targetRes,
				Live:   resource.NewGenericResource(liveObj),
			}

			result.RecreatedImmutable = append(result.RecreatedImmutable, recreatedImmutableRes)
		} else if found && different {
			updatedRes := struct {
				Target       resource.Resourcer
				LiveOriginal *resource.GenericResource
				LiveResult   *resource.GenericResource
			}{
				Target:       targetRes,
				LiveOriginal: resource.NewGenericResource(liveObj),
				LiveResult:   resource.NewGenericResource(resultObj),
			}

			result.Updated = append(result.Updated, updatedRes)
		} else if found && !different {
			unchangedRes := struct {
				Target resource.Resourcer
				Live   *resource.GenericResource
			}{
				Target: targetRes,
				Live:   resource.NewGenericResource(liveObj),
			}

			result.Unchanged = append(result.Unchanged, unchangedRes)
		} else {
			createdRes := struct {
				Target resource.Resourcer
				Result *resource.GenericResource
			}{
				Target: targetRes,
				Result: resource.NewGenericResource(resultObj),
			}

			result.Created = append(result.Created, createdRes)
		}
	}

	return result, nil
}

func compareLiveWithResult(liveObj *unstructured.Unstructured, resultObj *unstructured.Unstructured) bool {
	filterIgnore := func(p cmp.Path) bool {
		managedFieldsTimeRegex := regexp.MustCompile(`^\{\*unstructured.Unstructured\}\.Object\["metadata"\]\.\(map\[string\]any\)\["managedFields"\]\.\(\[\]any\)\[0\]\.\(map\[string\]any\)\["time"\]$`)
		if managedFieldsTimeRegex.MatchString(p.GoString()) {
			return true
		}

		return false
	}

	different := !cmp.Equal(liveObj, resultObj, cmp.FilterPath(filterIgnore, cmp.Ignore()))

	return different
}

func (c *Client) Delete(ctx context.Context, opts DeleteOptions, refs ...resource.Referencer) (*DeleteResult, []error) {
	var resultErrs []error
	result := &DeleteResult{}

	var fallbackNamespace string
	if opts.FallbackNamespace != "" {
		fallbackNamespace = opts.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, ref := range refs {
		gvr, namespaced, err := util.ConvertGVKtoGVR(ref.GroupVersionKind(), c.mapper)
		if err != nil {
			if strings.Contains(err.Error(), "no matches for kind") {
				continue
			}

			return nil, []error{fmt.Errorf("error converting kind %q to resource: %w", ref.GroupVersionKind(), err)}
		}

		var namespace string
		if ref.Namespace() != "" {
			namespace = ref.Namespace()
		} else {
			namespace = fallbackNamespace
		}

		var clientResource dynamic.ResourceInterface
		if namespaced {
			clientResource = c.dynamicClient.Resource(gvr).Namespace(namespace)
		} else {
			clientResource = c.dynamicClient.Resource(gvr)
		}

		if err := clientResource.Delete(ctx, ref.Name(), metav1.DeleteOptions{}); err != nil {
			if apierrors.IsNotFound(err) {
				result.NotFound = append(result.NotFound, ref)
				continue
			}

			resultErrs = append(resultErrs, fmt.Errorf("error deleting resource %q: %w", ref, err))

			if opts.ContinueOnError {
				continue
			} else {
				return nil, resultErrs
			}
		}

		result.Deleted = append(result.Deleted, ref)
	}

	return result, resultErrs
}

func (c *Client) StaticClient() kubernetes.Interface {
	return c.staticClient
}

func (c *Client) DynamicClient() dynamic.Interface {
	return c.dynamicClient
}

func (c *Client) DiscoveryClient() discovery.CachedDiscoveryInterface {
	return c.discoveryClient
}

func (c *Client) DiscoveryRESTMapper() meta.ResettableRESTMapper {
	return c.mapper
}

type GetOptions struct {
	FallbackNamespace string
}

type CreateOptions struct {
	FallbackNamespace string
	Recreate          bool
}

type UpdateOptions struct {
	FallbackNamespace   string
	RecreateOnImmutable bool
}

type SmartApplyOptions struct {
	FallbackNamespace             string
	RecreateOnImmutable           bool
	ContinueOnUnsupportedResource bool
	DryRun                        bool
}

type DeleteOptions struct {
	FallbackNamespace string
	ContinueOnError   bool
}

type GetResult struct {
	Found []struct {
		Target resource.Referencer
		Result *resource.GenericResource
	}
	NotFound []resource.Referencer
}

type CreateResult struct {
	Created []struct {
		Target resource.Resourcer
		Result *resource.GenericResource
	}
	Recreated []struct {
		Target resource.Resourcer
		Result *resource.GenericResource
	}
}

type UpdateResult struct {
	Updated []struct {
		Target resource.Resourcer
		Result *resource.GenericResource
	}
	RecreatedImmutable []struct {
		Target resource.Resourcer
	}
}

type SmartApplyResult struct {
	Created []struct {
		Target resource.Resourcer
		Result *resource.GenericResource
	}
	Updated []struct {
		Target       resource.Resourcer
		LiveOriginal *resource.GenericResource
		LiveResult   *resource.GenericResource
	}
	RecreatedImmutable []struct {
		Target resource.Resourcer
		Live   *resource.GenericResource
	}
	SkippedUnsupportedResource []struct {
		Target resource.Resourcer
	}
	Unchanged []struct {
		Target resource.Resourcer
		Live   *resource.GenericResource
	}
}

type DeleteResult struct {
	Deleted  []resource.Referencer
	NotFound []resource.Referencer
}
