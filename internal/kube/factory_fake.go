package kube

import (
	"context"
	"reflect"

	"github.com/samber/lo"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubectl/pkg/cmd/testing"
)

var _ ClientFactorier = (*FakeClientFactory)(nil)

type FakeClientFactory struct {
	discoveryClient discovery.CachedDiscoveryInterface
	dynamicClient   dynamic.Interface
	kubeClient      KubeClienter
	mapper          meta.ResettableRESTMapper
	staticClient    kubernetes.Interface
}

func NewFakeClientFactory(ctx context.Context) *FakeClientFactory {
	addToScheme.Do(func() {
		lo.Must0(apiextv1.AddToScheme(scheme.Scheme))
		lo.Must0(apiextv1beta1.AddToScheme(scheme.Scheme))
	})

	staticClient := fake.NewSimpleClientset()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme.Scheme)

	discoveryClient := testing.NewFakeCachedDiscoveryClient()
	discGroups, discResources := lo.Must2(staticClient.Discovery().ServerGroupsAndResources())
	discoveryClient.Groups = discGroups
	discoveryClient.Resources = discResources
	discoveryClient.PreferredResources = lo.Must(staticClient.Discovery().ServerPreferredResources())

	mapper := reflect.ValueOf(NewKubeMapper(ctx, discoveryClient)).Interface().(meta.ResettableRESTMapper)
	kubeClient := NewKubeClient(staticClient, dynamicClient, discoveryClient, mapper)

	clientFactory := &FakeClientFactory{
		discoveryClient: discoveryClient,
		dynamicClient:   dynamicClient,
		kubeClient:      kubeClient,
		mapper:          mapper,
		staticClient:    staticClient,
	}

	return clientFactory
}

func (f *FakeClientFactory) KubeClient() KubeClienter {
	return f.kubeClient
}

func (f *FakeClientFactory) Static() kubernetes.Interface {
	return f.staticClient
}

func (f *FakeClientFactory) Dynamic() dynamic.Interface {
	return f.dynamicClient
}

func (f *FakeClientFactory) Discovery() discovery.CachedDiscoveryInterface {
	return f.discoveryClient
}

func (f *FakeClientFactory) Mapper() meta.ResettableRESTMapper {
	return f.mapper
}

func (f *FakeClientFactory) LegacyClientGetter() *LegacyClientGetter {
	panic("not implemented yet")
}

func (f *FakeClientFactory) KubeConfig() *KubeConfig {
	panic("not implemented yet")
}
