package kube

import (
	"context"
	"encoding/base64"
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/werf/nelm/pkg/log"
)

type KubeConfig struct {
	LegacyClientConfig clientcmd.ClientConfig
	Namespace          string
	RawConfig          *api.Config
	RestConfig         *rest.Config
}

type KubeConfigOptions struct {
	APIServerAddress   string
	AuthProviderConfig map[string]string
	AuthProviderName   string
	BasicAuthPassword  string
	BasicAuthUsername  string
	BearerTokenData    string
	BearerTokenPath    string
	BurstLimit         int
	ContextCluster     string
	ContextCurrent     string
	ContextNamespace   string
	ContextUser        string
	ImpersonateGroups  []string
	ImpersonateUID     string
	ImpersonateUser    string
	KubeConfigBase64   string
	ProxyURL           string
	QPSLimit           int
	RequestTimeout     string
	SkipTLSVerify      bool
	TLSCAData          string
	TLSCAPath          string
	TLSClientCertData  string
	TLSClientCertPath  string
	TLSClientKeyData   string
	TLSClientKeyPath   string
	TLSServerName      string
}

func NewKubeConfig(ctx context.Context, kubeConfigPaths []string, opts KubeConfigOptions) (*KubeConfig, error) {
	overrides := &clientcmd.ConfigOverrides{
		AuthInfo: api.AuthInfo{
			AuthProvider: &api.AuthProviderConfig{
				Name:   opts.AuthProviderName,
				Config: opts.AuthProviderConfig,
			},
			ClientCertificate:     opts.TLSClientCertPath,
			ClientCertificateData: []byte(opts.TLSClientCertData),
			ClientKey:             opts.TLSClientKeyPath,
			ClientKeyData:         []byte(opts.TLSClientKeyData),
			Impersonate:           opts.ImpersonateUser,
			ImpersonateGroups:     opts.ImpersonateGroups,
			ImpersonateUID:        opts.ImpersonateUID,
			Password:              opts.BasicAuthPassword,
			Token:                 opts.BearerTokenData,
			TokenFile:             opts.BearerTokenPath,
			Username:              opts.BasicAuthUsername,
		},
		ClusterDefaults: clientcmd.ClusterDefaults,
		ClusterInfo: api.Cluster{
			CertificateAuthority:     opts.TLSCAPath,
			CertificateAuthorityData: []byte(opts.TLSCAData),
			InsecureSkipTLSVerify:    opts.SkipTLSVerify,
			ProxyURL:                 opts.ProxyURL,
			Server:                   opts.APIServerAddress,
			TLSServerName:            opts.TLSServerName,
		},
		Context: api.Context{
			AuthInfo:  opts.ContextUser,
			Cluster:   opts.ContextCluster,
			Namespace: opts.ContextNamespace,
		},
		CurrentContext: opts.ContextCurrent,
		Timeout:        opts.RequestTimeout,
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

	restConfig.QPS = float32(opts.QPSLimit)
	restConfig.Burst = opts.BurstLimit

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
