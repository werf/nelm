package resource

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	HideAll = "$$HIDE_ALL$$"
)

type SensitiveInfo struct {
	IsSensitive    bool
	SensitivePaths []string
}

func IsSensitive(groupKind schema.GroupKind, annotations map[string]string) bool {
	info := GetSensitiveInfo(groupKind, annotations)
	return info.IsSensitive
}

func GetSensitiveInfo(groupKind schema.GroupKind, annotations map[string]string) SensitiveInfo {
	if _, value, found := FindAnnotationOrLabelByKeyPattern(annotations, annotationKeyPatternSensitive); found {
		sensitive := lo.Must(strconv.ParseBool(value))
		if sensitive {
			return SensitiveInfo{IsSensitive: true, SensitivePaths: []string{HideAll}}
		}
	}

	if _, value, found := FindAnnotationOrLabelByKeyPattern(annotations, annotationKeyPatternSensitivePaths); found {
		var paths []string
		if err := json.Unmarshal([]byte(value), &paths); err == nil && len(paths) > 0 {
			return SensitiveInfo{IsSensitive: true, SensitivePaths: paths}
		}
	}

	if groupKind == (schema.GroupKind{Group: "", Kind: "Secret"}) {
		return SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"data.*"}}
	}

	return SensitiveInfo{IsSensitive: false, SensitivePaths: nil}
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

func RedactSensitiveData(unstruct *unstructured.Unstructured, sensitivePaths []string) *unstructured.Unstructured {
	if len(sensitivePaths) == 0 {
		return unstruct
	}

	copy := unstruct.DeepCopy()

	for _, path := range sensitivePaths {
		if path == HideAll {
			return &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": copy.GetAPIVersion(),
				"kind":       copy.GetKind(),
				"metadata": map[string]interface{}{
					"name":      copy.GetName(),
					"namespace": copy.GetNamespace(),
				},
			}}
		}

		redactAtPath(copy.Object, strings.Split(path, "."))
	}

	return copy
}

func redactAtPath(obj map[string]interface{}, pathParts []string) {
	if len(pathParts) == 0 {
		return
	}

	key := pathParts[0]
	if len(pathParts) == 1 {
		if key == "*" {
			for k, v := range obj {
				if val, ok := v.(string); ok {
					obj[k] = fmt.Sprintf("REDACTED (len %d bytes)", len(val))
				} else if val, ok := v.([]byte); ok {
					obj[k] = fmt.Sprintf("REDACTED (len %d bytes)", len(val))
				} else {
					obj[k] = "REDACTED"
				}
			}
		} else if val, exists := obj[key]; exists {
			if strVal, ok := val.(string); ok {
				obj[key] = fmt.Sprintf("REDACTED (len %d bytes)", len(strVal))
			} else if byteVal, ok := val.([]byte); ok {
				obj[key] = fmt.Sprintf("REDACTED (len %d bytes)", len(byteVal))
			} else {
				obj[key] = "REDACTED"
			}
		}
		return
	}

	if key == "*" {
		for _, v := range obj {
			if nestedObj, ok := v.(map[string]interface{}); ok {
				redactAtPath(nestedObj, pathParts[1:])
			}
		}
	} else if val, exists := obj[key]; exists {
		if nestedObj, ok := val.(map[string]interface{}); ok {
			redactAtPath(nestedObj, pathParts[1:])
		} else if slice, ok := val.([]interface{}); ok {
			redactAtPathInSlice(slice, pathParts[1:])
		}
	}
}

func redactAtPathInSlice(slice []interface{}, pathParts []string) {
	if len(pathParts) == 0 {
		return
	}

	key := pathParts[0]

	// Handle numeric indices
	if idx, err := strconv.Atoi(key); err == nil {
		if idx >= 0 && idx < len(slice) {
			if len(pathParts) == 1 {
				// Redact the element at this index
				if val, ok := slice[idx].(string); ok {
					slice[idx] = fmt.Sprintf("REDACTED (len %d bytes)", len(val))
				} else if val, ok := slice[idx].([]byte); ok {
					slice[idx] = fmt.Sprintf("REDACTED (len %d bytes)", len(val))
				} else {
					slice[idx] = "REDACTED"
				}
			} else {
				// Continue traversing deeper
				if nestedObj, ok := slice[idx].(map[string]interface{}); ok {
					redactAtPath(nestedObj, pathParts[1:])
				} else if nestedSlice, ok := slice[idx].([]interface{}); ok {
					redactAtPathInSlice(nestedSlice, pathParts[1:])
				}
			}
		}
	} else if key == "*" {
		// Handle wildcard for all elements in slice
		for i := range slice {
			if len(pathParts) == 1 {
				// Redact all elements
				if val, ok := slice[i].(string); ok {
					slice[i] = fmt.Sprintf("REDACTED (len %d bytes)", len(val))
				} else if val, ok := slice[i].([]byte); ok {
					slice[i] = fmt.Sprintf("REDACTED (len %d bytes)", len(val))
				} else {
					slice[i] = "REDACTED"
				}
			} else {
				// Continue traversing deeper for all elements
				if nestedObj, ok := slice[i].(map[string]interface{}); ok {
					redactAtPath(nestedObj, pathParts[1:])
				} else if nestedSlice, ok := slice[i].([]interface{}); ok {
					redactAtPathInSlice(nestedSlice, pathParts[1:])
				}
			}
		}
	}
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
