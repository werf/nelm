/*
Copyright The Helm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver // import "helm.sh/helm/v3/pkg/storage/driver"

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/metadata"

	rspb "github.com/werf/nelm/pkg/helm/pkg/release"
)

var b64 = base64.StdEncoding

var magicGzip = []byte{0x1f, 0x8b, 0x08}

var systemLabels = []string{"name", "owner", "status", "version", "createdAt", "modifiedAt"}

// lastVersionFromMetadata resolves the highest release revision matching selector
// using a metadata-only list. It transfers only object metadata (labels), never
// the release bodies stored in the objects' data, so it does not scale with
// release size or history depth.
func lastVersionFromMetadata(ctx context.Context, client metadata.Interface, gvr schema.GroupVersionResource, namespace, selector string) (int, error) {
	list, err := client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return 0, errors.Wrap(err, "list release metadata")
	}

	latest := 0
	for _, item := range list.Items {
		version, err := strconv.Atoi(item.Labels["version"])
		if err != nil {
			continue
		}

		if version > latest {
			latest = version
		}
	}

	if latest == 0 {
		return 0, ErrReleaseNotFound
	}

	return latest, nil
}

// encodeRelease encodes a release returning a base64 encoded
// gzipped string representation, or error.
func encodeRelease(rls *rspb.Release) (string, error) {
	b, err := json.Marshal(rls)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err = w.Write(b); err != nil {
		return "", err
	}
	w.Close()

	return b64.EncodeToString(buf.Bytes()), nil
}

// decodeRelease decodes the bytes of data into a release
// type. Data must contain a base64 encoded gzipped string of a
// valid release, otherwise an error is returned.
func decodeRelease(data string) (*rspb.Release, error) {
	// base64 decode string
	b, err := b64.DecodeString(data)
	if err != nil {
		return nil, err
	}

	// For backwards compatibility with releases that were stored before
	// compression was introduced we skip decompression if the
	// gzip magic header is not found
	if len(b) > 3 && bytes.Equal(b[0:3], magicGzip) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer r.Close()
		b2, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		b = b2
	}

	var rls rspb.Release
	// unmarshal release object bytes
	if err := json.Unmarshal(b, &rls); err != nil {
		return nil, err
	}
	return &rls, nil
}

// Checks if label is system
func isSystemLabel(key string) bool {
	for _, v := range GetSystemLabels() {
		if key == v {
			return true
		}
	}
	return false
}

// Removes system labels from labels map
func filterSystemLabels(lbs map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range lbs {
		if !isSystemLabel(k) {
			result[k] = v
		}
	}
	return result
}

// Checks if labels array contains system labels
func ContainsSystemLabels(lbs map[string]string) bool {
	for k := range lbs {
		if isSystemLabel(k) {
			return true
		}
	}
	return false
}

func GetSystemLabels() []string {
	return systemLabels
}
