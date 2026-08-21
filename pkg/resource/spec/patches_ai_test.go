//go:build ai_tests

package spec_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/resource/spec"
)

func TestAI_BuildRenderPatchedResourceSpecs_AllowsExplicitReleaseNamespace(t *testing.T) {
	res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

	patches, err := spec.CompilePatches([]spec.Patch{{Patch: `.metadata.namespace = "prod"`}})
	require.NoError(t, err)

	out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{res}, patches)
	require.NoError(t, err)
	require.Empty(t, out[0].Unstruct.GetNamespace())
}

func TestAI_BuildRenderPatchedResourceSpecs_ChainsRules(t *testing.T) {
	res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

	patches, err := spec.CompilePatches([]spec.Patch{
		{Patch: `.spec.replicas = 5`},
		{Patch: `.spec.replicas += 1`},
	})
	require.NoError(t, err)

	out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{res}, patches)
	require.NoError(t, err)

	replicas, _, err := unstructured.NestedInt64(out[0].Unstruct.Object, "spec", "replicas")
	require.NoError(t, err)
	require.Equal(t, int64(6), replicas)
}

func TestAI_BuildRenderPatchedResourceSpecs_ChainsRulesByMetadata(t *testing.T) {
	res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

	patches, err := spec.CompilePatches([]spec.Patch{
		{Patch: `.metadata.labels = {"tier": "backend"}`},
		{Match: spec.ResourceMatcher{Labels: map[string]string{"tier": "backend"}}, Patch: `.spec.replicas = 7`},
	})
	require.NoError(t, err)

	out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{res}, patches)
	require.NoError(t, err)

	replicas, _, err := unstructured.NestedInt64(out[0].Unstruct.Object, "spec", "replicas")
	require.NoError(t, err)
	require.Equal(t, int64(7), replicas)
}

func TestAI_BuildRenderPatchedResourceSpecs_ChartScope(t *testing.T) {
	inScope := renderedSpec(t, "cached", "", "myapp/charts/cache/templates/web.yaml", nil)
	outOfScope := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

	patches, err := spec.CompilePatches([]spec.Patch{{
		ChartScope: "myapp/charts/cache",
		Patch:      `del(.spec.replicas)`,
	}})
	require.NoError(t, err)

	out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{inScope, outOfScope}, patches)
	require.NoError(t, err)

	_, found, err := unstructured.NestedInt64(out[0].Unstruct.Object, "spec", "replicas")
	require.NoError(t, err)
	require.False(t, found)

	_, found, err = unstructured.NestedInt64(out[1].Unstruct.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found)
}

func TestAI_BuildRenderPatchedResourceSpecs_ContractViolations(t *testing.T) {
	tests := []struct {
		name    string
		program string
	}{
		{name: "no output", program: `empty`},
		{name: "multiple outputs", program: `., .`},
		{name: "not an object", program: `.metadata.name`},
		{name: "an object that is not a resource", program: `.spec`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

			patches, err := spec.CompilePatches([]spec.Patch{{Patch: tt.program}})
			require.NoError(t, err)

			_, err = spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{res}, patches)
			require.ErrorContains(t, err, "apply render patches to resource")
		})
	}
}

func TestAI_BuildRenderPatchedResourceSpecs_NamespaceSelector(t *testing.T) {
	// A resource without an explicit namespace ends up in the release namespace, so it is
	// matched by a selector naming the release namespace.
	releaseNamespaced := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)
	otherNamespaced := renderedSpec(t, "web", "other", "myapp/templates/other.yaml", nil)

	patches, err := spec.CompilePatches([]spec.Patch{{
		Match: spec.ResourceMatcher{Namespaces: []string{"prod"}},
		Patch: `del(.spec.replicas)`,
	}})
	require.NoError(t, err)

	out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{releaseNamespaced, otherNamespaced}, patches)
	require.NoError(t, err)

	_, found, err := unstructured.NestedInt64(out[0].Unstruct.Object, "spec", "replicas")
	require.NoError(t, err)
	require.False(t, found)

	_, found, err = unstructured.NestedInt64(out[1].Unstruct.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found)
}

func TestAI_BuildRenderPatchedResourceSpecs_NoPatches(t *testing.T) {
	res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

	out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{res}, nil)
	require.NoError(t, err)
	require.Equal(t, []*spec.ResourceSpec{res}, out)
}

func TestAI_BuildRenderPatchedResourceSpecs_PatchesMatchedOnly(t *testing.T) {
	deployment := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)
	secret := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": "creds"},
		"data":       map[string]interface{}{"key": "dmFsdWU="},
	}}
	secretSpec := spec.NewResourceSpec(secret, "prod", spec.ResourceSpecOptions{FilePath: "myapp/templates/creds.yaml"})

	patches, err := spec.CompilePatches([]spec.Patch{{
		Match: spec.ResourceMatcher{Kinds: []string{"Deployment"}},
		Patch: `.spec.replicas = 5`,
	}})
	require.NoError(t, err)

	out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{deployment, secretSpec}, patches)
	require.NoError(t, err)
	require.Len(t, out, 2)

	replicas, found, err := unstructured.NestedInt64(out[0].Unstruct.Object, "spec", "replicas")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(5), replicas)

	require.Equal(t, secret.Object, out[1].Unstruct.Object)

	// The input specs are never mutated.
	inputReplicas, _, err := unstructured.NestedInt64(deployment.Unstruct.Object, "spec", "replicas")
	require.NoError(t, err)
	require.Equal(t, int64(3), inputReplicas)
}

func TestAI_BuildRenderPatchedResourceSpecs_PreservesStoreAsNone(t *testing.T) {
	res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)
	crd := spec.NewResourceSpec(res.Unstruct, "prod", spec.ResourceSpecOptions{
		FilePath: "myapp/crds/foo.yaml",
		StoreAs:  common.StoreAsNone,
	})

	patches, err := spec.CompilePatches([]spec.Patch{
		{Match: spec.ResourceMatcher{Names: []string{"nonexistent"}}, Patch: `.spec.replicas = 5`},
		{Patch: `.metadata.annotations["helm.sh/hook"] = "post-install"`},
	})
	require.NoError(t, err)

	out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{crd}, patches)
	require.NoError(t, err)
	require.Equal(t, common.StoreAsNone, out[0].StoreAs)
}

func TestAI_BuildRenderPatchedResourceSpecs_RederivesStoreAs(t *testing.T) {
	t.Run("adding the hook annotation turns a regular resource into a hook", func(t *testing.T) {
		res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)
		require.Equal(t, common.StoreAsRegular, res.StoreAs)

		patches, err := spec.CompilePatches([]spec.Patch{{Patch: `.metadata.annotations["helm.sh/hook"] = "post-install"`}})
		require.NoError(t, err)

		out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{res}, patches)
		require.NoError(t, err)
		require.Equal(t, common.StoreAsHook, out[0].StoreAs)
	})

	t.Run("removing the hook annotation turns a hook into a regular resource", func(t *testing.T) {
		res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", map[string]interface{}{"helm.sh/hook": "post-install"})
		require.Equal(t, common.StoreAsHook, res.StoreAs)

		patches, err := spec.CompilePatches([]spec.Patch{{Patch: `del(.metadata.annotations["helm.sh/hook"])`}})
		require.NoError(t, err)

		out, err := spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{res}, patches)
		require.NoError(t, err)
		require.Equal(t, common.StoreAsRegular, out[0].StoreAs)
	})
}

func TestAI_BuildRenderPatchedResourceSpecs_RejectsIdentityChange(t *testing.T) {
	tests := []struct {
		name    string
		program string
	}{
		{name: "name", program: `.metadata.name = "other"`},
		{name: "kind", program: `.kind = "StatefulSet"`},
		{name: "apiVersion", program: `.apiVersion = "apps/v1beta1"`},
		{name: "namespace", program: `.metadata.namespace = "other"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

			patches, err := spec.CompilePatches([]spec.Patch{{Patch: tt.program}})
			require.NoError(t, err)

			_, err = spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{res}, patches)
			require.ErrorContains(t, err, "changed resource identity")
		})
	}
}

func TestAI_BuildRenderPatchedResourceSpecs_RejectsNonStringMetadata(t *testing.T) {
	tests := []struct {
		name    string
		program string
	}{
		{name: "annotations", program: `.metadata.annotations = {"werf.io/weight": 10}`},
		{name: "labels", program: `.metadata.labels = {"app": true}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := renderedSpec(t, "web", "", "myapp/templates/web.yaml", nil)

			patches, err := spec.CompilePatches([]spec.Patch{{Patch: tt.program}})
			require.NoError(t, err)

			_, err = spec.BuildRenderPatchedResourceSpecs(context.Background(), "prod", []*spec.ResourceSpec{res}, patches)
			require.ErrorContains(t, err, "expected string")
		})
	}
}

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
