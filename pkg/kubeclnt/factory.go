package kubeclnt

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/samber/lo"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	helm_v3 "github.com/werf/3p-helm/cmd/helm"
)

var addToScheme sync.Once

func NewClientFactory() (*ClientFactory, error) {
	addToScheme.Do(func() {
		lo.Must0(apiextv1.AddToScheme(scheme.Scheme))
		lo.Must0(apiextv1beta1.AddToScheme(scheme.Scheme))
	})

	var restClientGetter genericclioptions.RESTClientGetter
	if getterPtr := helm_v3.Settings.GetConfigP(); getterPtr != nil {
		restClientGetter = *getterPtr
	} else {
		restClientGetter = genericclioptions.NewConfigFlags(true)
	}

	factory := cmdutil.NewFactory(restClientGetter)

	if err := checkClusterConnectivity(factory); err != nil {
		return nil, fmt.Errorf("Kubernetes cluster unreachable: %w", err)
	}

	staticClient, err := factory.KubernetesClientSet()
	if err != nil {
		return nil, fmt.Errorf("error getting static kubernetes client: %w", err)
	}

	dynamicClient, err := factory.DynamicClient()
	if err != nil {
		return nil, fmt.Errorf("error getting dynamic kubernetes client: %w", err)
	}

	discoveryClient, err := factory.ToDiscoveryClient()
	if err != nil {
		return nil, fmt.Errorf("error getting discovery kubernetes client: %w", err)
	}

	var mapper meta.ResettableRESTMapper
	if m, err := factory.ToRESTMapper(); err != nil {
		return nil, fmt.Errorf("error getting REST mapper: %w", err)
	} else {
		mapper = reflect.ValueOf(m).Interface().(meta.ResettableRESTMapper)
	}

	kubeClient := NewKubeClient(staticClient, dynamicClient, discoveryClient, mapper)
	if err != nil {
		return nil, fmt.Errorf("error creating kube client: %w", err)
	}

	return &ClientFactory{
		kubeClient:      kubeClient,
		staticClient:    staticClient,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		mapper:          mapper,
	}, nil
}

type ClientFactory struct {
	kubeClient      KubeClienter
	staticClient    kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	mapper          meta.ResettableRESTMapper
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

func checkClusterConnectivity(factory cmdutil.Factory) error {
	client, err := factory.KubernetesClientSet()
	if err != nil {
		if err == genericclioptions.ErrEmptyConfig {
			return fmt.Errorf("incomplete configuration")
		}
		return fmt.Errorf("error getting kubernetes client: %w", err)
	}

	if _, err := client.ServerVersion(); err != nil {
		return fmt.Errorf("error getting kubernetes server version: %w", err)
	}

	return nil
}
