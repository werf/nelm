package kube

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
)

func NewStaticKubeClientFromKubeConfig(kubeConfig *KubeConfig) (*kubernetes.Clientset, error) {
	client, err := kubernetes.NewForConfig(kubeConfig.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("new kube client for config: %w", err)
	}

	return client, nil
}
