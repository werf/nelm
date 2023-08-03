package resourcev2

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewExternalDependency(name, filePath string, groupVersionKind schema.GroupVersionKind, mapper meta.ResettableRESTMapper, opts NewExternalDependencyOptions) *ExternalDependency {
	apiVersion, kind := groupVersionKind.ToAPIVersionAndKind()

	unstruct := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}

	if opts.Namespace != "" {
		unstruct.SetNamespace(opts.Namespace)
	}

	return &ExternalDependency{
		localBaseResource: newLocalBaseResource(unstruct, filePath, newLocalBaseResourceOptions{Mapper: mapper}),
		trackableResource: newTrackableResource(unstruct),
	}
}

type NewExternalDependencyOptions struct {
	Namespace string
}

type ExternalDependency struct {
	*localBaseResource
	*trackableResource
}

func (r *ExternalDependency) Validate() error {
	if err := r.localBaseResource.Validate(); err != nil {
		return err
	}

	if err := r.trackableResource.Validate(); err != nil {
		return err
	}

	return nil
}
