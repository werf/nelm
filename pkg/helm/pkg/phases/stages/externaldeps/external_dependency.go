package externaldeps

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"
)

func NewExternalDependency(name, resourceType, resourceName string) *ExternalDependency {
	return &ExternalDependency{
		Name:         name,
		ResourceType: resourceType,
		ResourceName: resourceName,
	}
}

type ExternalDependency struct {
	Name         string
	ResourceType string
	ResourceName string

	Namespace string
	Info      *resource.Info
}

func (d *ExternalDependency) GenerateInfo(gvkBuilder GVKBuilder, metaAccessor meta.MetadataAccessor, mapper meta.RESTMapper) error {
	gvk, err := gvkBuilder.BuildFromResource(d.ResourceType)
	if err != nil {
		return fmt.Errorf("error building GroupVersionKind from resource type: %w", err)
	}

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("error getting resource mapping: %w", err)
	}

	object := unstructured.Unstructured{}
	object.SetGroupVersionKind(*gvk)
	object.SetName(d.ResourceName)

	d.Info = &resource.Info{
		Mapping: mapping,
		Object:  &object,
		Name:    d.ResourceName,
	}

	if d.Info.Namespaced() {
		d.Info.Namespace = d.Namespace
		d.Info.Object.(*unstructured.Unstructured).SetNamespace(d.Namespace)
	}

	return nil
}
