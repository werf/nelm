package mutatorsv2

import (
	"context"

	"helm.sh/helm/v3/pkg/werf/kubeclientv2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewAnnotationsAndLabelsOnApplySetter(annotations, labels map[string]string) *AnnotationsAndLabelsOnApplySetter {
	return &AnnotationsAndLabelsOnApplySetter{
		annotations: annotations,
		labels:      labels,
	}
}

type AnnotationsAndLabelsOnApplySetter struct {
	annotations map[string]string
	labels      map[string]string
}

func (m *AnnotationsAndLabelsOnApplySetter) Mutate(ctx context.Context, info kubeclientv2.ApplyMutatableInfo, target *unstructured.Unstructured) error {
	setAnnotationsAndLabels(target, m.annotations, m.labels)
	return nil
}
