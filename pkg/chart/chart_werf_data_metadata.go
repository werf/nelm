package chart

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/werfdata"
)

type DownloadChartOptions struct {
	common.ChartRepoConnectionOptions

	ChartProvenanceKeyring  string
	ChartProvenanceStrategy string
	ChartVersion            string
	// ChartVersionLiteral, when true, resolves the tag of an OCI chart, given in chartRef or as
	// ChartVersion, only as the tag of that name ("+" read as "_"), never as a version constraint
	// matched against the tags of the repository.
	ChartVersionLiteral bool
}

// DownloadChart downloads a remote chart into destDir, verifying it as opts say, and returns the
// path of the archive. A local chart path is returned as is. destDir must be private to the caller,
// e.g. a new temporary directory: the archive is not verified again when rendered from there.
// Rendering the returned path doesn't download the chart again.
func DownloadChart(ctx context.Context, chartRef string, registryClient *registry.Client, destDir string, opts DownloadChartOptions) (string, error) {
	return downloadChart(ctx, chartRef, registryClient, destDir, chartDownloaderOptions{
		ChartRepoConnectionOptions: opts.ChartRepoConnectionOptions,
		ChartProvenanceKeyring:     opts.ChartProvenanceKeyring,
		ChartProvenanceStrategy:    opts.ChartProvenanceStrategy,
		ChartVersion:               opts.ChartVersion,
		ChartVersionLiteral:        opts.ChartVersionLiteral,
	})
}

// LoadChartWerfDataMetadata returns the .werf-data metadata of the local chart directory or archive
// at chartPath, empty if the chart has none. Only the metadata is read.
func LoadChartWerfDataMetadata(ctx context.Context, chartPath string) (*werfdata.Metadata, error) {
	metadataName := path.Join(werfdata.DirName, werfdata.MetadataFileName)

	info, err := os.Stat(chartPath)
	if err != nil {
		return nil, fmt.Errorf("stat chart: %w", err)
	}

	var data []byte
	if info.IsDir() {
		data, err = os.ReadFile(filepath.Join(chartPath, filepath.FromSlash(metadataName)))
		if errors.Is(err, fs.ErrNotExist) {
			return &werfdata.Metadata{}, nil
		} else if err != nil {
			return nil, fmt.Errorf("read %s: %w", metadataName, err)
		}
	} else {
		archiveFile, err := os.Open(chartPath)
		if err != nil {
			return nil, fmt.Errorf("open chart archive: %w", err)
		}
		defer archiveFile.Close()

		files, err := archive.LoadArchiveFiles(archiveFile)
		if err != nil {
			return nil, fmt.Errorf("read chart archive: %w", err)
		}

		for _, f := range files {
			if f.Name == metadataName {
				data = f.Data
			}
		}

		if data == nil {
			return &werfdata.Metadata{}, nil
		}
	}

	metadata, err := werfdata.ParseMetadata(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", werfdata.DirName, err)
	}

	return metadata, nil
}
