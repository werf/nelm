package chart

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
	helmdownloader "github.com/werf/nelm/pkg/helm/pkg/downloader"
	helmgetter "github.com/werf/nelm/pkg/helm/pkg/getter"
	"github.com/werf/nelm/pkg/helm/pkg/helmpath"
	helmregistry "github.com/werf/nelm/pkg/helm/pkg/registry"
	helmrepo "github.com/werf/nelm/pkg/helm/pkg/repo/v1"
	"github.com/werf/nelm/pkg/log"
)

type chartDownloaderOptions struct {
	common.ChartRepoConnectionOptions

	ChartProvenanceKeyring  string
	ChartProvenanceStrategy string
	ChartVersion            string
}

func downloadChart(ctx context.Context, chartPath string, registryClient *helmregistry.Client, opts RenderChartOptions) (string, error) {
	if (featgate.FeatGateRemoteCharts.Enabled() || featgate.FeatGatePreviewV2.Enabled()) && !isLocalChart(chartPath) {
		chartDownloader, chartRef, err := newChartDownloader(ctx, chartPath, registryClient, chartDownloaderOptions{
			ChartRepoConnectionOptions: opts.ChartRepoConnectionOptions,
			ChartProvenanceKeyring:     opts.ChartProvenanceKeyring,
			ChartProvenanceStrategy:    opts.ChartProvenanceStrategy,
			ChartVersion:               opts.ChartVersion,
		})
		if err != nil {
			return "", fmt.Errorf("construct chart downloader: %w", err)
		}

		// TODO(major): get rid of HELM_ env vars support
		if err := os.MkdirAll(envOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")), 0o755); err != nil {
			return "", fmt.Errorf("create repository cache directory: %w", err)
		}

		// TODO(major): get rid of HELM_ env vars support
		chartPath, _, err = chartDownloader.DownloadTo(chartRef, opts.ChartVersion, envOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")))
		if err != nil {
			return "", fmt.Errorf("download chart %q: %w", chartRef, err)
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
		},
		RegistryClient: registryClient,
		// TODO(major): get rid of HELM_ env vars support
		RepositoryConfig: envOr("HELM_REPOSITORY_CONFIG", helmpath.ConfigPath("repositories.yaml")),
		// TODO(major): get rid of HELM_ env vars support
		RepositoryCache: envOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")),
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
