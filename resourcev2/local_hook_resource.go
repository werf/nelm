package resourcev2

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

func NewLocalHookResource(unstruct *unstructured.Unstructured, filePath string, opts NewLocalHookResourceOptions) *LocalHookResource {
	return &LocalHookResource{
		localBaseResource:            newLocalBaseResource(unstruct, filePath, newLocalBaseResourceOptions{Mapper: opts.Mapper}),
		hookableResource:             newHookableResource(unstruct),
		recreatableResource:          newRecreatableResource(unstruct),
		autoDeletableResource:        newAutoDeletableResource(unstruct),
		neverDeletableResource:       newNeverDeletableResource(unstruct),
		replicableResource:           newReplicableResource(unstruct),
		weighableResource:            newWeighableResource(unstruct),
		trackableResource:            newTrackableResource(unstruct),
		externallyDependableResource: newExternallyDependableResource(unstruct, filePath, newExternallyDependableResourceOptions{Mapper: opts.Mapper, DiscoveryClient: opts.DiscoveryClient}),
	}
}

type NewLocalHookResourceOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type LocalHookResource struct {
	*localBaseResource
	*hookableResource
	*recreatableResource
	*autoDeletableResource
	*neverDeletableResource
	*replicableResource
	*weighableResource
	*trackableResource
	*externallyDependableResource
}

func (r *LocalHookResource) Validate() error {
	if err := r.localBaseResource.Validate(); err != nil {
		return err
	}

	if err := r.hookableResource.Validate(); err != nil {
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

func (r *LocalHookResource) PartOfRelease() bool {
	return false
}

func (r *LocalHookResource) ShouldHaveServiceMetadata() bool {
	return true
}

func BuildLocalHookResourcesFromManifests(manifests []string, opts BuildLocalHookResourcesFromManifestsOptions) ([]*LocalHookResource, error) {
	var localHookResources []*LocalHookResource
	for _, manifest := range manifests {
		var path string
		if strings.HasPrefix(manifest, "# Source: ") {
			firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
			path = strings.TrimPrefix(firstLine, "# Source: ")
		}

		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return nil, fmt.Errorf("error decoding hook from file %q: %w", path, err)
		}

		unstructObj := obj.(*unstructured.Unstructured)

		if IsCRD(unstructObj) {
			continue
		}

		resource := NewLocalHookResource(unstructObj, path, NewLocalHookResourceOptions{
			Mapper:          opts.Mapper,
			DiscoveryClient: opts.DiscoveryClient,
		})
		localHookResources = append(localHookResources, resource)
	}

	return localHookResources, nil
}

type BuildLocalHookResourcesFromManifestsOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}
