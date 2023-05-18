package kubeclient

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

func NewDeferredKubeDynamicClient(factory cmdutil.Factory) *DeferredKubeDynamicClient {
	return &DeferredKubeDynamicClient{
		factory: factory,
	}
}

type DeferredKubeDynamicClient struct {
	dynamic.Interface
	factory cmdutil.Factory
}

func (c *DeferredKubeDynamicClient) Init() error {
	dynamicClient, err := c.factory.DynamicClient()
	if err != nil {
		return fmt.Errorf("error getting dynamic client: %w", err)
	}

	c.Interface = dynamicClient

	return nil
}
