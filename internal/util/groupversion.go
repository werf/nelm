package util

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

func ParseKubectlResourceStringtoGVK(resource string, restMapper meta.RESTMapper, discClient discovery.CachedDiscoveryInterface) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind

	gvr := ParseKubectlResourceStringToGVR(resource)

	gvk, err := ConvertGVRtoGVK(gvr, restMapper)
	if err != nil {
		return gvk, fmt.Errorf("error converting group/version/resource to group/version/kind: %w", err)
	}

	return gvk, nil
}

func ConvertGVRtoGVK(gvr schema.GroupVersionResource, restMapper meta.RESTMapper) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	if preferredKinds, err := restMapper.KindsFor(gvr); err != nil {
		return gvk, fmt.Errorf("error matching a group/version/resource %q: %w", gvr.String(), err)
	} else if len(preferredKinds) == 0 {
		return gvk, fmt.Errorf("no matches for group/version/resource %q", gvr.String())
	} else {
		gvk = preferredKinds[0]
	}

	return gvk, nil
}

func ConvertGVKtoGVR(gvk schema.GroupVersionKind, mapper meta.RESTMapper) (gvr schema.GroupVersionResource, namespaced bool, err error) {
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("error getting resource mapping for group/version/kind %q: %w", gvk.String(), err)
	}

	return mapping.Resource, mapping.Scope == meta.RESTScopeNamespace, nil
}

func ParseKubectlResourceStringToGVR(resource string) schema.GroupVersionResource {
	var result schema.GroupVersionResource
	if gvr, gr := schema.ParseResourceArg(resource); gvr != nil {
		result = *gvr
	} else {
		result = gr.WithVersion("")
	}

	return result
}

func IsCRDFromGK(groupKind schema.GroupKind) bool {
	return groupKind == schema.GroupKind{
		Group: "apiextensions.k8s.io",
		Kind:  "CustomResourceDefinition",
	}
}

func IsCRDFromGR(groupKind schema.GroupResource) bool {
	return groupKind == schema.GroupResource{
		Group:    "apiextensions.k8s.io",
		Resource: "customresourcedefinitions",
	}
}
