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

func NewLocalGeneralCRD(unstruct *unstructured.Unstructured, filePath string, opts NewLocalGeneralCRDOptions) *LocalGeneralCRD {
	return &LocalGeneralCRD{
		LocalBaseResource:            resourceparts.NewLocalBaseResource(unstruct, filePath, resourceparts.NewLocalBaseResourceOptions{Mapper: opts.Mapper}),
		RecreatableResource:          resourceparts.NewRecreatableResource(unstruct),
		AutoDeletableResource:        resourceparts.NewAutoDeletableResource(unstruct),
		NeverDeletableResource:       resourceparts.NewNeverDeletableResource(unstruct),
		WeighableResource:            resourceparts.NewWeighableResource(unstruct),
		TrackableResource:            resourceparts.NewTrackableResource(resourceparts.NewTrackableResourceOptions{Unstructured: unstruct}),
		ExternallyDependableResource: resourceparts.NewExternallyDependableResource(unstruct, filePath, resourceparts.NewExternallyDependableResourceOptions{Mapper: opts.Mapper, DiscoveryClient: opts.DiscoveryClient}),
	}
}

type NewLocalGeneralCRDOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}

type LocalGeneralCRD struct {
	*resourceparts.LocalBaseResource
	*resourceparts.RecreatableResource
	*resourceparts.AutoDeletableResource
	*resourceparts.NeverDeletableResource
	*resourceparts.WeighableResource
	*resourceparts.TrackableResource
	*resourceparts.ExternallyDependableResource
}

func (r *LocalGeneralCRD) Validate() error {
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

func BuildLocalGeneralCRDsFromManifests(manifests []string, opts BuildLocalGeneralCRDsFromManifestsOptions) ([]*LocalGeneralCRD, error) {
	var localGeneralCRDs []*LocalGeneralCRD
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
		if !IsCRD(unstructObj) {
			continue
		}

		resource := NewLocalGeneralCRD(unstructObj, path, NewLocalGeneralCRDOptions{
			Mapper:          opts.Mapper,
			DiscoveryClient: opts.DiscoveryClient,
		})
		localGeneralCRDs = append(localGeneralCRDs, resource)
	}

	return localGeneralCRDs, nil
}

type BuildLocalGeneralCRDsFromManifestsOptions struct {
	Mapper          meta.ResettableRESTMapper
	DiscoveryClient discovery.CachedDiscoveryInterface
}
