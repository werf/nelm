package resourcev2

import (
	"helm.sh/helm/v3/pkg/werf/resourcev2/resourceparts"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
)

func NewLocalReleaseNamespace(unstruct *unstructured.Unstructured, opts NewLocalReleaseNamespaceOptions) *LocalReleaseNamespace {
	return &LocalReleaseNamespace{
		LocalBaseResource:            resourceparts.NewLocalBaseResource(unstruct, opts.FilePath, resourceparts.NewLocalBaseResourceOptions{Mapper: opts.Mapper}),
		RecreatableResource:          resourceparts.NewRecreatableResource(unstruct),
		NeverDeletableResource:       resourceparts.NewNeverDeletableResource(unstruct),
		TrackableResource:            resourceparts.NewTrackableResource(resourceparts.NewTrackableResourceOptions{Unstructured: unstruct}),
		ExternallyDependableResource: resourceparts.NewExternallyDependableResource(unstruct, "", resourceparts.NewExternallyDependableResourceOptions{Mapper: opts.Mapper, DiscoveryClient: opts.DiscoveryClient}),
	}
}

type NewLocalReleaseNamespaceOptions struct {
	FilePath        string
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type LocalReleaseNamespace struct {
	*resourceparts.LocalBaseResource
	*resourceparts.RecreatableResource
	*resourceparts.NeverDeletableResource
	*resourceparts.TrackableResource
	*resourceparts.ExternallyDependableResource
}

func (r *LocalReleaseNamespace) Validate() error {
	if err := r.LocalBaseResource.Validate(); err != nil {
		return err
	}

	if err := r.TrackableResource.Validate(); err != nil {
		return err
	}

	if err := r.ExternallyDependableResource.Validate(); err != nil {
		return err
	}

	return nil
}

func (r *LocalReleaseNamespace) Scope() resourceparts.ResourceScope {
	return resourceparts.ResourceScopeCluster
}
