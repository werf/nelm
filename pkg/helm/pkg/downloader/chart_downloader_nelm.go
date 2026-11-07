package downloader

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Masterminds/semver/v3"
	"sigs.k8s.io/yaml"

	"github.com/werf/nelm/v2/pkg/helm/pkg/chart/loader/archive"
	"github.com/werf/nelm/v2/pkg/helm/pkg/registry"
)

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

	return signedChartFileName(archiveData, ociRefTag(u))
}

// signedChartFileName returns the name a chart archive is signed under, <name>-<version>.tgz from
// its Chart.yaml. The tag an OCI chart is pulled by may differ from its version.
func signedChartFileName(archiveData []byte, tag string) (string, error) {
	files, err := archive.LoadArchiveFiles(bytes.NewReader(archiveData))
	if err != nil {
		return "", fmt.Errorf("load chart archive: %w", err)
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
			return "", fmt.Errorf("parse Chart.yaml: %w", err)
		}

		// A semver tag names the version the user asked for: a chart signed for another version
		// must not verify under it.
		if tagVersion, err := semver.StrictNewVersion(strings.ReplaceAll(tag, "_", "+")); err == nil && tagVersion.Original() != metadata.Version {
			return "", fmt.Errorf("chart version %q does not match the requested tag %q", metadata.Version, tag)
		}

		return fmt.Sprintf("%s-%s.tgz", metadata.Name, metadata.Version), nil
	}

	return "", errors.New("chart archive has no Chart.yaml")
}

func ociRefTag(u *url.URL) string {
	name := filepath.Base(u.Path)
	if idx := strings.LastIndexByte(name, ':'); idx >= 0 {
		return name[idx+1:]
	}

	return ""
}
