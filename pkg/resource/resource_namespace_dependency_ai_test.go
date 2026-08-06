//go:build ai_tests

package resource_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/resource"
	"github.com/werf/nelm/pkg/resource/spec"
)

func TestAI_ManualDeployDependencyNormalizesReleaseNamespace(t *testing.T) {
	const releaseNamespace = "test-namespace"

	t.Run("modern syntax with release namespace normalizes to empty", func(t *testing.T) {
		dep := manualDeployDep(t, releaseNamespace, map[string]string{
			"werf.io/deploy-dependency-target": "state=ready,kind=Deployment,group=apps,version=v1,name=target,namespace=" + releaseNamespace,
		})

		require.Equal(t, common.ResourceStateReady, dep.ResourceState)
		require.Equal(t, []string{""}, dep.Namespaces)
	})

	t.Run("modern syntax with foreign namespace kept literal", func(t *testing.T) {
		dep := manualDeployDep(t, releaseNamespace, map[string]string{
			"werf.io/deploy-dependency-target": "state=ready,kind=Deployment,group=apps,version=v1,name=target,namespace=other-ns",
		})

		require.Equal(t, []string{"other-ns"}, dep.Namespaces)
	})

	t.Run("modern syntax with omitted namespace yields empty list", func(t *testing.T) {
		dep := manualDeployDep(t, releaseNamespace, map[string]string{
			"werf.io/deploy-dependency-target": "state=ready,kind=Deployment,group=apps,version=v1,name=target",
		})

		require.Empty(t, dep.Namespaces)
	})

}

func manualDeployDep(t *testing.T, releaseNamespace string, annotations map[string]string) *resource.Dependency {
	t.Helper()

	resSpec := newDependentConfigMapSpec(releaseNamespace, annotations)

	res, err := resource.NewInstallableResource(context.Background(), resSpec, nil, releaseNamespace, resource.InstallableResourceOptions{})
	require.NoError(t, err)
	require.Len(t, res.ManualDependencies, 1)

	return res.ManualDependencies[0]
}

func newDependentConfigMapSpec(releaseNamespace string, annotations map[string]string) *spec.ResourceSpec {
	meta := map[string]interface{}{
		"name":      "dependent",
		"namespace": releaseNamespace,
	}
	if len(annotations) > 0 {
		anns := map[string]interface{}{}
		for k, v := range annotations {
			anns[k] = v
		}
		meta["annotations"] = anns
	}

	return spec.NewResourceSpec(&unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   meta,
			"data":       map[string]interface{}{"key": "value"},
		},
	}, releaseNamespace, spec.ResourceSpecOptions{})
}
