package fake

import (
	"context"
	"fmt"
	"reflect"

	"github.com/samber/lo"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/nelm/internal/kube"
)

var _ kube.ClientFactorier = (*ClientFactory)(nil)

type ClientFactory struct {
	discoveryClient discovery.CachedDiscoveryInterface
	dynamicClient   dynamic.Interface
	kubeClient      kube.KubeClienter
	mapper          meta.ResettableRESTMapper
	staticClient    kubernetes.Interface
}

func NewClientFactory(ctx context.Context) (*ClientFactory, error) {
	kube.AddToScheme.Do(func() {
		lo.Must0(apiextv1.AddToScheme(scheme.Scheme))
		lo.Must0(apiextv1beta1.AddToScheme(scheme.Scheme))
	})

	discoveryClient, err := NewCachedDiscoveryClient()
	if err != nil {
		return nil, fmt.Errorf("construct fake cached discovery client: %w", err)
	}

	mapper := reflect.ValueOf(kube.NewKubeMapper(ctx, discoveryClient)).Interface().(meta.ResettableRESTMapper)
	staticClient := NewStaticClient(mapper)
	dynamicClient := NewDynamicClient(staticClient, mapper)
	kubeClient := kube.NewKubeClient(staticClient, dynamicClient, discoveryClient, mapper)

	clientFactory := &ClientFactory{
		discoveryClient: discoveryClient,
		dynamicClient:   dynamicClient,
		kubeClient:      kubeClient,
		mapper:          mapper,
		staticClient:    staticClient,
	}

	return clientFactory, nil
}

func (f *ClientFactory) Discovery() discovery.CachedDiscoveryInterface {
	return f.discoveryClient
}

func (f *ClientFactory) Dynamic() dynamic.Interface {
	return f.dynamicClient
}

func (f *ClientFactory) KubeClient() kube.KubeClienter {
	return f.kubeClient
}

func (f *ClientFactory) KubeConfig() *kube.KubeConfig {
	panic("not implemented yet")
}

func (f *ClientFactory) LegacyClientGetter() *kube.LegacyClientGetter {
	panic("not implemented yet")
}

func (f *ClientFactory) Mapper() meta.ResettableRESTMapper {
	return f.mapper
}

func (f *ClientFactory) Static() kubernetes.Interface {
	return f.staticClient
}
