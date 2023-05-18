package kubeclient

import (
	"fmt"

	"k8s.io/client-go/discovery"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewDeferredKubeDiscoveryClient(factory cmdutil.Factory) *DeferredKubeDiscoveryClient {
	return &DeferredKubeDiscoveryClient{
		factory: factory,
	}
}

type DeferredKubeDiscoveryClient struct {
	discovery.CachedDiscoveryInterface
	factory cmdutil.Factory
}

func (c *DeferredKubeDiscoveryClient) Init() error {
	discoveryClient, err := c.factory.ToDiscoveryClient()
	if err != nil {
		return fmt.Errorf("error getting discovery client: %w", err)
	}

	c.CachedDiscoveryInterface = discoveryClient

	return nil
}
