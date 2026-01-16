package spec

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/pkg/common"
)

var (
	_ ResourcePatcher = (*ExtraMetadataPatcher)(nil)
	_ ResourcePatcher = (*ReleaseMetadataPatcher)(nil)
	_ ResourcePatcher = (*LegacyOnlyTrackJobsPatcher)(nil)
	_ ResourcePatcher = (*SecretStringDataPatcher)(nil)
)

type ResourcePatcher interface {
	Match(ctx context.Context, resourceInfo *ResourcePatcherResourceInfo) (matched bool, err error)
	Patch(ctx context.Context, matchedResourceInfo *ResourcePatcherResourceInfo) (output *unstructured.Unstructured, err error)
	Type() ResourcePatcherType
}

type ResourcePatcherResourceInfo struct {
	Obj       *unstructured.Unstructured
	Ownership common.Ownership
}

type ResourcePatcherType string

const (
	TypeExtraMetadataPatcher    ResourcePatcherType = "extra-metadata-patcher"
	TypeReleaseMetadataPatcher  ResourcePatcherType = "release-metadata-patcher"
	TypeOnlyTrackJobsPatcher    ResourcePatcherType = "only-track-jobs-patcher"
	TypeSecretStringDataPatcher ResourcePatcherType = "secret-string-data-patcher"
)

type ExtraMetadataPatcher struct {
	annotations map[string]string
	labels      map[string]string
}

func NewExtraMetadataPatcher(annotations, labels map[string]string) *ExtraMetadataPatcher {
	return &ExtraMetadataPatcher{
		annotations: annotations,
		labels:      labels,
	}
}

func (p *ExtraMetadataPatcher) Match(ctx context.Context, info *ResourcePatcherResourceInfo) (bool, error) {
	return true, nil
}

func (p *ExtraMetadataPatcher) Patch(ctx context.Context, info *ResourcePatcherResourceInfo) (*unstructured.Unstructured, error) {
	setAnnotationsAndLabels(info.Obj, p.annotations, p.labels)
	return info.Obj, nil
}

func (p *ExtraMetadataPatcher) Type() ResourcePatcherType {
	return TypeExtraMetadataPatcher
}

type ReleaseMetadataPatcher struct {
	releaseName      string
	releaseNamespace string
}

func NewReleaseMetadataPatcher(releaseName, releaseNamespace string) *ReleaseMetadataPatcher {
	return &ReleaseMetadataPatcher{
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}
}

func (p *ReleaseMetadataPatcher) Match(ctx context.Context, info *ResourcePatcherResourceInfo) (bool, error) {
	return info.Ownership == common.OwnershipRelease, nil
}

func (p *ReleaseMetadataPatcher) Patch(ctx context.Context, info *ResourcePatcherResourceInfo) (*unstructured.Unstructured, error) {
	annos := map[string]string{}
	annos["meta.helm.sh/release-name"] = p.releaseName
	annos["meta.helm.sh/release-namespace"] = p.releaseNamespace

	labels := map[string]string{}
	labels["app.kubernetes.io/managed-by"] = "Helm"

	setAnnotationsAndLabels(info.Obj, annos, labels)

	return info.Obj, nil
}

func (p *ReleaseMetadataPatcher) Type() ResourcePatcherType {
	return TypeReleaseMetadataPatcher
}

// TODO(v2): get rid of it when patching is implemented or when Kubedog compaitiblity with Helm charts improved
type LegacyOnlyTrackJobsPatcher struct{}

func NewLegacyOnlyTrackJobsPatcher() *LegacyOnlyTrackJobsPatcher {
	return &LegacyOnlyTrackJobsPatcher{}
}

func (p *LegacyOnlyTrackJobsPatcher) Match(ctx context.Context, info *ResourcePatcherResourceInfo) (bool, error) {
	annos := info.Obj.GetAnnotations()
	if annos == nil {
		return true, nil
	}

	if !lo.HasKey(annos, "helm.sh/hook") {
		return true, nil
	}

	switch info.Obj.GetKind() {
	case "Job", "Pod":
	default:
		return true, nil
	}

	return false, nil
}

func (p *LegacyOnlyTrackJobsPatcher) Patch(ctx context.Context, info *ResourcePatcherResourceInfo) (*unstructured.Unstructured, error) {
	annos := map[string]string{}
	annos["werf.io/fail-mode"] = string(multitrack.IgnoreAndContinueDeployProcess)
	annos["werf.io/track-termination-mode"] = string(multitrack.NonBlocking)

	setAnnotationsAndLabels(info.Obj, annos, nil)

	return info.Obj, nil
}

func (p *LegacyOnlyTrackJobsPatcher) Type() ResourcePatcherType {
	return TypeOnlyTrackJobsPatcher
}

type SecretStringDataPatcher struct{}

func NewSecretStringDataPatcher() *SecretStringDataPatcher {
	return &SecretStringDataPatcher{}
}

func (p *SecretStringDataPatcher) Match(ctx context.Context, info *ResourcePatcherResourceInfo) (bool, error) {
	return info.Obj.GroupVersionKind().GroupKind() == schema.GroupKind{Group: "", Kind: "Secret"}, nil
}

func (p *SecretStringDataPatcher) Patch(ctx context.Context, info *ResourcePatcherResourceInfo) (*unstructured.Unstructured, error) {
	unstructuredContent := info.Obj.UnstructuredContent()

	stringData, found, err := unstructured.NestedStringMap(unstructuredContent, "stringData")
	if err != nil {
		return nil, fmt.Errorf("failed to get stringData from secret: %w", err)
	}

	if !found {
		return info.Obj, nil
	}

	data, found, err := unstructured.NestedStringMap(unstructuredContent, "data")
	if err != nil {
		return nil, fmt.Errorf("failed to get data from secret: %w", err)
	}

	if !found {
		data = map[string]string{}
	}

	for key, val := range stringData {
		data[key] = base64.StdEncoding.EncodeToString([]byte(val))
	}

	err = unstructured.SetNestedStringMap(unstructuredContent, data, "data")
	if err != nil {
		return nil, err
	}

	unstructured.RemoveNestedField(unstructuredContent, "stringData")

	return info.Obj, nil
}

func (p *SecretStringDataPatcher) Type() ResourcePatcherType { return TypeSecretStringDataPatcher }
