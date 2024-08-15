package resrctransfrmr

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/log"
)

var _ ResourceTransformer = (*DropInvalidAnnotationsAndLabelsTransformer)(nil)

const TypeDropInvalidAnnotationsAndLabelsTransformer Type = "drop-invalid-annotations-and-labels-transformer"

func NewDropInvalidAnnotationsAndLabelsTransformer() *DropInvalidAnnotationsAndLabelsTransformer {
	return &DropInvalidAnnotationsAndLabelsTransformer{}
}

// TODO(3.0): remove this transformer. Replace it with proper early validation of resource Heads.
type DropInvalidAnnotationsAndLabelsTransformer struct{}

func (t *DropInvalidAnnotationsAndLabelsTransformer) Match(ctx context.Context, info *ResourceInfo) (matched bool, err error) {
	return true, nil
}

func (t *DropInvalidAnnotationsAndLabelsTransformer) Transform(ctx context.Context, info *ResourceInfo) ([]*unstructured.Unstructured, error) {
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

func (t *DropInvalidAnnotationsAndLabelsTransformer) Type() Type {
	return TypeDropInvalidAnnotationsAndLabelsTransformer
}
