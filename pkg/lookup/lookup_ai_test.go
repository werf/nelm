//go:build ai_tests

package lookup

import (
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/helm/pkg/chart"
	"github.com/werf/nelm/pkg/helm/pkg/chartutil"
	"github.com/werf/nelm/pkg/helm/pkg/engine"
	"github.com/werf/nelm/pkg/helm/pkg/werf/helmopts"
)

func TestAI_LocalClientProviderEmpty(t *testing.T) {
	provider := NewLocalClientProvider(nil)

	c := &chart.Chart{
		Metadata: &chart.Metadata{Name: "moby", Version: "1.2.3"},
		Templates: []*chart.File{
			{Name: "templates/empty", Data: []byte(`{{ (lookup "v1" "Pod" "default" "pod1") }}`)},
		},
		Values: map[string]any{},
	}

	vals, err := chartutil.CoalesceValues(c, map[string]any{"Values": map[string]any{}})
	require.NoError(t, err)

	out, err := engine.RenderWithClientProvider(c, vals, provider, helmopts.HelmOptions{})
	require.NoError(t, err)
	require.Equal(t, "map[]", out["moby/templates/empty"])
}

func TestAI_LocalClientProviderLookup(t *testing.T) {
	provider := NewLocalClientProvider([]*unstructured.Unstructured{
		makeUnstructured("v1", "Namespace", "default", ""),
		makeUnstructured("v1", "Pod", "pod1", "default"),
		makeUnstructured("v1", "Pod", "pod2", "ns1"),
		makeUnstructured("v1", "Pod", "pod3", "ns1"),
	})

	templates := map[string]string{
		"cluster-single":  `{{ (lookup "v1" "Namespace" "" "default").metadata.name }}`,
		"namespaced-get":  `{{ (lookup "v1" "Pod" "default" "pod1").metadata.name }}`,
		"namespaced-list": `{{ (lookup "v1" "Pod" "ns1" "").items | len }}`,
		"all-ns-list":     `{{ (lookup "v1" "Pod" "" "").items | len }}`,
		"missing-get":     `{{ (lookup "v1" "Pod" "" "absent") }}`,
	}
	expected := map[string]string{
		"cluster-single":  "default",
		"namespaced-get":  "pod1",
		"namespaced-list": "2",
		"all-ns-list":     "3",
		"missing-get":     "map[]",
	}

	c := &chart.Chart{
		Metadata: &chart.Metadata{Name: "moby", Version: "1.2.3"},
		Values:   map[string]any{},
	}

	for name, tpl := range templates {
		c.Templates = append(c.Templates, &chart.File{
			Name: path.Join("templates", name),
			Data: []byte(tpl),
		})
	}

	vals, err := chartutil.CoalesceValues(c, map[string]any{"Values": map[string]any{}})
	require.NoError(t, err)

	out, err := engine.RenderWithClientProvider(c, vals, provider, helmopts.HelmOptions{})
	require.NoError(t, err)

	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, want, out[path.Join("moby/templates", name)])
		})
	}
}

func TestAI_LocalClientProviderUnstubbedListEmptyProvider(t *testing.T) {
	provider := NewLocalClientProvider(nil)

	c := &chart.Chart{
		Metadata: &chart.Metadata{Name: "moby", Version: "1.2.3"},
		Templates: []*chart.File{
			{Name: "templates/list", Data: []byte(`{{ (lookup "v1" "Pod" "" "").items | len }}`)},
		},
		Values: map[string]any{},
	}

	vals, err := chartutil.CoalesceValues(c, map[string]any{"Values": map[string]any{}})
	require.NoError(t, err)

	out, err := engine.RenderWithClientProvider(c, vals, provider, helmopts.HelmOptions{})
	require.NoError(t, err)
	require.Equal(t, "0", out["moby/templates/list"])
}

func TestAI_LocalClientProviderUnstubbedListOtherKind(t *testing.T) {
	provider := NewLocalClientProvider([]*unstructured.Unstructured{
		makeUnstructured("v1", "Pod", "pod1", "default"),
	})

	c := &chart.Chart{
		Metadata: &chart.Metadata{Name: "moby", Version: "1.2.3"},
		Templates: []*chart.File{
			{Name: "templates/list", Data: []byte(`{{ (lookup "v1" "ConfigMap" "" "").items | len }}`)},
		},
		Values: map[string]any{},
	}

	vals, err := chartutil.CoalesceValues(c, map[string]any{"Values": map[string]any{}})
	require.NoError(t, err)

	out, err := engine.RenderWithClientProvider(c, vals, provider, helmopts.HelmOptions{})
	require.NoError(t, err)
	require.Equal(t, "0", out["moby/templates/list"])
}

func makeUnstructured(apiVersion, kind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name": name,
		},
	}}
	if namespace != "" {
		obj.Object["metadata"].(map[string]interface{})["namespace"] = namespace
	}

	return obj
}
