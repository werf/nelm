package action

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Masterminds/semver/v3"
	"github.com/gosimple/unidecode"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/lo"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"
	"oras.land/oras-go/v2/errdef"
	orasregistry "oras.land/oras-go/v2/registry"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/v2/pkg/chartyaml"
	"github.com/werf/nelm/v2/pkg/common"
	v3chart "github.com/werf/nelm/v2/pkg/helm/intern/chart/v3"
	helmaction "github.com/werf/nelm/v2/pkg/helm/pkg/action"
	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader"
	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	v2util "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2/util"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/log"
)

const (
	// DefaultChartCopyArchiveMaxExpandedSize is the default ChartCopyOptions.ArchiveMaxExpandedSize.
	DefaultChartCopyArchiveMaxExpandedSize int64 = 64 << 30

	chartCopyLayerFormatGzip    chartCopyLayerFormatType = "gzip"
	chartCopyLayerFormatTar     chartCopyLayerFormatType = "tar"
	chartCopyLayerFormatUnknown chartCopyLayerFormatType = "unknown"
	chartCopyLayerFormatZstd    chartCopyLayerFormatType = "zstd"

	// chartCopyArchiveEntryAnnotation marks the index.json entries of a chart copy archive: the chart,
	// tagged with its tag, and the images, tagged with their refs from the helm.sh/images annotation.
	chartCopyArchiveEntryAnnotation = "nelm.werf.io/archive-entry"
	chartCopyArchiveEntryChart      = "chart"
	chartCopyArchiveEntryImage      = "image"
	chartCopyArchiveMaxEntries      = 100_000
	chartCopyArchiveMaxMetadataSize = 16 << 20
	// chartCopyMaxTagLength is the maximum length of an OCI tag.
	chartCopyMaxTagLength    = 128
	dockerConfigMediaType    = "application/vnd.docker.container.image.v1+json"
	dockerLayerMediaType     = "application/vnd.docker.image.rootfs.diff.tar.gzip"
	dockerManifestMediaType  = "application/vnd.docker.distribution.manifest.v2+json"
	werfArchiveChartFileName = "chart.tar.gz"
	werfArchiveImagesDirName = "images"
)

type chartCopyLayerFormatType string

// chartCopyLayoutStore is a read-only store of the blobs of an OCI image layout tar. Unlike the oras
// one, it reads nothing up front: descriptors in an archive are untrusted, and the oras store
// indexes the whole graph, reading each manifest into a buffer of its declared size.
type chartCopyLayoutStore struct {
	blobs   map[digest.Digest]chartCopyTarMember
	tarPath string
}

func newChartCopyLayoutStore(ctx context.Context, tarPath string) (*chartCopyLayoutStore, error) {
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer tarFile.Close()

	store := &chartCopyLayoutStore{blobs: map[digest.Digest]chartCopyTarMember{}, tarPath: tarPath}

	tarReader := tar.NewReader(tarFile)
	for entries := 0; ; entries++ {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}

		if entries > chartCopyArchiveMaxEntries {
			return nil, fmt.Errorf("archive has more than %d entries", chartCopyArchiveMaxEntries)
		}

		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return store, nil
		}

		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}

		// The blobs are read at their offsets in the archive, which holds only the data of a
		// sparse file.
		if err := checkChartCopyTarMember(header); err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}

		name := chartCopyArchiveMemberName(header.Name)

		algorithm, encoded, found := strings.Cut(strings.TrimPrefix(name, ocispec.ImageBlobsDir+"/"), "/")
		if header.Typeflag != tar.TypeReg || !strings.HasPrefix(name, ocispec.ImageBlobsDir+"/") || !found {
			continue
		}

		dgst := digest.NewDigestFromEncoded(digest.Algorithm(algorithm), encoded)
		if err := dgst.Validate(); err != nil {
			return nil, fmt.Errorf("blob %q: %w", name, err)
		}

		if _, found := store.blobs[dgst]; found {
			return nil, fmt.Errorf("archive has duplicate member %q", name)
		}

		// Next leaves the archive positioned at the member data.
		offset, err := tarFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, fmt.Errorf("read archive: %w", err)
		}

		store.blobs[dgst] = chartCopyTarMember{Offset: offset, Size: header.Size}
	}
}

func (s *chartCopyLayoutStore) Exists(ctx context.Context, target ocispec.Descriptor) (bool, error) {
	member, found := s.blobs[target.Digest]

	return found && member.Size == target.Size, nil
}

// Fetch returns the blob of target, checking its declared size against the blob and, for the
// manifests and indexes that oras reads into memory, against the metadata limit.
func (s *chartCopyLayoutStore) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	member, found := s.blobs[target.Digest]
	if !found {
		return nil, fmt.Errorf("%s: %w", target.Digest, errdef.ErrNotFound)
	}

	if target.Size != member.Size {
		return nil, fmt.Errorf("blob %s is declared to be %d bytes, the blob in the archive is %d bytes", target.Digest, target.Size, member.Size)
	}

	if isChartCopyManifestMediaType(target.MediaType) && target.Size > chartCopyArchiveMaxMetadataSize {
		return nil, fmt.Errorf("manifest %s is %d bytes, more than %d bytes allowed", target.Digest, target.Size, chartCopyArchiveMaxMetadataSize)
	}

	tarFile, err := os.Open(s.tarPath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}

	if _, err := tarFile.Seek(member.Offset, io.SeekStart); err != nil {
		tarFile.Close()

		return nil, fmt.Errorf("read archive: %w", err)
	}

	return struct {
		io.Reader
		io.Closer
	}{Reader: io.LimitReader(tarFile, member.Size), Closer: tarFile}, nil
}

type chartCopyTarMember struct {
	Offset int64
	Size   int64
}

type ChartCopyOptions struct {
	common.ChartRepoConnectionOptions

	// ArchiveMaxExpandedSize limits the bytes decompressed from a source archive, nested image
	// tarballs included. Defaults to DefaultChartCopyArchiveMaxExpandedSize.
	ArchiveMaxExpandedSize int64
	// Images are copied in addition to the images in the helm.sh/images annotation, for charts that
	// list their images elsewhere, like werf bundles published before the annotation was written.
	// Only Image matters. The annotation is rewritten only for the entries it contains.
	Images []chartyaml.Image
	// ImagesFunc, when set, is called with the loaded source chart, before the images are resolved,
	// and the images it returns are copied like Images, for charts that list their images in values,
	// like werf bundles published before the helm.sh/images annotation was written.
	ImagesFunc func(ctx context.Context, chart helmchart.Charter) ([]chartyaml.Image, error)
	// RegistryCredentialsPath is the path to Docker config.json file with registry credentials.
	// Defaults to DefaultDockerConfig/config.json.
	RegistryCredentialsPath string
	// RewriteChart, when set, is called on a copy to a registry after the images are copied and
	// before the chart is saved, with the root chart, the destination repository and the ref of
	// every copied image, as written in the helm.sh/images annotations or in Images, mapped to the
	// ref of its copy. Changes it makes to the chart files, to the root chart annotations, to
	// dependency metadata and to values are pushed; parsed values it changes are written to
	// values.yaml, dropping its comments, unless it changes values.yaml too. Changing other root
	// Chart.yaml fields is an error. The helm.sh/images annotations of the chart and its
	// dependencies are rewritten by ChartCopy itself, before the hook.
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

type chartCopyResolvedImage struct {
	Desc    ocispec.Descriptor
	Ref     string
	Storage content.ReadOnlyStorage
	// Tag is empty for an image referenced by digest only.
	Tag string
}

type chartCopyTagClaim struct {
	// Digest is empty for the chart, whose digest is not known before it is pushed.
	Digest digest.Digest
	Owner  string
}

// chartCopyExpansion counts the bytes decompressed from an archive against Max.
type chartCopyExpansion struct {
	// Extracted counts the bytes extracted from tar members separately from Total, which counts
	// the decompressed streams the tars are read from.
	Extracted int64
	Max       int64
	Total     int64
}

// ExtractedReader is Reader for the content of a tar member, which tar.Reader produces from the
// stream counted by Reader.
func (e *chartCopyExpansion) ExtractedReader(ctx context.Context, reader io.Reader) io.Reader {
	return &chartCopyExpansionReader{counter: &e.Extracted, ctx: ctx, max: e.Max, reader: reader}
}

// Reader returns reader counting what it reads against the expansion limit and stopping on ctx
// cancellation.
func (e *chartCopyExpansion) Reader(ctx context.Context, reader io.Reader) io.Reader {
	return &chartCopyExpansionReader{counter: &e.Total, ctx: ctx, max: e.Max, reader: reader}
}

// chartCopyContextReader stops reading when ctx is done.
type chartCopyContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r chartCopyContextReader) Read(p []byte) (int, error) {
	if err := context.Cause(r.ctx); err != nil {
		return 0, err
	}

	return r.reader.Read(p) //nolint:wrapcheck // io.Reader returns io.EOF as is.
}

type chartCopyExpansionReader struct {
	counter *int64
	ctx     context.Context
	max     int64
	reader  io.Reader
}

func (r *chartCopyExpansionReader) Read(p []byte) (int, error) {
	if err := context.Cause(r.ctx); err != nil {
		return 0, err
	}

	n, err := r.reader.Read(p)

	*r.counter += int64(n)
	if *r.counter > r.max {
		return n, fmt.Errorf("archive expands to more than %d bytes", r.max)
	}

	return n, err //nolint:wrapcheck // io.Reader returns io.EOF as is.
}

// chartCopyHookState is the chart state before RewriteChart, to turn the changes it makes to the
// parsed chart into changes of the files the chart is saved from.
type chartCopyHookState struct {
	Annotations map[string]string
	// Metadata is the root chart metadata without annotations.
	Metadata []byte
	Values   []chartCopyValuesState
}

func newChartCopyHookState(chartAccessor helmchart.Accessor) (*chartCopyHookState, error) {
	metadata, err := chartCopyMetadataWithoutAnnotations(chartAccessor)
	if err != nil {
		return nil, fmt.Errorf("read chart metadata: %w", err)
	}

	values, err := chartCopyValuesStates(chartAccessor)
	if err != nil {
		return nil, fmt.Errorf("read chart values: %w", err)
	}

	return &chartCopyHookState{
		Annotations: maps.Clone(chartAnnotations(chartAccessor.Charter())),
		Metadata:    metadata,
		Values:      values,
	}, nil
}

// Persist adds the root chart annotations changed by the hook to edit and writes the values it
// changed into values.yaml, unless it changed values.yaml itself. Changes to other root Chart.yaml
// fields can't be saved and are an error.
func (s *chartCopyHookState) Persist(chartAccessor helmchart.Accessor, edit *chartyaml.EditOptions) error {
	metadata, err := chartCopyMetadataWithoutAnnotations(chartAccessor)
	if err != nil {
		return fmt.Errorf("read chart metadata: %w", err)
	}

	if !bytes.Equal(metadata, s.Metadata) {
		return errors.New("RewriteChart changed Chart.yaml fields other than annotations of the root chart, which chart copy can't save")
	}

	annotations := chartAnnotations(chartAccessor.Charter())
	for key, value := range annotations {
		if previous, found := s.Annotations[key]; !found || previous != value {
			edit.SetAnnotations = lo.Assign(edit.SetAnnotations, map[string]string{key: value})
		}
	}

	for key := range s.Annotations {
		if _, found := annotations[key]; !found {
			edit.DeleteAnnotations = append(edit.DeleteAnnotations, key)
		}
	}

	for _, state := range s.Values {
		stateAccessor, err := helmchart.NewAccessor(state.Chart)
		if err != nil {
			return fmt.Errorf("create chart accessor: %w", err)
		}

		values, err := json.Marshal(stateAccessor.Values())
		if err != nil {
			return fmt.Errorf("marshal values of chart %q: %w", stateAccessor.Name(), err)
		}

		rawValues := chartCopyRawValues(stateAccessor)
		if bytes.Equal(values, state.Values) || rawValues != nil && !bytes.Equal(rawValues.Data, state.RawValues) {
			continue
		}

		valuesYAML, err := yaml.Marshal(stateAccessor.Values())
		if err != nil {
			return fmt.Errorf("marshal values of chart %q: %w", stateAccessor.Name(), err)
		}

		if rawValues != nil {
			rawValues.Data = valuesYAML
		} else {
			addChartRawFile(state.Chart, &chartcommon.File{Name: v2util.ValuesfileName, Data: valuesYAML})
		}
	}

	return nil
}

type chartCopyValuesState struct {
	Chart helmchart.Charter
	// RawValues is nil when the chart has no values.yaml.
	RawValues []byte
	Values    []byte
}

type chartCopyImageRef struct {
	Digest string
	Repo   string
	Tag    string
}

// ChartCopy copies the chart at from, with every image in the helm.sh/images annotations of the
// chart and its dependencies, to to. Non-distributable layers are copied too; a blob the source
// does not serve is an error. Referrers of the images, like signatures and SBOMs, are not copied.
// Each address is either "oci://REPO[:TAG]" (or "docker://REPO[:TAG]" or "REPO[:TAG]", as in werf),
// or an archive: "archive:PATH" or a
// path ending in .tar, .tar.gz or .tgz. A source repository without a tag means the "latest" tag.
// An archive is written as an OCI image layout; the werf 3 bundle archive format is also read.
//
// When copying to a repository, the chart is renamed to the basename of the repository and the
// images are copied by digest into it, keeping their tags, and referenced as REPO[:TAG]@DIGEST. A
// tag that would point to different content is an error, raised before anything is pushed. With a
// tag, the chart version is ChartVersionFromTag of it and the chart is pushed under the version and
// the tag; without one the chart keeps its version and is pushed under it and the source tag, if
// any: a werf 3 archive has none, so only the version tag is pushed. The provenance is kept only if
// the chart doesn't change.
func ChartCopy(ctx context.Context, from, to string, opts ChartCopyOptions) error {
	if opts.Sign && (opts.SignKey == "" || opts.SignKeyring == "") {
		return errors.New("signing key name and keyring are required for signing")
	}

	if opts.RegistryCredentialsPath == "" {
		opts.RegistryCredentialsPath = filepath.Join(common.DefaultDockerConfig, "config.json")
	}

	switch {
	case opts.ArchiveMaxExpandedSize == 0:
		opts.ArchiveMaxExpandedSize = DefaultChartCopyArchiveMaxExpandedSize
	case opts.ArchiveMaxExpandedSize < 0:
		return fmt.Errorf("archive max expanded size must be positive, got %d", opts.ArchiveMaxExpandedSize)
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

	// The chart repo credentials are for the chart repositories only: image repositories on other
	// hosts must not get them.
	chartHosts := lo.FilterMap([]*chartCopyAddress{fromAddr, toAddr}, func(addr *chartCopyAddress, _ int) (string, bool) {
		return strings.Split(addr.Repo, "/")[0], addr.Repo != ""
	})

	clientOpts := []registry.ClientOption{registry.ClientOptBasicAuthHosts(chartHosts...)}

	// Plain HTTP is for the chart repositories only, even with TLS options, which are then for the
	// image repositories on other hosts.
	var plainHTTPHosts []string
	if opts.ChartRepoInsecure {
		plainHTTPHosts = chartHosts

		clientOpts = append(clientOpts, registry.ClientOptPlainHTTP())
	}

	registryClient, err := newChartRepoRegistryClient(ctx, opts.ChartRepoConnectionOptions, opts.RegistryCredentialsPath, clientOpts...)
	if err != nil {
		return fmt.Errorf("construct registry client: %w", err)
	}

	var source *chartCopySource
	if fromAddr.ArchivePath != "" {
		source, err = readChartCopyArchive(ctx, fromAddr.ArchivePath, tempDir, &chartCopyExpansion{Max: opts.ArchiveMaxExpandedSize})
	} else {
		source, err = readChartCopyRepo(ctx, registryClient, fromAddr, plainHTTPHosts)
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

	images, err := chartCopyImages(ctx, loadedChart)
	if err != nil {
		return fmt.Errorf("read images of chart from %q: %w", from, err)
	}

	if opts.ImagesFunc != nil {
		funcImages, err := opts.ImagesFunc(ctx, loadedChart)
		if err != nil {
			return fmt.Errorf("get images of chart from %q: %w", from, err)
		}

		images = append(images, funcImages...)
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
// prerelease version 0.0.0-<unix time of now>-<tag slug>, as werf does. The chart is pushed under
// its version, so a slug that would make the version longer than a tag can be is cut and suffixed
// with a hash of the tag to tell such tags apart.
func ChartVersionFromTag(ctx context.Context, tag string, now time.Time) string {
	if version := strings.TrimPrefix(tag, "v"); version != "" {
		if _, err := semver.StrictNewVersion(version); err == nil {
			return version
		}
	}

	tagSlug := chartVersionTagSlug(tag)
	if tagSlug == "" {
		return fmt.Sprintf("0.0.0-%d", now.Unix())
	}

	prefix := fmt.Sprintf("0.0.0-%d-", now.Unix())
	if len(prefix)+len(tagSlug) > chartCopyMaxTagLength {
		hash := sha256.Sum256([]byte(tag))
		hashSuffix := "-" + hex.EncodeToString(hash[:])[:8]
		tagSlug = strings.TrimRight(tagSlug[:chartCopyMaxTagLength-len(prefix)-len(hashSuffix)], "-") + hashSuffix
	}

	return prefix + tagSlug
}

func readChartCopyArchive(ctx context.Context, archivePath, tempDir string, expansion *chartCopyExpansion) (*chartCopySource, error) {
	tarPath, err := uncompressedChartCopyArchive(ctx, archivePath, tempDir, expansion)
	if err != nil {
		return nil, fmt.Errorf("decompress archive: %w", err)
	}

	var (
		indexData                  []byte
		isOCILayout, isWerfArchive bool
	)

	if err := walkTar(ctx, tarPath, func(header *tar.Header, reader io.Reader) error {
		switch header.Name {
		case ocispec.ImageLayoutFile:
			isOCILayout = true
		case ocispec.ImageIndexFile:
			var err error
			if indexData, err = readChartCopyArchiveMember(header, reader, chartCopyArchiveMaxMetadataSize); err != nil {
				return err
			}
		case werfArchiveChartFileName:
			isWerfArchive = true
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("detect archive format: %w", err)
	}

	switch {
	case isOCILayout:
		return readChartCopyOCILayout(ctx, tarPath, indexData)
	case isWerfArchive:
		return readWerfArchive(ctx, tarPath, tempDir, expansion)
	default:
		return nil, fmt.Errorf("unknown archive format: neither %s nor %s found", ocispec.ImageLayoutFile, werfArchiveChartFileName)
	}
}

func copyChartToRepo(ctx context.Context, registryClient *registry.Client, source *chartCopySource, loadedChart helmchart.Charter, imageRefs []string, to *chartCopyAddress, tempDir string, opts ChartCopyOptions) error {
	repository, err := registryClient.Generic().Repository(to.Repo)
	if err != nil {
		return fmt.Errorf("construct repository: %w", err)
	}

	chartAccessor, err := helmchart.NewAccessor(loadedChart)
	if err != nil {
		return fmt.Errorf("create chart accessor: %w", err)
	}

	sourceVersion := chartVersion(loadedChart)

	version, extraTag := sourceVersion, source.Tag
	if to.Tag != "" {
		version, extraTag = ChartVersionFromTag(ctx, to.Tag, time.Now()), strings.ReplaceAll(to.Tag, "+", "_")
	}

	versionTag := strings.ReplaceAll(version, "+", "_")
	if err := (orasregistry.Reference{Reference: versionTag}).ValidateReferenceAsTag(); err != nil {
		return fmt.Errorf("chart version %q can't be a tag: %w", version, err)
	}

	tagClaims := map[string]chartCopyTagClaim{}
	for _, tag := range lo.Compact([]string{versionTag, extraTag}) {
		if err := claimChartCopyTag(tagClaims, tag, chartCopyTagClaim{Owner: "the chart"}); err != nil {
			return fmt.Errorf("check destination tags: %w", err)
		}
	}

	images := make([]chartCopyResolvedImage, 0, len(imageRefs))
	for _, imageRef := range imageRefs {
		storage, desc, err := source.ResolveImage(ctx, imageRef)
		if err != nil {
			return fmt.Errorf("resolve image %q: %w", imageRef, err)
		}

		image := chartCopyResolvedImage{Desc: desc, Ref: imageRef, Storage: storage, Tag: parseChartCopyImageRef(imageRef).Tag}
		if image.Tag != "" {
			if err := claimChartCopyTag(tagClaims, image.Tag, chartCopyTagClaim{Digest: desc.Digest, Owner: fmt.Sprintf("image %q", imageRef)}); err != nil {
				return fmt.Errorf("check destination tags: %w", err)
			}
		}

		images = append(images, image)
	}

	imageMapping := map[string]string{}
	for _, image := range images {
		if err := copyChartCopyImage(ctx, image.Storage, repository, image.Desc); err != nil {
			return fmt.Errorf("copy image %q: %w", image.Ref, err)
		}

		newImageRef := to.Repo
		if image.Tag != "" {
			if err := repository.Tag(ctx, image.Desc, image.Tag); err != nil {
				return fmt.Errorf("tag copy of image %q: %w", image.Ref, err)
			}

			newImageRef += ":" + image.Tag
		}

		newImageRef += "@" + image.Desc.Digest.String()
		imageMapping[image.Ref] = newImageRef

		log.Default.Info(ctx, "Copied image %s to %s", image.Ref, newImageRef)
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
			chartAnnotations(loadedChart)[chartyaml.ImagesAnnotation] = newImagesAnnotation
		}
	}

	dependenciesChanged, err := rewriteChartCopyDependencyImages(ctx, chartAccessor, imageMapping)
	if err != nil {
		return fmt.Errorf("rewrite images of dependencies: %w", err)
	}

	snapshotBefore, err := chartCopySnapshot(chartAccessor)
	if err != nil {
		return fmt.Errorf("snapshot chart: %w", err)
	}

	if opts.RewriteChart != nil {
		hookState, err := newChartCopyHookState(chartAccessor)
		if err != nil {
			return fmt.Errorf("record chart state: %w", err)
		}

		if err := opts.RewriteChart(ctx, loadedChart, to.Repo, imageMapping); err != nil {
			return fmt.Errorf("rewrite chart: %w", err)
		}

		if err := hookState.Persist(chartAccessor, &edit); err != nil {
			return fmt.Errorf("save chart changes of RewriteChart: %w", err)
		}
	}

	snapshotAfter, err := chartCopySnapshot(chartAccessor)
	if err != nil {
		return fmt.Errorf("snapshot chart: %w", err)
	}

	chartData, provData := source.ChartData, source.ProvData
	if edit.Name != "" || edit.Version != "" || len(edit.SetAnnotations) > 0 || dependenciesChanged || !bytes.Equal(snapshotBefore, snapshotAfter) {
		if chartData, provData, err = saveChartCopy(ctx, loadedChart, chartAccessor, edit, source.ProvData != nil, tempDir, opts); err != nil {
			return fmt.Errorf("save chart: %w", err)
		}
	}

	if _, err := registryClient.PushContext(ctx, chartData, fmt.Sprintf("%s:%s", to.Repo, version), registry.PushOptProvData(provData), registry.PushOptExtraTags(lo.Compact([]string{extraTag}))); err != nil {
		return fmt.Errorf("push chart: %w", err)
	}

	return nil
}

// readWerfArchive reads the werf 3 bundle archive: chart.tar.gz and a docker-save tarball per image
// tag in images/<tag>.tar.gz. The images are converted into OCI blobs in a temporary store.
func readWerfArchive(ctx context.Context, tarPath, tempDir string, expansion *chartCopyExpansion) (*chartCopySource, error) {
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

	if err := walkTar(ctx, tarPath, func(header *tar.Header, reader io.Reader) error {
		name := header.Name

		switch {
		case name == werfArchiveChartFileName:
			var err error
			if source.ChartData, err = readChartCopyArchiveMember(header, reader, archive.MaxDecompressedChartSize); err != nil {
				return err
			}
		case path.Dir(name) == werfArchiveImagesDirName && strings.HasSuffix(name, ".tar.gz"):
			gzipReader, err := gzip.NewReader(reader)
			if err != nil {
				return fmt.Errorf("read %s: %w", name, err)
			}
			defer gzipReader.Close()

			desc, err := convertDockerArchiveImage(ctx, gzipReader, store, tempDir, expansion)
			if err != nil {
				return fmt.Errorf("read image %s: %w", name, err)
			}

			images[strings.TrimSuffix(path.Base(name), ".tar.gz")] = desc
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("read werf archive: %w", err)
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

func writeChartCopyArchive(ctx context.Context, source *chartCopySource, loadedChart helmchart.Charter, imageRefs []string, archivePath, tempDir string) error {
	layoutDir, err := os.MkdirTemp(tempDir, "oci-layout-")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}

	store, err := oci.New(layoutDir)
	if err != nil {
		return fmt.Errorf("create OCI image layout: %w", err)
	}

	chartDesc, err := pushChartCopyManifest(ctx, store, loadedChart, source.ChartData, source.ProvData)
	if err != nil {
		return fmt.Errorf("save chart manifest: %w", err)
	}

	// The entries are not tagged in the store: the chart tag and the image refs share its tag
	// namespace, so an image ref equal to the chart tag would replace the chart entry.
	chartDesc.Annotations = map[string]string{
		chartCopyArchiveEntryAnnotation: chartCopyArchiveEntryChart,
		ocispec.AnnotationRefName:       lo.CoalesceOrEmpty(source.Tag, strings.ReplaceAll(chartVersion(loadedChart), "+", "_")),
	}
	entries := []ocispec.Descriptor{chartDesc}

	for _, imageRef := range imageRefs {
		storage, desc, err := source.ResolveImage(ctx, imageRef)
		if err != nil {
			return fmt.Errorf("resolve image %q: %w", imageRef, err)
		}

		if err := copyChartCopyImage(ctx, storage, store, desc); err != nil {
			return fmt.Errorf("copy image %q: %w", imageRef, err)
		}

		desc.Annotations = map[string]string{
			chartCopyArchiveEntryAnnotation: chartCopyArchiveEntryImage,
			ocispec.AnnotationRefName:       imageRef,
		}
		entries = append(entries, desc)

		log.Default.Info(ctx, "Saved image %s", imageRef)
	}

	if err := writeChartCopyArchiveIndex(layoutDir, entries); err != nil {
		return fmt.Errorf("write OCI image layout index: %w", err)
	}

	return writeOCILayoutTar(ctx, layoutDir, archivePath, strings.HasSuffix(archivePath, ".gz") || strings.HasSuffix(archivePath, ".tgz"))
}

// convertDockerArchiveImage pushes the image of a docker-save tarball into store as a Docker
// manifest, gzipping uncompressed layers, and returns the manifest descriptor.
func convertDockerArchiveImage(ctx context.Context, in io.Reader, store *oci.Store, tempDir string, expansion *chartCopyExpansion) (ocispec.Descriptor, error) {
	dir, err := os.MkdirTemp(tempDir, "image-")
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)

	files := map[string]string{}

	tarReader := tar.NewReader(expansion.Reader(ctx, in))
	for entries := 0; ; entries++ {
		if err := context.Cause(ctx); err != nil {
			return ocispec.Descriptor{}, err
		}

		if entries > chartCopyArchiveMaxEntries {
			return ocispec.Descriptor{}, fmt.Errorf("image tarball has more than %d entries", chartCopyArchiveMaxEntries)
		}

		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("read image tarball: %w", err)
		}

		if err := checkChartCopyTarMember(header); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("read image tarball: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		name := chartCopyArchiveMemberName(header.Name)
		if _, found := files[name]; found {
			return ocispec.Descriptor{}, fmt.Errorf("image tarball has duplicate member %q", name)
		}

		filePath := filepath.Join(dir, strconv.Itoa(entries))
		if err := writeFileFromReader(filePath, expansion.ExtractedReader(ctx, tarReader)); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("extract %s from the image tarball: %w", name, err)
		}

		files[name] = filePath
	}

	if files["manifest.json"] == "" {
		return ocispec.Descriptor{}, errors.New("no manifest.json in the image tarball")
	}

	manifestData, err := readChartCopyFile(files["manifest.json"], chartCopyArchiveMaxMetadataSize)
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

	if manifest.Config, err = pushChartCopyFileBlob(ctx, store, files[chartCopyArchiveMemberName(tarballManifests[0].Config)], dockerConfigMediaType, false); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("push config %q: %w", tarballManifests[0].Config, err)
	}

	for _, layer := range tarballManifests[0].Layers {
		layerPath := files[chartCopyArchiveMemberName(layer)]
		if layerPath == "" {
			return ocispec.Descriptor{}, fmt.Errorf("layer %q not found in the image tarball", layer)
		}

		layerFormat, err := chartCopyLayerFormat(layerPath)
		if err != nil {
			return ocispec.Descriptor{}, err
		}

		switch layerFormat {
		case chartCopyLayerFormatZstd:
			return ocispec.Descriptor{}, fmt.Errorf("layer %q is zstd-compressed, only gzip-compressed and uncompressed tar layers are supported", layer)
		case chartCopyLayerFormatUnknown:
			return ocispec.Descriptor{}, fmt.Errorf("layer %q is neither a gzip-compressed nor an uncompressed tar", layer)
		}

		layerDesc, err := pushChartCopyFileBlob(ctx, store, layerPath, dockerLayerMediaType, layerFormat == chartCopyLayerFormatTar)
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

// copyChartCopyImage copies the image graph rooted at root by digest. Non-distributable layers,
// which oras skips, are copied too, and a blob the source does not serve is an error.
func copyChartCopyImage(ctx context.Context, src content.ReadOnlyStorage, dst content.Storage, root ocispec.Descriptor) error {
	if err := copyChartCopyForeignLayers(ctx, src, dst, root, map[digest.Digest]bool{}); err != nil {
		return err
	}

	if err := oras.CopyGraph(ctx, src, dst, root, oras.DefaultCopyGraphOptions); err != nil {
		if errors.Is(err, errdef.ErrNotFound) {
			return fmt.Errorf("the source does not serve a blob of the image: %w", err)
		}

		return fmt.Errorf("copy image graph: %w", err)
	}

	return nil
}

// chartCopyValuesStates returns the values of the chart and all its dependencies.
func chartCopyValuesStates(chartAccessor helmchart.Accessor) ([]chartCopyValuesState, error) {
	values, err := json.Marshal(chartAccessor.Values())
	if err != nil {
		return nil, fmt.Errorf("marshal values of chart %q: %w", chartAccessor.Name(), err)
	}

	state := chartCopyValuesState{Chart: chartAccessor.Charter(), Values: values}
	if rawValues := chartCopyRawValues(chartAccessor); rawValues != nil {
		state.RawValues = bytes.Clone(rawValues.Data)
	}

	states := []chartCopyValuesState{state}
	for _, dep := range chartAccessor.Dependencies() {
		depAccessor, err := helmchart.NewAccessor(dep)
		if err != nil {
			return nil, fmt.Errorf("create dependency accessor: %w", err)
		}

		depStates, err := chartCopyValuesStates(depAccessor)
		if err != nil {
			return nil, err
		}

		states = append(states, depStates...)
	}

	return states, nil
}

func copyChartCopyForeignLayers(ctx context.Context, src content.ReadOnlyStorage, dst content.Storage, desc ocispec.Descriptor, visited map[digest.Digest]bool) error {
	if visited[desc.Digest] {
		return nil
	}

	visited[desc.Digest] = true

	if isChartCopyForeignLayer(desc) {
		exists, err := dst.Exists(ctx, desc)
		if err != nil {
			return fmt.Errorf("check non-distributable layer %s in the destination: %w", desc.Digest, err)
		}

		if exists {
			return nil
		}

		reader, err := src.Fetch(ctx, desc)
		if err != nil {
			if errors.Is(err, errdef.ErrNotFound) {
				return fmt.Errorf("the source does not serve blob %s, a non-distributable layer of the image: %w", desc.Digest, err)
			}

			return fmt.Errorf("fetch non-distributable layer %s: %w", desc.Digest, err)
		}
		defer reader.Close()

		verifyReader := content.NewVerifyReader(reader, desc)
		if err := dst.Push(ctx, desc, verifyReader); err != nil {
			return fmt.Errorf("push non-distributable layer %s: %w", desc.Digest, err)
		}

		if err := verifyReader.Verify(); err != nil {
			return fmt.Errorf("verify non-distributable layer %s: %w", desc.Digest, err)
		}

		return nil
	}

	successors, err := content.Successors(ctx, src, desc)
	if err != nil {
		return fmt.Errorf("read successors of %s: %w", desc.Digest, err)
	}

	for _, successor := range successors {
		if err := copyChartCopyForeignLayers(ctx, src, dst, successor, visited); err != nil {
			return err
		}
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

	// A "+" is pushed as "_", as Helm does for chart versions.
	if image.Tag != "" {
		if err := (orasregistry.Reference{Reference: strings.ReplaceAll(image.Tag, "+", "_")}).ValidateReferenceAsTag(); err != nil {
			return nil, fmt.Errorf("invalid tag %q: %w", image.Tag, err)
		}
	}

	return &chartCopyAddress{Repo: image.Repo, Tag: image.Tag}, nil
}

// pushChartCopyFileBlob pushes the file into store, gzipping it first when compress is true.
func pushChartCopyFileBlob(ctx context.Context, store *oci.Store, filePath, mediaType string, compress bool) (ocispec.Descriptor, error) {
	if filePath == "" {
		return ocispec.Descriptor{}, errors.New("file not found in the image tarball")
	}

	if compress {
		if err := gzipFile(ctx, filePath, filePath+".gz"); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("compress blob: %w", err)
		}

		filePath += ".gz"
	}

	file, err := os.Open(filePath)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("open blob: %w", err)
	}
	defer file.Close()

	digester := digest.Canonical.Digester()

	size, err := io.Copy(digester.Hash(), chartCopyContextReader{ctx: ctx, reader: file})
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

	if err := pushIfMissing(ctx, store, desc, chartCopyContextReader{ctx: ctx, reader: file}); err != nil {
		return ocispec.Descriptor{}, err
	}

	return desc, nil
}

func readChartCopyRepo(ctx context.Context, registryClient *registry.Client, addr *chartCopyAddress, plainHTTPHosts []string) (*chartCopySource, error) {
	tag := strings.ReplaceAll(lo.CoalesceOrEmpty(addr.Tag, "latest"), "+", "_")

	result, err := registryClient.PullContext(ctx, fmt.Sprintf("%s:%s", addr.Repo, tag), registry.PullOptWithProv(true), registry.PullOptIgnoreMissingProv(true))
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

			repository.PlainHTTP = slices.Contains(plainHTTPHosts, repository.Reference.Registry)

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
func uncompressedChartCopyArchive(ctx context.Context, archivePath, tempDir string, expansion *chartCopyExpansion) (string, error) {
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
	if err := writeFileFromReader(tarPath, expansion.Reader(ctx, gzipReader)); err != nil {
		return "", fmt.Errorf("decompress %q: %w", archivePath, err)
	}

	return tarPath, nil
}

func walkTar(ctx context.Context, tarPath string, fn func(header *tar.Header, reader io.Reader) error) error {
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer tarFile.Close()

	names := map[string]bool{}

	tarReader := tar.NewReader(tarFile)
	for entries := 0; ; entries++ {
		if err := context.Cause(ctx); err != nil {
			return err
		}

		if entries > chartCopyArchiveMaxEntries {
			return fmt.Errorf("archive has more than %d entries", chartCopyArchiveMaxEntries)
		}

		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		if err := checkChartCopyTarMember(header); err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		if header.Typeflag != tar.TypeReg {
			continue
		}

		header.Name = chartCopyArchiveMemberName(header.Name)
		if names[header.Name] {
			return fmt.Errorf("archive has duplicate member %q", header.Name)
		}

		names[header.Name] = true

		if err := fn(header, chartCopyContextReader{ctx: ctx, reader: tarReader}); err != nil {
			return err
		}
	}
}

// writeOCILayoutTar writes the OCI image layout in dir into a tar at archivePath, replacing it only
// when the whole tar is written.
func writeOCILayoutTar(ctx context.Context, dir, archivePath string, compress bool) error {
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
		if err := addFileToTar(ctx, tarWriter, filepath.Join(dir, filepath.FromSlash(name)), name); err != nil {
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

	if err := context.Cause(ctx); err != nil {
		return err
	}

	// A temporary file is created readable by its owner only.
	if err := os.Chmod(tmpFile.Name(), 0o644); err != nil {
		return fmt.Errorf("set archive permissions: %w", err)
	}

	if err := os.Rename(tmpFile.Name(), archivePath); err != nil {
		return fmt.Errorf("move archive into place: %w", err)
	}

	return nil
}

func addChartRawFile(loadedChart helmchart.Charter, f *chartcommon.File) {
	switch c := loadedChart.(type) {
	case *v2chart.Chart:
		c.Raw = append(c.Raw, f)
	case *v3chart.Chart:
		c.Raw = append(c.Raw, f)
	default:
		panic(fmt.Sprintf("unexpected chart type %T", loadedChart))
	}
}

func addFileToTar(ctx context.Context, tarWriter *tar.Writer, filePath, name string) error {
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

	if _, err := io.Copy(tarWriter, chartCopyContextReader{ctx: ctx, reader: file}); err != nil {
		return fmt.Errorf("write %q into archive: %w", name, err)
	}

	return nil
}

// chartCopyArchiveMemberName normalizes a tar member name, so that names of the same file compare
// equal.
func chartCopyArchiveMemberName(name string) string {
	return strings.TrimPrefix(path.Clean("/"+name), "/")
}

// chartCopyImages returns the helm.sh/images entries of the chart and of all its dependencies.
func chartCopyImages(ctx context.Context, loadedChart helmchart.Charter) ([]chartyaml.Image, error) {
	chartAccessor, err := helmchart.NewAccessor(loadedChart)
	if err != nil {
		return nil, fmt.Errorf("create chart accessor: %w", err)
	}

	images, err := chartyaml.ParseImages(ctx, chartAnnotations(loadedChart)[chartyaml.ImagesAnnotation])
	if err != nil {
		return nil, fmt.Errorf("read images of chart %q: %w", chartAccessor.Name(), err)
	}

	for _, dep := range chartAccessor.Dependencies() {
		depImages, err := chartCopyImages(ctx, dep)
		if err != nil {
			return nil, err
		}

		images = append(images, depImages...)
	}

	return images, nil
}

// chartCopyLayerFormat detects the format of an image layer file by its magic bytes.
func chartCopyLayerFormat(filePath string) (chartCopyLayerFormatType, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", filePath, err)
	}
	defer file.Close()

	header := make([]byte, 512)

	n, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", fmt.Errorf("read %q: %w", filePath, err)
	}

	header = header[:n]

	switch {
	case bytes.HasPrefix(header, []byte{0x1f, 0x8b}):
		return chartCopyLayerFormatGzip, nil
	case bytes.HasPrefix(header, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		return chartCopyLayerFormatZstd, nil
	case len(header) >= 262 && bytes.Equal(header[257:262], []byte("ustar")):
		return chartCopyLayerFormatTar, nil
	default:
		return chartCopyLayerFormatUnknown, nil
	}
}

func chartCopyMetadataWithoutAnnotations(chartAccessor helmchart.Accessor) ([]byte, error) {
	metadata, err := json.Marshal(lo.OmitByKeys(chartAccessor.MetadataAsMap(), []string{"Annotations"}))
	if err != nil {
		return nil, fmt.Errorf("marshal chart metadata: %w", err)
	}

	return metadata, nil
}

func chartCopyRawValues(chartAccessor helmchart.Accessor) *chartcommon.File {
	rawValues, _ := lo.Find(chartAccessor.RawFiles(), func(f *chartcommon.File) bool {
		return f.Name == v2util.ValuesfileName
	})

	return rawValues
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

// chartVersionTagSlug slugs tag as werf's slug.Slug does: letters are lowercased and transliterated
// to ASCII, separators become "-", other characters are dropped, and "-" are not repeated nor lead
// or trail.
func chartVersionTagSlug(tag string) string {
	var result strings.Builder
	for _, r := range unidecode.Unidecode(tag) {
		var cursor rune

		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			cursor = r
		case r >= 'A' && r <= 'Z':
			cursor = unicode.ToLower(r)
		case strings.ContainsRune("-&(){}_[],.;:=+<>~/ ", r):
			cursor = '-'
		default:
			continue
		}

		if cursor == '-' && (result.Len() == 0 || strings.HasSuffix(result.String(), "-")) {
			continue
		}

		result.WriteRune(cursor)
	}

	return strings.TrimSuffix(result.String(), "-")
}

// checkChartCopyTarMember rejects sparse files: tar.Reader fills in their holes, producing more
// than the archive holds, unbounded by the expansion limit.
func checkChartCopyTarMember(header *tar.Header) error {
	if header.Typeflag == tar.TypeGNUSparse || lo.SomeBy(lo.Keys(header.PAXRecords), func(key string) bool {
		return strings.HasPrefix(key, "GNU.sparse.")
	}) {
		return fmt.Errorf("member %q is a sparse file, which is not supported", header.Name)
	}

	return nil
}

// claimChartCopyTag records that the copy pushes claim under tag, failing if the tag is already
// claimed for different content.
func claimChartCopyTag(claims map[string]chartCopyTagClaim, tag string, claim chartCopyTagClaim) error {
	previous, found := claims[tag]
	if !found {
		claims[tag] = claim

		return nil
	}

	if previous.Owner == claim.Owner || previous.Digest != "" && previous.Digest == claim.Digest {
		return nil
	}

	return fmt.Errorf("tag %q would be used for both %s and %s in the destination repository", tag, previous.Owner, claim.Owner)
}

func gzipFile(ctx context.Context, sourcePath, destinationPath string) error {
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
	if _, err := io.Copy(gzipWriter, chartCopyContextReader{ctx: ctx, reader: source}); err != nil {
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
	}

	if host == "docker.io" || host == "index.docker.io" {
		host = "registry-1.docker.io"

		if !strings.Contains(repoPath, "/") {
			repoPath = "library/" + repoPath
		}
	}

	return host + "/" + repoPath
}

func isChartCopyForeignLayer(desc ocispec.Descriptor) bool {
	switch desc.MediaType {
	case "application/vnd.oci.image.layer.nondistributable.v1.tar",
		"application/vnd.oci.image.layer.nondistributable.v1.tar+gzip",
		"application/vnd.oci.image.layer.nondistributable.v1.tar+zstd",
		"application/vnd.docker.image.rootfs.foreign.diff.tar.gzip":
		return true
	default:
		return false
	}
}

func isChartCopyManifestMediaType(mediaType string) bool {
	switch mediaType {
	case ocispec.MediaTypeImageManifest,
		ocispec.MediaTypeImageIndex,
		"application/vnd.oci.artifact.manifest.v1+json",
		dockerManifestMediaType,
		"application/vnd.docker.distribution.manifest.list.v2+json":
		return true
	default:
		return false
	}
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
func pushChartCopyManifest(ctx context.Context, store *oci.Store, loadedChart helmchart.Charter, chartData, provData []byte) (ocispec.Descriptor, error) {
	var metadata any
	switch c := loadedChart.(type) {
	case *v2chart.Chart:
		metadata = c.Metadata
	case *v3chart.Chart:
		metadata = c.Metadata
	default:
		panic(fmt.Sprintf("unexpected chart type %T", loadedChart))
	}

	configData, err := json.Marshal(metadata)
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

// readChartCopyArchiveMember reads an archive member of at most maxSize bytes.
func readChartCopyArchiveMember(header *tar.Header, reader io.Reader, maxSize int64) ([]byte, error) {
	if header.Size > maxSize {
		return nil, fmt.Errorf("%s is %d bytes, more than %d bytes allowed", header.Name, header.Size, maxSize)
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxSize))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", header.Name, err)
	}

	return data, nil
}

// readChartCopyFile reads a file of at most maxSize bytes.
func readChartCopyFile(filePath string, maxSize int64) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", filePath, err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", filePath, err)
	}

	if int64(len(data)) > maxSize {
		return nil, fmt.Errorf("file is more than %d bytes allowed", maxSize)
	}

	return data, nil
}

func readChartCopyOCILayout(ctx context.Context, tarPath string, indexData []byte) (*chartCopySource, error) {
	store, err := newChartCopyLayoutStore(ctx, tarPath)
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

	if chartDescs[0].Size > chartCopyArchiveMaxMetadataSize {
		return nil, fmt.Errorf("chart manifest is %d bytes, more than %d bytes allowed", chartDescs[0].Size, chartCopyArchiveMaxMetadataSize)
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
		if layer.Size > archive.MaxDecompressedChartSize {
			return nil, fmt.Errorf("chart manifest layer %s is %d bytes, more than %d bytes allowed", layer.Digest, layer.Size, archive.MaxDecompressedChartSize)
		}

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

// rewriteChartCopyDependencyImages rewrites the helm.sh/images annotations of all dependencies of
// the chart, which are saved from their metadata, and returns whether any changed.
func rewriteChartCopyDependencyImages(ctx context.Context, chartAccessor helmchart.Accessor, imageMapping map[string]string) (bool, error) {
	var changed bool
	for _, dep := range chartAccessor.Dependencies() {
		depAccessor, err := helmchart.NewAccessor(dep)
		if err != nil {
			return false, fmt.Errorf("create dependency accessor: %w", err)
		}

		annotations := chartAnnotations(dep)
		if imagesAnnotation := annotations[chartyaml.ImagesAnnotation]; imagesAnnotation != "" {
			newImagesAnnotation, err := chartyaml.RewriteImages(ctx, imagesAnnotation, imageMapping)
			if err != nil {
				return false, fmt.Errorf("rewrite %s annotation of dependency %q: %w", chartyaml.ImagesAnnotation, depAccessor.Name(), err)
			}

			if newImagesAnnotation != imagesAnnotation {
				annotations[chartyaml.ImagesAnnotation] = newImagesAnnotation
				changed = true
			}
		}

		depChanged, err := rewriteChartCopyDependencyImages(ctx, depAccessor, imageMapping)
		if err != nil {
			return false, err
		}

		changed = changed || depChanged
	}

	return changed, nil
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

// writeChartCopyArchiveIndex replaces index.json, which lists every manifest pushed into the store,
// like the per-platform manifests of an image index, with the entries.
func writeChartCopyArchiveIndex(layoutDir string, entries []ocispec.Descriptor) error {
	indexData, err := json.Marshal(ocispec.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageIndex,
		Manifests: entries,
	})
	if err != nil {
		return fmt.Errorf("marshal %s: %w", ocispec.ImageIndexFile, err)
	}

	if err := os.WriteFile(filepath.Join(layoutDir, ocispec.ImageIndexFile), indexData, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ocispec.ImageIndexFile, err)
	}

	return nil
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
