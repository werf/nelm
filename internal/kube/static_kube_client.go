package kube

import (
	"k8s.io/client-go/kubernetes"
)

func NewStaticKubeClientFromKubeConfig(kubeConfig *KubeConfig) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(kubeConfig.RestConfig)
}
