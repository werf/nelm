package mutatorsv2

import (
	"context"

	"helm.sh/helm/v3/pkg/werf/kubeclientv2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewServiceAnnotationsAndLabelsOnCreateSetter(annotations, labels map[string]string) *ServiceAnnotationsAndLabelsOnCreateSetter {
	return &ServiceAnnotationsAndLabelsOnCreateSetter{
		annotations: annotations,
		labels:      labels,
	}
}

type ServiceAnnotationsAndLabelsOnCreateSetter struct {
	annotations map[string]string
	labels      map[string]string
}

func (m *ServiceAnnotationsAndLabelsOnCreateSetter) Mutate(ctx context.Context, info kubeclientv2.CreateMutatableInfo, target *unstructured.Unstructured) error {
	if !info.ShouldHaveServiceMetadata() {
		return nil
	}

	setAnnotationsAndLabels(target, m.annotations, m.labels)
	return nil
}
