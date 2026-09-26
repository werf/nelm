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
	for _, variable := range []string{"$Values", "$Release", "$Chart", "$Capabilities"} {
		t.Run(variable, func(t *testing.T) {
			_, err := spec.CompilePatches([]spec.Patch{{Patch: `.spec.replicas = ` + variable + `.x`}})
			require.ErrorContains(t, err, "only available in renderPatches")
			require.ErrorContains(t, err, variable)
		})
	}
}

func TestAI_CompileRenderPatches_ExposesRenderContextVariables(t *testing.T) {
	patches, err := spec.CompileRenderPatches([]spec.Patch{{Patch: `
		.spec.replicas = $Values.replicaCount
		| .metadata.labels.release = $Release.Name
		| .metadata.labels.chart = $Chart.Name + "-" + $Chart.Version
		| .metadata.labels.kube = $Capabilities.KubeVersion.Version
	`}}, varsRenderContext())
	require.NoError(t, err)

	out, err := spec.ApplyPatches(context.Background(), patches, varsMeta("myapp/templates/web.yaml"), "prod", varsObj())
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
		Patch: `.metadata.labels.missing = ($Values.nope // "fallback")`,
	}}, spec.RenderContext{})
	require.NoError(t, err)

	out, err := spec.ApplyPatches(context.Background(), patches, varsMeta("myapp/templates/web.yaml"), "prod", varsObj())
	require.NoError(t, err)

	labels, _, err := unstructured.NestedStringMap(out.Object, "metadata", "labels")
	require.NoError(t, err)
	require.Equal(t, "fallback", labels["missing"])
}

func TestAI_CompileRenderPatches_ScopesSubchartRulesToTheirOwnChart(t *testing.T) {
	patches, err := spec.CompileRenderPatches([]spec.Patch{
		spec.NewChartScopedPatch("myapp/charts/cache", `
			.metadata.labels.chart = $Chart.Name + "-" + $Chart.Version
			| .metadata.labels.replicas = ($Values.replicaCount | tostring)
		`),
	}, varsRenderContext())
	require.NoError(t, err)

	out, err := spec.ApplyPatches(context.Background(), patches, varsMeta("myapp/charts/cache/templates/redis.yaml"), "prod", varsObj())
	require.NoError(t, err)

	labels, _, err := unstructured.NestedStringMap(out.Object, "metadata", "labels")
	require.NoError(t, err)
	require.Equal(t, "cache-4.5.6", labels["chart"])
	require.Equal(t, "9", labels["replicas"])
}

func TestAI_CompileRenderPatches_UnusedVariablesDoNotBreakPatch(t *testing.T) {
	patches, err := spec.CompileRenderPatches([]spec.Patch{{Patch: `del(.spec.replicas)`}}, spec.RenderContext{})
	require.NoError(t, err)

	out, err := spec.ApplyPatches(context.Background(), patches, varsMeta("myapp/templates/web.yaml"), "prod", varsObj())
	require.NoError(t, err)

	_, found, err := unstructured.NestedInt64(out.Object, "spec", "replicas")
	require.NoError(t, err)
	require.False(t, found)
}
