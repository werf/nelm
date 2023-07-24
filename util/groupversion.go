package util

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// FIXME(ilya-lesikov): this done server-side
func ParseResourceStringtoGVK(groupVersionResource string, restMapper meta.RESTMapper, discClient discovery.CachedDiscoveryInterface) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind

	gvr := ParseResourceStringToGVR(groupVersionResource)

	gvk, err := ConvertGVRtoGVK(gvr, restMapper)
	if err != nil {
		return gvk, fmt.Errorf("error converting group/version/resource to group/version/kind: %w", err)
	}

	return gvk, nil
}

// FIXME(ilya-lesikov): this done server-side
func ConvertGVRtoGVK(gvr schema.GroupVersionResource, restMapper meta.RESTMapper) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	if preferredKinds, err := restMapper.KindsFor(gvr); err != nil {
		return gvk, fmt.Errorf("error matching a group/version/resource: %w", err)
	} else if len(preferredKinds) == 0 {
		return gvk, fmt.Errorf("no matches for group/version/resource")
	} else {
		gvk = preferredKinds[0]
	}

	return gvk, nil
}

func ParseResourceStringToGVR(groupVersionResource string) schema.GroupVersionResource {
	var result schema.GroupVersionResource
	if gvr, gr := schema.ParseResourceArg(groupVersionResource); gvr != nil {
		result = *gvr
	} else {
		result = gr.WithVersion("")
	}

	return result
}

func ConvertGVKtoGVR(gvk schema.GroupVersionKind, mapper meta.RESTMapper) (gvr schema.GroupVersionResource, namespaced bool, err error) {
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("error getting resource mapping for %q: %w", gvk, err)
	}

	return mapping.Resource, mapping.Scope == meta.RESTScopeNamespace, nil
}

func IsResourceNamespaced(gvk schema.GroupVersionKind, mapper meta.RESTMapper) (bool, error) {
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, fmt.Errorf("error getting resource mapping for %q: %w", gvk, err)
	}

	return mapping.Scope == meta.RESTScopeNamespace, nil
}
