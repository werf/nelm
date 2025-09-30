package kube

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubectl/pkg/cmd/testing"
)

func NewFakeDiscoveryClient() *testing.FakeCachedDiscoveryClient {
	discoveryClient := testing.NewFakeCachedDiscoveryClient()
	discoveryClient.Groups = []*metav1.APIGroup{
		{
			Name: "",
			PreferredVersion: metav1.GroupVersionForDiscovery{
				GroupVersion: "v1", Version: "v1",
			},
			Versions: []metav1.GroupVersionForDiscovery{
				{GroupVersion: "v1", Version: "v1"},
			},
		},
		{
			Name: "apps",
			PreferredVersion: metav1.GroupVersionForDiscovery{
				GroupVersion: "apps/v1", Version: "v1",
			},
			Versions: []metav1.GroupVersionForDiscovery{
				{GroupVersion: "apps/v1", Version: "v1"},
			},
		},
	}
	discoveryClient.PreferredResources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "configmaps",
					SingularName: "configmap",
					Namespaced:   true,
					Version:      "v1",
					Kind:         "ConfigMap",
					Verbs:        []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
				},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{
					Name:         "deployments",
					SingularName: "deployment",
					Namespaced:   true,
					Group:        "apps",
					Version:      "v1",
					Kind:         "Deployment",
					Verbs:        []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
				},
			},
		},
	}
	discoveryClient.Resources = discoveryClient.PreferredResources

	return discoveryClient
}
