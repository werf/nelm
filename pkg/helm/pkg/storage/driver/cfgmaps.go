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

package driver // import "github.com/werf/nelm/v2/pkg/helm/pkg/storage/driver"

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kblabels "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/metadata"

	"github.com/werf/nelm/v2/pkg/helm/intern/logging"
	"github.com/werf/nelm/v2/pkg/helm/pkg/release"
	rspb "github.com/werf/nelm/v2/pkg/helm/pkg/release/v1"
)

var _ Driver = (*ConfigMaps)(nil)

var configMapsGVR = schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}

// ConfigMapsDriverName is the string name of the driver.
const ConfigMapsDriverName = "ConfigMap"

// ConfigMaps is a wrapper around an implementation of a kubernetes
// ConfigMapsInterface.
type ConfigMaps struct {
	impl corev1.ConfigMapInterface

	// Embed a LogHolder to provide logger functionality
	logging.LogHolder

	// MetadataClient, when set, lets LastVersion resolve the highest revision via a
	// metadata-only list that transfers no release bodies. Namespace is the
	// namespace it lists in and is required whenever MetadataClient is set.
	MetadataClient metadata.Interface
	Namespace      string
}

// NewConfigMaps initializes a new ConfigMaps wrapping an implementation of
// the kubernetes ConfigMapsInterface.
func NewConfigMaps(impl corev1.ConfigMapInterface) *ConfigMaps {
	c := &ConfigMaps{
		impl: impl,
	}
	c.SetLogger(slog.Default().Handler())
	return c
}

// Name returns the name of the driver.
func (cfgmaps *ConfigMaps) Name() string {
	return ConfigMapsDriverName
}

// LastVersion returns the highest revision of the named release, or
// ErrReleaseNotFound if the release does not exist. It reads only label
// metadata and decodes no release body.
func (cfgmaps *ConfigMaps) LastVersion(name string) (int, error) {
	selector := kblabels.Set{"owner": "helm", "name": name}.AsSelector().String()

	if cfgmaps.MetadataClient != nil {
		return lastVersionFromMetadata(context.Background(), cfgmaps.MetadataClient, configMapsGVR, cfgmaps.Namespace, selector)
	}

	// Safety net without a metadata client: a typed list still avoids decoding
	// release bodies, but unlike the metadata path it transfers them over the wire.
	list, err := cfgmaps.impl.List(context.Background(), metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return 0, fmt.Errorf("last version: failed to list: %w", err)
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

// Revisions returns the metadata of every revision of the named release, sorted
// by ascending version. It reads only labels and decodes no release body.
func (cfgmaps *ConfigMaps) Revisions(ctx context.Context, name string) ([]RevisionRecord, error) {
	if cfgmaps.Namespace == "" {
		return nil, fmt.Errorf("list revisions of release %q: namespace is required", name)
	}

	if errs := validation.IsValidLabelValue(name); len(errs) != 0 {
		return nil, fmt.Errorf("list revisions of release %q: invalid label value: %s", name, strings.Join(errs, "; "))
	}

	selector := kblabels.Set{"owner": "helm", "name": name}.AsSelector().String()

	var records []RevisionRecord

	if cfgmaps.MetadataClient != nil {
		opts := metav1.ListOptions{LabelSelector: selector, Limit: listLatestPageSize}

		for {
			list, err := cfgmaps.MetadataClient.Resource(configMapsGVR).Namespace(cfgmaps.Namespace).List(ctx, opts)
			if err != nil {
				return nil, fmt.Errorf("list revision metadata of release %q: %w", name, err)
			}

			for _, item := range list.Items {
				record, ok := revisionRecordFromLabels(cfgmaps.Namespace, item.Labels)
				if !ok {
					continue
				}

				records = append(records, record)
			}

			if list.Continue == "" {
				break
			}

			opts.Continue = list.Continue
		}

		sort.Slice(records, func(i, j int) bool { return records[i].Version < records[j].Version })

		return records, nil
	}

	// Safety net without a metadata client: a typed list still avoids decoding
	// release bodies, but unlike the metadata path it transfers them over the wire.
	opts := metav1.ListOptions{LabelSelector: selector, Limit: listLatestPageSize}

	for {
		list, err := cfgmaps.impl.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list revisions of release %q: %w", name, err)
		}

		for _, item := range list.Items {
			record, ok := revisionRecordFromLabels(cfgmaps.Namespace, item.Labels)
			if !ok {
				continue
			}

			records = append(records, record)
		}

		if list.Continue == "" {
			break
		}

		opts.Continue = list.Continue
	}

	sort.Slice(records, func(i, j int) bool { return records[i].Version < records[j].Version })

	return records, nil
}

// ListLatestReleases returns the highest revision of every release owned by Helm.
// Superseded revisions are dropped while paging, before any body is decoded, so
// the cost does not scale with the depth of the histories.
func (cfgmaps *ConfigMaps) ListLatestReleases(ctx context.Context) ([]*rspb.Release, error) {
	opts := metav1.ListOptions{
		LabelSelector: kblabels.Set{"owner": "helm"}.AsSelector().String(),
		Limit:         listLatestPageSize,
	}

	latestItems := map[string]v1.ConfigMap{}
	latestVersions := map[string]int{}

	for {
		list, err := cfgmaps.impl.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list latest releases: failed to list: %w", err)
		}

		for _, item := range list.Items {
			key, version, ok := releaseKeyAndVersionFromLabels(item.Namespace, item.Labels)
			if !ok {
				continue
			}

			if current, found := latestVersions[key]; found && current >= version {
				continue
			}

			latestItems[key] = item
			latestVersions[key] = version
		}

		if list.Continue == "" {
			break
		}

		opts.Continue = list.Continue
	}

	releases := make([]*rspb.Release, 0, len(latestItems))

	// The entry is dropped before its body is decoded, so the undecoded
	// survivors and the decoded releases are never both live.
	for key, item := range latestItems {
		delete(latestItems, key)

		rls, err := decodeRelease(item.Data["release"])
		if err != nil {
			cfgmaps.Logger().Debug("list latest releases: failed to decode release", slog.String("name", item.Name), slog.Any("error", err))

			_, version, _ := releaseKeyAndVersionFromLabels(item.Namespace, item.Labels)
			rls, err = cfgmaps.findPreviousValidRelease(ctx, item.Namespace, item.Labels["name"], version)
			if err != nil {
				return nil, err
			}
			if rls != nil {
				releases = append(releases, rls)
			}
			continue
		}

		if rls.Namespace == "" {
			rls.Namespace = item.Namespace
		}

		rls.Labels = item.Labels
		releases = append(releases, rls)
	}

	return releases, nil
}

// findPreviousValidRelease returns the newest decodable revision of the release
// below beforeVersion, or nil if the whole remaining history is corrupt. The
// scan is scoped to a single release in a single namespace, so it lists that
// release's own history only.
func (cfgmaps *ConfigMaps) findPreviousValidRelease(ctx context.Context, namespace, name string, beforeVersion int) (*rspb.Release, error) {
	opts := metav1.ListOptions{
		LabelSelector: kblabels.Set{"owner": "helm", "name": name}.AsSelector().String(),
		FieldSelector: fields.OneTermEqualSelector("metadata.namespace", namespace).String(),
		Limit:         listLatestPageSize,
	}

	var candidates []v1.ConfigMap

	for {
		list, err := cfgmaps.impl.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list latest releases: failed to list previous revisions: %w", err)
		}

		for _, item := range list.Items {
			_, version, ok := releaseKeyAndVersionFromLabels(item.Namespace, item.Labels)
			if !ok || item.Namespace != namespace || version >= beforeVersion {
				continue
			}

			candidates = append(candidates, item)
		}

		if list.Continue == "" {
			break
		}

		opts.Continue = list.Continue
	}

	sort.Slice(candidates, func(i, j int) bool {
		return releaseVersionFromLabels(candidates[i].Labels) > releaseVersionFromLabels(candidates[j].Labels)
	})

	for _, item := range candidates {
		rls, err := decodeRelease(item.Data["release"])
		if err != nil {
			cfgmaps.Logger().Debug("list latest releases: failed to decode release", slog.String("name", item.Name), slog.Any("error", err))
			continue
		}

		if rls.Namespace == "" {
			rls.Namespace = item.Namespace
		}

		rls.Labels = item.Labels

		return rls, nil
	}

	return nil, nil
}

// Get fetches the release named by key. The corresponding release is returned
// or error if not found.
func (cfgmaps *ConfigMaps) Get(key string) (release.Releaser, error) {
	// fetch the configmap holding the release named by key
	obj, err := cfgmaps.impl.Get(context.Background(), key, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrReleaseNotFound
		}

		cfgmaps.Logger().Debug("failed to get release", slog.String("key", key), slog.Any("error", err))
		return nil, err
	}
	// found the configmap, decode the base64 data string
	r, err := decodeRelease(obj.Data["release"])
	if err != nil {
		cfgmaps.Logger().Debug("failed to decode data", slog.String("key", key), slog.Any("error", err))
		return nil, err
	}
	r.Labels = filterSystemLabels(obj.Labels)
	// return the release object
	return r, nil
}

// List fetches all releases and returns the list releases such
// that filter(release) == true. An error is returned if the
// configmap fails to retrieve the releases.
func (cfgmaps *ConfigMaps) List(filter func(release.Releaser) bool) ([]release.Releaser, error) {
	lsel := kblabels.Set{"owner": "helm"}.AsSelector()
	opts := metav1.ListOptions{LabelSelector: lsel.String()}

	list, err := cfgmaps.impl.List(context.Background(), opts)
	if err != nil {
		cfgmaps.Logger().Debug("failed to list releases", slog.Any("error", err))
		return nil, err
	}

	var results []release.Releaser

	// iterate over the configmaps object list
	// and decode each release
	for _, item := range list.Items {
		rls, err := decodeRelease(item.Data["release"])
		if err != nil {
			cfgmaps.Logger().Debug("failed to decode release", slog.Any("item", item), slog.Any("error", err))
			continue
		}

		rls.Labels = item.Labels

		if filter(rls) {
			results = append(results, rls)
		}
	}
	return results, nil
}

// Query fetches all releases that match the provided map of labels.
// An error is returned if the configmap fails to retrieve the releases.
func (cfgmaps *ConfigMaps) Query(labels map[string]string) ([]release.Releaser, error) {
	ls := kblabels.Set{}
	for k, v := range labels {
		if errs := validation.IsValidLabelValue(v); len(errs) != 0 {
			return nil, fmt.Errorf("invalid label value: %q: %s", v, strings.Join(errs, "; "))
		}
		ls[k] = v
	}

	opts := metav1.ListOptions{LabelSelector: ls.AsSelector().String()}

	list, err := cfgmaps.impl.List(context.Background(), opts)
	if err != nil {
		cfgmaps.Logger().Debug("failed to query with labels", slog.Any("error", err))
		return nil, err
	}

	if len(list.Items) == 0 {
		return nil, ErrReleaseNotFound
	}

	var results []release.Releaser
	for _, item := range list.Items {
		rls, err := decodeRelease(item.Data["release"])
		if err != nil {
			cfgmaps.Logger().Debug("failed to decode release", slog.Any("error", err))
			continue
		}
		rls.Labels = item.Labels
		results = append(results, rls)
	}
	return results, nil
}

// Create creates a new ConfigMap holding the release. If the
// ConfigMap already exists, ErrReleaseExists is returned.
func (cfgmaps *ConfigMaps) Create(key string, rls release.Releaser) error {
	// set labels for configmaps object meta data
	var lbs labels

	rac, err := release.NewAccessor(rls)
	if err != nil {
		return err
	}

	lbs.init()
	lbs.fromMap(rac.Labels())
	lbs.set("createdAt", strconv.FormatInt(time.Now().Unix(), 10))

	rel, err := releaserToV1Release(rls)
	if err != nil {
		return err
	}

	// create a new configmap to hold the release
	obj, err := newConfigMapsObject(key, rel, lbs)
	if err != nil {
		cfgmaps.Logger().Debug("failed to encode release", slog.String("name", rac.Name()), slog.Any("error", err))
		return err
	}
	// push the configmap object out into the kubiverse
	if _, err := cfgmaps.impl.Create(context.Background(), obj, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return ErrReleaseExists
		}

		cfgmaps.Logger().Debug("failed to create release", slog.Any("error", err))
		return err
	}
	return nil
}

// Update updates the ConfigMap holding the release. If not found
// the ConfigMap is created to hold the release.
func (cfgmaps *ConfigMaps) Update(key string, rel release.Releaser) error {
	// set labels for configmaps object meta data
	var lbs labels

	rls, err := releaserToV1Release(rel)
	if err != nil {
		return err
	}

	lbs.init()
	lbs.fromMap(rls.Labels)
	lbs.set("modifiedAt", strconv.FormatInt(time.Now().Unix(), 10))

	// create a new configmap object to hold the release
	obj, err := newConfigMapsObject(key, rls, lbs)
	if err != nil {
		cfgmaps.Logger().Debug(
			"failed to encode release",
			slog.String("name", rls.Name),
			slog.Any("error", err),
		)
		return err
	}
	// push the configmap object out into the kubiverse
	_, err = cfgmaps.impl.Update(context.Background(), obj, metav1.UpdateOptions{})
	if err != nil {
		cfgmaps.Logger().Debug("failed to update release", slog.Any("error", err))
		return err
	}
	return nil
}

// UpdateLabels merges the given custom labels into the ConfigMap holding the
// release named by key without creating a new revision. System labels in the
// map are ignored to avoid corrupting release metadata.
func (cfgmaps *ConfigMaps) UpdateLabels(key string, lbls map[string]string) error {
	obj, err := cfgmaps.impl.Get(context.Background(), key, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ErrReleaseNotFound
		}
		return fmt.Errorf("update labels: failed to get %q: %w", key, err)
	}

	if obj.Labels == nil {
		obj.Labels = map[string]string{}
	}
	for k, v := range filterSystemLabels(lbls) {
		obj.Labels[k] = v
	}

	if _, err := cfgmaps.impl.Update(context.Background(), obj, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update labels: failed to update %q: %w", key, err)
	}
	return nil
}

// Delete deletes the ConfigMap holding the release named by key.
// Delete removes the release named by key. The stored body is decoded only after the
// object is gone, so a release whose body can no longer be decoded is still deleted; the
// returned release is nil in that case.
func (cfgmaps *ConfigMaps) Delete(key string) (release.Releaser, error) {
	obj, err := cfgmaps.impl.Get(context.Background(), key, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrReleaseNotFound
		}
		return nil, fmt.Errorf("delete: failed to get %q: %w", key, err)
	}

	if err := cfgmaps.impl.Delete(context.Background(), key, metav1.DeleteOptions{}); err != nil {
		return nil, err
	}

	rls, err := decodeRelease(obj.Data["release"])
	if err != nil {
		cfgmaps.Logger().Debug("delete: failed to decode data", slog.String("key", key), slog.Any("error", err))
		return nil, nil
	}
	rls.Labels = filterSystemLabels(obj.Labels)
	return rls, nil
}

// newConfigMapsObject constructs a kubernetes ConfigMap object
// to store a release. Each configmap data entry is the base64
// encoded gzipped string of a release.
//
// The following labels are used within each configmap:
//
//	"modifiedAt"     - timestamp indicating when this configmap was last modified. (set in Update)
//	"createdAt"      - timestamp indicating when this configmap was created. (set in Create)
//	"version"        - version of the release.
//	"status"         - status of the release (see pkg/release/status.go for variants)
//	"owner"          - owner of the configmap, currently "helm".
//	"name"           - name of the release.
func newConfigMapsObject(key string, rls *rspb.Release, lbs labels) (*v1.ConfigMap, error) {
	const owner = "helm"

	// encode the release
	s, err := encodeRelease(rls)
	if err != nil {
		return nil, err
	}

	if lbs == nil {
		lbs.init()
	}

	// apply custom labels
	lbs.fromMap(rls.Labels)

	// apply labels
	lbs.set("name", rls.Name)
	lbs.set("owner", owner)
	lbs.set("status", rls.Info.Status.String())
	lbs.set("version", strconv.Itoa(rls.Version))

	// create and return configmap object
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:   key,
			Labels: lbs.toMap(),
		},
		Data: map[string]string{"release": s},
	}, nil
}
