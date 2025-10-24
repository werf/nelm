package kube

import (
	"context"
	"encoding/base64"
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

type KubeConfig struct {
	LegacyClientConfig clientcmd.ClientConfig
	Namespace          string
	RawConfig          *api.Config
	RestConfig         *rest.Config
}

type KubeConfigOptions struct {
	common.KubeConnectionOptions

	KubeContextNamespace string
}

func NewKubeConfig(ctx context.Context, kubeConfigPaths []string, opts KubeConfigOptions) (*KubeConfig, error) {
	var authProviderConfig *api.AuthProviderConfig
	if opts.KubeAuthProviderName != "" || len(opts.KubeAuthProviderConfig) != 0 {
		authProviderConfig = &api.AuthProviderConfig{
			Name:   opts.KubeAuthProviderName,
			Config: opts.KubeAuthProviderConfig,
		}
	}

	overrides := &clientcmd.ConfigOverrides{
		AuthInfo: api.AuthInfo{
			AuthProvider:          authProviderConfig,
			ClientCertificate:     opts.KubeTLSClientCertPath,
			ClientCertificateData: []byte(opts.KubeTLSClientCertData),
			ClientKey:             opts.KubeTLSClientKeyPath,
			ClientKeyData:         []byte(opts.KubeTLSClientKeyData),
			Impersonate:           opts.KubeImpersonateUser,
			ImpersonateGroups:     opts.KubeImpersonateGroups,
			ImpersonateUID:        opts.KubeImpersonateUID,
			Password:              opts.KubeBasicAuthPassword,
			Token:                 opts.KubeBearerTokenData,
			TokenFile:             opts.KubeBearerTokenPath,
			Username:              opts.KubeBasicAuthUsername,
		},
		ClusterDefaults: clientcmd.ClusterDefaults,
		ClusterInfo: api.Cluster{
			CertificateAuthority:     opts.KubeTLSCAPath,
			CertificateAuthorityData: []byte(opts.KubeTLSCAData),
			InsecureSkipTLSVerify:    opts.KubeSkipTLSVerify,
			ProxyURL:                 opts.KubeProxyURL,
			Server:                   opts.KubeAPIServerAddress,
			TLSServerName:            opts.KubeTLSServerName,
		},
		Context: api.Context{
			AuthInfo:  opts.KubeContextUser,
			Cluster:   opts.KubeContextCluster,
			Namespace: opts.KubeContextNamespace,
		},
		CurrentContext: opts.KubeContextCurrent,
		Timeout:        opts.KubeRequestTimeout.String(),
	}

	var clientConfig clientcmd.ClientConfig
	if opts.KubeConfigBase64 != "" {
		config, err := loadKubeConfigBase64(opts.KubeConfigBase64)
		if err != nil {
			return nil, fmt.Errorf("load kubeconfig from base64: %w", err)
		}

		clientConfig = clientcmd.NewDefaultClientConfig(*config, overrides)
	} else {
		loadingRules := &clientcmd.ClientConfigLoadingRules{
			Precedence:          kubeConfigPaths,
			MigrationRules:      clientcmd.NewDefaultClientConfigLoadingRules().MigrationRules,
			DefaultClientConfig: &clientcmd.DefaultClientConfig,
		}

		clientConfig = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	}

	namespace, _, err := clientConfig.Namespace()
	if err != nil {
		return nil, fmt.Errorf("get namespace: %w", err)
	}

	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("get raw config: %w", err)
	}

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("get rest config: %w", err)
	}

	restConfig.QPS = float32(opts.KubeQPSLimit)
	restConfig.Burst = opts.KubeBurstLimit

	kubeConfig := &KubeConfig{
		LegacyClientConfig: clientConfig,
		Namespace:          namespace,
		RawConfig:          &rawConfig,
		RestConfig:         restConfig,
	}

	log.Default.TraceStruct(ctx, kubeConfig, "Constructed KubeConfig:")

	return kubeConfig, nil
}

func loadKubeConfigBase64(kubeConfigBase64 string) (*api.Config, error) {
	configData, err := base64.StdEncoding.DecodeString(kubeConfigBase64)
	if err != nil {
		return nil, fmt.Errorf("decode base64 string: %w", err)
	}

	config, err := clientcmd.Load(configData)
	if err != nil {
		return nil, fmt.Errorf("load data: %w", err)
	}

	return config, nil
}
