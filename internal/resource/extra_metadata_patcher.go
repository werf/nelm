package resource

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ ResourcePatcher = (*ExtraMetadataPatcher)(nil)

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
