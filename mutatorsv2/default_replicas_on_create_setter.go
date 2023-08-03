package mutatorsv2

import (
	"context"

	"helm.sh/helm/v3/pkg/werf/kubeclientv2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewDefaultReplicasOnCreateSetter() *DefaultReplicasOnCreateSetter {
	return &DefaultReplicasOnCreateSetter{}
}

type DefaultReplicasOnCreateSetter struct{}

func (m *DefaultReplicasOnCreateSetter) Mutate(ctx context.Context, info kubeclientv2.CreateMutatableInfo, target *unstructured.Unstructured) error {
	replicas, set := info.DefaultReplicasOnCreation()
	if !set {
		return nil
	}

	_ = unstructured.SetNestedField(target.UnstructuredContent(), replicas, "spec", "replicas")

	return nil
}
