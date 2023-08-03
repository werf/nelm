package resourcev2

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
)

func NewLocalReleaseNamespace(unstruct *unstructured.Unstructured, opts NewLocalReleaseNamespaceOptions) *LocalReleaseNamespace {
	return &LocalReleaseNamespace{
		localBaseResource:            newLocalBaseResource(unstruct, opts.FilePath, newLocalBaseResourceOptions{Mapper: opts.Mapper}),
		recreatableResource:          newRecreatableResource(unstruct),
		neverDeletableResource:       newNeverDeletableResource(unstruct),
		trackableResource:            newTrackableResource(unstruct),
		externallyDependableResource: newExternallyDependableResource(unstruct, "", newExternallyDependableResourceOptions{Mapper: opts.Mapper, DiscoveryClient: opts.DiscoveryClient}),
	}
}

type NewLocalReleaseNamespaceOptions struct {
	FilePath        string
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type LocalReleaseNamespace struct {
	*localBaseResource
	*recreatableResource
	*neverDeletableResource
	*trackableResource
	*externallyDependableResource
}

func (r *LocalReleaseNamespace) Validate() error {
	if err := r.localBaseResource.Validate(); err != nil {
		return err
	}

	if err := r.trackableResource.Validate(); err != nil {
		return err
	}

	if err := r.externallyDependableResource.Validate(); err != nil {
		return err
	}

	return nil
}

func (r *LocalReleaseNamespace) Scope() ResourceScope {
	return ResourceScopeCluster
}

func (r *LocalReleaseNamespace) PartOfRelease() bool {
	return false
}

func (r *LocalReleaseNamespace) ShouldHaveServiceMetadata() bool {
	return false
}
