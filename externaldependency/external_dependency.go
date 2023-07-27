package externaldependency

import (
	"helm.sh/helm/v3/pkg/werf/resourcev2/resourceparts"
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
		LocalBaseResource: resourceparts.NewLocalBaseResource(unstruct, filePath, resourceparts.NewLocalBaseResourceOptions{Mapper: mapper}),
		TrackableResource: resourceparts.NewTrackableResource(resourceparts.NewTrackableResourceOptions{Unstructured: unstruct}),
	}
}

type NewExternalDependencyOptions struct {
	Namespace string
}

type ExternalDependency struct {
	*resourceparts.LocalBaseResource
	*resourceparts.TrackableResource
}

func (r *ExternalDependency) Validate() error {
	if err := r.LocalBaseResource.Validate(); err != nil {
		return err
	}

	if err := r.TrackableResource.Validate(); err != nil {
		return err
	}

	return nil
}
