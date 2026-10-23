package action

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/gookit/color"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/plan"
	"github.com/werf/nelm/v2/pkg/release"
	"github.com/werf/nelm/v2/pkg/resource/spec"
)

func TestReleaseMustInstallMessage(t *testing.T) {
	tests := []struct {
		name             string
		releaseName      string
		releaseNamespace string
		reason           release.ReleaseOutdatedReason
		want             string
	}{
		{
			name:             "no-reason",
			releaseName:      "myrelease",
			releaseNamespace: "mynamespace",
			reason:           release.ReleaseOutdatedReasonNone,
			want:             `No resource changes planned, but still must install release "myrelease" (namespace: "mynamespace")`,
		},
		{
			name:             "values-changed",
			releaseName:      "myrelease",
			releaseNamespace: "mynamespace",
			reason:           release.ReleaseOutdatedReasonValuesChanged,
			want:             `No resource changes planned, but still must install release "myrelease" (namespace: "mynamespace") because ` + string(release.ReleaseOutdatedReasonValuesChanged),
		},
		{
			name:             "notes-changed",
			releaseName:      "myrelease",
			releaseNamespace: "mynamespace",
			reason:           release.ReleaseOutdatedReasonNotesChanged,
			want:             `No resource changes planned, but still must install release "myrelease" (namespace: "mynamespace") because ` + string(release.ReleaseOutdatedReasonNotesChanged),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := releaseMustInstallMessage(tt.releaseName, tt.releaseNamespace, tt.reason)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLogPlannedChangesListsResourcesWithoutShownDiffTogether(t *testing.T) {
	configMap := func(name, value string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": name},
			"data":       map[string]interface{}{"key": value},
		}}
	}
	crd := func(name string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata":   map[string]interface{}{"name": name},
		}}
	}
	configMapMeta := func(name string) *spec.ResourceMeta {
		return &spec.ResourceMeta{Name: name, GroupVersionKind: schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}}
	}
	crdMeta := func(name string) *spec.ResourceMeta {
		return &spec.ResourceMeta{Name: name, GroupVersionKind: schema.GroupVersionKind{Group: "apiextensions.k8s.io", Version: "v1", Kind: "CustomResourceDefinition"}}
	}

	artifact := &PlanArtifact{
		Data: &PlanArtifactData{Changes: []*plan.ResourceChange{
			{Type: "create", ResourceMeta: crdMeta("news.example.com"), After: crd("news.example.com")},
			{Type: "update", ResourceMeta: configMapMeta("insignificant"), Before: configMap("insignificant", "a"), After: configMap("insignificant", "a")},
			{Type: "update", ResourceMeta: configMapMeta("changed"), Before: configMap("changed", "a"), After: configMap("changed", "b")},
			{Type: "create", ResourceMeta: crdMeta("withreason.example.com"), After: crd("withreason.example.com"), Reason: "blind apply: could not read the live object"},
		}},
		Release: PlanArtifactRelease{Name: "rel", Namespace: "ns"},
	}

	recorder := recordLogs(t)

	opts := common.ResourceDiffOptions{}
	opts.ApplyDefaults()
	require.NoError(t, logPlannedChanges(context.Background(), artifact, opts))

	require.Len(t, recorder.blocks, 2)
	assert.Contains(t, recorder.blocks[0], "ConfigMap/changed")
	assert.Contains(t, recorder.blocks[1], "CustomResourceDefinition/withreason.example.com")

	headingIdx := slices.IndexFunc(recorder.lines, func(line string) bool { return strings.Contains(line, "Other changes") })
	require.NotEqual(t, -1, headingIdx)
	assert.Equal(t, "Other changes:", color.ClearCode(recorder.lines[headingIdx]))

	summaryIdx := slices.IndexFunc(recorder.lines, func(line string) bool { return strings.Contains(line, "Planned changes summary") })
	require.Greater(t, summaryIdx, headingIdx)

	var listed []string
	for _, line := range recorder.lines[headingIdx+1 : summaryIdx-1] {
		listed = append(listed, color.ClearCode(line))
	}

	assert.Equal(t, []string{
		"- Create CustomResourceDefinition/news.example.com (hidden verbose CRD changes)",
		"- Update ConfigMap/insignificant (hidden insignificant changes)",
	}, listed)
	assert.Equal(t, "", recorder.lines[summaryIdx-1])
}
