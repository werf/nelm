package kube

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/samber/lo"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
)

var (
	_ ClientFactorier = (*ClientFactory)(nil)

	AddToScheme sync.Once
)

type ClientFactorier interface {
	KubeClient() KubeClienter
	Static() kubernetes.Interface
	Dynamic() dynamic.Interface
	Discovery() discovery.CachedDiscoveryInterface
	Mapper() meta.ResettableRESTMapper
	LegacyClientGetter() *LegacyClientGetter
	KubeConfig() *KubeConfig
}

// Constructs all Kubernetes clients you may possibly need and makes it easy to pass them all
// around.
type ClientFactory struct {
	discoveryClient    discovery.CachedDiscoveryInterface
	dynamicClient      dynamic.Interface
	kubeClient         KubeClienter
	kubeConfig         *KubeConfig
	legacyClientGetter *LegacyClientGetter
	mapper             meta.ResettableRESTMapper
	staticClient       kubernetes.Interface
}

func NewClientFactory(ctx context.Context, kubeConfig *KubeConfig) (*ClientFactory, error) {
	AddToScheme.Do(func() {
		lo.Must0(apiextv1.AddToScheme(scheme.Scheme))
		lo.Must0(apiextv1beta1.AddToScheme(scheme.Scheme))
	})

	staticClient, err := NewStaticKubeClientFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("construct static kubernetes client: %w", err)
	}

	if _, err := staticClient.ServerVersion(); err != nil {
		return nil, fmt.Errorf("check kubernetes cluster version to check kubernetes connectivity: %w", err)
	}

	dynamicClient, err := NewDynamicKubeClientFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("construct dynamic kubernetes client: %w", err)
	}

	discoveryClient, err := NewDiscoveryKubeClientFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("construct discovery kubernetes client: %w", err)
	}

	mapper := reflect.ValueOf(NewKubeMapper(ctx, discoveryClient)).Interface().(meta.ResettableRESTMapper)
	kubeClient := NewKubeClient(staticClient, dynamicClient, discoveryClient, mapper)
	legacyClientGetter := NewLegacyClientGetter(discoveryClient, mapper, kubeConfig.RestConfig, kubeConfig.LegacyClientConfig)

	clientFactory := &ClientFactory{
		discoveryClient:    discoveryClient,
		dynamicClient:      dynamicClient,
		kubeClient:         kubeClient,
		kubeConfig:         kubeConfig,
		legacyClientGetter: legacyClientGetter,
		mapper:             mapper,
		staticClient:       staticClient,
	}

	return clientFactory, nil
}

func (f *ClientFactory) KubeClient() KubeClienter {
	return f.kubeClient
}

func (f *ClientFactory) Static() kubernetes.Interface {
	return f.staticClient
}

func (f *ClientFactory) Dynamic() dynamic.Interface {
	return f.dynamicClient
}

func (f *ClientFactory) Discovery() discovery.CachedDiscoveryInterface {
	return f.discoveryClient
}

func (f *ClientFactory) Mapper() meta.ResettableRESTMapper {
	return f.mapper
}

func (f *ClientFactory) LegacyClientGetter() *LegacyClientGetter {
	return f.legacyClientGetter
}

func (f *ClientFactory) KubeConfig() *KubeConfig {
	return f.kubeConfig
}
