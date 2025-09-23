package kube

import (
	"k8s.io/client-go/dynamic"
)

func NewDynamicKubeClientFromKubeConfig(kubeConfig *KubeConfig) (*dynamic.DynamicClient, error) {
	return dynamic.NewForConfig(kubeConfig.RestConfig)
}
