package resourcev2

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

func NewLocalGeneralResource(unstruct *unstructured.Unstructured, filePath string, opts NewLocalGeneralResourceOptions) *LocalGeneralResource {
	return &LocalGeneralResource{
		localBaseResource:            newLocalBaseResource(unstruct, filePath, newLocalBaseResourceOptions{Mapper: opts.Mapper}),
		helmManageableResource:       newHelmManageableResource(unstruct),
		recreatableResource:          newRecreatableResource(unstruct),
		autoDeletableResource:        newAutoDeletableResource(unstruct),
		neverDeletableResource:       newNeverDeletableResource(unstruct),
		replicableResource:           newReplicableResource(unstruct),
		weighableResource:            newWeighableResource(unstruct),
		trackableResource:            newTrackableResource(unstruct),
		externallyDependableResource: newExternallyDependableResource(unstruct, filePath, newExternallyDependableResourceOptions{Mapper: opts.Mapper, DiscoveryClient: opts.DiscoveryClient}),
	}
}

type NewLocalGeneralResourceOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type LocalGeneralResource struct {
	*localBaseResource
	*helmManageableResource
	*recreatableResource
	*autoDeletableResource
	*neverDeletableResource
	*replicableResource
	*weighableResource
	*trackableResource
	*externallyDependableResource
}

func (r *LocalGeneralResource) Validate() error {
	if err := r.localBaseResource.Validate(); err != nil {
		return err
	}

	if err := r.weighableResource.Validate(); err != nil {
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

func (r *LocalGeneralResource) PartOfRelease() bool {
	return true
}

func (r *LocalGeneralResource) ShouldHaveServiceMetadata() bool {
	return true
}

func BuildLocalGeneralResourcesFromManifests(manifests []string, opts BuildLocalGeneralResourcesFromManifestsOptions) ([]*LocalGeneralResource, error) {
	var localGeneralResources []*LocalGeneralResource
	for _, manifest := range manifests {
		var path string
		if strings.HasPrefix(manifest, "# Source: ") {
			firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
			path = strings.TrimPrefix(firstLine, "# Source: ")
		}

		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return nil, fmt.Errorf("error decoding resource from file %q: %w", path, err)
		}

		unstructObj := obj.(*unstructured.Unstructured)
		if IsCRD(unstructObj) {
			continue
		}

		resource := NewLocalGeneralResource(unstructObj, path, NewLocalGeneralResourceOptions{
			Mapper:          opts.Mapper,
			DiscoveryClient: opts.DiscoveryClient,
		})
		localGeneralResources = append(localGeneralResources, resource)
	}

	return localGeneralResources, nil
}

type BuildLocalGeneralResourcesFromManifestsOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}
