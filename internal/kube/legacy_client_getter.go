package kube

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var _ genericclioptions.RESTClientGetter = (*LegacyClientGetter)(nil)

type LegacyClientGetter struct {
	discoveryClient    discovery.CachedDiscoveryInterface
	mapper             meta.ResettableRESTMapper
	restConfig         *rest.Config
	legacyClientConfig clientcmd.ClientConfig
}

// TODO(v2): get rid
func NewLegacyClientGetter(discoveryClient discovery.CachedDiscoveryInterface, mapper meta.ResettableRESTMapper, restConfig *rest.Config, legacyClientConfig clientcmd.ClientConfig) *LegacyClientGetter {
	return &LegacyClientGetter{
		discoveryClient:    discoveryClient,
		mapper:             mapper,
		restConfig:         restConfig,
		legacyClientConfig: legacyClientConfig,
	}
}

func (g *LegacyClientGetter) ToRESTConfig() (*rest.Config, error) {
	return g.restConfig, nil
}

func (g *LegacyClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return g.discoveryClient, nil
}

func (g *LegacyClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return g.mapper, nil
}

func (g *LegacyClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return g.legacyClientConfig
}
