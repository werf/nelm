package spec

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/werf/nelm/pkg/log"
)

var (
	_ ResourceTransformer = (*DropInvalidAnnotationsAndLabelsTransformer)(nil)
	_ ResourceTransformer = (*ResourceListsTransformer)(nil)
)

type ResourceTransformer interface {
	Match(ctx context.Context, resourceInfo *ResourceTransformerResourceInfo) (matched bool, err error)
	Transform(ctx context.Context, matchedResourceInfo *ResourceTransformerResourceInfo) (output []*unstructured.Unstructured, err error)
	Type() ResourceTransformerType
}

type ResourceTransformerResourceInfo struct {
	Obj *unstructured.Unstructured
}

type ResourceTransformerType string

const (
	TypeDropInvalidAnnotationsAndLabelsTransformer ResourceTransformerType = "drop-invalid-annotations-and-labels-transformer"
	TypeResourceListsTransformer                   ResourceTransformerType = "resource-lists-transformer"
)

// TODO(v2): remove this transformer. Replace it with proper early validation of resource Heads.
type DropInvalidAnnotationsAndLabelsTransformer struct{}

func NewDropInvalidAnnotationsAndLabelsTransformer() *DropInvalidAnnotationsAndLabelsTransformer {
	return &DropInvalidAnnotationsAndLabelsTransformer{}
}

func (t *DropInvalidAnnotationsAndLabelsTransformer) Match(ctx context.Context, info *ResourceTransformerResourceInfo) (matched bool, err error) {
	return true, nil
}

func (t *DropInvalidAnnotationsAndLabelsTransformer) Transform(ctx context.Context, info *ResourceTransformerResourceInfo) ([]*unstructured.Unstructured, error) {
	annotations, _, _ := unstructured.NestedMap(info.Obj.Object, "metadata", "annotations")

	resultAnnotations := make(map[string]string)
	for annoKey, rawAnnoValue := range annotations {
		annoValue, valIsString := rawAnnoValue.(string)
		if !valIsString {
			log.Default.Warn(ctx, "Dropped invalid annotation %q in resource %q (%s): key is not a string", annoKey, info.Obj.GetName(), info.Obj.GroupVersionKind().String())
			continue
		}

		resultAnnotations[annoKey] = annoValue
	}

	labels, _, _ := unstructured.NestedMap(info.Obj.Object, "metadata", "labels")

	resultLabels := make(map[string]string)
	for labelKey, rawLabelValue := range labels {
		labelValue, valIsString := rawLabelValue.(string)
		if !valIsString {
			log.Default.Warn(ctx, "Dropped invalid label %q in resource %q (%s): key is not a string", labelKey, info.Obj.GetName(), info.Obj.GroupVersionKind().String())
			continue
		}

		resultLabels[labelKey] = labelValue
	}

	info.Obj.SetAnnotations(resultAnnotations)
	info.Obj.SetLabels(resultLabels)

	return []*unstructured.Unstructured{info.Obj}, nil
}

func (t *DropInvalidAnnotationsAndLabelsTransformer) Type() ResourceTransformerType {
	return TypeDropInvalidAnnotationsAndLabelsTransformer
}

type ResourceListsTransformer struct{}

func NewResourceListsTransformer() *ResourceListsTransformer {
	return &ResourceListsTransformer{}
}

func (t *ResourceListsTransformer) Match(ctx context.Context, info *ResourceTransformerResourceInfo) (matched bool, err error) {
	return info.Obj.IsList(), nil
}

func (t *ResourceListsTransformer) Transform(ctx context.Context, info *ResourceTransformerResourceInfo) ([]*unstructured.Unstructured, error) {
	var result []*unstructured.Unstructured

	if err := info.Obj.EachListItem(
		func(obj runtime.Object) error {
			result = append(result, obj.(*unstructured.Unstructured))
			return nil
		},
	); err != nil {
		return nil, fmt.Errorf("error iterating over list items: %w", err)
	}

	return result, nil
}

func (t *ResourceListsTransformer) Type() ResourceTransformerType {
	return TypeResourceListsTransformer
}
