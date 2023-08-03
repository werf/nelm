package kubeclientv2

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func NewClient(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper meta.ResettableRESTMapper, opts NewKubeClientOptions) *Client {
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
		fieldManager:           fieldManager,
		createResourceMutators: opts.CreateResourceMutators,
		applyResourceMutators:  opts.ApplyResourceMutators,
		staticClient:           staticClient,
		dynamicClient:          dynamicClient,
		discoveryClient:        discoveryClient,
		mapper:                 mapper,
		logger:                 logger,
	}
}

type NewKubeClientOptions struct {
	Logger                 log.Logger
	FieldManager           string
	CreateResourceMutators []CreateResourceMutator
	ApplyResourceMutators  []ApplyResourceMutator
}

type Client struct {
	fieldManager           string
	createResourceMutators []CreateResourceMutator
	applyResourceMutators  []ApplyResourceMutator
	staticClient           kubernetes.Interface
	dynamicClient          dynamic.Interface
	discoveryClient        discovery.CachedDiscoveryInterface
	mapper                 meta.ResettableRESTMapper
	logger                 log.Logger
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

func (c *Client) Create(ctx context.Context, res Creatable, opts CreateOptions) (*unstructured.Unstructured, error) {
	gvr, namespaced, err := util.ConvertGVKtoGVR(res.GroupVersionKind(), c.mapper)
	if err != nil {
		return nil, fmt.Errorf("error mapping GroupVersionKind %q to GroupVersionResource: %w", res.GroupVersionKind(), err)
	}

	namespace := c.resourceNamespace(res.Namespace(), opts.FallbackNamespace)
	clientResource := c.clientResource(gvr, namespaced, namespace)

	var target *unstructured.Unstructured
	if len(c.createResourceMutators) > 0 {
		target = res.Unstructured().DeepCopy()

		for _, mutator := range c.createResourceMutators {
			if err = mutator.Mutate(ctx, res, target); err != nil {
				return nil, fmt.Errorf("error mutating resource %q: %w", res.String(), err)
			}
		}
	} else {
		target = res.Unstructured()
	}

	c.logger.Debug("Creating resource %q ...", res.String())
	resultObj, err := clientResource.Create(ctx, target, metav1.CreateOptions{
		FieldManager: c.fieldManager,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating resource %q: %w", res.String(), err)
	}
	c.logger.Debug("Created resource %q", res.String())

	return resultObj, nil
}

func (c *Client) Apply(ctx context.Context, res Appliable, opts ApplyOptions) (*unstructured.Unstructured, error) {
	gvr, namespaced, err := util.ConvertGVKtoGVR(res.GroupVersionKind(), c.mapper)
	if err != nil {
		return nil, fmt.Errorf("error mapping GroupVersionKind %q to GroupVersionResource: %w", res.GroupVersionKind(), err)
	}

	namespace := c.resourceNamespace(res.Namespace(), opts.FallbackNamespace)
	clientResource := c.clientResource(gvr, namespaced, namespace)

	var dryRun []string
	if opts.DryRun {
		dryRun = []string{metav1.DryRunAll}
	}

	var target *unstructured.Unstructured
	if len(c.applyResourceMutators) > 0 {
		target = res.Unstructured().DeepCopy()

		for _, mutator := range c.applyResourceMutators {
			if err = mutator.Mutate(ctx, res, target); err != nil {
				return nil, fmt.Errorf("error mutating resource %q: %w", res.String(), err)
			}
		}
	} else {
		target = res.Unstructured()
	}

	c.logger.Debug("Applying resource %q ...", res.String())
	resultObj, err := clientResource.Apply(ctx, res.Name(), target, metav1.ApplyOptions{
		DryRun:       dryRun,
		Force:        true,
		FieldManager: c.fieldManager,
	})
	if err != nil {
		return nil, fmt.Errorf("error applying resource %q: %w", res.String(), err)
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

type Creatable interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	Unstructured() *unstructured.Unstructured
	PartOfRelease() bool
	ShouldHaveServiceMetadata() bool
	DefaultReplicasOnCreation() (replicas int, set bool)
	String() string
}

type Appliable interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	Unstructured() *unstructured.Unstructured
	PartOfRelease() bool
	ShouldHaveServiceMetadata() bool
	String() string
}

type CreateResourceMutator interface {
	Mutate(ctx context.Context, info CreateMutatableInfo, target *unstructured.Unstructured) error
}

type ApplyResourceMutator interface {
	Mutate(ctx context.Context, info ApplyMutatableInfo, target *unstructured.Unstructured) error
}

type CreateMutatableInfo interface {
	PartOfRelease() bool
	ShouldHaveServiceMetadata() bool
	DefaultReplicasOnCreation() (replicas int, set bool)
	String() string
}

type ApplyMutatableInfo interface {
	PartOfRelease() bool
	ShouldHaveServiceMetadata() bool
	String() string
}

type GetOptions struct {
	FallbackNamespace string
}

type CreateOptions struct {
	FallbackNamespace string
}

type ApplyOptions struct {
	FallbackNamespace string
	DryRun            bool
}
