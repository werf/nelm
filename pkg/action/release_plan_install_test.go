package action

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/v2/pkg/common"
	"github.com/werf/nelm/v2/pkg/log"
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

type recordingLogger struct {
	log.Logger

	lines  []string
	blocks []string
}

func (l *recordingLogger) Info(_ context.Context, format string, a ...interface{}) {
	l.lines = append(l.lines, fmt.Sprintf(format, a...))
}

func (l *recordingLogger) InfoBlockErr(_ context.Context, opts log.BlockOptions, fn func() error) error {
	l.blocks = append(l.blocks, opts.BlockTitle)
	return fn()
}

func TestLogPlannedChangesCollapsesResourcesWithoutDiff(t *testing.T) {
	obj := func(name, value string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]interface{}{"name": name},
			"data":       map[string]interface{}{"key": value},
		}}
	}
	meta := func(name string) *spec.ResourceMeta {
		return &spec.ResourceMeta{Name: name, GroupVersionKind: schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}}
	}

	artifact := &PlanArtifact{
		Data: &PlanArtifactData{Changes: []*plan.ResourceChange{
			{Type: "update", ResourceMeta: meta("same"), Before: obj("same", "a"), After: obj("same", "a")},
			{Type: "update", ResourceMeta: meta("changed"), Before: obj("changed", "a"), After: obj("changed", "b")},
		}},
		Release: PlanArtifactRelease{Name: "rel", Namespace: "ns"},
	}

	recorder := &recordingLogger{}
	origLogger := log.Default
	log.Default = recorder
	t.Cleanup(func() { log.Default = origLogger })

	opts := common.ResourceDiffOptions{}
	opts.ApplyDefaults()
	require.NoError(t, logPlannedChanges(context.Background(), artifact, opts))

	// Only the resource with a real diff gets a block.
	require.Len(t, recorder.blocks, 1)
	assert.Contains(t, recorder.blocks[0], "changed")

	output := strings.Join(recorder.lines, "\n")
	assert.NotContains(t, output, plan.HiddenInsignificantChanges)
	assert.Contains(t, output, "Other changes")
	assert.Contains(t, output, "- ")
	assert.Contains(t, output, "ConfigMap/same")
}

func TestIsHiddenDiff(t *testing.T) {
	assert.True(t, isHiddenDiff(plan.HiddenInsignificantChanges))
	assert.True(t, isHiddenDiff(plan.HiddenVerboseCRDChanges))
	assert.False(t, isHiddenDiff("- a\n+ b"))
	assert.False(t, isHiddenDiff(""))
}
