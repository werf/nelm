package kubeclientv2

import (
	"context"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/resourcewaiter"
	"helm.sh/helm/v3/pkg/werf/util"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func NewClient(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper meta.ResettableRESTMapper, waiter Waiter, opts NewKubeClientOptions) *Client {
	var logger log.Logger
	if opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = log.NewNullLogger()
	}

	var fieldManager string
	if opts.FieldManager != "" {
		fieldManager = opts.FieldManager
	} else {
		fieldManager = common.DefaultFieldManager
	}

	return &Client{
		staticClient:    staticClient,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		mapper:          mapper,
		waiter:          waiter,

		fieldManager: fieldManager,

		logger: logger,
	}
}

type NewKubeClientOptions struct {
	Logger       log.Logger
	FieldManager string
}

type Client struct {
	staticClient    kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	mapper          meta.ResettableRESTMapper
	waiter          Waiter

	fieldManager string

	logger log.Logger
}

func (c *Client) Get(ctx context.Context, res Gettable, opts GetOptions) (*unstructured.Unstructured, error) {
	gvr, namespaced, err := util.ConvertGVKtoGVR(res.GroupVersionKind(), c.mapper)
	if err != nil {
		return nil, fmt.Errorf("error mapping GroupVersionKind %q to GroupVersionResource: %w", res.GroupVersionKind(), err)
	}

	namespace := c.resourceNamespace(res.Namespace(), opts.FallbackNamespace)
	clientResource := c.clientResource(gvr, namespaced, namespace)

	c.logger.Debug("Getting resource %q ...", res.String())
	r, err := clientResource.Get(ctx, res.Name(), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting resource %q: %w", res.String(), err)
	}
	c.logger.Debug("Got resource %q", res.String())

	return r, nil
}

func (c *Client) Apply(ctx context.Context, res Appliable, opts ApplyOptions) (*unstructured.Unstructured, error) {
	gvr, namespaced, err := util.ConvertGVKtoGVR(res.GroupVersionKind(), c.mapper)
	if err != nil {
		return nil, fmt.Errorf("error mapping GroupVersionKind %q to GroupVersionResource: %w", res.GroupVersionKind(), err)
	}

	namespace := c.resourceNamespace(res.Namespace(), opts.FallbackNamespace)
	clientResource := c.clientResource(gvr, namespaced, namespace)

	if opts.DryRun {
		c.logger.Debug("Dry-run applying resource %q ...", res.String())
		resultObj, err := clientResource.Apply(ctx, res.Name(), res.Unstructured(), metav1.ApplyOptions{
			DryRun:       []string{metav1.DryRunAll},
			Force:        true,
			FieldManager: c.fieldManager,
		})
		if err != nil {
			return nil, fmt.Errorf("error dry-run applying resource %q: %w", res.String(), err)
		}
		c.logger.Debug("Dry-run applied resource %q", res.String())

		return resultObj, nil
	}

	c.logger.Debug("Applying resource %q ...", res.String())
	resultObj, err := clientResource.Apply(ctx, res.Name(), res.Unstructured(), metav1.ApplyOptions{
		Force:        true,
		FieldManager: c.fieldManager,
	})
	if err != nil {
		gotImmutableError := apierrors.IsInvalid(err) && strings.Contains(err.Error(), "field is immutable")
		// FIXME(ilya-lesikov): parse from annotations? different defaults?
		recreateIfImmutable := false

		if !recreateIfImmutable || !gotImmutableError {
			return nil, fmt.Errorf("error applying resource %q: %w", res.String(), err)
		}

		c.logger.Info("Resource %q is immutable, recreating ...", res.String())

		c.logger.Debug("Deleting resource %q ...", res.String())
		if err := clientResource.Delete(ctx, res.Name(), metav1.DeleteOptions{}); apierrors.IsNotFound(err) {
			c.logger.Debug("Resource %q not found, skipping deletion", res.String())
		} else if err != nil {
			return nil, fmt.Errorf("error deleting resource %q: %w", res.String(), err)
		} else {
			c.logger.Debug("Deleted resource %q", res.String())
		}

		c.logger.Debug("Waiting for resource %q deletion ...", res.String())
		if err := c.waiter.WaitDeletion(ctx, res, resourcewaiter.WaitDeletionOptions{}); err != nil {
			return nil, fmt.Errorf("error waiting for resource %q deletion: %w", res.String(), err)
		}
		c.logger.Debug("Waited for resource %q deletion", res.String())

		c.logger.Debug("Applying resource %q ...", res.String())
		resultObj, err = clientResource.Apply(ctx, res.Name(), res.Unstructured(), metav1.ApplyOptions{
			Force:        true,
			FieldManager: c.fieldManager,
		})
		if err != nil {
			return nil, fmt.Errorf("error applying resource %q: %w", res.String(), err)
		}
	}
	c.logger.Debug("Applied resource %q", res.String())

	return resultObj, nil
}

func (c *Client) resourceNamespace(namespace, fallbackNamespace string) string {
	var result string
	if namespace != "" {
		result = namespace
	} else if fallbackNamespace != "" {
		result = fallbackNamespace
	} else {
		result = v1.NamespaceDefault
	}

	return result
}

func (c *Client) clientResource(gvr schema.GroupVersionResource, namespaced bool, namespace string) dynamic.ResourceInterface {
	if namespaced {
		return c.dynamicClient.Resource(gvr).Namespace(namespace)
	}

	return c.dynamicClient.Resource(gvr)
}

type Gettable interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	String() string
}

type Appliable interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	Unstructured() *unstructured.Unstructured
	String() string
}

type Waiter interface {
	WaitDeletion(ctx context.Context, res resourcewaiter.Waitable, opts resourcewaiter.WaitDeletionOptions) error
}

type GetOptions struct {
	FallbackNamespace string
}

type ApplyOptions struct {
	FallbackNamespace string
	DryRun            bool
}
