package mutator

import (
	"fmt"

	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewReleaseMetadataMutator(releaseName, releaseNamespace string) *ReleaseMetadataMutator {
	return &ReleaseMetadataMutator{
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}
}

type ReleaseMetadataMutator struct {
	releaseName      string
	releaseNamespace string
}

func (m *ReleaseMetadataMutator) Mutate(res resource.Resourcer, operationType common.ClientOperationType) (resource.Resourcer, error) {
	if !res.PartOfRelease() {
		return res, nil
	}

	switch operationType {
	case common.ClientOperationTypeCreate, common.ClientOperationTypeUpdate, common.ClientOperationTypeSmartApply:
	default:
		return res, nil
	}

	if err := unstructured.SetNestedField(res.Unstructured().UnstructuredContent(), m.releaseName, "metadata", "annotations", "meta.helm.sh/release-name"); err != nil {
		return nil, fmt.Errorf("error adding release name annotation: %w", err)
	}

	if err := unstructured.SetNestedField(res.Unstructured().UnstructuredContent(), m.releaseNamespace, "metadata", "annotations", "meta.helm.sh/release-namespace"); err != nil {
		return nil, fmt.Errorf("error adding release namespace annotation: %w", err)
	}

	if err := unstructured.SetNestedField(res.Unstructured().UnstructuredContent(), "Helm", "metadata", "labels", "app.kubernetes.io/managed-by"); err != nil {
		return nil, fmt.Errorf("error adding managed-by Helm label: %w", err)
	}

	return res, nil
}
