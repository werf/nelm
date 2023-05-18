package kubeclient

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewDeferredKubeStaticClient(factory cmdutil.Factory) *DeferredKubeStaticClient {
	return &DeferredKubeStaticClient{
		factory: factory,
	}
}

type DeferredKubeStaticClient struct {
	kubernetes.Interface
	factory cmdutil.Factory
}

func (c *DeferredKubeStaticClient) Init() error {
	staticClient, err := c.factory.KubernetesClientSet()
	if err != nil {
		return fmt.Errorf("error getting typed client: %w", err)
	}

	c.Interface = staticClient

	return nil
}
