package mutatorsv2

import (
	"context"

	"helm.sh/helm/v3/pkg/werf/kubeclientv2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewReleaseMetadataOnCreateSetter(releaseName, releaseNamespace string) *ReleaseMetadataOnCreateSetter {
	return &ReleaseMetadataOnCreateSetter{
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}
}

type ReleaseMetadataOnCreateSetter struct {
	releaseName      string
	releaseNamespace string
}

func (m *ReleaseMetadataOnCreateSetter) Mutate(ctx context.Context, info kubeclientv2.CreateMutatableInfo, target *unstructured.Unstructured) error {
	if !info.PartOfRelease() {
		return nil
	}

	setReleaseMetadata(target, m.releaseName, m.releaseNamespace)

	return nil
}

func setReleaseMetadata(res *unstructured.Unstructured, releaseName, releaseNamespace string) {
	annos := res.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}
	annos["meta.helm.sh/release-name"] = releaseName
	annos["meta.helm.sh/release-namespace"] = releaseNamespace
	res.SetAnnotations(annos)

	labels := res.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["app.kubernetes.io/managed-by"] = "Helm"
	res.SetLabels(labels)
}
