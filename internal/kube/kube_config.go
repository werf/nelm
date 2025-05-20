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

type KubeConfigOptions struct {
	AuthInfo              string
	BurstLimit            int
	CertificateAuthority  string
	ClientCertificate     string
	ClientKey             string
	Cluster               string
	CurrentContext        string
	DisableCompression    bool
	Impersonate           string
	ImpersonateGroups     []string
	ImpersonateUID        string
	InsecureSkipTLSVerify bool
	KubeConfigBase64      string
	Namespace             string
	Password              string
	QPSLimit              int
	Server                string
	TLSServerName         string
	Timeout               string
	Token                 string
	Username              string
}

func NewKubeConfig(ctx context.Context, kubeConfigPaths []string, opts KubeConfigOptions) (*KubeConfig, error) {
	overrides := &clientcmd.ConfigOverrides{
		AuthInfo: api.AuthInfo{
			ClientCertificate: opts.ClientCertificate,
			ClientKey:         opts.ClientKey,
			Impersonate:       opts.Impersonate,
			ImpersonateGroups: opts.ImpersonateGroups,
			ImpersonateUID:    opts.ImpersonateUID,
			Password:          opts.Password,
			Token:             opts.Token,
			Username:          opts.Username,
		},
		ClusterDefaults: clientcmd.ClusterDefaults,
		ClusterInfo: api.Cluster{
			CertificateAuthority:  opts.CertificateAuthority,
			DisableCompression:    opts.DisableCompression,
			InsecureSkipTLSVerify: opts.InsecureSkipTLSVerify,
			Server:                opts.Server,
			TLSServerName:         opts.TLSServerName,
		},
		Context: api.Context{
			AuthInfo:  opts.AuthInfo,
			Cluster:   opts.Cluster,
			Namespace: opts.Namespace,
		},
		CurrentContext: opts.CurrentContext,
		Timeout:        opts.Timeout,
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

type KubeConfig struct {
	LegacyClientConfig clientcmd.ClientConfig
	Namespace          string
	RawConfig          *api.Config
	RestConfig         *rest.Config
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
