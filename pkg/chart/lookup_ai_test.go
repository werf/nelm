//go:build ai_tests

package chart

import (
	"context"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	chartcommon "github.com/werf/nelm/pkg/helm/pkg/chart/common"
	chartcommonutil "github.com/werf/nelm/pkg/helm/pkg/chart/common/util"
	v2chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	helmengine "github.com/werf/nelm/pkg/helm/pkg/engine"
)

func TestAI_LocalClientProviderEmpty(t *testing.T) {
	provider := newLocalClientProvider(nil)

	c := &v2chart.Chart{
		Metadata: &v2chart.Metadata{Name: "moby", Version: "1.2.3"},
		Templates: []*chartcommon.File{
			{Name: "templates/empty", Data: []byte(`{{ (lookup "v1" "Pod" "default" "pod1") }}`)},
		},
		Values: map[string]any{},
	}

	vals, err := chartcommonutil.CoalesceValues(c, map[string]any{"Values": map[string]any{}})
	require.NoError(t, err)

	out, err := helmengine.RenderWithClientProvider(context.Background(), c, vals, provider)
	require.NoError(t, err)
	require.Equal(t, "map[]", out["moby/templates/empty"])
}

func TestAI_LocalClientProviderLookup(t *testing.T) {
	provider := newLocalClientProvider([]*unstructured.Unstructured{
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

	c := &v2chart.Chart{
		Metadata: &v2chart.Metadata{Name: "moby", Version: "1.2.3"},
		Values:   map[string]any{},
	}

	for name, tpl := range templates {
		c.Templates = append(c.Templates, &chartcommon.File{
			Name: path.Join("templates", name),
			Data: []byte(tpl),
		})
	}

	vals, err := chartcommonutil.CoalesceValues(c, map[string]any{"Values": map[string]any{}})
	require.NoError(t, err)

	out, err := helmengine.RenderWithClientProvider(context.Background(), c, vals, provider)
	require.NoError(t, err)

	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, want, out[path.Join("moby/templates", name)])
		})
	}
}

func TestAI_LocalClientProviderNamespaceIsolation(t *testing.T) {
	provider := newLocalClientProvider([]*unstructured.Unstructured{
		makeUnstructured("v1", "Pod", "pod1", "default"),
	})

	templates := map[string]string{
		"same-ns-get":  `{{ (lookup "v1" "Pod" "default" "pod1").metadata.name }}`,
		"other-ns-get": `{{ (lookup "v1" "Pod" "other" "pod1") }}`,
	}
	expected := map[string]string{
		"same-ns-get":  "pod1",
		"other-ns-get": "map[]",
	}

	c := &v2chart.Chart{
		Metadata: &v2chart.Metadata{Name: "moby", Version: "1.2.3"},
		Values:   map[string]any{},
	}

	for name, tpl := range templates {
		c.Templates = append(c.Templates, &chartcommon.File{
			Name: path.Join("templates", name),
			Data: []byte(tpl),
		})
	}

	vals, err := chartcommonutil.CoalesceValues(c, map[string]any{"Values": map[string]any{}})
	require.NoError(t, err)

	out, err := helmengine.RenderWithClientProvider(context.Background(), c, vals, provider)
	require.NoError(t, err)

	for name, want := range expected {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, want, out[path.Join("moby/templates", name)])
		})
	}
}

func TestAI_LocalClientProviderUnstubbedListEmptyProvider(t *testing.T) {
	provider := newLocalClientProvider(nil)

	c := &v2chart.Chart{
		Metadata: &v2chart.Metadata{Name: "moby", Version: "1.2.3"},
		Templates: []*chartcommon.File{
			{Name: "templates/list", Data: []byte(`{{ (lookup "v1" "Pod" "" "").items | len }}`)},
		},
		Values: map[string]any{},
	}

	vals, err := chartcommonutil.CoalesceValues(c, map[string]any{"Values": map[string]any{}})
	require.NoError(t, err)

	out, err := helmengine.RenderWithClientProvider(context.Background(), c, vals, provider)
	require.NoError(t, err)
	require.Equal(t, "0", out["moby/templates/list"])
}

func TestAI_LocalClientProviderUnstubbedListOtherKind(t *testing.T) {
	provider := newLocalClientProvider([]*unstructured.Unstructured{
		makeUnstructured("v1", "Pod", "pod1", "default"),
	})

	c := &v2chart.Chart{
		Metadata: &v2chart.Metadata{Name: "moby", Version: "1.2.3"},
		Templates: []*chartcommon.File{
			{Name: "templates/list", Data: []byte(`{{ (lookup "v1" "ConfigMap" "" "").items | len }}`)},
		},
		Values: map[string]any{},
	}

	vals, err := chartcommonutil.CoalesceValues(c, map[string]any{"Values": map[string]any{}})
	require.NoError(t, err)

	out, err := helmengine.RenderWithClientProvider(context.Background(), c, vals, provider)
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
