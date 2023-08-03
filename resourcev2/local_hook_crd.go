package resourcev2

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

func NewLocalHookCRD(unstruct *unstructured.Unstructured, filePath string, opts NewLocalHookCRDOptions) *LocalHookCRD {
	return &LocalHookCRD{
		localBaseResource:            newLocalBaseResource(unstruct, filePath, newLocalBaseResourceOptions{Mapper: opts.Mapper}),
		hookableResource:             newHookableResource(unstruct),
		recreatableResource:          newRecreatableResource(unstruct),
		autoDeletableResource:        newAutoDeletableResource(unstruct),
		neverDeletableResource:       newNeverDeletableResource(unstruct),
		weighableResource:            newWeighableResource(unstruct),
		trackableResource:            newTrackableResource(unstruct),
		externallyDependableResource: newExternallyDependableResource(unstruct, filePath, newExternallyDependableResourceOptions{Mapper: opts.Mapper, DiscoveryClient: opts.DiscoveryClient}),
	}
}

type NewLocalHookCRDOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type LocalHookCRD struct {
	*localBaseResource
	*hookableResource
	*recreatableResource
	*autoDeletableResource
	*neverDeletableResource
	*weighableResource
	*trackableResource
	*externallyDependableResource
}

func (r *LocalHookCRD) Validate() error {
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

func (r *LocalHookCRD) PartOfRelease() bool {
	return false
}

func (r *LocalHookCRD) ShouldHaveServiceMetadata() bool {
	return true
}

func BuildLocalHookCRDsFromManifests(manifests []string, opts BuildLocalHookCRDsFromManifestsOptions) ([]*LocalHookCRD, error) {
	var localHookCRDs []*LocalHookCRD
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

		if !IsCRD(unstructObj) {
			continue
		}

		crd := NewLocalHookCRD(unstructObj, path, NewLocalHookCRDOptions{
			Mapper:          opts.Mapper,
			DiscoveryClient: opts.DiscoveryClient,
		})
		localHookCRDs = append(localHookCRDs, crd)
	}

	return localHookCRDs, nil
}

type BuildLocalHookCRDsFromManifestsOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}
