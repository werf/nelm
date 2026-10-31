package action

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/gosimple/slug"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/lo"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
	orasregistry "oras.land/oras-go/v2/registry"

	"github.com/werf/nelm/v2/pkg/chartyaml"
	"github.com/werf/nelm/v2/pkg/common"
	helmaction "github.com/werf/nelm/v2/pkg/helm/pkg/action"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/log"
)

const (
	// chartCopyArchiveEntryAnnotation marks the index.json entries of a chart copy archive: the chart,
	// tagged with its tag, and the images, tagged with their refs from the helm.sh/images annotation.
	chartCopyArchiveEntryAnnotation = "nelm.werf.io/archive-entry"
	chartCopyArchiveEntryChart      = "chart"
	chartCopyArchiveEntryImage      = "image"
	dockerConfigMediaType           = "application/vnd.docker.container.image.v1+json"
	dockerLayerMediaType            = "application/vnd.docker.image.rootfs.diff.tar.gzip"
	dockerManifestMediaType         = "application/vnd.docker.distribution.manifest.v2+json"
	werfArchiveChartFileName        = "chart.tar.gz"
	werfArchiveImagesDirName        = "images"
)

type ChartCopyOptions struct {
	common.ChartRepoConnectionOptions

	// Images are copied in addition to the images in the helm.sh/images annotation, for charts that
	// list their images elsewhere, like werf bundles published before the annotation was written.
	// Only Image matters. The annotation is rewritten only for the entries it contains.
	Images []chartyaml.Image
	// RegistryCredentialsPath is the path to Docker config.json file with registry credentials.
	// Defaults to DefaultDockerConfig/config.json.
	RegistryCredentialsPath string
	// RewriteChart, when set, is called on a copy to a registry after the images are copied and
	// before the chart is saved, with the root chart, the destination repository and the ref of
	// every copied image, as written in the helm.sh/images annotation, mapped to the ref of its
	// copy. Changes it makes to the chart are pushed. The helm.sh/images annotation is rewritten
	// by ChartCopy itself.
	RewriteChart func(ctx context.Context, chart helmchart.Charter, repo string, imageMapping map[string]string) error
	// Sign, when true, signs the copy with SignKey from SignKeyring if the copy changes the chart.
	// Without it the provenance of a changed chart is dropped.
	Sign bool
	// SignKey is the name of the key to sign with.
	SignKey string
	// SignKeyring is the path to the keyring with the signing key.
	SignKeyring string
	// SignPassphraseFile is the path to a file with the passphrase of the signing key, "-" for
	// stdin. Prompted for if not set.
	SignPassphraseFile string
	// TempDirPath is the directory for temporary files. Defaults to the system one.
	TempDirPath string
}

type chartCopyAddress struct {
	ArchivePath string
	Repo        string
	Tag         string
}

type chartCopySource struct {
	ChartData []byte
	// ProvData is nil for a chart without provenance.
	ProvData []byte
	// ResolveImage returns the storage with the image of a helm.sh/images entry and the image's
	// root descriptor.
	ResolveImage func(ctx context.Context, imageRef string) (content.ReadOnlyStorage, ocispec.Descriptor, error)
	// Tag is empty when the source has no tag.
	Tag string
}

type chartCopyImageRef struct {
	Digest string
	Repo   string
	Tag    string
}

// ChartCopy copies the chart at from, with every image in its helm.sh/images annotation, to to.
// Each address is either "oci://REPO[:TAG]" (or "REPO[:TAG]"), or an archive: "archive:PATH" or a
// path ending in .tar, .tar.gz or .tgz. A source repository without a tag means the "latest" tag.
// An archive is written as an OCI image layout; the werf 3 bundle archive format is also read.
//
// When copying to a repository, the chart is renamed to the basename of the repository and the
// images are copied by digest into it, keeping their tags. With a tag, the chart version is
// ChartVersionFromTag of it and the chart is pushed under the version and the tag; without one the
// chart keeps its version and is pushed under it and the source tag. The provenance is kept only if
// the chart doesn't change.
func ChartCopy(ctx context.Context, from, to string, opts ChartCopyOptions) error {
	if opts.Sign && (opts.SignKey == "" || opts.SignKeyring == "") {
		return errors.New("signing key name and keyring are required for signing")
	}

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = filepath.Join(common.DefaultDockerConfig, "config.json")
	}

	fromAddr, err := parseChartCopyAddress(from)
	if err != nil {
		return fmt.Errorf("parse source address %q: %w", from, err)
	}

	toAddr, err := parseChartCopyAddress(to)
	if err != nil {
		return fmt.Errorf("parse destination address %q: %w", to, err)
	}

	tempDir, err := os.MkdirTemp(opts.TempDirPath, "chart-copy-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	registryClient, err := newChartRepoRegistryClient(ctx, opts.ChartRepoConnectionOptions, opts.RegistryCredentialsPath)
	if err != nil {
		return fmt.Errorf("construct registry client: %w", err)
	}

	var source *chartCopySource
	if fromAddr.ArchivePath != "" {
		source, err = readChartCopyArchive(ctx, fromAddr.ArchivePath, tempDir)
	} else {
		source, err = readChartCopyRepo(ctx, registryClient, fromAddr)
	}

	if err != nil {
		return fmt.Errorf("read chart from %q: %w", from, err)
	}

	loadCtx := common.ContextWithHelmOptions(ctx, common.HelmOptions{
		ChartLoadOpts: common.ChartLoadOptions{
			ChartType: common.LegacyChartTypeBundle,
			NoSecrets: true,
		},
	})

	loadedChart, err := loader.LoadArchive(loadCtx, bytes.NewReader(source.ChartData))
	if err != nil {
		return fmt.Errorf("load chart from %q: %w", from, err)
	}

	images, err := chartyaml.ParseImages(ctx, chartAnnotations(loadedChart)[chartyaml.ImagesAnnotation])
	if err != nil {
		return fmt.Errorf("read images of chart from %q: %w", from, err)
	}

	imageRefs := lo.Uniq(lo.Compact(lo.Map(append(images, opts.Images...), func(image chartyaml.Image, _ int) string {
		return image.Image
	})))

	if toAddr.ArchivePath != "" {
		if err := writeChartCopyArchive(ctx, source, loadedChart, imageRefs, toAddr.ArchivePath, tempDir); err != nil {
			return fmt.Errorf("write chart archive %q: %w", toAddr.ArchivePath, err)
		}
	} else if err := copyChartToRepo(ctx, registryClient, source, loadedChart, imageRefs, toAddr, tempDir, opts); err != nil {
		return fmt.Errorf("copy chart to %q: %w", toAddr.Repo, err)
	}

	log.Default.Info(ctx, "Copied chart %s to %s", from, to)

	return nil
}

// ChartVersionFromTag returns the version of a chart pushed under tag: a strict semver tag,
// optionally prefixed with "v", is used as is without the prefix; any other tag gives the
// prerelease version 0.0.0-<unix time of now>-<tag slug>.
func ChartVersionFromTag(ctx context.Context, tag string, now time.Time) string {
	if version := strings.TrimPrefix(tag, "v"); version != "" {
		if _, err := semver.StrictNewVersion(version); err == nil {
			return version
		}
	}

	tagSlug := strings.Trim(strings.ReplaceAll(slug.Make(tag), "_", "-"), "-")
	if tagSlug == "" {
		return fmt.Sprintf("0.0.0-%d", now.Unix())
	}

	return fmt.Sprintf("0.0.0-%d-%s", now.Unix(), tagSlug)
}

func readChartCopyArchive(ctx context.Context, archivePath, tempDir string) (*chartCopySource, error) {
	tarPath, err := uncompressedChartCopyArchive(archivePath, tempDir)
	if err != nil {
		return nil, err
	}

	var (
		indexData                  []byte
		isOCILayout, isWerfArchive bool
	)

	if err := walkTar(tarPath, func(header *tar.Header, reader io.Reader) error {
		switch path.Clean(header.Name) {
		case ocispec.ImageLayoutFile:
			isOCILayout = true
		case ocispec.ImageIndexFile:
			var err error
			if indexData, err = io.ReadAll(reader); err != nil {
				return fmt.Errorf("read %s: %w", ocispec.ImageIndexFile, err)
			}
		case werfArchiveChartFileName:
			isWerfArchive = true
		}

		return nil
	}); err != nil {
		return nil, err
	}

	switch {
	case isOCILayout:
		return readChartCopyOCILayout(ctx, tarPath, indexData)
	case isWerfArchive:
		return readWerfArchive(ctx, tarPath, tempDir)
	default:
		return nil, fmt.Errorf("unknown archive format: neither %s nor %s found", ocispec.ImageLayoutFile, werfArchiveChartFileName)
	}
}

// readWerfArchive reads the werf 3 bundle archive: chart.tar.gz and a docker-save tarball per image
// tag in images/<tag>.tar.gz. The images are converted into OCI blobs in a temporary store.
func readWerfArchive(ctx context.Context, tarPath, tempDir string) (*chartCopySource, error) {
	storeDir, err := os.MkdirTemp(tempDir, "werf-archive-images-")
	if err != nil {
		return nil, fmt.Errorf("create temporary directory: %w", err)
	}

	store, err := oci.New(storeDir)
	if err != nil {
		return nil, fmt.Errorf("create image store: %w", err)
	}

	source := &chartCopySource{}
	images := map[string]ocispec.Descriptor{}

	if err := walkTar(tarPath, func(header *tar.Header, reader io.Reader) error {
		name := path.Clean(header.Name)

		switch {
		case name == werfArchiveChartFileName:
			var err error
			if source.ChartData, err = io.ReadAll(reader); err != nil {
				return fmt.Errorf("read %s: %w", name, err)
			}
		case path.Dir(name) == werfArchiveImagesDirName && strings.HasSuffix(name, ".tar.gz"):
			gzipReader, err := gzip.NewReader(reader)
			if err != nil {
				return fmt.Errorf("read %s: %w", name, err)
			}
			defer gzipReader.Close()

			desc, err := convertDockerArchiveImage(ctx, gzipReader, store, tempDir)
			if err != nil {
				return fmt.Errorf("read image %s: %w", name, err)
			}

			images[strings.TrimSuffix(path.Base(name), ".tar.gz")] = desc
		}

		return nil
	}); err != nil {
		return nil, err
	}

	if source.ChartData == nil {
		return nil, fmt.Errorf("no %s in the archive", werfArchiveChartFileName)
	}

	source.ResolveImage = func(ctx context.Context, imageRef string) (content.ReadOnlyStorage, ocispec.Descriptor, error) {
		tag := parseChartCopyImageRef(imageRef).Tag

		desc, found := images[tag]
		if !found {
			return nil, ocispec.Descriptor{}, fmt.Errorf("no %s/%s.tar.gz in the archive", werfArchiveImagesDirName, tag)
		}

		return store, desc, nil
	}

	return source, nil
}

// convertDockerArchiveImage pushes the image of a docker-save tarball into store as a Docker
// manifest, gzipping uncompressed layers, and returns the manifest descriptor.
func convertDockerArchiveImage(ctx context.Context, in io.Reader, store *oci.Store, tempDir string) (ocispec.Descriptor, error) {
	dir, err := os.MkdirTemp(tempDir, "image-")
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)

	files := map[string]string{}

	tarReader := tar.NewReader(in)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("read image tarball: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		filePath := filepath.Join(dir, strconv.Itoa(len(files)))
		if err := writeFileFromReader(filePath, tarReader); err != nil {
			return ocispec.Descriptor{}, err
		}

		files[path.Clean(header.Name)] = filePath
	}

	if files["manifest.json"] == "" {
		return ocispec.Descriptor{}, errors.New("no manifest.json in the image tarball")
	}

	manifestData, err := os.ReadFile(files["manifest.json"])
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("read manifest.json: %w", err)
	}

	var tarballManifests []struct {
		Config string   `json:"Config"`
		Layers []string `json:"Layers"`
	}
	if err := json.Unmarshal(manifestData, &tarballManifests); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("parse manifest.json: %w", err)
	}

	if len(tarballManifests) != 1 {
		return ocispec.Descriptor{}, fmt.Errorf("expected one image in manifest.json, found %d", len(tarballManifests))
	}

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: dockerManifestMediaType,
	}

	if manifest.Config, err = pushChartCopyFileBlob(ctx, store, files[path.Clean(tarballManifests[0].Config)], dockerConfigMediaType, false); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push config %q: %w", tarballManifests[0].Config, err)
	}

	for _, layer := range tarballManifests[0].Layers {
		layerDesc, err := pushChartCopyFileBlob(ctx, store, files[path.Clean(layer)], dockerLayerMediaType, true)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("push layer %q: %w", layer, err)
		}

		manifest.Layers = append(manifest.Layers, layerDesc)
	}

	manifestData, err = json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshal manifest: %w", err)
	}

	desc := content.NewDescriptorFromBytes(dockerManifestMediaType, manifestData)
	if err := pushIfMissing(ctx, store, desc, bytes.NewReader(manifestData)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push manifest: %w", err)
	}

	return desc, nil
}

func writeChartCopyArchive(ctx context.Context, source *chartCopySource, loadedChart helmchart.Charter, imageRefs []string, archivePath, tempDir string) error {
	layoutDir, err := os.MkdirTemp(tempDir, "oci-layout-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}

	store, err := oci.New(layoutDir)
	if err != nil {
		return fmt.Errorf("create OCI image layout: %w", err)
	}

	chartAccessor, err := helmchart.NewAccessor(loadedChart)
	if err != nil {
		return fmt.Errorf("create chart accessor: %w", err)
	}

	chartDesc, err := pushChartCopyManifest(ctx, store, chartAccessor, source.ChartData, source.ProvData)
	if err != nil {
		return err
	}

	chartDesc.Annotations = map[string]string{chartCopyArchiveEntryAnnotation: chartCopyArchiveEntryChart}
	if err := store.Tag(ctx, chartDesc, lo.CoalesceOrEmpty(source.Tag, strings.ReplaceAll(chartVersion(loadedChart), "+", "_"))); err != nil {
		return fmt.Errorf("tag chart: %w", err)
	}

	for _, imageRef := range imageRefs {
		storage, desc, err := source.ResolveImage(ctx, imageRef)
		if err != nil {
			return fmt.Errorf("resolve image %q: %w", imageRef, err)
		}

		if err := oras.CopyGraph(ctx, storage, store, desc, oras.DefaultCopyGraphOptions); err != nil {
			return fmt.Errorf("copy image %q: %w", imageRef, err)
		}

		desc.Annotations = map[string]string{chartCopyArchiveEntryAnnotation: chartCopyArchiveEntryImage}
		if err := store.Tag(ctx, desc, imageRef); err != nil {
			return fmt.Errorf("tag image %q: %w", imageRef, err)
		}

		log.Default.Info(ctx, "Saved image %s", imageRef)
	}

	if err := keepAnnotatedIndexEntries(layoutDir); err != nil {
		return err
	}

	return writeOCILayoutTar(layoutDir, archivePath, strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tgz"))
}

func copyChartToRepo(ctx context.Context, registryClient *registry.Client, source *chartCopySource, loadedChart helmchart.Charter, imageRefs []string, to *chartCopyAddress, tempDir string, opts ChartCopyOptions) error {
	repository, err := registryClient.Generic().Repository(to.Repo)
	if err != nil {
		return fmt.Errorf("construct repository: %w", err)
	}

	imageMapping := map[string]string{}
	for _, imageRef := range imageRefs {
		storage, desc, err := source.ResolveImage(ctx, imageRef)
		if err != nil {
			return fmt.Errorf("resolve image %q: %w", imageRef, err)
		}

		if err := oras.CopyGraph(ctx, storage, repository, desc, oras.DefaultCopyGraphOptions); err != nil {
			return fmt.Errorf("copy image %q: %w", imageRef, err)
		}

		image := parseChartCopyImageRef(imageRef)
		if image.Tag != "" {
			if err := repository.Tag(ctx, desc, image.Tag); err != nil {
				return fmt.Errorf("tag copy of image %q: %w", imageRef, err)
			}
		}

		newImageRef := to.Repo
		if image.Tag != "" {
			newImageRef += ":" + image.Tag
		}

		if image.Digest != "" || image.Tag == "" {
			newImageRef += "@" + desc.Digest.String()
		}

		imageMapping[imageRef] = newImageRef

		log.Default.Info(ctx, "Copied image %s to %s", imageRef, newImageRef)
	}

	chartAccessor, err := helmchart.NewAccessor(loadedChart)
	if err != nil {
		return fmt.Errorf("create chart accessor: %w", err)
	}

	sourceVersion := chartVersion(loadedChart)

	version, extraTag := sourceVersion, source.Tag
	if to.Tag != "" {
		version, extraTag = ChartVersionFromTag(ctx, to.Tag, time.Now()), to.Tag
	}

	edit := chartyaml.EditOptions{}
	if name := path.Base(to.Repo); name != chartAccessor.Name() {
		edit.Name = name
	}

	if version != sourceVersion {
		edit.Version = version
	}

	imagesAnnotation := chartAnnotations(loadedChart)[chartyaml.ImagesAnnotation]
	if imagesAnnotation != "" {
		newImagesAnnotation, err := chartyaml.RewriteImages(ctx, imagesAnnotation, imageMapping)
		if err != nil {
			return fmt.Errorf("rewrite %s annotation: %w", chartyaml.ImagesAnnotation, err)
		}

		if newImagesAnnotation != imagesAnnotation {
			edit.SetAnnotations = map[string]string{chartyaml.ImagesAnnotation: newImagesAnnotation}
		}
	}

	snapshotBefore, err := chartCopySnapshot(chartAccessor)
	if err != nil {
		return err
	}

	if opts.RewriteChart != nil {
		if err := opts.RewriteChart(ctx, loadedChart, to.Repo, imageMapping); err != nil {
			return fmt.Errorf("rewrite chart: %w", err)
		}
	}

	snapshotAfter, err := chartCopySnapshot(chartAccessor)
	if err != nil {
		return err
	}

	chartData, provData := source.ChartData, source.ProvData
	if edit.Name != "" || edit.Version != "" || len(edit.SetAnnotations) > 0 || !bytes.Equal(snapshotBefore, snapshotAfter) {
		if chartData, provData, err = saveChartCopy(ctx, loadedChart, chartAccessor, edit, source.ProvData != nil, tempDir, opts); err != nil {
			return err
		}
	}

	if _, err := registryClient.Push(chartData, fmt.Sprintf("%s:%s", to.Repo, version), registry.PushOptProvData(provData), registry.PushOptExtraTags(lo.Compact([]string{extraTag}))); err != nil {
		return fmt.Errorf("push chart: %w", err)
	}

	return nil
}

func parseChartCopyAddress(addr string) (*chartCopyAddress, error) {
	if archivePath, found := strings.CutPrefix(addr, "archive:"); found {
		if archivePath == "" {
			return nil, errors.New("archive path is empty")
		}

		return &chartCopyAddress{ArchivePath: archivePath}, nil
	}

	if !strings.Contains(addr, "://") && lo.SomeBy([]string{".tar", ".tar.gz", ".tgz"}, func(suffix string) bool {
		return strings.HasSuffix(addr, suffix)
	}) {
		return &chartCopyAddress{ArchivePath: addr}, nil
	}

	ref := strings.TrimPrefix(strings.TrimPrefix(addr, registry.OCIScheme+"://"), "docker://")
	if strings.Contains(ref, "@") {
		return nil, errors.New("digest references are not supported, use a tag")
	}

	image := parseChartCopyImageRef(ref)
	if _, err := orasregistry.ParseReference(image.Repo); err != nil {
		return nil, fmt.Errorf("parse repository: %w", err)
	}

	return &chartCopyAddress{Repo: image.Repo, Tag: image.Tag}, nil
}

// pushChartCopyFileBlob pushes the file into store, gzipping it first when gzipped is true and the
// file is not compressed yet.
func pushChartCopyFileBlob(ctx context.Context, store *oci.Store, filePath, mediaType string, gzipped bool) (ocispec.Descriptor, error) {
	if filePath == "" {
		return ocispec.Descriptor{}, errors.New("file not found in the image tarball")
	}

	if gzipped {
		compressed, err := isGzipFile(filePath)
		if err != nil {
			return ocispec.Descriptor{}, err
		}

		if !compressed {
			if err := gzipFile(filePath, filePath+".gz"); err != nil {
				return ocispec.Descriptor{}, err
			}

			filePath += ".gz"
		}
	}

	file, err := os.Open(filePath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("open blob: %w", err)
	}
	defer file.Close()

	digester := digest.Canonical.Digester()

	size, err := io.Copy(digester.Hash(), file)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("digest blob: %w", err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("rewind blob: %w", err)
	}

	desc := ocispec.Descriptor{
		Digest:    digester.Digest(),
		MediaType: mediaType,
		Size:      size,
	}

	if err := pushIfMissing(ctx, store, desc, file); err != nil {
		return ocispec.Descriptor{}, err
	}

	return desc, nil
}

func readChartCopyRepo(ctx context.Context, registryClient *registry.Client, addr *chartCopyAddress) (*chartCopySource, error) {
	tag := lo.CoalesceOrEmpty(addr.Tag, "latest")

	result, err := registryClient.Pull(fmt.Sprintf("%s:%s", addr.Repo, tag), registry.PullOptWithProv(true), registry.PullOptIgnoreMissingProv(true))
	if err != nil {
		return nil, fmt.Errorf("pull chart: %w", err)
	}

	return &chartCopySource{
		ChartData: result.Chart.Data,
		ProvData:  lo.Ternary(len(result.Prov.Data) > 0, result.Prov.Data, nil),
		ResolveImage: func(ctx context.Context, imageRef string) (content.ReadOnlyStorage, ocispec.Descriptor, error) {
			image := parseChartCopyImageRef(imageRef)

			repository, err := registryClient.Generic().Repository(imageRepositoryAddress(image.Repo))
			if err != nil {
				return nil, ocispec.Descriptor{}, fmt.Errorf("construct repository: %w", err)
			}

			desc, err := repository.Resolve(ctx, lo.CoalesceOrEmpty(image.Digest, image.Tag, "latest"))
			if err != nil {
				return nil, ocispec.Descriptor{}, fmt.Errorf("resolve image: %w", err)
			}

			return repository, desc, nil
		},
		Tag: tag,
	}, nil
}

// uncompressedChartCopyArchive returns the path to the tar of the archive, decompressing a gzipped
// archive into tempDir.
func uncompressedChartCopyArchive(archivePath, tempDir string) (string, error) {
	compressed, err := isGzipFile(archivePath)
	if err != nil {
		return "", err
	}

	if !compressed {
		return archivePath, nil
	}

	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive: %w", err)
	}
	defer archiveFile.Close()

	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return "", fmt.Errorf("read archive: %w", err)
	}
	defer gzipReader.Close()

	tarPath := filepath.Join(tempDir, "archive.tar")
	if err := writeFileFromReader(tarPath, gzipReader); err != nil {
		return "", err
	}

	return tarPath, nil
}

// writeOCILayoutTar writes the OCI image layout in dir into a tar at archivePath, replacing it only
// when the whole tar is written.
func writeOCILayoutTar(dir, archivePath string, compress bool) error {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return fmt.Errorf("create archive directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(archivePath), filepath.Base(archivePath)+".tmp")
	if err != nil {
		return fmt.Errorf("create temporary archive: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	var (
		writer     io.Writer = tmpFile
		gzipWriter *gzip.Writer
	)

	if compress {
		gzipWriter = gzip.NewWriter(tmpFile)
		writer = gzipWriter
	}

	tarWriter := tar.NewWriter(writer)

	names := []string{ocispec.ImageLayoutFile, ocispec.ImageIndexFile}

	algorithmDirs, err := os.ReadDir(filepath.Join(dir, ocispec.ImageBlobsDir))
	if err != nil {
		return fmt.Errorf("read blobs directory: %w", err)
	}

	for _, algorithmDir := range algorithmDirs {
		blobs, err := os.ReadDir(filepath.Join(dir, ocispec.ImageBlobsDir, algorithmDir.Name()))
		if err != nil {
			return fmt.Errorf("read blobs directory: %w", err)
		}

		for _, blob := range blobs {
			names = append(names, path.Join(ocispec.ImageBlobsDir, algorithmDir.Name(), blob.Name()))
		}
	}

	for _, name := range names {
		if err := addFileToTar(tarWriter, filepath.Join(dir, filepath.FromSlash(name)), name); err != nil {
			return err
		}
	}

	if err := tarWriter.Close(); err != nil {
		return fmt.Errorf("write archive: %w", err)
	}

	if gzipWriter != nil {
		if err := gzipWriter.Close(); err != nil {
			return fmt.Errorf("write archive: %w", err)
		}
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("write archive: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), archivePath); err != nil {
		return fmt.Errorf("move archive into place: %w", err)
	}

	return nil
}

func addFileToTar(tarWriter *tar.Writer, filePath, name string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open %q: %w", filePath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat %q: %w", filePath, err)
	}

	if err := tarWriter.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     name,
		Mode:     0o644,
		Size:     info.Size(),
		ModTime:  info.ModTime(),
	}); err != nil {
		return fmt.Errorf("write %q into archive: %w", name, err)
	}

	if _, err := io.Copy(tarWriter, file); err != nil {
		return fmt.Errorf("write %q into archive: %w", name, err)
	}

	return nil
}

// chartCopySnapshot serializes everything a chart archive is saved from, to tell whether the chart
// changed.
func chartCopySnapshot(chartAccessor helmchart.Accessor) ([]byte, error) {
	var dependencies []json.RawMessage
	for _, dep := range chartAccessor.Dependencies() {
		depAccessor, err := helmchart.NewAccessor(dep)
		if err != nil {
			return nil, fmt.Errorf("create dependency accessor: %w", err)
		}

		depSnapshot, err := chartCopySnapshot(depAccessor)
		if err != nil {
			return nil, err
		}

		dependencies = append(dependencies, depSnapshot)
	}

	snapshot, err := json.Marshal([]any{
		chartAccessor.MetadataAsMap(),
		chartAccessor.RawFiles(),
		chartAccessor.Files(),
		chartAccessor.Templates(),
		chartAccessor.RuntimeFiles(),
		chartAccessor.WerfDataFiles(),
		chartAccessor.Values(),
		chartAccessor.Schema(),
		dependencies,
	})
	if err != nil {
		return nil, fmt.Errorf("serialize chart: %w", err)
	}

	return snapshot, nil
}

func gzipFile(sourcePath, destinationPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open %q: %w", sourcePath, err)
	}
	defer source.Close()

	destination, err := os.Create(destinationPath)
	if err != nil {
		return fmt.Errorf("create %q: %w", destinationPath, err)
	}
	defer destination.Close()

	gzipWriter := gzip.NewWriter(destination)
	if _, err := io.Copy(gzipWriter, source); err != nil {
		return fmt.Errorf("compress %q: %w", sourcePath, err)
	}

	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("compress %q: %w", sourcePath, err)
	}

	if err := destination.Close(); err != nil {
		return fmt.Errorf("write %q: %w", destinationPath, err)
	}

	return nil
}

// imageRepositoryAddress returns the address to connect to for an image repository, expanding Docker
// Hub short names as Docker does.
func imageRepositoryAddress(repo string) string {
	host, repoPath, found := strings.Cut(repo, "/")
	if !found || !strings.ContainsAny(host, ".:") && host != "localhost" {
		host, repoPath = "docker.io", repo
		if !strings.Contains(repoPath, "/") {
			repoPath = "library/" + repoPath
		}
	}

	if host == "docker.io" || host == "index.docker.io" {
		host = "registry-1.docker.io"
	}

	return host + "/" + repoPath
}

func isGzipFile(filePath string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, fmt.Errorf("open %q: %w", filePath, err)
	}
	defer file.Close()

	magic := make([]byte, 2)
	if _, err := io.ReadFull(file, magic); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}

		return false, fmt.Errorf("read %q: %w", filePath, err)
	}

	return magic[0] == 0x1f && magic[1] == 0x8b, nil
}

// keepAnnotatedIndexEntries drops from index.json the manifests the OCI store lists only because
// they were pushed, like the per-platform manifests of an image index, leaving the chart and the
// images.
func keepAnnotatedIndexEntries(layoutDir string) error {
	indexPath := filepath.Join(layoutDir, ocispec.ImageIndexFile)

	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", ocispec.ImageIndexFile, err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		return fmt.Errorf("parse %s: %w", ocispec.ImageIndexFile, err)
	}

	index.Manifests = lo.Filter(index.Manifests, func(desc ocispec.Descriptor, _ int) bool {
		return desc.Annotations[chartCopyArchiveEntryAnnotation] != ""
	})

	if indexData, err = json.Marshal(index); err != nil {
		return fmt.Errorf("marshal %s: %w", ocispec.ImageIndexFile, err)
	}

	if err := os.WriteFile(indexPath, indexData, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ocispec.ImageIndexFile, err)
	}

	return nil
}

func parseChartCopyImageRef(ref string) chartCopyImageRef {
	var result chartCopyImageRef

	if i := strings.LastIndex(ref, "@"); i >= 0 {
		result.Digest = ref[i+1:]
		ref = ref[:i]
	}

	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		result.Tag = ref[i+1:]
		ref = ref[:i]
	}

	result.Repo = ref

	return result
}

// pushChartCopyManifest pushes the chart and its provenance into store as Helm does into a
// registry.
func pushChartCopyManifest(ctx context.Context, store *oci.Store, chartAccessor helmchart.Accessor, chartData, provData []byte) (ocispec.Descriptor, error) {
	configData, err := json.Marshal(chartAccessor.MetadataAsMap())
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshal chart metadata: %w", err)
	}

	configDesc, err := oras.PushBytes(ctx, store, registry.ConfigMediaType, configData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push chart config: %w", err)
	}

	chartDesc, err := oras.PushBytes(ctx, store, registry.ChartLayerMediaType, chartData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push chart layer: %w", err)
	}

	layers := []ocispec.Descriptor{chartDesc}
	if provData != nil {
		provDesc, err := oras.PushBytes(ctx, store, registry.ProvLayerMediaType, provData)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("push provenance layer: %w", err)
		}

		layers = append(layers, provDesc)
	}

	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Digest < layers[j].Digest
	})

	manifestData, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    layers,
	})
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("marshal chart manifest: %w", err)
	}

	manifestDesc, err := oras.PushBytes(ctx, store, ocispec.MediaTypeImageManifest, manifestData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push chart manifest: %w", err)
	}

	return manifestDesc, nil
}

func pushIfMissing(ctx context.Context, store *oci.Store, desc ocispec.Descriptor, reader io.Reader) error {
	exists, err := store.Exists(ctx, desc)
	if err != nil {
		return fmt.Errorf("check blob %s: %w", desc.Digest, err)
	}

	if exists {
		return nil
	}

	if err := store.Push(ctx, desc, reader); err != nil {
		return fmt.Errorf("push blob %s: %w", desc.Digest, err)
	}

	return nil
}

func readChartCopyOCILayout(ctx context.Context, tarPath string, indexData []byte) (*chartCopySource, error) {
	store, err := oci.NewFromTar(ctx, tarPath)
	if err != nil {
		return nil, fmt.Errorf("open OCI image layout: %w", err)
	}

	var index ocispec.Index
	if err := json.Unmarshal(indexData, &index); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ocispec.ImageIndexFile, err)
	}

	chartDescs := lo.Filter(index.Manifests, func(desc ocispec.Descriptor, _ int) bool {
		return desc.Annotations[chartCopyArchiveEntryAnnotation] == chartCopyArchiveEntryChart
	})
	if len(chartDescs) != 1 {
		return nil, fmt.Errorf("expected one chart in the archive, found %d", len(chartDescs))
	}

	manifestData, err := content.FetchAll(ctx, store, chartDescs[0])
	if err != nil {
		return nil, fmt.Errorf("read chart manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("parse chart manifest: %w", err)
	}

	source := &chartCopySource{
		ResolveImage: func(ctx context.Context, imageRef string) (content.ReadOnlyStorage, ocispec.Descriptor, error) {
			desc, found := lo.Find(index.Manifests, func(desc ocispec.Descriptor) bool {
				return desc.Annotations[chartCopyArchiveEntryAnnotation] == chartCopyArchiveEntryImage && desc.Annotations[ocispec.AnnotationRefName] == imageRef
			})
			if !found {
				return nil, ocispec.Descriptor{}, errors.New("image not found in the archive")
			}

			return store, desc, nil
		},
		Tag: chartDescs[0].Annotations[ocispec.AnnotationRefName],
	}

	for _, layer := range manifest.Layers {
		switch layer.MediaType {
		case registry.ChartLayerMediaType, registry.LegacyChartLayerMediaType:
			if source.ChartData, err = content.FetchAll(ctx, store, layer); err != nil {
				return nil, fmt.Errorf("read chart layer: %w", err)
			}
		case registry.ProvLayerMediaType:
			if source.ProvData, err = content.FetchAll(ctx, store, layer); err != nil {
				return nil, fmt.Errorf("read provenance layer: %w", err)
			}
		}
	}

	if source.ChartData == nil {
		return nil, fmt.Errorf("chart manifest has no layer with media type %s", registry.ChartLayerMediaType)
	}

	return source, nil
}

// saveChartCopy returns the archive of the edited chart and its provenance, signing it if
// requested.
func saveChartCopy(ctx context.Context, loadedChart helmchart.Charter, chartAccessor helmchart.Accessor, edit chartyaml.EditOptions, sourceSigned bool, tempDir string, opts ChartCopyOptions) ([]byte, []byte, error) {
	destination, err := os.MkdirTemp(tempDir, "chart-")
	if err != nil {
		return nil, nil, fmt.Errorf("create temporary directory: %w", err)
	}

	archivePath, err := saveChartPackArchive(loadedChart, chartAccessor.Files(), chartAccessor.WerfDataFiles(), edit, destination)
	if err != nil {
		return nil, nil, fmt.Errorf("save chart archive: %w", err)
	}

	if err := rewriteChartPackArchiveMetadata(ctx, archivePath, chartAccessor, edit); err != nil {
		return nil, nil, fmt.Errorf("write Chart.yaml and Chart.lock into the archive: %w", err)
	}

	chartData, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, nil, fmt.Errorf("read chart archive: %w", err)
	}

	if !opts.Sign {
		if sourceSigned {
			log.Default.Warn(ctx, "Chart is signed, but its copy is not: the copy changes the chart and no signing key is given")
		}

		return chartData, nil, nil
	}

	pkg := &helmaction.Package{
		Key:            opts.SignKey,
		Keyring:        opts.SignKeyring,
		PassphraseFile: opts.SignPassphraseFile,
	}

	if err := pkg.Clearsign(archivePath); err != nil {
		return nil, nil, fmt.Errorf("sign chart archive: %w", err)
	}

	provData, err := os.ReadFile(archivePath + ".prov")
	if err != nil {
		return nil, nil, fmt.Errorf("read provenance file: %w", err)
	}

	return chartData, provData, nil
}

func walkTar(tarPath string, fn func(header *tar.Header, reader io.Reader) error) error {
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer tarFile.Close()

	tarReader := tar.NewReader(tarFile)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		if err := fn(header, tarReader); err != nil {
			return err
		}
	}
}

func writeFileFromReader(filePath string, reader io.Reader) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create %q: %w", filePath, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("write %q: %w", filePath, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("write %q: %w", filePath, err)
	}

	return nil
}
