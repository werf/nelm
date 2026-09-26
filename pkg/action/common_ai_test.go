//go:build ai_tests

package action //nolint:testpackage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	helmchart "github.com/werf/nelm/v2/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/v2/pkg/helm/pkg/chart/common"
	v2chart "github.com/werf/nelm/v2/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestAI_RenderContextFor_NoSubchartsLeavesNoEntries(t *testing.T) {
	accessor, err := helmchart.NewAccessor(&v2chart.Chart{Metadata: &v2chart.Metadata{Name: "app"}})
	require.NoError(t, err)

	withChart, err := renderContextFor(accessor, map[string]interface{}{})
	require.NoError(t, err)
	require.Empty(t, withChart.Subcharts)

	withoutChart, err := renderContextFor(nil, map[string]interface{}{})
	require.NoError(t, err)
	require.Empty(t, withoutChart.Subcharts)
}

func TestAI_RenderContextFor_ScopesEachSubchartToItsOwnValuesAndMetadata(t *testing.T) {
	cache := &v2chart.Chart{Metadata: &v2chart.Metadata{Name: "cache", Version: "4.5.6"}}
	inner := &v2chart.Chart{Metadata: &v2chart.Metadata{Name: "inner", Version: "7.8.9"}}
	cache.AddDependency(inner)

	parent := &v2chart.Chart{Metadata: &v2chart.Metadata{Name: "app", Version: "1.2.3"}}
	parent.AddDependency(cache)

	accessor, err := helmchart.NewAccessor(parent)
	require.NoError(t, err)

	renderedValues := map[string]interface{}{
		"Chart": map[string]interface{}{"Name": "app"},
		"Values": chartcommon.Values{
			"replicaCount": int64(1),
			"cache": map[string]interface{}{
				"replicaCount": int64(2),
				"inner":        map[string]interface{}{"replicaCount": int64(3)},
			},
		},
	}

	renderContext, err := renderContextFor(accessor, renderedValues)
	require.NoError(t, err)

	require.Equal(t, renderedValues["Values"], renderContext.Values)

	cacheCtx, found := renderContext.Subcharts["app/charts/cache"]
	require.True(t, found)
	require.Equal(t, "cache", cacheCtx.Chart.(map[string]interface{})["Name"])
	require.Equal(t, int64(2), cacheCtx.Values.(map[string]interface{})["replicaCount"])

	innerCtx, found := renderContext.Subcharts["app/charts/cache/charts/inner"]
	require.True(t, found)
	require.Equal(t, "inner", innerCtx.Chart.(map[string]interface{})["Name"])
	require.Equal(t, int64(3), innerCtx.Values.(map[string]interface{})["replicaCount"])
}

func TestAI_ResolvePatches_DefaultPatchesDisableKeepsLegacyPatches(t *testing.T) {
	chart := aiChartWithPatches(t, "app", "renderPatches:\n- patch: .order += [\"chart\"]\n")

	legacy := spec.Patches{Render: []spec.Patch{{Patch: `.order += ["legacy"]`}}}

	patches, err := resolvePatches(chart, true, nil, legacy, spec.RenderContext{})
	require.NoError(t, err)

	require.Equal(t, []interface{}{"legacy"}, aiApplyOrder(t, patches.Render, "app/templates/web.yaml"))
}

func TestAI_ResolvePatches_InvalidLegacyPatchFailsClosed(t *testing.T) {
	_, err := resolvePatches(nil, true, nil, spec.Patches{
		Render: []spec.Patch{{Patch: "del(.spec.replicas"}},
	}, spec.RenderContext{})
	require.ErrorContains(t, err, "compile render patches")
}

func TestAI_ResolvePatches_LegacyPatchesAppliedLast(t *testing.T) {
	chart := aiChartWithPatches(t, "app", "diffPatches:\n- patch: .order += [\"chart\"]\nrenderPatches:\n- patch: .order += [\"chart\"]\n")
	patchesFile := aiWritePatchesFile(t, "diffPatches:\n- patch: .order += [\"file\"]\nrenderPatches:\n- patch: .order += [\"file\"]\n")

	legacy := spec.Patches{
		Diff:   []spec.Patch{{Patch: `.order += ["legacy"]`}},
		Render: []spec.Patch{{Patch: `.order += ["legacy"]`}},
	}

	patches, err := resolvePatches(chart, false, []string{patchesFile}, legacy, spec.RenderContext{})
	require.NoError(t, err)

	require.Equal(t, []interface{}{"chart", "file", "legacy"}, aiApplyOrder(t, patches.Diff, "app/templates/web.yaml"))
	require.Equal(t, []interface{}{"chart", "file", "legacy"}, aiApplyOrder(t, patches.Render, "app/templates/web.yaml"))
}

func TestAI_ResolvePatches_LegacyPatchesScopeViaMatchCharts(t *testing.T) {
	legacy := spec.Patches{
		Render: []spec.Patch{{
			Match: spec.ResourceMatcher{Charts: []string{"app/charts/cache"}},
			Patch: `.order += ["legacy"]`,
		}},
	}

	patches, err := resolvePatches(nil, true, nil, legacy, spec.RenderContext{})
	require.NoError(t, err)

	require.Equal(t, []interface{}{"legacy"}, aiApplyOrder(t, patches.Render, "app/charts/cache/templates/redis.yaml"))
	require.Empty(t, aiApplyOrder(t, patches.Render, "app/templates/web.yaml"))
}

func aiApplyOrder(t *testing.T, patches []*spec.CompiledPatch, filePath string) []interface{} {
	t.Helper()

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "web"},
		"order":      []interface{}{},
	}}

	meta := &spec.ResourceMeta{
		Name:             "web",
		GroupVersionKind: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		FilePath:         filePath,
	}

	out, err := spec.ApplyPatches(context.Background(), patches, meta, "prod", obj)
	require.NoError(t, err)

	order, ok := out.Object["order"].([]interface{})
	require.True(t, ok)

	return order
}

func aiChartWithPatches(t *testing.T, name, patchesYAML string) helmchart.Accessor {
	t.Helper()

	chart := &v2chart.Chart{
		Metadata: &v2chart.Metadata{Name: name},
		Files:    []*chartcommon.File{{Name: "patches.yaml", Data: []byte(patchesYAML)}},
	}

	accessor, err := helmchart.NewAccessor(chart)
	require.NoError(t, err)

	return accessor
}

func aiWritePatchesFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "patches.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}
