//go:build ai_tests

package spec_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/v2/pkg/resource/spec"
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

func varsMeta(filePath string) *spec.ResourceMeta {
	return metaFor("Deployment", "apps", "v1", "web", "", filePath, nil, nil)
}

func varsObj() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "web", "labels": map[string]interface{}{}},
		"spec":       map[string]interface{}{"replicas": int64(1)},
	}}
}

// varsRenderContext mirrors a two-chart tree: the parent myapp with the subchart
// cache, each with its own values section and metadata.
func varsRenderContext() spec.RenderContext {
	return spec.RenderContext{
		Values: map[string]interface{}{
			"replicaCount": int64(7),
			"cache":        map[string]interface{}{"replicaCount": int64(9)},
		},
		Release:      map[string]interface{}{"Name": "myrel"},
		Chart:        map[string]interface{}{"Name": "myapp", "Version": "1.2.3"},
		Capabilities: map[string]interface{}{"KubeVersion": map[string]interface{}{"Version": "v1.30.0"}},
		Subcharts: map[string]spec.SubchartContext{
			"myapp/charts/cache": {
				Values: map[string]interface{}{"replicaCount": int64(9)},
				Chart:  map[string]interface{}{"Name": "cache", "Version": "4.5.6"},
			},
		},
	}
}
