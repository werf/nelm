package resourcev2

import (
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/werf/resourcev2/resourceparts"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

func NewLocalHookResource(unstruct *unstructured.Unstructured, filePath string, opts NewLocalHookResourceOptions) *LocalHookResource {
	return &LocalHookResource{
		LocalBaseResource:            resourceparts.NewLocalBaseResource(unstruct, filePath, resourceparts.NewLocalBaseResourceOptions{Mapper: opts.Mapper}),
		HookableResource:             resourceparts.NewHookableResource(unstruct),
		RecreatableResource:          resourceparts.NewRecreatableResource(unstruct),
		AutoDeletableResource:        resourceparts.NewAutoDeletableResource(unstruct),
		NeverDeletableResource:       resourceparts.NewNeverDeletableResource(unstruct),
		WeighableResource:            resourceparts.NewWeighableResource(unstruct),
		TrackableResource:            resourceparts.NewTrackableResource(resourceparts.NewTrackableResourceOptions{Unstructured: unstruct}),
		ExternallyDependableResource: resourceparts.NewExternallyDependableResource(unstruct, filePath, resourceparts.NewExternallyDependableResourceOptions{Mapper: opts.Mapper, DiscoveryClient: opts.DiscoveryClient}),
	}
}

type NewLocalHookResourceOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type LocalHookResource struct {
	*resourceparts.LocalBaseResource
	*resourceparts.HookableResource
	*resourceparts.RecreatableResource
	*resourceparts.AutoDeletableResource
	*resourceparts.NeverDeletableResource
	*resourceparts.WeighableResource
	*resourceparts.TrackableResource
	*resourceparts.ExternallyDependableResource
}

func (r *LocalHookResource) Validate() error {
	if err := r.LocalBaseResource.Validate(); err != nil {
		return err
	}

	if err := r.HookableResource.Validate(); err != nil {
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
