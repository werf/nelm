package resourcev2

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
)

func NewLocalStandaloneCRD(unstruct *unstructured.Unstructured, filePath string, opts NewLocalStandaloneCRDOptions) *LocalStandaloneCRD {
	return &LocalStandaloneCRD{
		localBaseResource:            newLocalBaseResource(unstruct, filePath, newLocalBaseResourceOptions{Mapper: opts.Mapper}),
		neverDeletableResource:       newNeverDeletableResource(unstruct),
		trackableResource:            newTrackableResource(unstruct),
		weighableResource:            newWeighableResource(unstruct),
		externallyDependableResource: newExternallyDependableResource(unstruct, filePath, newExternallyDependableResourceOptions{Mapper: opts.Mapper, DiscoveryClient: opts.DiscoveryClient}),
	}
}

type NewLocalStandaloneCRDOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type LocalStandaloneCRD struct {
	*localBaseResource
	*neverDeletableResource
	*weighableResource
	*trackableResource
	*externallyDependableResource
}

func (r *LocalStandaloneCRD) Validate() error {
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

func (r *LocalStandaloneCRD) PartOfRelease() bool {
	return false
}

func (r *LocalStandaloneCRD) ShouldHaveServiceMetadata() bool {
	return false
}

func BuildLocalStandaloneCRDsFromManifests(manifests []string, opts BuildLocalStandaloneCRDsFromManifestsOptions) ([]*LocalStandaloneCRD, error) {
	var localStandaloneCRDs []*LocalStandaloneCRD
	for _, manifest := range manifests {
		var path string
		if strings.HasPrefix(manifest, "# Source: ") {
			firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
			path = strings.TrimPrefix(firstLine, "# Source: ")
		}

		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return nil, fmt.Errorf("error decoding CRD from file %q: %w", path, err)
		}

		unstructObj := obj.(*unstructured.Unstructured)

		if !IsCRD(unstructObj) {
			continue
		}

		crd := NewLocalStandaloneCRD(unstructObj, path, NewLocalStandaloneCRDOptions{
			Mapper:          opts.Mapper,
			DiscoveryClient: opts.DiscoveryClient,
		})

		localStandaloneCRDs = append(localStandaloneCRDs, crd)
	}

	return localStandaloneCRDs, nil
}

type BuildLocalStandaloneCRDsFromManifestsOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}
