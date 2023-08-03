package mutatorsv2

import (
	"context"

	"helm.sh/helm/v3/pkg/werf/kubeclientv2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewServiceAnnotationsAndLabelsOnApplySetter(annotations, labels map[string]string) *ServiceAnnotationsAndLabelsOnApplySetter {
	return &ServiceAnnotationsAndLabelsOnApplySetter{
		annotations: annotations,
		labels:      labels,
	}
}

type ServiceAnnotationsAndLabelsOnApplySetter struct {
	annotations map[string]string
	labels      map[string]string
}

func (m *ServiceAnnotationsAndLabelsOnApplySetter) Mutate(ctx context.Context, info kubeclientv2.ApplyMutatableInfo, target *unstructured.Unstructured) error {
	if !info.ShouldHaveServiceMetadata() {
		return nil
	}

	setAnnotationsAndLabels(target, m.annotations, m.labels)
	return nil
}
