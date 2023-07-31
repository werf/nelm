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

func NewLocalStandaloneCRD(unstruct *unstructured.Unstructured, filePath string, opts NewLocalStandaloneCRDOptions) *LocalStandaloneCRD {
	return &LocalStandaloneCRD{
		LocalBaseResource:            resourceparts.NewLocalBaseResource(unstruct, filePath, resourceparts.NewLocalBaseResourceOptions{Mapper: opts.Mapper}),
		NeverDeletableResource:       resourceparts.NewNeverDeletableResource(unstruct),
		TrackableResource:            resourceparts.NewTrackableResource(resourceparts.NewTrackableResourceOptions{Unstructured: unstruct}),
		WeighableResource:            resourceparts.NewWeighableResource(unstruct),
		ExternallyDependableResource: resourceparts.NewExternallyDependableResource(unstruct, filePath, resourceparts.NewExternallyDependableResourceOptions{Mapper: opts.Mapper, DiscoveryClient: opts.DiscoveryClient}),
	}
}

type NewLocalStandaloneCRDOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type LocalStandaloneCRD struct {
	*resourceparts.LocalBaseResource
	*resourceparts.NeverDeletableResource
	*resourceparts.WeighableResource
	*resourceparts.TrackableResource
	*resourceparts.ExternallyDependableResource
}

func (r *LocalStandaloneCRD) Validate() error {
	if err := r.LocalBaseResource.Validate(); err != nil {
		return err
	}

	if err := r.WeighableResource.Validate(); err != nil {
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
