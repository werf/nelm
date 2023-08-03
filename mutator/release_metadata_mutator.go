package mutator

import (
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/resource"
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

	annos := res.Unstructured().GetAnnotations()
	if annos == nil {
		annos = make(map[string]string)
	}
	annos["meta.helm.sh/release-name"] = m.releaseName
	annos["meta.helm.sh/release-namespace"] = m.releaseNamespace
	res.Unstructured().SetAnnotations(annos)

	labels := res.Unstructured().GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["app.kubernetes.io/managed-by"] = "Helm"
	res.Unstructured().SetLabels(labels)

	return res, nil
}
