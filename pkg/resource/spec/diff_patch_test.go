package spec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	helmchart "github.com/werf/nelm/pkg/helm/pkg/chart"
	chartcommon "github.com/werf/nelm/pkg/helm/pkg/chart/common"
	v2chart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	"github.com/werf/nelm/pkg/resource/spec"
)

func TestApplyDiffPatches(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "web"},
		"spec":       map[string]interface{}{"replicas": int64(3)},
	}}
	meta := metaFor("Deployment", "apps", "v1", "web", "", "myapp/templates/web.yaml", nil, nil)

	t.Run("no rules returns unchanged deep copy", func(t *testing.T) {
		out, err := spec.ApplyDiffPatches(nil, meta, "prod", obj)
		require.NoError(t, err)
		require.Equal(t, obj.Object, out.Object)
		require.NotSame(t, &obj.Object, &out.Object)
	})

	t.Run("non-matching rule leaves object unchanged", func(t *testing.T) {
		c, err := spec.CompileDiffPatch(spec.DiffPatch{
			Match: spec.ResourceMatcher{Names: []string{"other"}},
			Patch: "del(.spec.replicas)",
		})
		require.NoError(t, err)

		out, err := spec.ApplyDiffPatches([]*spec.CompiledDiffPatch{c}, meta, "prod", obj)
		require.NoError(t, err)
		require.Equal(t, int64(3), out.Object["spec"].(map[string]interface{})["replicas"])
	})

	t.Run("matching rules chain in order", func(t *testing.T) {
		c1, err := spec.CompileDiffPatch(spec.DiffPatch{Patch: "del(.spec.replicas)"})
		require.NoError(t, err)
		c2, err := spec.CompileDiffPatch(spec.DiffPatch{Patch: `.metadata.name = "patched"`})
		require.NoError(t, err)

		out, err := spec.ApplyDiffPatches([]*spec.CompiledDiffPatch{c1, c2}, meta, "prod", obj)
		require.NoError(t, err)

		_, hasReplicas := out.Object["spec"].(map[string]interface{})["replicas"]
		require.False(t, hasReplicas)
		require.Equal(t, "patched", out.Object["metadata"].(map[string]interface{})["name"])
	})
}

func TestCollectChartPatches_NilChart(t *testing.T) {
	patches, err := spec.CollectChartPatches(nil)
	require.NoError(t, err)
	require.Empty(t, patches)
}

func TestCollectChartPatches_ScopingAndOrder(t *testing.T) {
	// Tree: app (root) -> [cache subchart]
	cache := chartWithPatches("cache", "diffPatches:\n- patch: del(.cacheField)\n")
	app := chartWithPatches("app", "diffPatches:\n- patch: del(.appField)\n", cache)

	accessor, err := helmchart.NewAccessor(app)
	require.NoError(t, err)

	patches, err := spec.CollectChartPatches(accessor)
	require.NoError(t, err)
	require.Len(t, patches, 2)

	// Leaf-first ordering: subchart (cache) rule before parent (app) rule.
	require.Equal(t, "del(.cacheField)", patches[0].Patch)
	require.Equal(t, "del(.appField)", patches[1].Patch)

	// Scoping: subchart rule constrained to its subtree; parent rule to the root.
	require.Equal(t, "app/charts/cache", patches[0].ChartScope)
	require.Equal(t, "app", patches[1].ChartScope)
}

func TestCompileDiffPatch_DefaultsType(t *testing.T) {
	c, err := spec.CompileDiffPatch(spec.DiffPatch{Patch: "."})
	require.NoError(t, err)
	require.NotNil(t, c)
}

func TestCompileDiffPatch_FailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		patch spec.DiffPatch
	}{
		{
			name:  "invalid jq program",
			patch: spec.DiffPatch{Patch: "del(.spec.replicas"},
		},
		{
			name:  "empty patch body",
			patch: spec.DiffPatch{Patch: "   "},
		},
		{
			name:  "unsupported type",
			patch: spec.DiffPatch{Type: "jsonPointer", Patch: "."},
		},
		{
			name: "invalid regexp in selector",
			patch: spec.DiffPatch{
				Match: spec.ResourceMatcher{Names: []string{"/(/"}},
				Patch: ".",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := spec.CompileDiffPatch(tt.patch)
			require.Error(t, err)
		})
	}
}

func TestCompiledDiffPatch_ChartScope(t *testing.T) {
	scope := "app/charts/cache"
	tests := []struct {
		name     string
		filePath string
		want     bool
	}{
		{name: "resource in the scoped subtree matches", filePath: "app/charts/cache/templates/redis.yaml", want: true},
		{name: "resource in a nested sub-subchart matches", filePath: "app/charts/cache/charts/inner/templates/x.yaml", want: true},
		{name: "parent resource does not match", filePath: "app/templates/web.yaml", want: false},
		{name: "sibling subchart resource does not match", filePath: "app/charts/postgres/templates/db.yaml", want: false},
		{name: "prefix-collision sibling does not match", filePath: "app/charts/cache-extra/templates/x.yaml", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := spec.CompileDiffPatch(spec.DiffPatch{ChartScope: scope, Patch: "."})
			require.NoError(t, err)

			meta := metaFor("Deployment", "apps", "v1", "web", "", tt.filePath, nil, nil)
			require.Equal(t, tt.want, c.Match(meta, ""))
		})
	}
}

func TestCompiledDiffPatch_Match(t *testing.T) {
	tests := []struct {
		name      string
		selector  spec.ResourceMatcher
		meta      *spec.ResourceMeta
		namespace string
		want      bool
	}{
		{
			name:     "empty selector matches all",
			selector: spec.ResourceMatcher{},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", nil, nil),
			want:     true,
		},
		{
			name:     "exact name match",
			selector: spec.ResourceMatcher{Names: []string{"web"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", nil, nil),
			want:     true,
		},
		{
			name:     "name mismatch",
			selector: spec.ResourceMatcher{Names: []string{"web"}},
			meta:     metaFor("Deployment", "apps", "v1", "api", "", "", nil, nil),
			want:     false,
		},
		{
			name:     "name is case-sensitive",
			selector: spec.ResourceMatcher{Names: []string{"Web"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", nil, nil),
			want:     false,
		},
		{
			name:     "kind is case-insensitive",
			selector: spec.ResourceMatcher{Kinds: []string{"deployment"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", nil, nil),
			want:     true,
		},
		{
			name:     "regexp name match",
			selector: spec.ResourceMatcher{Names: []string{"/web-.*/"}},
			meta:     metaFor("Deployment", "apps", "v1", "web-123", "", "", nil, nil),
			want:     true,
		},
		{
			name:     "regexp is anchored full match",
			selector: spec.ResourceMatcher{Names: []string{"/web/"}},
			meta:     metaFor("Deployment", "apps", "v1", "web-123", "", "", nil, nil),
			want:     false,
		},
		{
			name:     "literal dot does not over-match",
			selector: spec.ResourceMatcher{Names: []string{"my.app"}},
			meta:     metaFor("Deployment", "apps", "v1", "myXapp", "", "", nil, nil),
			want:     false,
		},
		{
			name:     "fields AND together - kind matches but group does not",
			selector: spec.ResourceMatcher{Kinds: []string{"Deployment"}, Groups: []string{"batch"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", nil, nil),
			want:     false,
		},
		{
			name:     "list values OR together",
			selector: spec.ResourceMatcher{Names: []string{"api", "web"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", nil, nil),
			want:     true,
		},
		{
			name:     "labels all must match",
			selector: spec.ResourceMatcher{Labels: map[string]string{"tier": "backend", "team": "pay"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", map[string]string{"tier": "backend", "team": "pay"}, nil),
			want:     true,
		},
		{
			name:     "labels mismatch when one missing",
			selector: spec.ResourceMatcher{Labels: map[string]string{"tier": "backend", "team": "pay"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", map[string]string{"tier": "backend"}, nil),
			want:     false,
		},
		{
			name:     "empty-value label selector does not match a missing key",
			selector: spec.ResourceMatcher{Labels: map[string]string{"tier": ""}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", map[string]string{"other": "x"}, nil),
			want:     false,
		},
		{
			name:     "empty-value label selector matches an explicit empty value",
			selector: spec.ResourceMatcher{Labels: map[string]string{"tier": ""}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", map[string]string{"tier": ""}, nil),
			want:     true,
		},
		{
			name:     "annotations match",
			selector: spec.ResourceMatcher{Annotations: map[string]string{"team": "payments"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", nil, map[string]string{"team": "payments"}),
			want:     true,
		},
		{
			name:      "namespace matches release namespace via true object namespace",
			selector:  spec.ResourceMatcher{Namespaces: []string{"prod"}},
			meta:      metaFor("Deployment", "apps", "v1", "web", "", "", nil, nil),
			namespace: "prod",
			want:      true,
		},
		{
			name:      "namespace explicit match",
			selector:  spec.ResourceMatcher{Namespaces: []string{"other"}},
			meta:      metaFor("Deployment", "apps", "v1", "web", "other", "", nil, nil),
			namespace: "other",
			want:      true,
		},
		{
			name:      "namespace-scoped selector does not match cluster-scoped resource",
			selector:  spec.ResourceMatcher{Namespaces: []string{"prod"}},
			meta:      metaFor("ClusterRole", "rbac.authorization.k8s.io", "v1", "viewer", "", "", nil, nil),
			namespace: "",
			want:      false,
		},
		{
			name:     "chart top-level match",
			selector: spec.ResourceMatcher{Charts: []string{"myapp"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "myapp/templates/web.yaml", nil, nil),
			want:     true,
		},
		{
			name:     "chart subchart alias match",
			selector: spec.ResourceMatcher{Charts: []string{"cache"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "myapp/charts/cache/templates/redis.yaml", nil, nil),
			want:     true,
		},
		{
			name:     "chart selector does not match upstream name when aliased",
			selector: spec.ResourceMatcher{Charts: []string{"redis"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "myapp/charts/cache/templates/redis.yaml", nil, nil),
			want:     false,
		},
		{
			name:     "parent chart does not match subchart-originated resource",
			selector: spec.ResourceMatcher{Charts: []string{"myapp"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "myapp/charts/cache/templates/redis.yaml", nil, nil),
			want:     false,
		},
		{
			name:     "full chart-path matches the subchart",
			selector: spec.ResourceMatcher{Charts: []string{"myapp/charts/cache"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "myapp/charts/cache/templates/redis.yaml", nil, nil),
			want:     true,
		},
		{
			name:     "full chart-path disambiguates identically-named subcharts (match)",
			selector: spec.ResourceMatcher{Charts: []string{"parent/charts/child1/charts/mychart"}},
			meta:     metaFor("ConfigMap", "", "v1", "cm", "", "parent/charts/child1/charts/mychart/templates/cm.yaml", nil, nil),
			want:     true,
		},
		{
			name:     "full chart-path disambiguates identically-named subcharts (no match on the other)",
			selector: spec.ResourceMatcher{Charts: []string{"parent/charts/child1/charts/mychart"}},
			meta:     metaFor("ConfigMap", "", "v1", "cm", "", "parent/charts/child2/charts/subchild3/charts/mychart/templates/cm.yaml", nil, nil),
			want:     false,
		},
		{
			name:     "bare alias still matches both identically-named subcharts",
			selector: spec.ResourceMatcher{Charts: []string{"mychart"}},
			meta:     metaFor("ConfigMap", "", "v1", "cm", "", "parent/charts/child2/charts/subchild3/charts/mychart/templates/cm.yaml", nil, nil),
			want:     true,
		},
		{
			name:     "regex over the chart-path targets one subtree",
			selector: spec.ResourceMatcher{Charts: []string{"/parent/charts/child2/.*/mychart/"}},
			meta:     metaFor("ConfigMap", "", "v1", "cm", "", "parent/charts/child2/charts/subchild3/charts/mychart/templates/cm.yaml", nil, nil),
			want:     true,
		},
		{
			name:     "standalone CRD path matches leading chart segment",
			selector: spec.ResourceMatcher{Charts: []string{"myapp"}},
			meta:     metaFor("CustomResourceDefinition", "apiextensions.k8s.io", "v1", "foos", "", "myapp/crds/foo.yaml", nil, nil),
			want:     true,
		},
		{
			name:     "empty FilePath never matches chart rule",
			selector: spec.ResourceMatcher{Charts: []string{"myapp"}},
			meta:     metaFor("Deployment", "apps", "v1", "web", "", "", nil, nil),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := spec.CompileDiffPatch(spec.DiffPatch{Match: tt.selector, Patch: "."})
			require.NoError(t, err)
			require.Equal(t, tt.want, c.Match(tt.meta, tt.namespace))
		})
	}
}

func TestCompiledDiffPatch_Transform(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"spec": map[string]interface{}{
			"replicas": int64(3),
			"selector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "web"}},
		},
	}}

	t.Run("happy path removes field without mutating input", func(t *testing.T) {
		c, err := spec.CompileDiffPatch(spec.DiffPatch{Patch: "del(.spec.replicas)"})
		require.NoError(t, err)

		out, err := c.Transform(obj)
		require.NoError(t, err)

		outSpec := out.Object["spec"].(map[string]interface{})
		_, hasReplicas := outSpec["replicas"]
		require.False(t, hasReplicas)

		inSpec := obj.Object["spec"].(map[string]interface{})
		require.Equal(t, int64(3), inSpec["replicas"])
	})

	// Kubernetes unstructured objects store integers as int64, which gojq rejects
	// unless the input is number-normalized. These programs would silently
	// mis-compare or panic without that normalization.
	numeric := []struct {
		name    string
		program string
		want    interface{}
	}{
		{name: "comparison", program: `if .spec.replicas == 3 then .spec.replicas = 0 else . end`, want: int64(0)},
		{name: "greater-than", program: `if .spec.replicas > 2 then .spec.replicas = 0 else . end`, want: int64(0)},
		{name: "arithmetic", program: `.spec.replicas += 1`, want: int64(4)},
		{name: "assignment-with-arithmetic", program: `.spec.replicas = (.spec.replicas + 1)`, want: int64(4)},
		{name: "walk-zeroes-numbers", program: `walk(if type == "number" then 0 else . end)`, want: int64(0)},
	}

	for _, tt := range numeric {
		t.Run("numeric "+tt.name, func(t *testing.T) {
			c, err := spec.CompileDiffPatch(spec.DiffPatch{Patch: tt.program})
			require.NoError(t, err)

			out, err := c.Transform(obj)
			require.NoError(t, err)
			require.Equal(t, tt.want, out.Object["spec"].(map[string]interface{})["replicas"])
		})
	}

	failures := []struct {
		name    string
		program string
	}{
		{name: "no output", program: "empty"},
		{name: "multiple outputs", program: ".spec, .kind"},
		{name: "scalar output", program: ".kind"},
		{name: "array output", program: "[.]"},
		{name: "null output", program: ".missing"},
	}

	for _, tt := range failures {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			c, err := spec.CompileDiffPatch(spec.DiffPatch{Patch: tt.program})
			require.NoError(t, err)

			_, err = c.Transform(obj)
			require.Error(t, err)
		})
	}
}

func TestLoadPatchesFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")

	require.NoError(t, os.WriteFile(a, []byte("diffPatches:\n- patch: del(.a)\n"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("diffPatches:\n- patch: del(.b)\n"), 0o644))

	patches, err := spec.LoadPatchesFiles([]string{a, b})
	require.NoError(t, err)
	require.Len(t, patches, 2)
	require.Equal(t, "del(.a)", patches[0].Patch)
	require.Equal(t, "del(.b)", patches[1].Patch)

	_, err = spec.LoadPatchesFiles([]string{filepath.Join(dir, "missing.yaml")})
	require.Error(t, err)
}

func TestParsePatchesFile(t *testing.T) {
	t.Run("parses diffPatches", func(t *testing.T) {
		data := []byte(`
diffPatches:
- match:
    kinds: [Deployment]
  patch: del(.spec.replicas)
- patch: del(.data.foo)
`)
		patches, err := spec.ParsePatchesFile(data)
		require.NoError(t, err)
		require.Len(t, patches, 2)
		require.Equal(t, []string{"Deployment"}, patches[0].Match.Kinds)
		require.Equal(t, "del(.spec.replicas)", patches[0].Patch)
	})

	t.Run("rejects unknown top-level key", func(t *testing.T) {
		_, err := spec.ParsePatchesFile([]byte("bogusKey: []\n"))
		require.Error(t, err)
	})

	t.Run("empty file yields no patches", func(t *testing.T) {
		patches, err := spec.ParsePatchesFile([]byte("{}\n"))
		require.NoError(t, err)
		require.Empty(t, patches)
	})
}

func chartWithPatches(name, patchesYAML string, deps ...*v2chart.Chart) *v2chart.Chart {
	ch := &v2chart.Chart{Metadata: &v2chart.Metadata{Name: name}}
	if patchesYAML != "" {
		ch.Files = []*chartcommon.File{{Name: "patches.yaml", Data: []byte(patchesYAML)}}
	}

	if len(deps) > 0 {
		ch.AddDependency(deps...)
	}

	return ch
}

func metaFor(kind, group, version, name, namespace, filePath string, labels, annotations map[string]string) *spec.ResourceMeta {
	return &spec.ResourceMeta{
		Name:             name,
		Namespace:        namespace,
		GroupVersionKind: schema.GroupVersionKind{Group: group, Version: version, Kind: kind},
		FilePath:         filePath,
		Labels:           labels,
		Annotations:      annotations,
	}
}
