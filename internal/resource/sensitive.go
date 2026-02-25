package resource

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ohler55/ojg/jp"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
)

const HideAll = "$$HIDE_ALL$$"

type SensitiveInfo struct {
	IsSensitive    bool
	SensitivePaths []string
}

func (i *SensitiveInfo) FullySensitive() bool {
	return i.IsSensitive && len(i.SensitivePaths) == 1 && i.SensitivePaths[0] == HideAll
}

func IsSensitive(groupKind schema.GroupKind, annotations map[string]string) bool {
	info := GetSensitiveInfo(groupKind, annotations)

	return info.IsSensitive
}

func GetSensitiveInfo(groupKind schema.GroupKind, annotations map[string]string) SensitiveInfo {
	// Check for werf.io/sensitive-paths (comma-separated)
	if _, value, found := spec.FindAnnotationOrLabelByKeyPattern(annotations, common.AnnotationKeyPatternSensitivePaths); found {
		paths := ParseSensitivePaths(value)
		if len(paths) > 0 {
			return SensitiveInfo{IsSensitive: true, SensitivePaths: paths}
		}
	}

	useNewBehavior := featgate.FeatGateFieldSensitive.Enabled() || featgate.FeatGatePreviewV2.Enabled()

	// Check for werf.io/sensitive annotation
	if _, value, found := spec.FindAnnotationOrLabelByKeyPattern(annotations, common.AnnotationKeyPatternSensitive); found {
		sensitive := lo.Must(strconv.ParseBool(value))
		if sensitive {
			if useNewBehavior {
				// V2 behavior: only hide data.* and stringData.*
				return SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"data.*", "stringData.*"}}
			} else {
				// V1 behavior: hide everything
				return SensitiveInfo{IsSensitive: true, SensitivePaths: []string{HideAll}}
			}
		} else {
			return SensitiveInfo{IsSensitive: false, SensitivePaths: nil}
		}
	}

	// Default behavior for Secrets
	if groupKind == (schema.GroupKind{Group: "", Kind: "Secret"}) {
		if useNewBehavior {
			// V2 behavior: only hide data.* and stringData.*
			return SensitiveInfo{IsSensitive: true, SensitivePaths: []string{"data.*", "stringData.*"}}
		} else {
			// V1 behavior: hide everything
			return SensitiveInfo{IsSensitive: true, SensitivePaths: []string{HideAll}}
		}
	}

	return SensitiveInfo{IsSensitive: false, SensitivePaths: nil}
}

func ParseSensitivePaths(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	var (
		paths   []string
		current strings.Builder
	)

	escaped := false

	for _, r := range value {
		if escaped {
			current.WriteRune(r)

			escaped = false
		} else if r == '\\' {
			escaped = true
		} else if r == ',' {
			if path := strings.TrimSpace(current.String()); path != "" {
				paths = append(paths, path)
			}

			current.Reset()
		} else {
			current.WriteRune(r)
		}
	}

	if path := strings.TrimSpace(current.String()); path != "" {
		paths = append(paths, path)
	}

	return paths
}

func RedactSensitiveData(unstruct *unstructured.Unstructured, sensitivePaths []string) *unstructured.Unstructured {
	if len(sensitivePaths) == 0 {
		return unstruct.DeepCopy()
	}

	return redactSensitiveData(unstruct.DeepCopy(), sensitivePaths)
}

func redactSensitiveData(unstruct *unstructured.Unstructured, sensitivePaths []string) *unstructured.Unstructured {
	for _, pathExpr := range sensitivePaths {
		if pathExpr == HideAll {
			return &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": unstruct.GetAPIVersion(),
				"kind":       unstruct.GetKind(),
				"metadata": map[string]interface{}{
					"name":      unstruct.GetName(),
					"namespace": unstruct.GetNamespace(),
				},
			}}
		}

		x := lo.Must(jp.ParseString(pathExpr))
		redactAtJSONPath(unstruct.Object, &x)
	}

	return unstruct
}

func redactAtJSONPath(obj map[string]interface{}, jsonPath *jp.Expr) {
	jsonPath.MustModify(obj, func(element interface{}) (interface{}, bool) {
		return createSensitiveReplacement(element), true
	})
}

func createSensitiveReplacement(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(v)))[:12]
		return fmt.Sprintf("<hidden %d sensitive bytes, hash %s>", len(v), hash)
	case []byte:
		hash := fmt.Sprintf("%x", sha256.Sum256(v))[:12]
		return fmt.Sprintf("<hidden %d sensitive bytes, hash %s>", len(v), hash)
	case []interface{}:
		jsonData, _ := json.Marshal(v)
		hash := fmt.Sprintf("%x", sha256.Sum256(jsonData))[:12]

		return fmt.Sprintf("<hidden %d sensitive entries, hash %s>", len(v), hash)
	case map[string]interface{}:
		jsonData, _ := json.Marshal(v)
		hash := fmt.Sprintf("%x", sha256.Sum256(jsonData))[:12]

		return fmt.Sprintf("<hidden %d sensitive entries, hash %s>", len(v), hash)
	default:
		// For other types, convert to string and hash
		str := fmt.Sprintf("%v", v)
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(str)))[:12]

		return fmt.Sprintf("<hidden %d sensitive bytes, hash %s>", len(str), hash)
	}
}
