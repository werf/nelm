package kube

import (
	"fmt"

	"k8s.io/client-go/dynamic"
)

func NewDynamicKubeClientFromKubeConfig(kubeConfig *KubeConfig) (*dynamic.DynamicClient, error) {
	client, err := dynamic.NewForConfig(kubeConfig.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("new client for config: %w", err)
	}

	return client, nil
}
