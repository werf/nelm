package resource

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/common"
)

var (
	_ ResourcePatcher = (*ExtraMetadataPatcher)(nil)
	_ ResourcePatcher = (*ReleaseMetadataPatcher)(nil)
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

const TypeExtraMetadataPatcher ResourcePatcherType = "extra-metadata-patcher"

func NewExtraMetadataPatcher(annotations, labels map[string]string) *ExtraMetadataPatcher {
	return &ExtraMetadataPatcher{
		annotations: annotations,
		labels:      labels,
	}
}

type ExtraMetadataPatcher struct {
	annotations map[string]string
	labels      map[string]string
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

const TypeReleaseMetadataPatcher ResourcePatcherType = "release-metadata-patcher"

func NewReleaseMetadataPatcher(releaseName, releaseNamespace string) *ReleaseMetadataPatcher {
	return &ReleaseMetadataPatcher{
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}
}

type ReleaseMetadataPatcher struct {
	releaseName      string
	releaseNamespace string
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
