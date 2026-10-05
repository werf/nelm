package release

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"time"

	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
)

const (
	storageLabelCreatedAt  = "createdAt"
	storageLabelModifiedAt = "modifiedAt"
	storageLabelName       = "name"
	storageLabelOwner      = "owner"
	storageLabelStatus     = "status"
	storageLabelVersion    = "version"
	storageObjectType      = "helm.sh/release.v1"
	storageOwner           = "helm"
)

var (
	storageSystemLabels = []string{storageLabelCreatedAt, storageLabelModifiedAt, storageLabelName, storageLabelOwner, storageLabelStatus, storageLabelVersion}
	gzipMagic           = []byte{0x1f, 0x8b, 0x08}
)

// storedObject is a release revision as the backends see it: the storage object identity,
// its labels and the encoded body. Body is nil when only the metadata was read.
type storedObject struct {
	Body      []byte
	Key       string
	Labels    map[string]string
	Namespace string
}

func newStoredObject(namespace string, rls *helmrelease.Release, timestampLabel string) (*storedObject, error) {
	body, err := encodeRelease(rls)
	if err != nil {
		return nil, fmt.Errorf("encode release %q (revision: %d): %w", rls.Name, rls.Version, err)
	}

	labels := maps.Clone(rls.Labels)
	if labels == nil {
		labels = map[string]string{}
	}

	labels[timestampLabel] = strconv.FormatInt(time.Now().Unix(), 10)
	labels[storageLabelName] = rls.Name
	labels[storageLabelOwner] = storageOwner
	labels[storageLabelStatus] = rls.Info.Status.String()
	labels[storageLabelVersion] = strconv.Itoa(rls.Version)

	return &storedObject{
		Body:      body,
		Key:       storageKey(rls.Name, rls.Version),
		Labels:    labels,
		Namespace: namespace,
	}, nil
}

func decodeStoredObject(obj *storedObject) (*helmrelease.Release, error) {
	rls, err := decodeRelease(obj.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: object %q (namespace: %q): %w", ErrReleaseUndecodable, obj.Key, obj.Namespace, err)
	}

	if rls.Namespace == "" {
		rls.Namespace = obj.Namespace
	}

	rls.Labels = maps.Clone(obj.Labels)

	return rls, nil
}

// decodeRelease streams the body through base64 and gzip decoding instead of materializing
// each intermediate form. Bodies stored before Helm started compressing them are plain
// base64-encoded JSON.
func decodeRelease(body []byte) (*helmrelease.Release, error) {
	reader := bufio.NewReader(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(body)))

	var jsonReader io.Reader = reader

	magic, err := reader.Peek(len(gzipMagic))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode base64: %w", err)
	}

	if bytes.Equal(magic, gzipMagic) {
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("create gzip reader: %w", err)
		}
		defer gzipReader.Close()

		jsonReader = gzipReader
	}

	decoder := json.NewDecoder(jsonReader)

	rls := &helmrelease.Release{}
	if err := decoder.Decode(rls); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	// Reading to the end validates the gzip checksum and rejects data after the release.
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("decode release: unexpected data after release")
		}

		return nil, fmt.Errorf("decode release: %w", err)
	}

	return rls, nil
}

func encodeRelease(rls *helmrelease.Release) ([]byte, error) {
	var buf bytes.Buffer

	base64Writer := base64.NewEncoder(base64.StdEncoding, &buf)

	gzipWriter, err := gzip.NewWriterLevel(base64Writer, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}

	if err := json.NewEncoder(gzipWriter).Encode(rls); err != nil {
		return nil, fmt.Errorf("encode release: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("close gzip writer: %w", err)
	}

	if err := base64Writer.Close(); err != nil {
		return nil, fmt.Errorf("close base64 writer: %w", err)
	}

	return buf.Bytes(), nil
}

// revisionFromStoredObject returns ok=false for a storage object that is not a release
// revision: one without a name or without a version label, which every stored revision
// carries. A version label that is present but does not parse is an error naming the
// object, so it can be removed by hand: left in place, it would collide with the next
// revision number.
func revisionFromStoredObject(obj *storedObject) (Revision, bool, error) {
	name := obj.Labels[storageLabelName]
	if name == "" {
		return Revision{}, false, nil
	}

	versionLabel, found := obj.Labels[storageLabelVersion]
	if !found {
		return Revision{}, false, nil
	}

	version, err := strconv.Atoi(versionLabel)
	if err != nil {
		return Revision{}, false, fmt.Errorf("release object %q (namespace: %q): unparseable version label %q", obj.Key, obj.Namespace, versionLabel)
	}

	return Revision{
		Name:      name,
		Namespace: obj.Namespace,
		Status:    obj.Labels[storageLabelStatus],
		Version:   version,
	}, true, nil
}

func storageKey(name string, version int) string {
	return fmt.Sprintf("sh.helm.release.v1.%s.v%d", name, version)
}

func withoutSystemLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, len(labels))
	for key, value := range labels {
		if slices.Contains(storageSystemLabels, key) {
			continue
		}

		result[key] = value
	}

	return result
}
