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
	"github.com/werf/nelm/internal/log"
)

// TODO(ilya-lesikov): pass all missing options
type ChartDownloaderOptions struct {
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

func NewChartDownloader(ctx context.Context, chartRef string, registryClient *helmregistry.Client, opts ChartDownloaderOptions) (*helmdownloader.ChartDownloader, string, error) {
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

		rUrl, err := url.Parse(opts.RepoURL)
		if err != nil {
			return nil, "", fmt.Errorf("parse repo URL: %w", err)
		}

		cUrl, err := url.Parse(chartURL)
		if err != nil {
			return nil, "", fmt.Errorf("parse chart URL: %w", err)
		}

		if rUrl.Scheme == cUrl.Scheme && rUrl.Host == cUrl.Host {
			downloader.Options = append(downloader.Options, helmgetter.WithBasicAuth(opts.Username, opts.Password))
		} else {
			downloader.Options = append(downloader.Options, helmgetter.WithBasicAuth("", ""))
		}

		chartRef = chartURL
	}

	return downloader, chartRef, nil
}
