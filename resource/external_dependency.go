package resource

import (
	"fmt"

	"helm.sh/helm/v3/pkg/werf/util"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
)

func NewExternalDependency(id, resourceType, name, namespace string, restMapper meta.ResettableRESTMapper, discClient discovery.CachedDiscoveryInterface) (*ExternalDependency, error) {
	gvr := util.ParseResourceStringToGVR(resourceType)

	gvk, err := util.ParseResourceStringtoGVK(resourceType, restMapper, discClient)
	if err != nil {
		return nil, fmt.Errorf("error parsing resource type %q: %w", resourceType, err)
	}

	ref := NewResourcedReference(name, namespace, ResourcedReferenceOptions{
		GroupVersionKind:     gvk,
		GroupVersionResource: gvr,
	})

	return &ExternalDependency{
		ResourcedReference: ref,
		id:                 id,
	}, nil
}

func NewLocalExternalDependency(id, resourceType, name, namespace string) *ExternalDependency {
	gvr := util.ParseResourceStringToGVR(resourceType)

	ref := NewResourcedReference(name, namespace, ResourcedReferenceOptions{
		GroupVersionResource: gvr,
	})

	return &ExternalDependency{
		ResourcedReference: ref,
		id:                 id,
	}
}

type ExternalDependency struct {
	*ResourcedReference

	id string
}

func (d *ExternalDependency) ID() string {
	return d.id
}
