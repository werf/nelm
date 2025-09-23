package chart

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/werf/3p-helm/pkg/cli"
	helmdownloader "github.com/werf/3p-helm/pkg/downloader"
	helmgetter "github.com/werf/3p-helm/pkg/getter"
	"github.com/werf/3p-helm/pkg/helmpath"
	helmregistry "github.com/werf/3p-helm/pkg/registry"
	helmrepo "github.com/werf/3p-helm/pkg/repo"
	"github.com/werf/nelm/pkg/featgate"
	"github.com/werf/nelm/pkg/log"
)

// TODO(ilya-lesikov): pass all missing options
type chartDownloaderOptions struct {
	KeyringFile        string
	PassCredentialsAll bool
	CertFile           string
	KeyFile            string
	CaFile             string
	SkipTLSVerify      bool
	Insecure           bool
	RepoURL            string
	Username           string
	Password           string
	Version            string
}

func newChartDownloader(ctx context.Context, chartRef string, registryClient *helmregistry.Client, opts chartDownloaderOptions) (*helmdownloader.ChartDownloader, string, error) {
	var out io.Writer
	if log.Default.AcceptLevel(ctx, log.WarningLevel) {
		out = os.Stdout
	} else {
		out = io.Discard
	}

	downloader := &helmdownloader.ChartDownloader{
		Out:     out,
		Keyring: opts.KeyringFile,
		Getters: helmgetter.Providers{helmgetter.HttpProvider, helmgetter.OCIProvider},
		Options: []helmgetter.Option{
			helmgetter.WithPassCredentialsAll(opts.PassCredentialsAll),
			helmgetter.WithTLSClientConfig(opts.CertFile, opts.KeyFile, opts.CaFile),
			helmgetter.WithInsecureSkipVerifyTLS(opts.SkipTLSVerify),
			helmgetter.WithPlainHTTP(opts.Insecure),
			helmgetter.WithRegistryClient(registryClient),
		},
		RegistryClient:   registryClient,
		RepositoryConfig: cli.EnvOr("HELM_REPOSITORY_CONFIG", helmpath.ConfigPath("repositories.yaml")),
		RepositoryCache:  cli.EnvOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")),
	}

	if opts.PassCredentialsAll || opts.RepoURL == "" {
		downloader.Options = append(downloader.Options, helmgetter.WithBasicAuth(opts.Username, opts.Password))
	} else {
		chartURL, err := helmrepo.FindChartInAuthAndTLSAndPassRepoURL(opts.RepoURL, opts.Username, opts.Password, chartRef, opts.Version, opts.CertFile, opts.KeyFile, opts.CaFile, opts.SkipTLSVerify, opts.PassCredentialsAll, helmgetter.Providers{helmgetter.HttpProvider, helmgetter.OCIProvider})
		if err != nil {
			return nil, "", fmt.Errorf("get chart URL: %w", err)
		}

		rURL, err := url.Parse(opts.RepoURL)
		if err != nil {
			return nil, "", fmt.Errorf("parse repo URL: %w", err)
		}

		cURL, err := url.Parse(chartURL)
		if err != nil {
			return nil, "", fmt.Errorf("parse chart URL: %w", err)
		}

		if rURL.Scheme == cURL.Scheme && rURL.Host == cURL.Host {
			downloader.Options = append(downloader.Options, helmgetter.WithBasicAuth(opts.Username, opts.Password))
		} else {
			downloader.Options = append(downloader.Options, helmgetter.WithBasicAuth("", ""))
		}

		chartRef = chartURL
	}

	return downloader, chartRef, nil
}

func downloadChart(ctx context.Context, chartPath string, opts RenderChartOptions) (string, error) {
	if (featgate.FeatGateRemoteCharts.Enabled() || featgate.FeatGatePreviewV2.Enabled()) && !isLocalChart(chartPath) {
		chartDownloader, chartRef, err := newChartDownloader(ctx, chartPath, opts.RegistryClient, chartDownloaderOptions{
			CaFile:        opts.KubeCAPath,
			SkipTLSVerify: opts.ChartRepoSkipTLSVerify,
			Insecure:      opts.ChartRepoInsecure,
			Version:       opts.ChartVersion,
		})
		if err != nil {
			return "", fmt.Errorf("construct chart downloader: %w", err)
		}

		if err := os.MkdirAll(cli.EnvOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")), 0o755); err != nil {
			return "", fmt.Errorf("create repository cache directory: %w", err)
		}

		chartPath, _, err = chartDownloader.DownloadTo(chartRef, opts.ChartVersion, cli.EnvOr("HELM_REPOSITORY_CACHE", helmpath.CachePath("repository")))
		if err != nil {
			return "", fmt.Errorf("download chart %q: %w", chartRef, err)
		}
	}

	return chartPath, nil
}
