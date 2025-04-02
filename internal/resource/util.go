package resource

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func IsSensitive(groupKind schema.GroupKind, annotations map[string]string) bool {
	if groupKind == (schema.GroupKind{Group: "", Kind: "Secret"}) {
		return true
	}

	if _, value, found := FindAnnotationOrLabelByKeyPattern(annotations, annotationKeyPatternSensitive); found {
		sensitive := lo.Must(strconv.ParseBool(value))

		if sensitive {
			return true
		}
	}

	return false
}

func IsHook(annotations map[string]string) bool {
	_, _, found := FindAnnotationOrLabelByKeyPattern(annotations, annotationKeyPatternHook)
	return found
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
