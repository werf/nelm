//go:build ai_tests

package plan

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/pkg/legacy/progrep"
)

func TestAI_BuildResolvedNamespaces(t *testing.T) {
	mapper := newFakeRESTMapper()
	releaseNS := "release-ns"

	opNamespaced := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
	}
	opNamespacedExplicit := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm2", "custom-ns", gvkConfigMap)},
	}
	opClusterScoped := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("my-ns", "", gvkNamespace)},
	}
	opMeta := &Operation{
		Type: OperationTypeNoop, Version: OperationVersionNoop, Category: OperationCategoryMeta,
		Config: &OperationConfigNoop{OpID: "stage/start"},
	}
	opTrack := &Operation{
		Type: OperationTypeTrackReadiness, Version: OperationVersionTrackReadiness, Category: OperationCategoryTrack,
		Config: &OperationConfigTrackReadiness{ResourceMeta: makeResourceMeta("dep1", "", gvkDeployment)},
	}

	p := buildTestPlan([]*Operation{opNamespaced, opNamespacedExplicit, opClusterScoped, opMeta, opTrack}, nil)

	resolved := buildResolvedNamespaces(p, releaseNS, mapper)

	assert.Equal(t, releaseNS, resolved[opNamespaced.ID()])
	assert.Equal(t, "custom-ns", resolved[opNamespacedExplicit.ID()])
	assert.Equal(t, "", resolved[opClusterScoped.ID()])

	_, metaPresent := resolved[opMeta.ID()]
	assert.False(t, metaPresent, "meta operations should not appear in resolved namespaces")

	assert.Equal(t, releaseNS, resolved[opTrack.ID()])
}

func TestAI_ExtractObjectRef(t *testing.T) {
	resolvedNS := map[string]string{}

	tests := []struct {
		name     string
		op       *Operation
		wantName string
		wantGVK  schema.GroupVersionKind
	}{
		{
			name: "Create",
			op: &Operation{
				Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
				Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
			},
			wantName: "cm1",
			wantGVK:  gvkConfigMap,
		},
		{
			name: "Update",
			op: &Operation{
				Type: OperationTypeUpdate, Version: OperationVersionUpdate, Category: OperationCategoryResource,
				Config: &OperationConfigUpdate{ResourceSpec: makeResourceSpec("cm2", "", gvkConfigMap)},
			},
			wantName: "cm2",
			wantGVK:  gvkConfigMap,
		},
		{
			name: "Apply",
			op: &Operation{
				Type: OperationTypeApply, Version: OperationVersionApply, Category: OperationCategoryResource,
				Config: &OperationConfigApply{ResourceSpec: makeResourceSpec("svc1", "", gvkService)},
			},
			wantName: "svc1",
			wantGVK:  gvkService,
		},
		{
			name: "Recreate",
			op: &Operation{
				Type: OperationTypeRecreate, Version: OperationVersionRecreate, Category: OperationCategoryResource,
				Config: &OperationConfigRecreate{ResourceSpec: makeResourceSpec("dep1", "", gvkDeployment)},
			},
			wantName: "dep1",
			wantGVK:  gvkDeployment,
		},
		{
			name: "Delete",
			op: &Operation{
				Type: OperationTypeDelete, Version: OperationVersionDelete, Category: OperationCategoryResource,
				Config: &OperationConfigDelete{ResourceMeta: makeResourceMeta("cm3", "", gvkConfigMap)},
			},
			wantName: "cm3",
			wantGVK:  gvkConfigMap,
		},
		{
			name: "TrackReadiness",
			op: &Operation{
				Type: OperationTypeTrackReadiness, Version: OperationVersionTrackReadiness, Category: OperationCategoryTrack,
				Config: &OperationConfigTrackReadiness{ResourceMeta: makeResourceMeta("dep2", "", gvkDeployment)},
			},
			wantName: "dep2",
			wantGVK:  gvkDeployment,
		},
		{
			name: "TrackPresence",
			op: &Operation{
				Type: OperationTypeTrackPresence, Version: OperationVersionTrackPresence, Category: OperationCategoryTrack,
				Config: &OperationConfigTrackPresence{ResourceMeta: makeResourceMeta("svc2", "", gvkService)},
			},
			wantName: "svc2",
			wantGVK:  gvkService,
		},
		{
			name: "TrackAbsence",
			op: &Operation{
				Type: OperationTypeTrackAbsence, Version: OperationVersionTrackAbsence, Category: OperationCategoryTrack,
				Config: &OperationConfigTrackAbsence{ResourceMeta: makeResourceMeta("cm4", "", gvkConfigMap)},
			},
			wantName: "cm4",
			wantGVK:  gvkConfigMap,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedNS[tt.op.ID()] = "test-ns"

			ref := extractObjectRef(tt.op, resolvedNS)

			assert.Equal(t, tt.wantName, ref.Name)
			assert.Equal(t, tt.wantGVK, ref.GroupVersionKind)
			assert.Equal(t, "test-ns", ref.Namespace)
		})
	}
}

func TestAI_ExtractObjectRef_PanicsOnUnexpectedConfig(t *testing.T) {
	op := &Operation{
		Type:     OperationTypeNoop,
		Version:  OperationVersionNoop,
		Category: OperationCategoryMeta,
		Config:   &OperationConfigNoop{OpID: "test"},
	}

	assert.Panics(t, func() {
		extractObjectRef(op, map[string]string{})
	})
}

func TestAI_MapOperationStatus(t *testing.T) {
	tests := []struct {
		input    OperationStatus
		expected progrep.OperationStatus
	}{
		{input: OperationStatusUnknown, expected: progrep.OperationStatusPending},
		{input: OperationStatusPending, expected: progrep.OperationStatusProgressing},
		{input: OperationStatusCompleted, expected: progrep.OperationStatusCompleted},
		{input: OperationStatusFailed, expected: progrep.OperationStatusFailed},
	}

	for _, tt := range tests {
		name := string(tt.input)
		if name == "" {
			name = "unknown"
		}

		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.expected, mapOperationStatus(tt.input))
		})
	}
}

func TestAI_MapOperationStatus_DefaultCase(t *testing.T) {
	assert.Equal(t, progrep.OperationStatusPending, mapOperationStatus("some-unexpected-status"))
}

func TestAI_MapOperationType(t *testing.T) {
	tests := []struct {
		input    OperationType
		expected progrep.OperationType
	}{
		{input: OperationTypeCreate, expected: progrep.OperationTypeCreate},
		{input: OperationTypeUpdate, expected: progrep.OperationTypeUpdate},
		{input: OperationTypeDelete, expected: progrep.OperationTypeDelete},
		{input: OperationTypeApply, expected: progrep.OperationTypeApply},
		{input: OperationTypeRecreate, expected: progrep.OperationTypeRecreate},
		{input: OperationTypeTrackReadiness, expected: progrep.OperationTypeTrackReadiness},
		{input: OperationTypeTrackPresence, expected: progrep.OperationTypeTrackPresence},
		{input: OperationTypeTrackAbsence, expected: progrep.OperationTypeTrackAbsence},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			assert.Equal(t, tt.expected, mapOperationType(tt.input))
		})
	}
}

func TestAI_MapOperationType_PanicsOnUnknown(t *testing.T) {
	assert.Panics(t, func() {
		mapOperationType("unknown-type")
	})
}

func TestAI_NewLegacyProgressReporter_AcceptsBufferedChannel(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)

	assert.NotPanics(t, func() {
		r := NewLegacyProgressReporter(ch)
		assert.NotNil(t, r)
	})
}

func TestAI_NewLegacyProgressReporter_PanicsOnUnbufferedChannel(t *testing.T) {
	ch := make(chan progrep.ProgressReport)

	assert.Panics(t, func() {
		NewLegacyProgressReporter(ch)
	})
}

func TestAI_ReportOperationStatus_SetsStatusAndReports(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
	}
	p := buildTestPlan([]*Operation{op}, nil)
	reporter.startStage(p, map[string]string{op.ID(): "default"})
	drainChannel(ch)

	reportOperationStatus(op, OperationStatusPending, reporter)

	assert.Equal(t, OperationStatusPending, op.Status)

	reports := drainChannel(ch)
	require.NotEmpty(t, reports)

	activeOps := reports[len(reports)-1].StageReports[0].Operations
	require.Len(t, activeOps, 1)
	assert.Equal(t, progrep.OperationStatusProgressing, activeOps[0].Status)
}

func TestAI_ReportOperationStatus_SetsStatusWithoutReporter(t *testing.T) {
	op := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
	}

	reportOperationStatus(op, OperationStatusCompleted, nil)
	assert.Equal(t, OperationStatusCompleted, op.Status)
}

func TestAI_ReportStatus_SendsSnapshot(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	ops := []*Operation{
		{
			Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
			Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
		},
		{
			Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
			Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("svc1", "", gvkService)},
		},
	}
	p := buildTestPlan(ops, nil)

	resolvedNS := map[string]string{
		ops[0].ID(): "default",
		ops[1].ID(): "default",
	}

	reporter.startStage(p, resolvedNS)
	drainChannel(ch)

	reporter.ReportStatus(ops[0].ID(), progrep.OperationStatusCompleted)

	reports := drainChannel(ch)
	require.NotEmpty(t, reports, "expected at least one report after ReportStatus")

	last := reports[len(reports)-1]
	require.Len(t, last.StageReports, 1)

	activeOps := last.StageReports[0].Operations
	require.Len(t, activeOps, 2)

	opStatuses := map[string]progrep.OperationStatus{}
	for _, op := range activeOps {
		opStatuses[op.Name] = op.Status
	}

	assert.Equal(t, progrep.OperationStatusCompleted, opStatuses["cm1"])
	assert.Equal(t, progrep.OperationStatusPending, opStatuses["svc1"])
}

func TestAI_ReportStatus_UnknownOpIDIsIgnored(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	ops := []*Operation{
		{
			Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
			Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
		},
	}
	p := buildTestPlan(ops, nil)

	reporter.startStage(p, map[string]string{ops[0].ID(): "default"})
	drainChannel(ch)

	reporter.ReportStatus("nonexistent/op/id", progrep.OperationStatusCompleted)

	reports := drainChannel(ch)
	assert.Empty(t, reports, "expected no report for unknown op ID")
}

func TestAI_ReportStatus_WaitingForPopulation(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	opA := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
	}
	opB := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("svc1", "", gvkService)},
	}

	p := buildTestPlan([]*Operation{opA, opB}, map[int][]int{1: {0}})

	resolvedNS := map[string]string{
		opA.ID(): "default",
		opB.ID(): "default",
	}
	reporter.startStage(p, resolvedNS)

	reports := drainChannel(ch)
	require.NotEmpty(t, reports)

	last := reports[len(reports)-1]
	activeOps := last.StageReports[0].Operations

	var opBReport *progrep.Operation
	for i := range activeOps {
		if activeOps[i].Name == "svc1" {
			opBReport = &activeOps[i]
		}
	}

	require.NotNil(t, opBReport, "expected svc1 in operations")
	require.Len(t, opBReport.WaitingFor, 1, "svc1 should be waiting for cm1")
	assert.Equal(t, "cm1", opBReport.WaitingFor[0].Name)

	reporter.ReportStatus(opA.ID(), progrep.OperationStatusCompleted)
	reports = drainChannel(ch)
	require.NotEmpty(t, reports)

	last = reports[len(reports)-1]
	activeOps = last.StageReports[0].Operations

	opBReport = nil
	for i := range activeOps {
		if activeOps[i].Name == "svc1" {
			opBReport = &activeOps[i]
		}
	}

	require.NotNil(t, opBReport)
	assert.Empty(t, opBReport.WaitingFor, "svc1 should no longer be waiting after cm1 completed")
}

func TestAI_ResolveNamespace(t *testing.T) {
	mapper := newFakeRESTMapper()
	releaseNS := "release-ns"

	tests := []struct {
		name     string
		gvk      schema.GroupVersionKind
		ns       string
		expected string
	}{
		{
			name:     "namespaced with explicit namespace",
			gvk:      gvkConfigMap,
			ns:       "custom-ns",
			expected: "custom-ns",
		},
		{
			name:     "namespaced with empty namespace falls back to releaseNamespace",
			gvk:      gvkConfigMap,
			ns:       "",
			expected: releaseNS,
		},
		{
			name:     "cluster-scoped returns empty",
			gvk:      gvkNamespace,
			ns:       "",
			expected: "",
		},
		{
			name:     "cluster-scoped ignores explicit namespace",
			gvk:      gvkNamespace,
			ns:       "should-be-ignored",
			expected: "",
		},
		{
			name:     "unknown GVK with namespace uses as-is",
			gvk:      gvkCRD,
			ns:       "crd-ns",
			expected: "crd-ns",
		},
		{
			name:     "unknown GVK without namespace falls back to releaseNamespace",
			gvk:      gvkCRD,
			ns:       "",
			expected: releaseNS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveNamespace(tt.gvk, tt.ns, releaseNS, mapper)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestAI_SendNonBlocking_DropsWhenFull(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)

	ch <- progrep.ProgressReport{}

	sendNonBlocking(ch, progrep.ProgressReport{StageReports: []progrep.StageReport{{}}})

	assert.Len(t, ch, 1)

	msg := <-ch
	assert.Empty(t, msg.StageReports, "expected the original empty report, not the dropped one")
}

func TestAI_StartStage_FiltersNonResourceOps(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	ops := []*Operation{
		{
			Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
			Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
		},
		{
			Type: OperationTypeNoop, Version: OperationVersionNoop, Category: OperationCategoryMeta,
			Config: &OperationConfigNoop{OpID: "stage/start"},
		},
	}
	p := buildTestPlan(ops, nil)
	reporter.startStage(p, map[string]string{ops[0].ID(): "default"})

	reports := drainChannel(ch)
	require.NotEmpty(t, reports)

	last := reports[len(reports)-1]
	require.Len(t, last.StageReports, 1)

	assert.Len(t, last.StageReports[0].Operations, 1)
	assert.Equal(t, "cm1", last.StageReports[0].Operations[0].Name)
}

func TestAI_StartStage_FreezesPreviousStage(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	ops1 := []*Operation{
		{
			Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
			Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
		},
	}
	p1 := buildTestPlan(ops1, nil)
	reporter.startStage(p1, map[string]string{ops1[0].ID(): "default"})

	reporter.ReportStatus(ops1[0].ID(), progrep.OperationStatusCompleted)
	drainChannel(ch)

	ops2 := []*Operation{
		{
			Type: OperationTypeDelete, Version: OperationVersionDelete, Category: OperationCategoryResource,
			Config: &OperationConfigDelete{ResourceMeta: makeResourceMeta("svc1", "", gvkService)},
		},
	}
	p2 := buildTestPlan(ops2, nil)
	reporter.startStage(p2, map[string]string{ops2[0].ID(): "default"})

	reports := drainChannel(ch)
	require.NotEmpty(t, reports)

	last := reports[len(reports)-1]
	require.Len(t, last.StageReports, 2, "expected frozen stage + active stage")

	frozenOps := last.StageReports[0].Operations
	require.Len(t, frozenOps, 1)
	assert.Equal(t, "cm1", frozenOps[0].Name)
	assert.Equal(t, progrep.OperationStatusCompleted, frozenOps[0].Status)

	activeOps := last.StageReports[1].Operations
	require.Len(t, activeOps, 1)
	assert.Equal(t, "svc1", activeOps[0].Name)
	assert.Equal(t, progrep.OperationStatusPending, activeOps[0].Status)
}

func TestAI_Stop_SendsFinalReport(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	ops := []*Operation{
		{
			Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
			Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
		},
	}
	p := buildTestPlan(ops, nil)
	reporter.startStage(p, map[string]string{ops[0].ID(): "default"})
	reporter.ReportStatus(ops[0].ID(), progrep.OperationStatusCompleted)

	drainChannel(ch)

	ctx := context.Background()
	reporter.Stop(ctx)

	reports := drainChannel(ch)
	require.Len(t, reports, 1, "Stop should send exactly one final report")

	finalOps := reports[0].StageReports[0].Operations
	require.Len(t, finalOps, 1)
	assert.Equal(t, progrep.OperationStatusCompleted, finalOps[0].Status)
}

func TestAI_Stop_SkipsOnCanceledContext(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	ch <- progrep.ProgressReport{}

	reporter := NewLegacyProgressReporter(ch)

	ops := []*Operation{
		{
			Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
			Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
		},
	}
	p := buildTestPlan(ops, nil)
	reporter.startStage(p, map[string]string{ops[0].ID(): "default"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reporter.Stop(ctx)

	assert.Len(t, ch, 1)
}
