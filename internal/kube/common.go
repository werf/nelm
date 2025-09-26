package kube

import (
	"path/filepath"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	DefaultKubectlCacheDir      = filepath.Join(homedir.HomeDir(), ".kube", "cache")
	KubectlCacheDirEnv          = "KUBECACHEDIR"
	KubectlHTTPCacheSubdir      = "http"
	KubectlDiscoveryCacheSubdir = "discovery"
)

func init() {
	genericclioptions.ErrEmptyConfig = clientcmd.NewEmptyConfigError("missing or incomplete kubeconfig")
}
