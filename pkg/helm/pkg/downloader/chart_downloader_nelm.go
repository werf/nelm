package downloader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Masterminds/semver/v3"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
	"github.com/werf/nelm/v2/pkg/helm/pkg/getter"
	"github.com/werf/nelm/v2/pkg/helm/pkg/provenance"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
	"github.com/werf/nelm/v2/pkg/log"
)

// ErrKeyringRequired is returned when a chart is to be verified, or is signed and verified if
// possible, without a keyring.
var ErrKeyringRequired = errors.New("keyring with the public keys to verify the chart provenance is required")

// A digest reference of a repository with a long basename produces a file name exceeding NAME_MAX
// once the atomic write temp suffix or the .prov extension is appended.
const maxDownloadFileNameLen = 200

func boundedFileName(name string) string {
	if len(name) <= maxDownloadFileNameLen {
		return name
	}

	sum := sha256.Sum256([]byte(name))
	suffix := "-" + hex.EncodeToString(sum[:])[:16] + ".tgz"

	prefix := name[:maxDownloadFileNameLen-len(suffix)]
	for !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}

	return prefix + suffix
}

// chartSignedName returns the name the chart archive at path is signed under: name for an HTTP
// chart, <name>-<version>.tgz from Chart.yaml for an OCI chart, which may be pulled by a tag that
// differs from its version.
func chartSignedName(path, name string, u *url.URL) (string, error) {
	if u.Scheme != registry.OCIScheme {
		return name, nil
	}

	archiveData, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read chart archive: %w", err)
	}

	return signedChartFileName(archiveData)
}

// signedChartFileName returns the name a chart archive is signed under, <name>-<version>.tgz from
// its Chart.yaml. The tag an OCI chart is pulled by may differ from its version.
func signedChartFileName(archiveData []byte) (string, error) {
	name, version, err := chartArchiveNameVersion(archiveData)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s-%s.tgz", name, version), nil
}

// checkTagNamesChartVersion checks that a tag that is a strict semver version, with an optional
// leading "v" and "_" for "+", names the version of the chart, itself with an optional leading
// "v". Other tags name no version.
func checkTagNamesChartVersion(archiveData []byte, tag string) error {
	tagVersion, err := semver.StrictNewVersion(strings.ReplaceAll(strings.TrimPrefix(tag, "v"), "_", "+"))
	if err != nil {
		return nil
	}

	_, version, err := chartArchiveNameVersion(archiveData)
	if err != nil {
		return err
	}

	chartVersion, err := semver.StrictNewVersion(strings.TrimPrefix(version, "v"))
	if err != nil && tagVersion.Original() != version || err == nil && tagVersion.String() != chartVersion.String() {
		return fmt.Errorf("chart version %q does not match the requested tag %q", version, tag)
	}

	return nil
}

func chartArchiveNameVersion(archiveData []byte) (string, string, error) {
	files, err := archive.LoadArchiveFiles(bytes.NewReader(archiveData))
	if err != nil {
		return "", "", fmt.Errorf("load chart archive: %w", err)
	}

	for _, f := range files {
		if f.Name != "Chart.yaml" {
			continue
		}

		var metadata struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}
		if err := yaml.Unmarshal(f.Data, &metadata); err != nil {
			return "", "", fmt.Errorf("parse Chart.yaml: %w", err)
		}

		return metadata.Name, metadata.Version, nil
	}

	return "", "", errors.New("chart archive has no Chart.yaml")
}

// checkBeforeDownload rejects what fails regardless of the chart, before any request is made.
func (c *ChartDownloader) checkBeforeDownload(ref, version string) error {
	if c.Verify == VerifyAlways && c.Keyring == "" {
		return fmt.Errorf("verify chart %s: %w", ref, ErrKeyringRequired)
	}

	if !registry.IsOCI(ref) {
		return nil
	}

	if err := registry.ValidateChartTag(version); err != nil {
		return err
	}

	u, err := url.Parse(ref)
	if err != nil {
		return fmt.Errorf("invalid chart URL format: %s", ref)
	}

	_, tag := splitOCITag(u.Path)

	return registry.ValidateChartTag(tag)
}

// pinOCIDigest resolves the tag of an OCI chart URL to its manifest digest, unless desc is the
// already resolved manifest, and returns the URL of the manifest by digest, so that the chart and
// its provenance come from the same manifest even if the tag moves meanwhile, and the tag. A URL by
// digest is returned as is, with no tag. The content digest is passed through: a manifest digest is
// not the digest of the chart archive.
func (c *ChartDownloader) pinOCIDigest(digest string, u *url.URL, desc *ocispec.Descriptor) (string, *url.URL, string, error) {
	repoPath, tag := splitOCITag(u.Path)
	if tag == "" {
		return digest, u, "", nil
	}

	ctx := c.requestContext()

	if desc == nil {
		repository, err := c.RegistryClient.Generic().Repository(u.Host + "/" + strings.TrimPrefix(repoPath, "/"))
		if err != nil {
			return "", nil, "", err
		}

		resolved, err := repository.Resolve(ctx, strings.ReplaceAll(tag, "+", "_"))
		if err != nil {
			return "", nil, "", fmt.Errorf("resolve chart %s: %w", u, err)
		}

		desc = &resolved
	}

	log.Default.Info(logContext(ctx), "Resolved chart %s to digest %s", u, desc.Digest)

	pinned := *u
	pinned.Path = repoPath + "@" + desc.Digest.String()

	return digest, &pinned, tag, nil
}

func isVersionConstraint(tag string) bool {
	if _, err := semver.NewVersion(tag); err == nil {
		return false
	}

	_, err := semver.NewConstraint(tag)

	return err == nil
}

// splitOCITag returns the repository path and the tag of the path of an OCI chart URL, no tag if the
// path has a digest or no tag.
func splitOCITag(p string) (string, string) {
	base := path.Base(p)
	idx := strings.LastIndexByte(base, ':')
	if strings.Contains(base, "@") || idx < 0 {
		return p, ""
	}

	return strings.TrimSuffix(p, base[idx:]), base[idx+1:]
}

func (c *ChartDownloader) requestContext() context.Context {
	return getter.RequestContext(c.Options...)
}

// logContext returns ctx if it carries a logger, else the default logger's context: the getter
// context of a Helm caller may carry none.
func logContext(ctx context.Context) context.Context {
	if ctx.Value(log.LogboekLoggerCtxKeyName) == nil {
		return context.Background()
	}

	return ctx
}

func logVerification(ctx context.Context, chartURL string, ver *provenance.Verification) {
	if ver == nil || ver.SignedBy == nil {
		return
	}

	signers := slices.Sorted(maps.Keys(ver.SignedBy.Identities))
	log.Default.Info(logContext(ctx), "Verified chart %s signed by %s", chartURL, strings.Join(signers, ", "))
}

// resolveChartVersion is ResolveChartVersion that also returns the descriptor of the manifest of
// the tag of the OCI chart URL if it resolved it, nil otherwise.
func (c *ChartDownloader) resolveChartVersion(ref, version string) (string, *url.URL, *ocispec.Descriptor, error) {
	u, err := url.Parse(ref)
	if err != nil || !registry.IsOCI(u.String()) || c.RegistryClient == nil {
		digest, resolvedURL, err := c.ResolveChartVersion(ref, version)

		return digest, resolvedURL, nil, err
	}

	return c.resolveOCIChartVersion(ref, version, u)
}

func (c *ChartDownloader) resolveOCIChartVersion(ref, version string, u *url.URL) (string, *url.URL, *ocispec.Descriptor, error) {
	if c.LiteralTag && !strings.Contains(path.Base(u.Path), "@") {
		literalURL, err := literalOCITagURL(u, version)

		return "", literalURL, nil, err
	}

	// A tag is resolved to its digest on download, with the context of the request. A tag that
	// may be a version constraint is left to ValidateReferenceContext, which matches it against
	// the tags of the repository if no such tag exists.
	if repoPath, tag := splitOCITag(u.Path); version == "" && tag != "" && !isVersionConstraint(tag) {
		u.Path = repoPath + ":" + strings.ReplaceAll(tag, "+", "_")
		return "", u, nil, nil
	}

	return c.RegistryClient.ValidateReferenceContext(c.requestContext(), ref, version, u)
}

// literalOCITagURL returns the OCI chart URL u with the tag of u or version, whichever is given, as
// is but for "+" encoded as "_".
func literalOCITagURL(u *url.URL, version string) (*url.URL, error) {
	repoPath, tag := splitOCITag(u.Path)
	tag = strings.ReplaceAll(tag, "+", "_")

	if version != "" {
		versionTag := strings.ReplaceAll(version, "+", "_")
		if tag != "" && tag != versionTag {
			return nil, fmt.Errorf("chart reference and version mismatch: %s is not %s", version, tag)
		}

		tag = versionTag
	}

	if tag == "" {
		return nil, fmt.Errorf("chart reference %s has no tag", u)
	}

	literal := *u
	literal.Path = repoPath + ":" + tag

	return &literal, nil
}
