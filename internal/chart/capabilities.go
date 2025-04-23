package chart

import (
	"context"
	"fmt"

	"k8s.io/client-go/discovery"

	helmaction "github.com/werf/3p-helm/pkg/action"
	helmchartutil "github.com/werf/3p-helm/pkg/chartutil"
	"github.com/werf/nelm/internal/log"
)

type BuildCapabilitiesOptions struct {
	APIVersions     *helmchartutil.VersionSet
	DiscoveryClient discovery.CachedDiscoveryInterface
	KubeVersion     *helmchartutil.KubeVersion
}

func BuildCapabilities(ctx context.Context, opts BuildCapabilitiesOptions) (*helmchartutil.Capabilities, error) {
	capabilities := &helmchartutil.Capabilities{
		HelmVersion: helmchartutil.DefaultCapabilities.HelmVersion,
	}

	if opts.DiscoveryClient != nil {
		opts.DiscoveryClient.Invalidate()

		if opts.KubeVersion != nil {
			capabilities.KubeVersion = *opts.KubeVersion
		} else {
			kubeVersion, err := opts.DiscoveryClient.ServerVersion()
			if err != nil {
				return nil, fmt.Errorf("get kubernetes server version: %w", err)
			}

			capabilities.KubeVersion = helmchartutil.KubeVersion{
				Version: kubeVersion.GitVersion,
				Major:   kubeVersion.Major,
				Minor:   kubeVersion.Minor,
			}
		}

		if opts.APIVersions != nil {
			capabilities.APIVersions = *opts.APIVersions
		} else {
			apiVersions, err := helmaction.GetVersionSet(opts.DiscoveryClient)
			if err != nil {
				if discovery.IsGroupDiscoveryFailedError(err) {
					log.Default.Warn(ctx, "Discovery failed: %s", err.Error())
				} else {
					return nil, fmt.Errorf("get version set: %w", err)
				}
			}

			capabilities.APIVersions = apiVersions
		}
	} else {
		if opts.KubeVersion != nil {
			capabilities.KubeVersion = *opts.KubeVersion
		} else {
			capabilities.KubeVersion = helmchartutil.DefaultCapabilities.KubeVersion
		}

		if opts.APIVersions != nil {
			capabilities.APIVersions = *opts.APIVersions
		} else {
			capabilities.APIVersions = helmchartutil.DefaultCapabilities.APIVersions
		}
	}

	return capabilities, nil
}
