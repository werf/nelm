//go:build ai_tests

package spec_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestAI_CompilePatches_RejectsRenderContextVariables(t *testing.T) {
	for _, variable := range []string{"$values", "$release", "$chart", "$capabilities"} {
		t.Run(variable, func(t *testing.T) {
			_, err := spec.CompilePatches([]spec.Patch{{Patch: `.spec.replicas = ` + variable + `.x`}})
			require.ErrorContains(t, err, "variable not defined: "+variable)
		})
	}
}

func TestAI_CompileRenderPatches_ExposesRenderContextVariables(t *testing.T) {
	renderContext := spec.RenderContext{
		Values:       map[string]interface{}{"replicaCount": int64(7)},
		Release:      map[string]interface{}{"Name": "myrel"},
		Chart:        map[string]interface{}{"Name": "myapp", "Version": "1.2.3"},
		Capabilities: map[string]interface{}{"KubeVersion": map[string]interface{}{"Version": "v1.30.0"}},
	}

	patches, err := spec.CompileRenderPatches([]spec.Patch{{Patch: `
		.spec.replicas = $values.replicaCount
		| .metadata.labels.release = $release.Name
		| .metadata.labels.chart = $chart.Name + "-" + $chart.Version
		| .metadata.labels.kube = $capabilities.KubeVersion.Version
	`}}, renderContext)
	require.NoError(t, err)

	out, err := spec.ApplyPatches(context.Background(), patches, varsMeta(), "prod", varsObj())
	require.NoError(t, err)

	replicas, found, err := unstructured.NestedInt64(out.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(7), replicas)

	labels, _, err := unstructured.NestedStringMap(out.Object, "metadata", "labels")
	require.NoError(t, err)
	require.Equal(t, "myrel", labels["release"])
	require.Equal(t, "myapp-1.2.3", labels["chart"])
	require.Equal(t, "v1.30.0", labels["kube"])
}

func TestAI_CompileRenderPatches_NilRenderContextValuesAreNull(t *testing.T) {
	patches, err := spec.CompileRenderPatches([]spec.Patch{{
		Patch: `.metadata.labels.missing = ($values.nope // "fallback")`,
	}}, spec.RenderContext{})
	require.NoError(t, err)

	out, err := spec.ApplyPatches(context.Background(), patches, varsMeta(), "prod", varsObj())
	require.NoError(t, err)

	labels, _, err := unstructured.NestedStringMap(out.Object, "metadata", "labels")
	require.NoError(t, err)
	require.Equal(t, "fallback", labels["missing"])
}

func TestAI_CompileRenderPatches_UnusedVariablesDoNotBreakPatch(t *testing.T) {
	patches, err := spec.CompileRenderPatches([]spec.Patch{{Patch: `del(.spec.replicas)`}}, spec.RenderContext{})
	require.NoError(t, err)

	out, err := spec.ApplyPatches(context.Background(), patches, varsMeta(), "prod", varsObj())
	require.NoError(t, err)

	_, found, err := unstructured.NestedInt64(out.Object, "spec", "replicas")
	require.NoError(t, err)
	require.False(t, found)
}

func varsMeta() *spec.ResourceMeta {
	return metaFor("Deployment", "apps", "v1", "web", "", "myapp/templates/web.yaml", nil, nil)
}

func varsObj() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "web", "labels": map[string]interface{}{}},
		"spec":       map[string]interface{}{"replicas": int64(1)},
	}}
}
