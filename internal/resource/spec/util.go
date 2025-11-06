package spec

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/pkg/common"
)

func GVRtoGVK(gvr schema.GroupVersionResource, restMapper meta.RESTMapper) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind
	if preferredKinds, err := restMapper.KindsFor(gvr); err != nil {
		return gvk, fmt.Errorf("match GroupVersionResource %q: %w", gvr.String(), err)
	} else if len(preferredKinds) == 0 {
		return gvk, fmt.Errorf("no matches for %q", gvr.String())
	} else {
		gvk = preferredKinds[0]
	}

	return gvk, nil
}

func GVKtoGVR(gvk schema.GroupVersionKind, mapper meta.RESTMapper) (gvr schema.GroupVersionResource, namespaced bool, err error) {
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, false, fmt.Errorf("get resource mapping for %q: %w", gvk.String(), err)
	}

	return mapping.Resource, mapping.Scope == meta.RESTScopeNamespace, nil
}

func Namespaced(gvk schema.GroupVersionKind, mapper meta.RESTMapper) (bool, error) {
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, fmt.Errorf("get resource mapping for %q: %w", gvk.String(), err)
	}

	return mapping.Scope == meta.RESTScopeNamespace, nil
}

func ParseKubectlResourceStringtoGVK(resource string, restMapper meta.RESTMapper) (schema.GroupVersionKind, error) {
	var gvk schema.GroupVersionKind

	gvr := ParseKubectlResourceStringToGVR(resource)

	gvk, err := GVRtoGVK(gvr, restMapper)
	if err != nil {
		return gvk, fmt.Errorf("convert GroupVersionResource to GroupVersionKind: %w", err)
	}

	return gvk, nil
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

func IsCRD(groupKind schema.GroupKind) bool {
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

func IsHook(annotations map[string]string) bool {
	_, _, found := FindAnnotationOrLabelByKeyPattern(annotations, common.AnnotationKeyPatternHook)
	return found
}

func IsWebhook(groupKind schema.GroupKind) bool {
	return groupKind == schema.GroupKind{
		Group: "admissionregistration.k8s.io",
		Kind:  "MutatingWebhookConfiguration",
	} || groupKind == schema.GroupKind{
		Group: "admissionregistration.k8s.io",
		Kind:  "ValidatingWebhookConfiguration",
	}
}

func IsReleaseNamespace(resourceName string, resourceGVK schema.GroupVersionKind, releaseNamespace string) bool {
	return resourceGVK.Group == "" && resourceGVK.Kind == "Namespace" && resourceName == releaseNamespace
}

func FindAnnotationOrLabelByKeyPattern(annotationsOrLabels map[string]string, pattern *regexp.Regexp) (key, value string, found bool) {
	key, found = lo.FindKeyBy(annotationsOrLabels, func(k, _ string) bool {
		return pattern.MatchString(k)
	})
	if found {
		value = strings.TrimSpace(annotationsOrLabels[key])
	}

	return key, value, found
}

func FindAnnotationsOrLabelsByKeyPattern(annotationsOrLabels map[string]string, pattern *regexp.Regexp) (result map[string]string, found bool) {
	result = map[string]string{}

	for key, value := range annotationsOrLabels {
		if pattern.MatchString(key) {
			result[key] = strings.TrimSpace(value)
		}
	}

	return result, len(result) > 0
}

func setAnnotationsAndLabels(res *unstructured.Unstructured, annotations, labels map[string]string) {
	if len(annotations) > 0 {
		annos := res.GetAnnotations()
		if annos == nil {
			annos = map[string]string{}
		}

		for k, v := range annotations {
			annos[k] = v
		}

		res.SetAnnotations(annos)
	}

	if len(labels) > 0 {
		lbls := res.GetLabels()
		if lbls == nil {
			lbls = map[string]string{}
		}

		for k, v := range labels {
			lbls[k] = v
		}

		res.SetLabels(lbls)
	}
}
