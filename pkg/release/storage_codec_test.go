package release

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	chartv2 "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	helmreleasecommon "github.com/werf/nelm/v2/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
)

func TestDecodeRelease_Garbage(t *testing.T) {
	for _, body := range [][]byte{nil, []byte("not base64!"), []byte(base64.StdEncoding.EncodeToString([]byte("not json")))} {
		_, err := decodeRelease(body)
		require.Error(t, err)
	}
}

func TestDecodeRelease_HelmEncodedBody(t *testing.T) {
	rls := newTestReleaseWithStatus("myrel", 3, helmreleasecommon.StatusDeployed)
	rls.Manifest = "kind: ConfigMap"

	decoded, err := decodeRelease(encodeHelmRelease(t, rls))
	require.NoError(t, err)
	assert.Equal(t, rls, decoded)
}

func TestDecodeRelease_RejectsCorruptGzip(t *testing.T) {
	body := encodeHelmRelease(t, newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusDeployed))

	compressed, err := base64.StdEncoding.DecodeString(string(body))
	require.NoError(t, err)

	truncated := compressed[:len(compressed)-4]
	_, err = decodeRelease([]byte(base64.StdEncoding.EncodeToString(truncated)))
	require.Error(t, err, "a truncated gzip trailer must not be accepted")

	badChecksum := bytes.Clone(compressed)
	badChecksum[len(badChecksum)-8] ^= 0xff
	_, err = decodeRelease([]byte(base64.StdEncoding.EncodeToString(badChecksum)))
	require.Error(t, err, "a gzip checksum mismatch must not be accepted")
}

func TestDecodeRelease_RejectsTrailingData(t *testing.T) {
	data, err := json.Marshal(newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusDeployed))
	require.NoError(t, err)

	_, err = decodeRelease([]byte(base64.StdEncoding.EncodeToString(append(data, []byte(`{"garbage":true}`)...))))
	require.Error(t, err)
}

func TestDecodeRelease_UncompressedBody(t *testing.T) {
	rls := newTestReleaseWithStatus("myrel", 1, helmreleasecommon.StatusDeployed)

	data, err := json.Marshal(rls)
	require.NoError(t, err)

	decoded, err := decodeRelease([]byte(base64.StdEncoding.EncodeToString(data)))
	require.NoError(t, err)
	assert.Equal(t, rls, decoded)
}

func TestDecodeStoredObject_TakesNamespaceFromObjectWhenBodyHasNone(t *testing.T) {
	rls := newTestRelease("", "myrel", 1, helmreleasecommon.StatusDeployed)

	obj := &storedObject{
		Namespace: "from-object",
		Key:       "sh.helm.release.v1.myrel.v1",
		Labels:    map[string]string{"owner": "helm"},
		Body:      encodeHelmRelease(t, rls),
	}

	decoded, err := decodeStoredObject(obj)
	require.NoError(t, err)
	assert.Equal(t, "from-object", decoded.Namespace)

	decoded.Labels["owner"] = "changed"
	assert.Equal(t, "helm", obj.Labels["owner"], "the decoded release must not share labels with the stored object")
}

func TestDecodeStoredSummary_MatchesFullDecode(t *testing.T) {
	rls := newTestReleaseWithStatus("myrel", 3, helmreleasecommon.StatusDeployed)
	rls.Manifest = "kind: ConfigMap"
	rls.Info.LastDeployed = time.Date(2025, 1, 2, 3, 4, 5, 6, time.UTC)
	rls.Info.Annotations = map[string]string{"managed-by": "deckhouse"}
	rls.Chart = &chartv2.Chart{Metadata: &chartv2.Metadata{Name: "mychart", Version: "1.2.3", AppVersion: "4.5.6"}}
	obj := &storedObject{Namespace: "ns", Key: "key", Body: encodeHelmRelease(t, rls)}

	summary, err := decodeStoredSummary(obj)
	require.NoError(t, err)
	assert.Equal(t, &ReleaseSummary{
		Annotations: map[string]string{"managed-by": "deckhouse"},
		Chart:       &ReleaseSummaryChart{AppVersion: "4.5.6", Name: "mychart", Version: "1.2.3"},
		DeployedAt:  rls.Info.LastDeployed,
	}, summary)

	full, err := decodeStoredObject(obj)
	require.NoError(t, err)
	assert.True(t, full.Info.LastDeployed.Equal(summary.DeployedAt))
}

func TestDecodeStoredSummary_OptionalFields(t *testing.T) {
	for name, body := range map[string]string{
		"no chart":           `{"info":{"status":"deployed"}}`,
		"chart without meta": `{"info":{"status":"deployed"},"chart":{}}`,
		"empty deploy time":  `{"info":{"last_deployed":""}}`,
		"null deploy time":   `{"info":{"last_deployed":null}}`,
	} {
		t.Run(name, func(t *testing.T) {
			summary, err := decodeStoredSummary(&storedObject{Body: []byte(base64.StdEncoding.EncodeToString([]byte(body)))})
			require.NoError(t, err)
			assert.Nil(t, summary.Chart)
			assert.True(t, summary.DeployedAt.IsZero())
		})
	}
}

func TestDecodeStoredSummary_Undecodable(t *testing.T) {
	for name, body := range map[string][]byte{
		"garbage":          []byte("not base64!"),
		"no info":          []byte(base64.StdEncoding.EncodeToString([]byte(`{"name":"myrel"}`))),
		"bad deploy time":  []byte(base64.StdEncoding.EncodeToString([]byte(`{"info":{"last_deployed":"yesterday"}}`))),
		"bad annotations":  []byte(base64.StdEncoding.EncodeToString([]byte(`{"info":{"annotations":{"a":1}}}`))),
		"trailing release": []byte(base64.StdEncoding.EncodeToString([]byte(`{"info":{}}{}`))),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeStoredSummary(&storedObject{Namespace: "ns", Key: "key", Body: body})
			require.ErrorIs(t, err, ErrReleaseUndecodable)
		})
	}
}

func TestEncodeRelease_ReadableByHelm(t *testing.T) {
	rls := newTestReleaseWithStatus("myrel", 2, helmreleasecommon.StatusSuperseded)
	rls.Manifest = "kind: Secret"

	body, err := encodeRelease(rls)
	require.NoError(t, err)

	compressed, err := base64.StdEncoding.DecodeString(string(body))
	require.NoError(t, err)

	gzipReader, err := gzip.NewReader(bytes.NewReader(compressed))
	require.NoError(t, err)

	data, err := io.ReadAll(gzipReader)
	require.NoError(t, err)

	helmDecoded := &helmrelease.Release{}
	require.NoError(t, json.Unmarshal(data, helmDecoded))
	assert.Equal(t, rls, helmDecoded)
}
