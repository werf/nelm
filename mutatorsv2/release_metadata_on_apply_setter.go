package mutatorsv2

import (
	"context"

	"helm.sh/helm/v3/pkg/werf/kubeclientv2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func NewReleaseMetadataOnApplySetter(releaseName, releaseNamespace string) *ReleaseMetadataOnApplySetter {
	return &ReleaseMetadataOnApplySetter{
		releaseName:      releaseName,
		releaseNamespace: releaseNamespace,
	}
}

type ReleaseMetadataOnApplySetter struct {
	releaseName      string
	releaseNamespace string
}

func (m *ReleaseMetadataOnApplySetter) Mutate(ctx context.Context, info kubeclientv2.ApplyMutatableInfo, target *unstructured.Unstructured) error {
	if !info.PartOfRelease() {
		return nil
	}

	setReleaseMetadata(target, m.releaseName, m.releaseNamespace)

	return nil
}
