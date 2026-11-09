package chart

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/werf/nelm/v2/pkg/common"
	helmdownloader "github.com/werf/nelm/v2/pkg/helm/pkg/downloader"
	helmgetter "github.com/werf/nelm/v2/pkg/helm/pkg/getter"
	"github.com/werf/nelm/v2/pkg/helm/pkg/helmpath"
	helmregistry "github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	helmrepo "github.com/werf/nelm/v2/pkg/helm/pkg/repo/v1"
	"github.com/werf/nelm/v2/pkg/log"
)

type chartDownloaderOptions struct {
	common.ChartRepoConnectionOptions

	ChartProvenanceKeyring  string
	ChartProvenanceStrategy string
	ChartVersion            string
	ChartVersionLiteral     bool
}

// downloadChart downloads and verifies a remote chart into destDir, which must be private to the
// caller: the archive is verified there and not again when it is loaded.
func downloadChart(ctx context.Context, chartPath string, registryClient *helmregistry.Client, destDir string, opts chartDownloaderOptions) (string, error) {
	if !isLocalChart(chartPath) {
		chartDownloader, chartRef, err := newChartDownloader(ctx, chartPath, registryClient, opts)
		if err != nil {
			return "", fmt.Errorf("construct chart downloader: %w", err)
		}

		chartPath, _, err = chartDownloader.DownloadTo(chartRef, opts.ChartVersion, destDir)
		if err != nil {
			return "", fmt.Errorf("download chart %q: %w", chartRef, err)
		}

		// A relative path would be taken for a remote chart again.
		if chartPath, err = filepath.Abs(chartPath); err != nil {
			return "", fmt.Errorf("get absolute path of downloaded chart: %w", err)
		}
	}

	return chartPath, nil
}

func newChartDownloader(ctx context.Context, chartRef string, registryClient *helmregistry.Client, opts chartDownloaderOptions) (*helmdownloader.ChartDownloader, string, error) {
	var out io.Writer
	if log.Default.AcceptLevel(ctx, log.WarningLevel) {
		// TODO(log):
		out = os.Stdout
	} else {
		out = io.Discard
	}

	downloader := &helmdownloader.ChartDownloader{
		Out:     out,
		Verify:  parseVerificationStrategy(opts.ChartProvenanceStrategy),
		Keyring: opts.ChartProvenanceKeyring,
		Getters: helmgetter.Getters(),
		Options: []helmgetter.Option{
			helmgetter.WithPassCredentialsAll(opts.ChartRepoPassCreds),
			helmgetter.WithTLSClientConfig(opts.ChartRepoCertPath, opts.ChartRepoKeyPath, opts.ChartRepoCAPath),
			helmgetter.WithInsecureSkipVerifyTLS(opts.ChartRepoSkipTLSVerify),
			helmgetter.WithPlainHTTP(opts.ChartRepoInsecure),
			helmgetter.WithRegistryClient(registryClient),
			helmgetter.WithTimeout(opts.ChartRepoRequestTimeout),
			helmgetter.WithContext(ctx),
		},
		RegistryClient:   registryClient,
		RepositoryConfig: helmpath.ConfigPath("repositories.yaml"),
		RepositoryCache:  helmpath.CachePath("repository"),
		ContentCache:     helmpath.CachePath("content"),
		LiteralTag:       opts.ChartVersionLiteral,
	}

	if opts.ChartRepoURL != "" {
		chartURL, err := helmrepo.FindChartInRepoURL(opts.ChartRepoURL, chartRef, helmgetter.Getters(),
			helmrepo.WithChartVersion(opts.ChartVersion),
			helmrepo.WithUsernamePassword(opts.ChartRepoBasicAuthUsername, opts.ChartRepoBasicAuthPassword),
			helmrepo.WithClientTLS(opts.ChartRepoCertPath, opts.ChartRepoKeyPath, opts.ChartRepoCAPath),
			helmrepo.WithInsecureSkipTLSVerify(opts.ChartRepoSkipTLSVerify),
			helmrepo.WithPassCredentialsAll(opts.ChartRepoPassCreds),
		)
		if err != nil {
			return nil, "", fmt.Errorf("get chart URL: %w", err)
		}

		rURL, err := url.Parse(opts.ChartRepoURL)
		if err != nil {
			return nil, "", fmt.Errorf("parse repo URL: %w", err)
		}

		cURL, err := url.Parse(chartURL)
		if err != nil {
			return nil, "", fmt.Errorf("parse chart URL: %w", err)
		}

		if opts.ChartRepoPassCreds || (rURL.Scheme == cURL.Scheme && rURL.Host == cURL.Host) {
			downloader.Options = append(downloader.Options, helmgetter.WithBasicAuth(opts.ChartRepoBasicAuthUsername, opts.ChartRepoBasicAuthPassword))
		} else {
			downloader.Options = append(downloader.Options, helmgetter.WithBasicAuth("", ""))
		}

		chartRef = chartURL
	} else {
		downloader.Options = append(downloader.Options, helmgetter.WithBasicAuth(opts.ChartRepoBasicAuthUsername, opts.ChartRepoBasicAuthPassword))
	}

	return downloader, chartRef, nil
}
