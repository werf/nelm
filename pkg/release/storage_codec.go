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
	storageAnnotationReleasedBy      = "nelm.werf.io/released-by"
	storageAnnotationReleasedByValue = "nelm"
	storageLabelCreatedAt            = "createdAt"
	storageLabelModifiedAt           = "modifiedAt"
	storageLabelName                 = "name"
	storageLabelOwner                = "owner"
	storageLabelStatus               = "status"
	storageLabelVersion              = "version"
	storageObjectType                = "helm.sh/release.v1"
	storageOwner                     = "helm"
)

var (
	storageSystemLabels = []string{storageLabelCreatedAt, storageLabelModifiedAt, storageLabelName, storageLabelOwner, storageLabelStatus, storageLabelVersion}
	gzipMagic           = []byte{0x1f, 0x8b, 0x08}
)

type storedObject struct {
	Annotations map[string]string
	Body        []byte
	Key         string
	Labels      map[string]string
	Namespace   string
}

func newStoredObject(namespace string, rls *helmrelease.Release, timestampLabel string) (*storedObject, error) {
	annotations := maps.Clone(rls.Info.Annotations)
	if annotations == nil {
		annotations = map[string]string{}
	}

	if writer, found := annotations[storageAnnotationReleasedBy]; found && writer != storageAnnotationReleasedByValue {
		return nil, fmt.Errorf("annotation %q is reserved for %q", storageAnnotationReleasedBy, storageAnnotationReleasedByValue)
	}

	annotations[storageAnnotationReleasedBy] = storageAnnotationReleasedByValue

	storedRelease := *rls
	storedInfo := *rls.Info
	storedInfo.Annotations = nil
	storedRelease.Info = &storedInfo

	body, err := encodeRelease(&storedRelease)
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
		Annotations: annotations,
		Body:        body,
		Key:         storageKey(rls.Name, rls.Version),
		Labels:      labels,
		Namespace:   namespace,
	}, nil
}

func decodeStoredObject(obj *storedObject) (*helmrelease.Release, error) {
	rls, err := decodeRelease(obj.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: object %q (namespace: %q): %w", ErrReleaseUndecodable, obj.Key, obj.Namespace, err)
	}

	if rls.Info == nil {
		return nil, fmt.Errorf("%w: object %q (namespace: %q): release info is required", ErrReleaseUndecodable, obj.Key, obj.Namespace)
	}

	if rls.Namespace == "" {
		rls.Namespace = obj.Namespace
	}

	rls.Labels = maps.Clone(obj.Labels)
	if obj.Annotations[storageAnnotationReleasedBy] == storageAnnotationReleasedByValue {
		rls.Info.Annotations = maps.Clone(obj.Annotations)
	}

	return rls, nil
}

func decodeRelease(body []byte) (*helmrelease.Release, error) {
	rls := &helmrelease.Release{}
	if err := decodeReleaseBody(body, rls); err != nil {
		return nil, err
	}

	return rls, nil
}

func decodeStoredSummary(obj *storedObject) (*ReleaseSummary, error) {
	var body struct {
		Info *struct {
			Annotations  map[string]string `json:"annotations"`
			LastDeployed json.RawMessage   `json:"last_deployed"`
		} `json:"info"`
		Chart *struct {
			Metadata *ReleaseSummaryChart `json:"metadata"`
		} `json:"chart"`
	}

	if err := decodeReleaseBody(obj.Body, &body); err != nil {
		return nil, fmt.Errorf("%w: object %q (namespace: %q): %w", ErrReleaseUndecodable, obj.Key, obj.Namespace, err)
	}

	if body.Info == nil {
		return nil, fmt.Errorf("%w: object %q (namespace: %q): release info is required", ErrReleaseUndecodable, obj.Key, obj.Namespace)
	}

	summary := &ReleaseSummary{Annotations: body.Info.Annotations}
	if obj.Annotations[storageAnnotationReleasedBy] == storageAnnotationReleasedByValue {
		summary.Annotations = maps.Clone(obj.Annotations)
	}

	// Matches helmrelease.Info, which reads an empty string as a zero time.
	if raw := string(body.Info.LastDeployed); raw != "" && raw != "null" && raw != `""` {
		if err := json.Unmarshal(body.Info.LastDeployed, &summary.DeployedAt); err != nil {
			return nil, fmt.Errorf("%w: object %q (namespace: %q): decode last deployment time: %w", ErrReleaseUndecodable, obj.Key, obj.Namespace, err)
		}
	}

	if body.Chart != nil {
		summary.Chart = body.Chart.Metadata
	}

	return summary, nil
}

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

	if obj.Key != storageKey(name, version) {
		return Revision{}, false, nil
	}

	var annotations map[string]string
	if obj.Annotations[storageAnnotationReleasedBy] == storageAnnotationReleasedByValue {
		annotations = maps.Clone(obj.Annotations)
	}

	return Revision{
		Annotations: annotations,
		Name:        name,
		Namespace:   obj.Namespace,
		Status:      obj.Labels[storageLabelStatus],
		Version:     version,
	}, true, nil
}

func decodeReleaseBody(body []byte, target any) error {
	reader := bufio.NewReader(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(body)))

	var jsonReader io.Reader = reader

	magic, err := reader.Peek(len(gzipMagic))
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode base64: %w", err)
	}

	if bytes.Equal(magic, gzipMagic) {
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("create gzip reader: %w", err)
		}
		defer gzipReader.Close()

		jsonReader = gzipReader
	}

	decoder := json.NewDecoder(jsonReader)

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode release: %w", err)
	}

	// Reading to EOF verifies the gzip checksum.
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("decode release: unexpected data after release")
		}

		return fmt.Errorf("decode release: %w", err)
	}

	return nil
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
