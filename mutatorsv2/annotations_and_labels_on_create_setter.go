package mutatorsv2

import (
	"context"

	"helm.sh/helm/v3/pkg/werf/kubeclientv2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewAnnotationsAndLabelsOnCreateSetter(annotations, labels map[string]string) *AnnotationsAndLabelsOnCreateSetter {
	return &AnnotationsAndLabelsOnCreateSetter{
		annotations: annotations,
		labels:      labels,
	}
}

type AnnotationsAndLabelsOnCreateSetter struct {
	annotations map[string]string
	labels      map[string]string
}

func (m *AnnotationsAndLabelsOnCreateSetter) Mutate(ctx context.Context, info kubeclientv2.CreateMutatableInfo, target *unstructured.Unstructured) error {
	setAnnotationsAndLabels(target, m.annotations, m.labels)
	return nil
}

func setAnnotationsAndLabels(res *unstructured.Unstructured, annotations, labels map[string]string) {
	if len(annotations) > 0 {
		annos := res.GetAnnotations()
		if annos == nil {
			annos = map[string]string{}
		}
		for k, v := range annotations {
			annos[k] = v
		}
		res.SetAnnotations(annos)
	}

	if len(labels) > 0 {
		labels = res.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		for k, v := range labels {
			labels[k] = v
		}
		res.SetLabels(labels)
	}
}
