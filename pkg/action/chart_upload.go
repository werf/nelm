package action

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/samber/lo"
	orasregistry "oras.land/oras-go/v2/registry"

	"github.com/werf/nelm/v2/pkg/common"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/log"
)

const DefaultChartUploadLogLevel = log.InfoLevel

type ChartUploadOptions struct {
	common.ChartRepoConnectionOptions

	// ExtraTags are pushed in addition to the chart version tag, with "+" pushed as "_".
	ExtraTags []string
	// LegacyLogRegistryStreamOut receives the Helm registry client output, like the "Pushed:" and
	// "Digest:" lines. Defaults to io.Discard.
	LegacyLogRegistryStreamOut io.Writer
	// RegistryCredentialsPath is the path to Docker config.json file with registry credentials.
	// Defaults to DefaultDockerConfig/config.json.
	RegistryCredentialsPath string
}

type ChartUploadResult struct {
	// ManifestDigest is the digest of the pushed chart manifest.
	ManifestDigest string
	// Tags are the tags the chart is pushed under, the version tag first.
	Tags []string
}

// ChartUpload pushes the chart archive at archivePath into the repository REMOTE/<chart name>, where
// remote is "oci://HOST[/PATH]", under the chart version and opts.ExtraTags, with "+" pushed as "_".
// The provenance file <archivePath>.prov is pushed with the chart if it exists. A tag equal to the
// tag of an image from the helm.sh/images annotations of the chart or its dependencies in the same
// repository is an error, raised before anything is pushed, as the chart would replace the image.
// Loopback registries are reached over plain HTTP unless TLS options are given.
func ChartUpload(ctx context.Context, archivePath, remote string, opts ChartUploadOptions) (*ChartUploadResult, error) {
	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = filepath.Join(common.DefaultDockerConfig, "config.json")
	}

	if opts.LegacyLogRegistryStreamOut == nil {
		opts.LegacyLogRegistryStreamOut = io.Discard
	}

	if !registry.IsOCI(remote) {
		return nil, fmt.Errorf("remote %q must start with %q", remote, registry.OCIScheme+"://")
	}

	archiveInfo, err := os.Stat(archivePath)
	if err != nil {
		return nil, fmt.Errorf("stat chart archive: %w", err)
	}

	if archiveInfo.IsDir() {
		return nil, fmt.Errorf("chart archive %q is a directory", archivePath)
	}

	chartData, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("read chart archive: %w", err)
	}

	provData, err := os.ReadFile(archivePath + ".prov")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read chart provenance file: %w", err)
	}

	loadCtx := common.ContextWithHelmOptions(ctx, common.HelmOptions{
		ChartLoadOpts: common.ChartLoadOptions{
			ChartType: common.LegacyChartTypeBundle,
			NoSecrets: true,
		},
	})

	loadedChart, err := loader.LoadArchive(loadCtx, bytes.NewReader(chartData))
	if err != nil {
		return nil, fmt.Errorf("load chart archive %q: %w", archivePath, err)
	}

	chartAccessor, err := helmchart.NewAccessor(loadedChart)
	if err != nil {
		return nil, fmt.Errorf("create chart accessor: %w", err)
	}

	repo := path.Join(strings.TrimPrefix(remote, registry.OCIScheme+"://"), chartAccessor.Name())
	if _, err := orasregistry.ParseReference(repo); err != nil {
		return nil, fmt.Errorf("parse repository %q: %w", repo, err)
	}

	version := chartVersion(loadedChart)
	tags := lo.Uniq(lo.Map(append([]string{version}, opts.ExtraTags...), func(tag string, _ int) string {
		return strings.ReplaceAll(tag, "+", "_")
	}))

	if err := checkChartUploadTagsDoNotOverwriteImages(ctx, loadedChart, repo, tags); err != nil {
		return nil, err
	}

	// Plain HTTP is for loopback registries without TLS options and, when insecure, the chart
	// repository.
	repoHost := strings.Split(repo, "/")[0]

	var insecureHosts []string
	if opts.ChartRepoInsecure {
		insecureHosts = []string{repoHost}
	}

	registryClient, err := newChartRepoRegistryClient(ctx, opts.ChartRepoConnectionOptions, opts.RegistryCredentialsPath,
		registry.ClientOptBasicAuthHosts(repoHost),
		registry.ClientOptPlainHTTPHost(func(host string) bool {
			return chartRepoPlainHTTP(host, insecureHosts, hasChartRepoTLSOptions(opts.ChartRepoConnectionOptions))
		}),
		registry.ClientOptWriter(opts.LegacyLogRegistryStreamOut),
	)
	if err != nil {
		return nil, fmt.Errorf("construct registry client: %w", err)
	}

	// The archive modification time is the chart creation time, as Helm pushes it.
	pushOpts := []registry.PushOption{
		registry.PushOptCreationTime(archiveInfo.ModTime().Format(time.RFC3339)),
		registry.PushOptExtraTags(tags[1:]),
	}
	if provData != nil {
		pushOpts = append(pushOpts, registry.PushOptProvData(provData))
	}

	pushResult, err := registryClient.PushContext(ctx, chartData, fmt.Sprintf("%s:%s", repo, version), pushOpts...)
	if err != nil {
		return nil, fmt.Errorf("push chart to %q: %w", repo, err)
	}

	log.Default.Info(ctx, "Uploaded chart %s:%s with tags %s (digest %s)", repo, version, strings.Join(tags, ", "), pushResult.Manifest.Digest)

	return &ChartUploadResult{
		ManifestDigest: pushResult.Manifest.Digest,
		Tags:           tags,
	}, nil
}

// checkChartUploadTagsDoNotOverwriteImages fails if a tag equals the tag of an image from the
// helm.sh/images annotations of the chart or its dependencies in repo.
func checkChartUploadTagsDoNotOverwriteImages(ctx context.Context, loadedChart helmchart.Charter, repo string, tags []string) error {
	images, err := chartCopyImages(ctx, loadedChart)
	if err != nil {
		log.Default.Warn(ctx, "Not checking that the chart tags do not overwrite the chart images: read chart images: %s", err)

		return nil
	}

	repoAddress := imageRepositoryAddress(repo)
	for _, image := range images {
		imageRef := parseChartCopyImageRef(image.Image)
		if imageRef.Tag == "" || imageRepositoryAddress(imageRef.Repo) != repoAddress || !lo.Contains(tags, imageRef.Tag) {
			continue
		}

		return fmt.Errorf("chart tag %q would overwrite the image %q %s in the same repository: push the chart with another tag or use another image tag", imageRef.Tag, image.Name, image.Image)
	}

	return nil
}
