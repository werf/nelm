package kubeclient

import (
	"errors"
	"fmt"
	sync "sync"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var addToScheme sync.Once

func NewDeferredKubeClient(getter genericclioptions.RESTClientGetter) *DeferredKubeClient {
	if getter == nil {
		getter = genericclioptions.NewConfigFlags(true)
	}

	addToScheme.Do(func() {
		if err := apiextv1.AddToScheme(scheme.Scheme); err != nil {
			panic(err)
		}
		if err := apiextv1beta1.AddToScheme(scheme.Scheme); err != nil {
			panic(err)
		}
	})

	factory := cmdutil.NewFactory(getter)

	deferredKubeStaticClient := NewDeferredKubeStaticClient(factory)
	deferredKubeDynamicClient := NewDeferredKubeDynamicClient(factory)
	deferredKubeDiscoveryClient := NewDeferredKubeDiscoveryClient(factory)
	deferredKubeMapper := NewDeferredKubeMapper(factory)

	return &DeferredKubeClient{
		factory:         factory,
		staticClient:    deferredKubeStaticClient,
		dynamicClient:   deferredKubeDynamicClient,
		discoveryClient: deferredKubeDiscoveryClient,
		mapper:          deferredKubeMapper,
	}
}

type DeferredKubeClient struct {
	factory         cmdutil.Factory
	staticClient    *DeferredKubeStaticClient
	dynamicClient   *DeferredKubeDynamicClient
	discoveryClient *DeferredKubeDiscoveryClient
	mapper          *DeferredKubeMapper
}

func (c *DeferredKubeClient) Init() error {
	if err := checkClusterConnectivity(c.factory); err != nil {
		return fmt.Errorf("Kubernetes cluster unreachable: %w", err)
	}

	if err := c.staticClient.Init(); err != nil {
		return fmt.Errorf("error initializing deferred static kube client: %w", err)
	}

	if err := c.dynamicClient.Init(); err != nil {
		return fmt.Errorf("error initializing deferred dynamic kube client: %w", err)
	}

	if err := c.discoveryClient.Init(); err != nil {
		return fmt.Errorf("error initializing deferred discovery kube client: %w", err)
	}

	if err := c.mapper.Init(); err != nil {
		return fmt.Errorf("error initializing deferred kube mapper: %w", err)
	}

	return nil
}

func (c *DeferredKubeClient) Static() *DeferredKubeStaticClient {
	return c.staticClient
}

func (c *DeferredKubeClient) Dynamic() *DeferredKubeDynamicClient {
	return c.dynamicClient
}

func (c *DeferredKubeClient) Discovery() *DeferredKubeDiscoveryClient {
	return c.discoveryClient
}

func (c *DeferredKubeClient) Mapper() *DeferredKubeMapper {
	return c.mapper
}

func checkClusterConnectivity(factory cmdutil.Factory) error {
	client, err := factory.KubernetesClientSet()
	if err != nil {
		if err == genericclioptions.ErrEmptyConfig {
			return errors.New("incomplete configuration")
		}
		return fmt.Errorf("error getting kubernetes client: %w", err)
	}

	if _, err := client.ServerVersion(); err != nil {
		return fmt.Errorf("error getting kubernetes server version: %w", err)
	}

	return nil
}
