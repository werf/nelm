package chartextender

import "github.com/werf/3p-helm/pkg/chart"

type GetHelmChartMetadataOptions struct {
	OverrideAppVersion string
	DefaultAPIVersion  string
	DefaultName        string
	DefaultVersion     string
}

func AutosetChartMetadata(metadataIn *chart.Metadata, opts GetHelmChartMetadataOptions) *chart.Metadata {
	var metadata *chart.Metadata
	if metadataIn == nil {
		metadata = &chart.Metadata{}
	} else {
		metadata = metadataIn
	}

	if metadata.APIVersion == "" {
		metadata.APIVersion = opts.DefaultAPIVersion
	}

	if metadata.Name == "" {
		metadata.Name = opts.DefaultName
	}

	if opts.OverrideAppVersion != "" {
		metadata.AppVersion = opts.OverrideAppVersion
	}

	if metadata.Version == "" {
		metadata.Version = opts.DefaultVersion
	}

	return metadata
}
