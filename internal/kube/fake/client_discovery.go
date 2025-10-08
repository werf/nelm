package fake

import (
	"github.com/chanced/caps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	discfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/testing"
)

var _ discovery.CachedDiscoveryInterface = (*CachedDiscoveryClient)(nil)

type CachedDiscoveryClient struct {
	*discfake.FakeDiscovery
}

func NewCachedDiscoveryClient() (*CachedDiscoveryClient, error) {
	discClient := &discfake.FakeDiscovery{
		Fake: &testing.Fake{},
	}

	for _, gv := range scheme.Scheme.PreferredVersionAllGroups() {
		resourceList := &metav1.APIResourceList{
			GroupVersion: gv.String(),
		}

		for kind := range scheme.Scheme.KnownTypes(gv) {
			resource := metav1.APIResource{
				Name:         caps.ToLower(kind) + "s",
				SingularName: caps.ToLower(kind),
				Namespaced:   true,
				Group:        gv.Group,
				Version:      gv.Version,
				Kind:         kind,
				Verbs:        []string{"create", "delete", "deletecollection", "get", "list", "patch", "update", "watch"},
			}

			resourceList.APIResources = append(resourceList.APIResources, resource)
		}

		discClient.Resources = append(discClient.Resources, resourceList)
	}

	return &CachedDiscoveryClient{
		FakeDiscovery: discClient,
	}, nil
}

func (c *CachedDiscoveryClient) Fresh() bool {
	return true
}

func (c *CachedDiscoveryClient) Invalidate() {}
