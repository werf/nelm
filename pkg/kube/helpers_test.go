package kube

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	dynfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func newTestKubeClientWithConfigMap() (*KubeClient, *dynfake.FakeDynamicClient, *spec.ResourceSpec) {
	client, _, _ := newTestKubeClient(0)
	dynamicClient := dynfake.NewSimpleDynamicClient(scheme.Scheme, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	})
	client.dynamicClient = dynamicClient

	resSpec := spec.NewResourceSpec(&unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "cm", "namespace": "default"},
	}}, "default", spec.ResourceSpecOptions{})

	return client, dynamicClient, resSpec
}
