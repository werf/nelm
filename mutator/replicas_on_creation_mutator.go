package mutator

import (
	"helm.sh/helm/v3/pkg/werf/annotation"
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewReplicasOnCreationMutator() *ReplicasOnCreationMutator {
	return &ReplicasOnCreationMutator{}
}

type ReplicasOnCreationMutator struct{}

func (m *ReplicasOnCreationMutator) Mutate(res resource.Resourcer, operationType common.ClientOperationType) (resource.Resourcer, error) {
	if operationType != common.ClientOperationTypeCreate {
		return res, nil
	}

	var replicasOnCreationAnno *annotation.AnnotationReplicasOnCreation
	for key, value := range res.Unstructured().GetAnnotations() {
		anno := annotation.AnnotationFactory(key, value)
		if a, ok := anno.(*annotation.AnnotationReplicasOnCreation); ok {
			replicasOnCreationAnno = a
			break
		}
	}

	if replicasOnCreationAnno == nil {
		return res, nil
	}

	switch res.GroupVersionKind().GroupKind() {
	case schema.GroupKind{Group: "apps", Kind: "Deployment"},
		schema.GroupKind{Group: "apps", Kind: "StatefulSet"},
		schema.GroupKind{Group: "apps", Kind: "ReplicaSet"},
		schema.GroupKind{Group: "apps", Kind: "DaemonSet"}:
		_ = unstructured.SetNestedField(res.Unstructured().UnstructuredContent(), replicasOnCreationAnno, "spec", "replicas")
	}

	return res, nil
}
