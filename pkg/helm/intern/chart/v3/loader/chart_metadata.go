package loader

import chart "github.com/werf/nelm/pkg/helm/intern/chart/v3"

type autosetChartMetadataOptions struct {
	OverrideAppVersion string
	DefaultAPIVersion  string
	DefaultName        string
	DefaultVersion     string
}

func autosetChartMetadata(metadataIn *chart.Metadata, opts autosetChartMetadataOptions) *chart.Metadata {
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
