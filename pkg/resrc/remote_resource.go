package resrc

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/resrcid"
)

const TypeRemoteResource Type = "remote-resource"

func NewRemoteResource(unstruct *unstructured.Unstructured, opts RemoteResourceOptions) *RemoteResource {
	resID := resrcid.NewResourceIDFromUnstruct(unstruct, resrcid.ResourceIDOptions{
		DefaultNamespace: opts.FallbackNamespace,
		Mapper:           opts.Mapper,
	})

	return &RemoteResource{
		ResourceID: resID,
		unstruct:   unstruct,
		mapper:     opts.Mapper,
	}
}

type RemoteResourceOptions struct {
	FallbackNamespace string
	Mapper            meta.ResettableRESTMapper
}

type RemoteResource struct {
	*resrcid.ResourceID

	unstruct *unstructured.Unstructured
	mapper   meta.ResettableRESTMapper
}

func (r *RemoteResource) Unstructured() *unstructured.Unstructured {
	return r.unstruct
}

func (r *RemoteResource) Type() Type {
	return TypeRemoteResource
}

func (r *RemoteResource) FixManagedFields() (changed bool, err error) {
	return fixManagedFields(r.unstruct)
}

func (r *RemoteResource) AdoptableBy(releaseName, releaseNamespace string) (adoptable bool, nonAdoptableReason string) {
	return adoptableBy(r.unstruct, releaseName, releaseNamespace)
}

func (r *RemoteResource) KeepOnDelete() bool {
	if err := validateResourcePolicy(r.unstruct); err != nil {
		return true
	}

	return keepOnDelete(r.unstruct)
}
