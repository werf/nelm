package resource

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ ResourcePatcher = (*ReleaseMetadataPatcher)(nil)

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
	return info.ManageableBy == ManageableBySingleRelease, nil
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
