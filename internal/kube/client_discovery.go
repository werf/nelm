package kube

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/client-go/discovery/cached/disk"
)

func NewDiscoveryKubeClientFromKubeConfig(kubeConfig *KubeConfig) (*disk.CachedDiscoveryClient, error) {
	var cacheDir string
	if dir := os.Getenv(KubectlCacheDirEnv); dir != "" {
		cacheDir = dir
	} else {
		cacheDir = DefaultKubectlCacheDir
	}

	httpCacheDir := filepath.Join(cacheDir, KubectlHTTPCacheSubdir)
	discoveryCacheDir := computeDiscoveryCacheDir(filepath.Join(cacheDir, KubectlDiscoveryCacheSubdir), kubeConfig.RestConfig.Host)

	client, err := disk.NewCachedDiscoveryClientForConfig(kubeConfig.RestConfig, discoveryCacheDir, httpCacheDir, 6*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("new discovery client for config: %w", err)
	}

	return client, nil
}

// Taken from: https://github.com/kubernetes/cli-runtime/blob/e447e205e17575154e7108dbd67e6965499488a0/pkg/genericclioptions/config_flags.go#L485
func computeDiscoveryCacheDir(parentDir, host string) string {
	schemelessHost := strings.Replace(strings.Replace(host, "https://", "", 1), "http://", "", 1)

	safeHost := regexp.MustCompile(`[^(\w/.)]`).ReplaceAllString(schemelessHost, "_")

	return filepath.Join(parentDir, safeHost)
}
