package resrc

import (
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const TypeReleaseNamespace Type = "release-namespace"

func NewReleaseNamespace(unstruct *unstructured.Unstructured, opts ReleaseNamespaceOptions) *ReleaseNamespace {
	resID := resrcid.NewResourceIDFromUnstruct(unstruct, resrcid.ResourceIDOptions{
		FilePath: opts.FilePath,
		Mapper:   opts.Mapper,
	})

	return &ReleaseNamespace{
		ResourceID: resID,
		unstruct:   unstruct,
		mapper:     opts.Mapper,
	}
}

type ReleaseNamespaceOptions struct {
	FilePath string
	Mapper   meta.ResettableRESTMapper
}

type ReleaseNamespace struct {
	*resrcid.ResourceID

	unstruct *unstructured.Unstructured
	mapper   meta.ResettableRESTMapper
}

func (r *ReleaseNamespace) Validate() error {
	return nil
}

func (r *ReleaseNamespace) Unstructured() *unstructured.Unstructured {
	return r.unstruct
}

func (r *ReleaseNamespace) ManageableBy() ManageableBy {
	return ManageableByAnyone
}

func (r *ReleaseNamespace) Type() Type {
	return TypeReleaseNamespace
}
