//go:build ai_tests

package spec_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/resource/spec"
)

func renderedSpec(t *testing.T, name, namespace, filePath string, annotations map[string]interface{}) *spec.ResourceSpec {
	t.Helper()

	metadata := map[string]interface{}{"name": name}
	if namespace != "" {
		metadata["namespace"] = namespace
	}

	if annotations != nil {
		metadata["annotations"] = annotations
	}

	unstruct := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   metadata,
		"spec":       map[string]interface{}{"replicas": int64(3)},
	}}

	return spec.NewResourceSpec(unstruct, "prod", spec.ResourceSpecOptions{FilePath: filePath})
}
