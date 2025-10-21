package chart

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/werf/3p-helm/pkg/cli"
	helmdownloader "github.com/werf/3p-helm/pkg/downloader"
	helmgetter "github.com/werf/3p-helm/pkg/getter"
	"github.com/werf/3p-helm/pkg/helmpath"
	helmregistry "github.com/werf/3p-helm/pkg/registry"
	helmrepo "github.com/werf/3p-helm/pkg/repo"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

type chartDownloaderOptions struct {
	ChartProvenanceKeyring     string
	ChartProvenanceStrategy    string
	ChartRepoBasicAuthPassword string
	ChartRepoBasicAuthUsername string
	ChartRepoCAPath            string
	ChartRepoCertPath          string
	ChartRepoInsecure          bool
	ChartRepoKeyPath           string
	ChartRepoPassCreds         bool
	ChartRepoRequestTimeout    time.Duration
	ChartRepoNoTLSVerify       bool
	ChartRepoURL               string
	ChartVersion               string
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
		Verify:  helmdownloader.VerificationStrategyString(opts.ChartProvenanceStrategy).ToVerificationStrategy(),
		Keyring: opts.ChartProvenanceKeyring,
		Getters: helmgetter.Providers{helmgetter.HttpProvider, helmgetter.OCIProvider},
		Options: []helmgetter.Option{
			helmgetter.WithPassCredentialsAll(opts.ChartRepoPassCreds),
			helmgetter.WithTLSClientConfig(opts.ChartRepoCertPath, opts.ChartRepoKeyPath, opts.ChartRepoCAPath),
			helmgetter.WithInsecureSkipVerifyTLS(opts.ChartRepoNoTLSVerify),
			helmgetter.WithPlainHTTP(opts.ChartRepoInsecure),
			helmgetter.WithRegistryClient(registryClient),
			helmgetter.WithTimeout(opts.ChartRepoRequestTimeout),
		},
		RegistryClient: registryClient,
		// TODO(v2): get rid of HELM_ env vars support
		RepositoryConfig: cli.EnvOr("HELM_REPOSITORY_CONFIG", helmpath.ConfigPath("repositories.yaml")),
		// TODO(v2): get rid of HELM_ env vars support
		RepositoryCache: cli.EnvOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")),
	}

	if opts.ChartRepoURL != "" {
		chartURL, err := helmrepo.FindChartInAuthAndTLSAndPassRepoURL(opts.ChartRepoURL, opts.ChartRepoBasicAuthUsername, opts.ChartRepoBasicAuthPassword, chartRef, opts.ChartVersion, opts.ChartRepoCertPath, opts.ChartRepoKeyPath, opts.ChartRepoCAPath, opts.ChartRepoNoTLSVerify, opts.ChartRepoPassCreds, helmgetter.Providers{helmgetter.HttpProvider, helmgetter.OCIProvider})
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

func downloadChart(ctx context.Context, chartPath string, registryClient *helmregistry.Client, opts RenderChartOptions) (string, error) {
	if (featgate.FeatGateRemoteCharts.Enabled() || featgate.FeatGatePreviewV2.Enabled()) && !isLocalChart(chartPath) {
		chartDownloader, chartRef, err := newChartDownloader(ctx, chartPath, registryClient, chartDownloaderOptions{
			ChartProvenanceKeyring:     opts.ChartProvenanceKeyring,
			ChartProvenanceStrategy:    opts.ChartProvenanceStrategy,
			ChartRepoBasicAuthPassword: opts.ChartRepoBasicAuthPassword,
			ChartRepoBasicAuthUsername: opts.ChartRepoBasicAuthUsername,
			ChartRepoCAPath:            opts.ChartRepoCAPath,
			ChartRepoCertPath:          opts.ChartRepoCertPath,
			ChartRepoInsecure:          opts.ChartRepoInsecure,
			ChartRepoKeyPath:           opts.ChartRepoKeyPath,
			ChartRepoPassCreds:         opts.ChartRepoPassCreds,
			ChartRepoRequestTimeout:    opts.ChartRepoRequestTimeout,
			ChartRepoNoTLSVerify:       opts.ChartRepoNoTLSVerify,
			ChartRepoURL:               opts.ChartRepoURL,
			ChartVersion:               opts.ChartVersion,
		})
		if err != nil {
			return "", fmt.Errorf("construct chart downloader: %w", err)
		}

		// TODO(v2): get rid of HELM_ env vars support
		if err := os.MkdirAll(cli.EnvOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")), 0o755); err != nil {
			return "", fmt.Errorf("create repository cache directory: %w", err)
		}

		// TODO(v2): get rid of HELM_ env vars support
		chartPath, _, err = chartDownloader.DownloadTo(chartRef, opts.ChartVersion, cli.EnvOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")))
		if err != nil {
			return "", fmt.Errorf("download chart %q: %w", chartRef, err)
		}
	}

	return chartPath, nil
}
