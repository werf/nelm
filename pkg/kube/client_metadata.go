package kube

import (
	"fmt"

	"k8s.io/client-go/metadata"
)

func NewMetadataKubeClientFromKubeConfig(kubeConfig *KubeConfig) (metadata.Interface, error) {
	client, err := metadata.NewForConfig(kubeConfig.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("new metadata client for config: %w", err)
	}

	return client, nil
}
