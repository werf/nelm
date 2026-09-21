//go:build ai_tests

package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/pkg/common"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
	"github.com/werf/nelm/pkg/legacy/progrep"
	"github.com/werf/nelm/pkg/release"
)

func TestAI_BuildResolvedNamespaces(t *testing.T) {
	mapper := newFakeRESTMapper()
	releaseNS := "release-ns"

	opNamespaced := createConfigMapOp("cm1")
	opNamespacedExplicit := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm2", "custom-ns", gvkConfigMap)},
	}
	opClusterScoped := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("my-ns", "", gvkNamespace)},
	}
	opMeta := stageMetaOp("stage/install/start")
	opTrack := &Operation{
		Type: OperationTypeTrackReadiness, Version: OperationVersionTrackReadiness, Category: OperationCategoryTrack,
		Config: &OperationConfigTrackReadiness{ResourceMeta: makeResourceMeta("dep1", "", gvkDeployment)},
	}

	p := buildTestPlan([]*Operation{opNamespaced, opNamespacedExplicit, opClusterScoped, opMeta, opTrack}, nil)

	resolved := buildResolvedNamespaces(p, releaseNS, mapper)

	assert.Equal(t, releaseNS, resolved[opNamespaced.ID()])
	assert.Equal(t, "custom-ns", resolved[opNamespacedExplicit.ID()])
	assert.Empty(t, resolved[opClusterScoped.ID()])

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
			name:     "Create",
			op:       createConfigMapOp("cm1"),
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
	assert.Panics(t, func() {
		extractObjectRef(stageMetaOp("stage/install/start"), map[string]string{})
	})
}

func TestAI_MapOperationCategory_PanicsOnUnknown(t *testing.T) {
	assert.Panics(t, func() {
		mapOperationCategory("unknown-category")
	})
}

func TestAI_MapOperationType_PanicsOnNoopWithoutStageSuffix(t *testing.T) {
	assert.Panics(t, func() {
		mapOperationType(stageMetaOp("something-else"))
	})
}

func TestAI_MapOperationType_PanicsOnUnknown(t *testing.T) {
	assert.Panics(t, func() {
		mapOperationType(&Operation{Type: "unknown-type"})
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

func TestAI_ProgressReport_JSONShape(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	start := stageMetaOp("stage/install/start")
	cm := createConfigMapOp("cm1")
	p := buildTestPlan([]*Operation{start, cm}, map[int][]int{1: {0}})

	startTestPlan(reporter, p, nil, StartPlanOptions{})

	reports := drainChannel(ch)
	require.NotEmpty(t, reports)

	raw, err := json.Marshal(reports[len(reports)-1])
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	require.Contains(t, decoded, "operations")
	assert.NotContains(t, decoded, "stageReports")

	operations, ok := decoded["operations"].([]any)
	require.True(t, ok)
	require.Len(t, operations, 2)

	startJSON, ok := operations[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, start.ID(), startJSON["id"])
	assert.Equal(t, "meta", startJSON["category"])
	assert.Equal(t, "StageStart", startJSON["type"])
	assert.Equal(t, "Pending", startJSON["status"])
	assert.Equal(t, []any{}, startJSON["dependsOn"], "dependsOn must be an empty array, not null")
	assert.NotContains(t, startJSON, "waitingFor")

	cmJSON, ok := operations[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, cm.ID(), cmJSON["id"])
	assert.Equal(t, "resource", cmJSON["category"])
	assert.Equal(t, []any{start.ID()}, cmJSON["dependsOn"])
	assert.Equal(t, "ConfigMap", cmJSON["Kind"])
	assert.Equal(t, "cm1", cmJSON["name"])
	assert.Equal(t, "default", cmJSON["namespace"])
}

func TestAI_ReportOperationStatus_SetsStatusAndReports(t *testing.T) {
	tests := []struct {
		planStatus   OperationStatus
		reportStatus progrep.OperationStatus
	}{
		{planStatus: OperationStatusUnknown, reportStatus: progrep.OperationStatusPending},
		{planStatus: OperationStatusPending, reportStatus: progrep.OperationStatusProgressing},
		{planStatus: OperationStatusCompleted, reportStatus: progrep.OperationStatusCompleted},
		{planStatus: OperationStatusFailed, reportStatus: progrep.OperationStatusFailed},
	}

	for _, tt := range tests {
		t.Run(string(tt.reportStatus), func(t *testing.T) {
			ch := make(chan progrep.ProgressReport, 64)
			reporter := NewLegacyProgressReporter(ch)

			op := createConfigMapOp("cm1")
			startTestPlan(reporter, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})
			drainChannel(ch)

			reportOperationStatus(op, tt.planStatus, reporter)

			assert.Equal(t, tt.planStatus, op.Status)

			ops := lastReportOperations(t, ch)
			require.Len(t, ops, 1)
			assert.Equal(t, tt.reportStatus, ops[0].Status)
		})
	}
}

func TestAI_ReportOperationStatus_SetsStatusWithoutReporter(t *testing.T) {
	op := createConfigMapOp("cm1")

	reportOperationStatus(op, OperationStatusCompleted, nil)
	assert.Equal(t, OperationStatusCompleted, op.Status)
}

func TestAI_ReportStatus_ConcurrentCallsAreSafe(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	reporter := NewLegacyProgressReporter(ch)

	ops := lo.Times(50, func(i int) *Operation {
		return createConfigMapOp(fmt.Sprintf("cm-%02d", i))
	})
	startTestPlan(reporter, buildTestPlan(ops, nil), nil, StartPlanOptions{})

	var wg sync.WaitGroup
	for _, op := range ops {
		wg.Add(1)

		go func(op *Operation) {
			defer wg.Done()

			reporter.ReportStatus(op.ID(), progrep.OperationStatusProgressing)
			reporter.ReportStatus(op.ID(), progrep.OperationStatusCompleted)
		}(op)
	}

	wg.Wait()
	drainChannel(ch)

	reporter.Stop(context.Background())

	final := lastReportOperations(t, ch)
	require.Len(t, final, 50)

	for _, op := range final {
		assert.Equal(t, progrep.OperationStatusCompleted, op.Status)
	}
}

func TestAI_ReportStatus_DoesNotPanicOnClosedChannel(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	p := buildTestPlan([]*Operation{op}, nil)
	startTestPlan(reporter, p, nil, StartPlanOptions{})

	close(ch)

	assert.NotPanics(t, func() {
		reporter.ReportStatus(op.ID(), progrep.OperationStatusCompleted)
	})
}

func TestAI_ReportStatus_KeepsOperationOrder(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	a := createConfigMapOp("cm-a")
	b := createConfigMapOp("cm-b")
	c := createConfigMapOp("cm-c")
	untouched := []*InstallableResourceInfo{makeUntouchedInfo("cm-u", "default", gvkConfigMap)}
	startTestPlan(reporter, buildTestPlan([]*Operation{a, b, c}, map[int][]int{2: {0, 1}}), untouched, StartPlanOptions{})

	before := operationIDs(lastReportOperations(t, ch))

	reporter.ReportStatus(c.ID(), progrep.OperationStatusCompleted)
	reporter.ReportStatus(a.ID(), progrep.OperationStatusFailed)

	assert.Equal(t, before, operationIDs(lastReportOperations(t, ch)))
}

func TestAI_ReportStatus_PreviousPlanOperationsNotAddressable(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op1 := createConfigMapOp("cm1")
	startTestPlan(reporter, buildTestPlan([]*Operation{op1}, nil), nil, StartPlanOptions{})

	op2 := createConfigMapOp("cm2")
	startTestPlan(reporter, buildTestPlan([]*Operation{op2}, nil), nil, StartPlanOptions{})
	drainChannel(ch)

	reporter.ReportStatus(op1.ID(), progrep.OperationStatusCompleted)
	assert.Empty(t, drainChannel(ch), "operations of an earlier plan must not be addressable")

	reporter.ReportStatus(op2.ID(), progrep.OperationStatusCompleted)

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 2)
	assert.Equal(t, progrep.OperationStatusCanceled, ops[0].Status, "the ignored status change did not leak into the previous plan")
	assert.Equal(t, progrep.OperationStatusCompleted, ops[1].Status)
}

func TestAI_ReportStatus_SendsSnapshot(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	cm := createConfigMapOp("cm1")
	svc := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("svc1", "", gvkService)},
	}
	p := buildTestPlan([]*Operation{cm, svc}, nil)

	startTestPlan(reporter, p, nil, StartPlanOptions{})
	drainChannel(ch)

	reporter.ReportStatus(cm.ID(), progrep.OperationStatusCompleted)

	ops := operationsByID(lastReportOperations(t, ch))
	require.Len(t, ops, 2)

	assert.Equal(t, progrep.OperationStatusCompleted, ops[cm.ID()].Status)
	assert.Equal(t, progrep.OperationStatusPending, ops[svc.ID()].Status)
}

func TestAI_ReportStatus_SentReportsAreImmutable(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	startTestPlan(reporter, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})

	initial := lastReportOperations(t, ch)
	require.Equal(t, progrep.OperationStatusPending, initial[0].Status)

	reporter.ReportStatus(op.ID(), progrep.OperationStatusCompleted)

	assert.Equal(t, progrep.OperationStatusPending, initial[0].Status, "a report handed to the consumer must not change afterwards")
	assert.Equal(t, progrep.OperationStatusCompleted, lastReportOperations(t, ch)[0].Status)
}

func TestAI_ReportStatus_SkipsSnapshotWhileChannelIsFull(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	startTestPlan(reporter, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})
	require.Len(t, ch, 1, "the initial report fills the channel")

	reporter.ReportStatus(op.ID(), progrep.OperationStatusCompleted)

	stale := <-ch
	assert.Equal(t, progrep.OperationStatusPending, stale.Operations[0].Status, "the report already in the channel is left as is")
	assert.Empty(t, ch, "no snapshot is queued while the consumer is behind")

	reporter.Stop(context.Background())

	final := <-ch
	assert.Equal(t, progrep.OperationStatusCompleted, final.Operations[0].Status, "the status change is kept and reaches the final report")
}

func TestAI_ReportStatus_UnknownOpIDIsIgnored(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	p := buildTestPlan([]*Operation{createConfigMapOp("cm1")}, nil)

	startTestPlan(reporter, p, nil, StartPlanOptions{})
	drainChannel(ch)

	reporter.ReportStatus("nonexistent/op/id", progrep.OperationStatusCompleted)

	assert.Empty(t, drainChannel(ch), "expected no report for unknown op ID")
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

func TestAI_ResolveNamespace_UnknownGVKFallbackIsFast(t *testing.T) {
	mapper := newFakeRESTMapper()
	startedAt := time.Now()

	got := resolveNamespace(gvkCRD, "", "release-ns", mapper)

	assert.Equal(t, "release-ns", got)
	assert.Less(t, time.Since(startedAt), 100*time.Millisecond)
}

func TestAI_SendNonBlocking_DoesNotPanicOnClosedChannel(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	close(ch)

	assert.NotPanics(t, func() {
		sendNonBlocking(ch, progrep.ProgressReport{})
	})
}

func TestAI_SendNonBlocking_DropsWhenFull(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)

	ch <- progrep.ProgressReport{}

	sendNonBlocking(ch, progrep.ProgressReport{Operations: []progrep.Operation{{}}})

	assert.Len(t, ch, 1)

	msg := <-ch
	assert.Empty(t, msg.Operations, "expected the original empty report, not the dropped one")
}

func TestAI_StartPlan_CancelsNothingAfterCompletedPlan(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	start := stageMetaOp("stage/install/start")
	a := createConfigMapOp("cm-a")
	b := createConfigMapOp("cm-b")
	end := stageMetaOp("stage/install/end")
	untouched := []*InstallableResourceInfo{makeUntouchedInfo("cm-untouched", "default", gvkConfigMap)}
	firstPlanOps := []*Operation{start, a, b, end}
	startTestPlan(reporter, buildTestPlan(firstPlanOps, map[int][]int{1: {0}, 2: {0}, 3: {1, 2}}), untouched, StartPlanOptions{})

	for _, op := range firstPlanOps {
		reporter.ReportStatus(op.ID(), progrep.OperationStatusCompleted)
	}

	drainChannel(ch)

	next := createConfigMapOp("cm-next")
	startTestPlan(reporter, buildTestPlan([]*Operation{next}, nil), nil, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, len(firstPlanOps)+2)

	for _, op := range ops {
		assert.NotEqual(t, progrep.OperationStatusCanceled, op.Status, "a fully completed plan leaves nothing to cancel: %s", op.ID)
	}

	byID := operationsByID(ops)
	for _, op := range firstPlanOps {
		assert.Equal(t, progrep.OperationStatusCompleted, byID[op.ID()].Status, op.ID())
	}

	assert.Equal(t, progrep.OperationStatusCompleted, byID["noop/1/0/default::ConfigMap:cm-untouched"].Status)
	assert.Equal(t, progrep.OperationStatusPending, byID["2/"+next.ID()].Status)
}

func TestAI_StartPlan_CancelsPendingOperationsOfPreviousPlan(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	done := createConfigMapOp("cm-done")
	failed := createConfigMapOp("cm-failed")
	neverStarted := createConfigMapOp("cm-never")
	untouched := []*InstallableResourceInfo{makeUntouchedInfo("cm-untouched", "default", gvkConfigMap)}
	startTestPlan(reporter, buildTestPlan([]*Operation{done, failed, neverStarted}, map[int][]int{1: {0}, 2: {1}}), untouched, StartPlanOptions{})

	reporter.ReportStatus(done.ID(), progrep.OperationStatusCompleted)
	reporter.ReportStatus(failed.ID(), progrep.OperationStatusFailed)
	drainChannel(ch)

	next := createConfigMapOp("cm-next")
	startTestPlan(reporter, buildTestPlan([]*Operation{next}, nil), nil, StartPlanOptions{})

	ops := operationsByID(lastReportOperations(t, ch))
	assert.Equal(t, progrep.OperationStatusCompleted, ops[done.ID()].Status)
	assert.Equal(t, progrep.OperationStatusFailed, ops[failed.ID()].Status)
	assert.Equal(t, progrep.OperationStatusCanceled, ops[neverStarted.ID()].Status, "an operation the previous plan never reached is canceled")
	assert.Equal(t, progrep.OperationStatusCompleted, ops["noop/1/0/default::ConfigMap:cm-untouched"].Status)
	assert.Equal(t, progrep.OperationStatusPending, ops["2/"+next.ID()].Status, "the new plan starts Pending")
}

func TestAI_StartPlan_DependsOnSortedByID(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	a := createConfigMapOp("cm-a")
	b := createConfigMapOp("cm-b")
	c := createConfigMapOp("cm-c")
	p := buildTestPlan([]*Operation{a, b, c}, map[int][]int{2: {1, 0}})

	startTestPlan(reporter, p, nil, StartPlanOptions{})

	ops := operationsByID(lastReportOperations(t, ch))
	assert.Equal(t, []string{a.ID(), b.ID()}, ops[c.ID()].DependsOn)
	assert.Empty(t, ops[a.ID()].DependsOn)
	assert.Empty(t, ops[b.ID()].DependsOn)
}

func TestAI_StartPlan_DependsOnUsesPrefixedIDsInLaterPlans(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	startTestPlan(reporter, buildTestPlan([]*Operation{createConfigMapOp("cm1")}, nil), nil, StartPlanOptions{})

	a := createConfigMapOp("cm-a")
	b := createConfigMapOp("cm-b")
	startTestPlan(reporter, buildTestPlan([]*Operation{a, b}, map[int][]int{1: {0}}), nil, StartPlanOptions{})

	ops := operationsByID(lastReportOperations(t, ch))
	require.Contains(t, ops, "2/"+b.ID())
	assert.Equal(t, []string{"2/" + a.ID()}, ops["2/"+b.ID()].DependsOn)
}

func TestAI_StartPlan_IncludesMetaAndReleaseOperations(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	start := stageMetaOp("stage/install/start")
	cm := createConfigMapOp("cm1")
	end := stageMetaOp("stage/install/end")
	rel := &Operation{
		Type: OperationTypeDeleteRelease, Version: OperationVersionDeleteRelease, Category: OperationCategoryRelease,
		Config: &OperationConfigDeleteRelease{ReleaseName: "rel", ReleaseNamespace: "default", ReleaseRevision: 1},
	}
	p := buildTestPlan([]*Operation{start, cm, end, rel}, map[int][]int{1: {0}, 2: {1}, 3: {2}})

	startTestPlan(reporter, p, nil, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Equal(t, []string{start.ID(), cm.ID(), end.ID(), rel.ID()}, operationIDs(ops))

	assert.Equal(t, progrep.OperationTypeStageStart, ops[0].Type)
	assert.Equal(t, progrep.OperationCategoryMeta, ops[0].Category)
	assert.Equal(t, progrep.ObjectRef{}, ops[0].ObjectRef)
	assert.Empty(t, ops[0].DependsOn)

	assert.Equal(t, progrep.OperationTypeCreate, ops[1].Type)
	assert.Equal(t, progrep.OperationCategoryResource, ops[1].Category)
	assert.Equal(t, []string{start.ID()}, ops[1].DependsOn)

	assert.Equal(t, progrep.OperationTypeStageEnd, ops[2].Type)
	assert.Equal(t, progrep.OperationCategoryMeta, ops[2].Category)
	assert.Equal(t, []string{cm.ID()}, ops[2].DependsOn)

	assert.Equal(t, progrep.OperationTypeDeleteRelease, ops[3].Type)
	assert.Equal(t, progrep.OperationCategoryRelease, ops[3].Category)
	assert.Equal(t, progrep.ObjectRef{}, ops[3].ObjectRef)
	assert.Equal(t, []string{end.ID()}, ops[3].DependsOn)

	for _, op := range ops {
		assert.Equal(t, progrep.OperationStatusPending, op.Status)
	}
}

func TestAI_StartPlan_LaterPlanRootsDependOnPreviousPlanSinks(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	a := createConfigMapOp("cm-a")
	b := createConfigMapOp("cm-b")
	c := createConfigMapOp("cm-c")
	untouched := []*InstallableResourceInfo{makeUntouchedInfo("cm-untouched", "default", gvkConfigMap)}
	startTestPlan(reporter, buildTestPlan([]*Operation{a, b, c}, map[int][]int{1: {0}}), untouched, StartPlanOptions{})

	x := createConfigMapOp("cm-x")
	y := createConfigMapOp("cm-y")
	z := createConfigMapOp("cm-z")
	startTestPlan(reporter, buildTestPlan([]*Operation{x, y, z}, map[int][]int{1: {0}}), untouched, StartPlanOptions{})

	ops := operationsByID(lastReportOperations(t, ch))

	previousSinks := []string{b.ID(), c.ID()}
	assert.Equal(t, previousSinks, ops["2/"+x.ID()].DependsOn, "root of the next plan depends on all sinks of the previous plan")
	assert.Equal(t, previousSinks, ops["2/"+z.ID()].DependsOn)
	assert.Equal(t, []string{"2/" + x.ID()}, ops["2/"+y.ID()].DependsOn, "non-root operations keep their own predecessors only")
	assert.Empty(t, ops["noop/1/0/default::ConfigMap:cm-untouched"].DependsOn, "untouched resources stay edgeless")
	assert.Empty(t, ops[a.ID()].DependsOn, "the first plan has nothing to depend on")
}

func TestAI_StartPlan_MetaOperationStatusReported(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	start := stageMetaOp("stage/install/start")
	p := buildTestPlan([]*Operation{start}, nil)

	startTestPlan(reporter, p, nil, StartPlanOptions{})
	drainChannel(ch)

	reporter.ReportStatus(start.ID(), progrep.OperationStatusCompleted)

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 1)
	assert.Equal(t, progrep.OperationStatusCompleted, ops[0].Status)
}

func TestAI_StartPlan_OperationFields(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	op.Iteration = 1
	p := buildTestPlan([]*Operation{op}, nil)

	startTestPlan(reporter, p, nil, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 1)

	assert.Equal(t, op.ID(), ops[0].ID)
	assert.Equal(t, "create/1/1/::ConfigMap:cm1", ops[0].ID)
	assert.Equal(t, progrep.OperationCategoryResource, ops[0].Category)
	assert.Equal(t, progrep.OperationTypeCreate, ops[0].Type)
	assert.Equal(t, 1, ops[0].Iteration)
	assert.Equal(t, progrep.OperationStatusPending, ops[0].Status)
	assert.Equal(t, gvkConfigMap, ops[0].GroupVersionKind)
	assert.Equal(t, "cm1", ops[0].Name)
	assert.Equal(t, "default", ops[0].Namespace)
	assert.Empty(t, ops[0].DependsOn)
}

func TestAI_StartPlan_OperationNamespaceResolution(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	defaulted := createConfigMapOp("cm1")
	explicit := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("cm2", "custom-ns", gvkConfigMap)},
	}
	clusterScoped := &Operation{
		Type: OperationTypeCreate, Version: OperationVersionCreate, Category: OperationCategoryResource,
		Config: &OperationConfigCreate{ResourceSpec: makeResourceSpec("my-ns", "ignored", gvkNamespace)},
	}
	unknownKind := &Operation{
		Type: OperationTypeTrackReadiness, Version: OperationVersionTrackReadiness, Category: OperationCategoryTrack,
		Config: &OperationConfigTrackReadiness{ResourceMeta: makeResourceMeta("widget", "", gvkCRD)},
	}

	p := buildTestPlan([]*Operation{defaulted, explicit, clusterScoped, unknownKind}, nil)
	reporter.StartPlan(p, "release-ns", nil, newFakeRESTMapper(), StartPlanOptions{})

	ops := operationsByID(lastReportOperations(t, ch))
	assert.Equal(t, "release-ns", ops[defaulted.ID()].Namespace)
	assert.Equal(t, "custom-ns", ops[explicit.ID()].Namespace)
	assert.Empty(t, ops[clusterScoped.ID()].Namespace)
	assert.Equal(t, "release-ns", ops[unknownKind.ID()].Namespace, "unknown kinds are assumed namespaced")
}

func TestAI_StartPlan_OperationTypesAndCategories(t *testing.T) {
	rel := &release.VersionedRelease{
		Accessor: lo.Must(helmrel.NewAccessor(&helmrelease.Release{
			Name:      "rel",
			Namespace: "default",
			Version:   1,
			Info:      &helmrelease.Info{},
		})),
	}

	tests := []struct {
		name         string
		op           *Operation
		wantType     progrep.OperationType
		wantCategory progrep.OperationCategory
	}{
		{
			name:         "Create",
			op:           createConfigMapOp("cm1"),
			wantType:     progrep.OperationTypeCreate,
			wantCategory: progrep.OperationCategoryResource,
		},
		{
			name: "Update",
			op: &Operation{
				Type: OperationTypeUpdate, Version: OperationVersionUpdate, Category: OperationCategoryResource,
				Config: &OperationConfigUpdate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
			},
			wantType:     progrep.OperationTypeUpdate,
			wantCategory: progrep.OperationCategoryResource,
		},
		{
			name: "Apply",
			op: &Operation{
				Type: OperationTypeApply, Version: OperationVersionApply, Category: OperationCategoryResource,
				Config: &OperationConfigApply{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
			},
			wantType:     progrep.OperationTypeApply,
			wantCategory: progrep.OperationCategoryResource,
		},
		{
			name: "Recreate",
			op: &Operation{
				Type: OperationTypeRecreate, Version: OperationVersionRecreate, Category: OperationCategoryResource,
				Config: &OperationConfigRecreate{ResourceSpec: makeResourceSpec("cm1", "", gvkConfigMap)},
			},
			wantType:     progrep.OperationTypeRecreate,
			wantCategory: progrep.OperationCategoryResource,
		},
		{
			name: "Delete",
			op: &Operation{
				Type: OperationTypeDelete, Version: OperationVersionDelete, Category: OperationCategoryResource,
				Config: &OperationConfigDelete{ResourceMeta: makeResourceMeta("cm1", "", gvkConfigMap)},
			},
			wantType:     progrep.OperationTypeDelete,
			wantCategory: progrep.OperationCategoryResource,
		},
		{
			name: "TrackReadiness",
			op: &Operation{
				Type: OperationTypeTrackReadiness, Version: OperationVersionTrackReadiness, Category: OperationCategoryTrack,
				Config: &OperationConfigTrackReadiness{ResourceMeta: makeResourceMeta("dep1", "", gvkDeployment)},
			},
			wantType:     progrep.OperationTypeTrackReadiness,
			wantCategory: progrep.OperationCategoryTrack,
		},
		{
			name: "TrackPresence",
			op: &Operation{
				Type: OperationTypeTrackPresence, Version: OperationVersionTrackPresence, Category: OperationCategoryTrack,
				Config: &OperationConfigTrackPresence{ResourceMeta: makeResourceMeta("svc1", "", gvkService)},
			},
			wantType:     progrep.OperationTypeTrackPresence,
			wantCategory: progrep.OperationCategoryTrack,
		},
		{
			name: "TrackAbsence",
			op: &Operation{
				Type: OperationTypeTrackAbsence, Version: OperationVersionTrackAbsence, Category: OperationCategoryTrack,
				Config: &OperationConfigTrackAbsence{ResourceMeta: makeResourceMeta("cm1", "", gvkConfigMap)},
			},
			wantType:     progrep.OperationTypeTrackAbsence,
			wantCategory: progrep.OperationCategoryTrack,
		},
		{
			name: "CreateRelease",
			op: &Operation{
				Type: OperationTypeCreateRelease, Version: OperationVersionCreateRelease, Category: OperationCategoryRelease,
				Config: &OperationConfigCreateRelease{Release: rel},
			},
			wantType:     progrep.OperationTypeCreateRelease,
			wantCategory: progrep.OperationCategoryRelease,
		},
		{
			name: "UpdateRelease",
			op: &Operation{
				Type: OperationTypeUpdateRelease, Version: OperationVersionUpdateRelease, Category: OperationCategoryRelease,
				Config: &OperationConfigUpdateRelease{Release: rel},
			},
			wantType:     progrep.OperationTypeUpdateRelease,
			wantCategory: progrep.OperationCategoryRelease,
		},
		{
			name: "DeleteRelease",
			op: &Operation{
				Type: OperationTypeDeleteRelease, Version: OperationVersionDeleteRelease, Category: OperationCategoryRelease,
				Config: &OperationConfigDeleteRelease{ReleaseName: "rel", ReleaseNamespace: "default", ReleaseRevision: 1},
			},
			wantType:     progrep.OperationTypeDeleteRelease,
			wantCategory: progrep.OperationCategoryRelease,
		},
		{
			name:         "StageStart",
			op:           stageMetaOp("stage/install/start"),
			wantType:     progrep.OperationTypeStageStart,
			wantCategory: progrep.OperationCategoryMeta,
		},
		{
			name:         "StageEnd",
			op:           stageMetaOp("stage/install/end"),
			wantType:     progrep.OperationTypeStageEnd,
			wantCategory: progrep.OperationCategoryMeta,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan progrep.ProgressReport, 64)
			reporter := NewLegacyProgressReporter(ch)

			startTestPlan(reporter, buildTestPlan([]*Operation{tt.op}, nil), nil, StartPlanOptions{})

			ops := lastReportOperations(t, ch)
			require.Len(t, ops, 1)
			assert.Equal(t, tt.op.ID(), ops[0].ID)
			assert.Equal(t, tt.wantType, ops[0].Type)
			assert.Equal(t, tt.wantCategory, ops[0].Category)
		})
	}
}

func TestAI_StartPlan_RealPlansFormSingleChainedGraph(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	preInstall := realInstallableInfo("cm-pre", common.StagePreInstall)
	install := realInstallableInfo("cm-main", common.StageInstall)
	install.MustDeleteOnFailedInstall = true

	untouched := realInstallableInfo("cm-untouched", common.StageInstall)
	untouched.MustInstall = ResourceInstallTypeNone
	untouched.MustTrackReadiness = false

	infos := []*InstallableResourceInfo{preInstall, install, untouched}

	relInfos := []*ReleaseInfo{{
		Release: &release.VersionedRelease{
			Accessor: lo.Must(helmrel.NewAccessor(&helmrelease.Release{
				Name:      "rel",
				Namespace: "default",
				Version:   1,
				Info:      &helmrelease.Info{},
			})),
		},
		Must:                   ReleaseTypeInstall,
		MustFailOnFailedDeploy: true,
	}}

	installPlan, err := BuildPlan(context.Background(), infos, nil, relInfos, "default", BuildPlanOptions{})
	require.NoError(t, err)

	startTestPlan(reporter, installPlan, infos, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	byID := operationsByID(ops)

	planOps := lo.Filter(ops, func(op progrep.Operation, _ int) bool {
		return op.Type != progrep.OperationTypeNoOp
	})
	assert.Len(t, planOps, len(installPlan.Operations()), "every plan operation is reported")
	assert.Equal(t, "noop/1/0/default::ConfigMap:cm-untouched", ops[0].ID, "untouched resource comes first")

	roots, sinks := graphRootsAndSinks(planOps)
	require.Equal(t, []string{stageOperationID(common.StageInit, common.StageStartSuffix)}, roots, "a real plan has a single root")
	require.Equal(t, []string{stageOperationID(common.StageFinal, common.StageEndSuffix)}, sinks, "a real plan has a single sink")

	createRelID := OperationID(OperationTypeCreateRelease, OperationVersionCreateRelease, 0, releaseID("default", "rel", 1))
	updateRelID := OperationID(OperationTypeUpdateRelease, OperationVersionUpdateRelease, 0, releaseID("default", "rel", 1))

	assert.Equal(t, progrep.OperationTypeCreateRelease, byID[createRelID].Type)
	assert.Equal(t, progrep.OperationCategoryRelease, byID[createRelID].Category)
	assert.Equal(t, progrep.OperationTypeUpdateRelease, byID[updateRelID].Type)
	assert.Equal(t, progrep.OperationCategoryRelease, byID[updateRelID].Category)

	preApplyID := OperationID(OperationTypeApply, OperationVersionApply, 0, preInstall.ID())
	mainApplyID := OperationID(OperationTypeApply, OperationVersionApply, 0, install.ID())
	mainTrackID := OperationID(OperationTypeTrackReadiness, OperationVersionTrackReadiness, 0, install.ID())

	assert.True(t, reachesThroughDependsOn(byID, mainApplyID, preApplyID), "cross-stage order survives through meta operations")
	assert.True(t, reachesThroughDependsOn(byID, updateRelID, mainTrackID), "release update depends on tracking")
	assert.False(t, reachesThroughDependsOn(byID, preApplyID, mainApplyID))

	for _, id := range []string{createRelID, preApplyID, mainApplyID} {
		reportOperationStatus(lo.Must(installPlan.Operation(id)), OperationStatusCompleted, reporter)
	}

	reportOperationStatus(lo.Must(installPlan.Operation(mainTrackID)), OperationStatusFailed, reporter)
	drainChannel(ch)

	failurePlan, err := BuildFailurePlan(installPlan, infos, relInfos, BuildFailurePlanOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, failurePlan.Operations())

	startTestPlan(reporter, failurePlan, infos, StartPlanOptions{})

	ops = lastReportOperations(t, ch)
	byID = operationsByID(ops)

	failureOps := lo.Filter(ops, func(op progrep.Operation, _ int) bool {
		return strings.HasPrefix(op.ID, "2/")
	})
	assert.Len(t, failureOps, len(failurePlan.Operations()))
	assert.Len(t, ops, len(planOps)+1+len(failureOps), "install plan, untouched and failure plan are all kept")

	failureRoots, _ := graphRootsAndSinks(failureOps)
	require.Len(t, failureRoots, 1, "a real failure plan has a single root")
	assert.Equal(t, sinks, byID[failureRoots[0]].DependsOn, "the failure plan continues from the sink of the install plan")

	failureDeleteID := "2/" + OperationID(OperationTypeDelete, OperationVersionDelete, 0, install.ID())
	assert.True(t, reachesThroughDependsOn(byID, failureDeleteID, mainTrackID), "the whole run is one connected graph")
	assert.Equal(t, progrep.OperationStatusFailed, byID[mainTrackID].Status)
	assert.Equal(t, progrep.OperationStatusCanceled, byID[sinks[0]].Status, "the install plan never reached its end")
	assert.Equal(t, progrep.OperationStatusCanceled, byID[updateRelID].Status)
	assert.Equal(t, progrep.OperationStatusCompleted, byID[mainApplyID].Status)
	assert.Equal(t, progrep.OperationStatusPending, byID[failureDeleteID].Status)
}

func TestAI_StartPlan_SecondPlanAppendedWithPrefix(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op1 := createConfigMapOp("cm1")
	startTestPlan(reporter, buildTestPlan([]*Operation{op1}, nil), nil, StartPlanOptions{})
	reporter.ReportStatus(op1.ID(), progrep.OperationStatusCompleted)
	drainChannel(ch)

	op2 := &Operation{
		Type: OperationTypeDelete, Version: OperationVersionDelete, Category: OperationCategoryResource,
		Config: &OperationConfigDelete{ResourceMeta: makeResourceMeta("svc1", "", gvkService)},
	}
	startTestPlan(reporter, buildTestPlan([]*Operation{op2}, nil), nil, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Equal(t, []string{op1.ID(), "2/" + op2.ID()}, operationIDs(ops))

	assert.Equal(t, progrep.OperationStatusCompleted, ops[0].Status)
	assert.Equal(t, "cm1", ops[0].Name)

	assert.Equal(t, progrep.OperationStatusPending, ops[1].Status)
	assert.Equal(t, "svc1", ops[1].Name)
	assert.Equal(t, progrep.OperationTypeDelete, ops[1].Type)
}

func TestAI_StartPlan_SkippedPlanDoesNotBreakChain(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op1 := createConfigMapOp("cm1")
	startTestPlan(reporter, buildTestPlan([]*Operation{op1}, nil), nil, StartPlanOptions{})

	skipped := stageMetaOp("stage/install/start")
	startTestPlan(reporter, buildTestPlan([]*Operation{skipped}, nil), nil, StartPlanOptions{UntouchedResourcesOnly: true})

	op3 := createConfigMapOp("cm3")
	startTestPlan(reporter, buildTestPlan([]*Operation{op3}, nil), nil, StartPlanOptions{})

	ops := operationsByID(lastReportOperations(t, ch))
	require.Contains(t, ops, "3/"+op3.ID())
	assert.Equal(t, []string{op1.ID()}, ops["3/"+op3.ID()].DependsOn, "a plan without reported operations is skipped over by the chain")
}

func TestAI_StartPlan_ThirdPlanGetsOwnPrefix(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")

	startTestPlan(reporter, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})
	startTestPlan(reporter, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})
	startTestPlan(reporter, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	assert.Equal(t, []string{op.ID(), "2/" + op.ID(), "3/" + op.ID()}, operationIDs(ops))
}

func TestAI_StartPlan_TopologicalOrderIsDeterministic(t *testing.T) {
	a := createConfigMapOp("cm-a")
	b := createConfigMapOp("cm-b")
	c := createConfigMapOp("cm-c")
	d := createConfigMapOp("cm-d")

	var firstOrder []string

	for i := 0; i < 20; i++ {
		ch := make(chan progrep.ProgressReport, 64)
		reporter := NewLegacyProgressReporter(ch)

		p := buildTestPlan([]*Operation{d, c, b, a}, map[int][]int{1: {3, 2}})
		startTestPlan(reporter, p, nil, StartPlanOptions{})

		order := operationIDs(lastReportOperations(t, ch))
		require.Len(t, order, 4)

		assert.Less(t, lo.IndexOf(order, a.ID()), lo.IndexOf(order, c.ID()), "a precedes its dependent c")
		assert.Less(t, lo.IndexOf(order, b.ID()), lo.IndexOf(order, c.ID()), "b precedes its dependent c")

		if firstOrder == nil {
			firstOrder = order

			continue
		}

		require.Equal(t, firstOrder, order, "order must not depend on map iteration")
	}
}

func TestAI_StartPlan_UntouchedAbsentResourceIncluded(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	p := buildTestPlan([]*Operation{op}, nil)

	absent := &InstallableResourceInfo{
		ResourceMeta: makeResourceMeta("cm2", "default", gvkConfigMap),
		MustInstall:  ResourceInstallTypeNone,
	}

	startTestPlan(reporter, p, []*InstallableResourceInfo{absent}, StartPlanOptions{})

	ops := operationsByID(lastReportOperations(t, ch))
	require.Len(t, ops, 2, "a resource without operations is untouched even if absent from the cluster")

	absentID := OperationID(OperationTypeNoop, OperationVersionNoop, 0, absent.ID())
	require.Contains(t, ops, absentID)
	assert.Equal(t, progrep.OperationTypeNoOp, ops[absentID].Type)
	assert.Equal(t, progrep.OperationStatusCompleted, ops[absentID].Status)
}

func TestAI_StartPlan_UntouchedDeduplicatedAcrossIterations(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	op.Iteration = 1
	p := buildTestPlan([]*Operation{op}, nil)

	iterationZero := makeUntouchedInfo("cm1", "default", gvkConfigMap)

	startTestPlan(reporter, p, []*InstallableResourceInfo{iterationZero}, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 1, "a resource deployed in a later iteration is not untouched")
	assert.Equal(t, op.ID(), ops[0].ID)
}

func TestAI_StartPlan_UntouchedDeduplicatedAgainstPlanOp(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := &Operation{
		Type: OperationTypeTrackReadiness, Version: OperationVersionTrackReadiness, Category: OperationCategoryTrack,
		Config: &OperationConfigTrackReadiness{ResourceMeta: makeResourceMeta("dep1", "default", gvkDeployment)},
	}
	p := buildTestPlan([]*Operation{op}, nil)

	untouched := makeUntouchedInfo("dep1", "default", gvkDeployment)

	startTestPlan(reporter, p, []*InstallableResourceInfo{untouched}, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 1, "force-tracked untouched resource must appear exactly once via its plan op")
	assert.Equal(t, op.ID(), ops[0].ID)
	assert.Equal(t, progrep.OperationStatusPending, ops[0].Status)
}

func TestAI_StartPlan_UntouchedDeduplicatedByObjectRefNotByInfoNamespace(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	p := buildTestPlan(nil, nil)

	explicitNS := makeUntouchedInfo("cm1", "default", gvkConfigMap)
	defaultedNS := makeUntouchedInfo("cm1", "", gvkConfigMap)

	startTestPlan(reporter, p, []*InstallableResourceInfo{explicitNS, defaultedNS}, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 1, "infos resolving to the same object must be emitted once")
	assert.Equal(t, "default", ops[0].Namespace)
}

func TestAI_StartPlan_UntouchedFields(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	p := buildTestPlan([]*Operation{op}, nil)

	untouched := makeUntouchedInfo("cm2", "default", gvkConfigMap)

	startTestPlan(reporter, p, []*InstallableResourceInfo{untouched}, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 2)

	untouchedOp := ops[0]
	assert.Equal(t, "noop/1/0/default::ConfigMap:cm2", untouchedOp.ID)
	assert.Equal(t, OperationID(OperationTypeNoop, OperationVersionNoop, 0, untouched.ID()), untouchedOp.ID)
	assert.Equal(t, progrep.OperationCategoryResource, untouchedOp.Category)
	assert.Equal(t, progrep.OperationTypeNoOp, untouchedOp.Type)
	assert.Equal(t, 0, untouchedOp.Iteration)
	assert.Equal(t, progrep.OperationStatusCompleted, untouchedOp.Status)
	assert.Equal(t, gvkConfigMap, untouchedOp.GroupVersionKind)
	assert.Equal(t, "cm2", untouchedOp.Name)
	assert.Equal(t, "default", untouchedOp.Namespace)
	assert.NotNil(t, untouchedOp.DependsOn)
	assert.Empty(t, untouchedOp.DependsOn)
}

func TestAI_StartPlan_UntouchedFirstSortedByID(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm-a")
	p := buildTestPlan([]*Operation{op}, nil)

	untouched := []*InstallableResourceInfo{
		makeUntouchedInfo("cm-z", "default", gvkConfigMap),
		makeUntouchedInfo("cm-m", "default", gvkConfigMap),
	}

	startTestPlan(reporter, p, untouched, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	assert.Equal(t, []string{
		"noop/1/0/default::ConfigMap:cm-m",
		"noop/1/0/default::ConfigMap:cm-z",
		op.ID(),
	}, operationIDs(ops))
}

func TestAI_StartPlan_UntouchedIgnoredInLaterPlans(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	untouched := []*InstallableResourceInfo{makeUntouchedInfo("cm1", "default", gvkConfigMap)}

	startTestPlan(reporter, buildTestPlan(nil, nil), untouched, StartPlanOptions{})

	op := createConfigMapOp("cm2")
	startTestPlan(reporter, buildTestPlan([]*Operation{op}, nil), untouched, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	assert.Equal(t, []string{
		"noop/1/0/default::ConfigMap:cm1",
		"2/" + op.ID(),
	}, operationIDs(ops), "only the first plan contributes untouched resources")
}

func TestAI_StartPlan_UntouchedInventoryDeduplicated(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	p := buildTestPlan(nil, nil)

	untouched1 := makeUntouchedInfo("cm1", "default", gvkConfigMap)
	untouched2 := makeUntouchedInfo("cm1", "default", gvkConfigMap)

	startTestPlan(reporter, p, []*InstallableResourceInfo{untouched1, untouched2}, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 1, "duplicate untouched infos must be emitted once")
	assert.Equal(t, "cm1", ops[0].Name)
}

func TestAI_StartPlan_UntouchedIterationsGetDistinctIDs(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	gvkWebhookV1 := schema.GroupVersionKind{Group: "admissionregistration.k8s.io", Version: "v1", Kind: "MutatingWebhookConfiguration"}
	gvkWebhookV1beta1 := schema.GroupVersionKind{Group: "admissionregistration.k8s.io", Version: "v1beta1", Kind: "MutatingWebhookConfiguration"}

	first := makeUntouchedInfo("hook", "", gvkWebhookV1)
	second := makeUntouchedInfo("hook", "", gvkWebhookV1beta1)
	second.Iteration = 1

	startTestPlan(reporter, buildTestPlan(nil, nil), []*InstallableResourceInfo{first, second}, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 2, "same-named resources of different API versions are distinct objects")

	assert.Equal(t, []string{
		"noop/1/0/:admissionregistration.k8s.io:MutatingWebhookConfiguration:hook",
		"noop/1/1/:admissionregistration.k8s.io:MutatingWebhookConfiguration:hook",
	}, operationIDs(ops))
	assert.Equal(t, 0, ops[0].Iteration)
	assert.Equal(t, 1, ops[1].Iteration)
}

func TestAI_StartPlan_UntouchedKeptWhenLaterPlanTouchesResource(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	untouched := []*InstallableResourceInfo{
		makeUntouchedInfo("cm-untouched", "default", gvkConfigMap),
		makeUntouchedInfo("svc-shared", "default", gvkService),
	}

	mainOp := createConfigMapOp("cm-main")
	startTestPlan(reporter, buildTestPlan([]*Operation{mainOp}, nil), untouched, StartPlanOptions{})
	drainChannel(ch)

	failureOp := &Operation{
		Type: OperationTypeDelete, Version: OperationVersionDelete, Category: OperationCategoryResource,
		Config: &OperationConfigDelete{ResourceMeta: makeResourceMeta("svc-shared", "default", gvkService)},
	}
	startTestPlan(reporter, buildTestPlan([]*Operation{failureOp}, nil), untouched, StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Equal(t, []string{
		"noop/1/0/default::ConfigMap:cm-untouched",
		"noop/1/0/default::Service:svc-shared",
		mainOp.ID(),
		"2/" + failureOp.ID(),
	}, operationIDs(ops))

	assert.Equal(t, progrep.OperationStatusCompleted, ops[1].Status, "the untouched entry of the first plan is a fact of history and stays")
	assert.Equal(t, progrep.OperationTypeNoOp, ops[1].Type)
	assert.Equal(t, progrep.OperationStatusPending, ops[3].Status)
	assert.Equal(t, progrep.OperationTypeDelete, ops[3].Type)
}

func TestAI_StartPlan_UntouchedNamespaceResolution(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	explicit := makeUntouchedInfo("cm1", "custom-ns", gvkConfigMap)
	defaulted := makeUntouchedInfo("cm2", "", gvkConfigMap)
	clusterScoped := makeUntouchedInfo("my-ns", "", gvkNamespace)

	reporter.StartPlan(buildTestPlan(nil, nil), "release-ns", []*InstallableResourceInfo{explicit, defaulted, clusterScoped}, newFakeRESTMapper(), StartPlanOptions{})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 3)

	namespaces := map[string]string{}
	for _, o := range ops {
		namespaces[o.Name] = o.Namespace
	}

	assert.Equal(t, "custom-ns", namespaces["cm1"])
	assert.Equal(t, "release-ns", namespaces["cm2"])
	assert.Empty(t, namespaces["my-ns"])
}

func TestAI_StartPlan_UntouchedNotAddressableByReportStatus(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	p := buildTestPlan([]*Operation{op}, nil)

	untouched := makeUntouchedInfo("cm2", "default", gvkConfigMap)

	startTestPlan(reporter, p, []*InstallableResourceInfo{untouched}, StartPlanOptions{})
	drainChannel(ch)

	reporter.ReportStatus(OperationID(OperationTypeNoop, OperationVersionNoop, 0, untouched.ID()), progrep.OperationStatusFailed)
	assert.Empty(t, drainChannel(ch), "untouched entry ID must not be addressable by ReportStatus")

	reporter.ReportStatus(op.ID(), progrep.OperationStatusCompleted)

	ops := operationsByID(lastReportOperations(t, ch))
	require.Len(t, ops, 2)
	assert.Equal(t, progrep.OperationStatusCompleted, ops[op.ID()].Status)
	assert.Equal(t, progrep.OperationStatusCompleted, ops["noop/1/0/default::ConfigMap:cm2"].Status)
}

func TestAI_StartPlan_UntouchedResourcesOnly(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	start := stageMetaOp("stage/install/start")
	end := stageMetaOp("stage/install/end")
	rel := &Operation{
		Type: OperationTypeDeleteRelease, Version: OperationVersionDeleteRelease, Category: OperationCategoryRelease,
		Config: &OperationConfigDeleteRelease{ReleaseName: "rel", ReleaseNamespace: "default", ReleaseRevision: 1},
	}
	p := buildTestPlan([]*Operation{start, end, rel}, map[int][]int{1: {0}, 2: {1}})

	untouched := []*InstallableResourceInfo{
		makeUntouchedInfo("cm1", "default", gvkConfigMap),
		makeUntouchedInfo("cm2", "default", gvkConfigMap),
	}

	startTestPlan(reporter, p, untouched, StartPlanOptions{UntouchedResourcesOnly: true})

	ops := lastReportOperations(t, ch)
	require.Len(t, ops, 2, "a plan that is not executed contributes no operations of its own")

	for _, op := range ops {
		assert.Equal(t, progrep.OperationTypeNoOp, op.Type)
		assert.Equal(t, progrep.OperationStatusCompleted, op.Status)
	}

	reporter.ReportStatus(start.ID(), progrep.OperationStatusCompleted)
	assert.Empty(t, drainChannel(ch), "omitted plan operations must not be addressable")
}

func TestAI_Stop_CancelsPendingOperations(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	failed := createConfigMapOp("cm-failed")
	neverStarted := createConfigMapOp("cm-never")
	startTestPlan(reporter, buildTestPlan([]*Operation{failed, neverStarted}, map[int][]int{1: {0}}), nil, StartPlanOptions{})

	reporter.ReportStatus(failed.ID(), progrep.OperationStatusFailed)
	drainChannel(ch)

	reporter.Stop(context.Background())

	reports := drainChannel(ch)
	require.Len(t, reports, 1)

	ops := operationsByID(reports[0].Operations)
	assert.Equal(t, progrep.OperationStatusFailed, ops[failed.ID()].Status)
	assert.Equal(t, progrep.OperationStatusCanceled, ops[neverStarted.ID()].Status, "the final report leaves nothing Pending")
}

func TestAI_Stop_DoesNotPanicOnClosedChannel(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	reporter := NewLegacyProgressReporter(ch)

	startTestPlan(reporter, buildTestPlan([]*Operation{createConfigMapOp("cm1")}, nil), nil, StartPlanOptions{})

	close(ch)

	assert.NotPanics(t, func() {
		reporter.Stop(context.Background())
	})
}

func TestAI_Stop_LeavesCompletedOperationsAlone(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	untouched := []*InstallableResourceInfo{makeUntouchedInfo("cm2", "default", gvkConfigMap)}
	startTestPlan(reporter, buildTestPlan([]*Operation{op}, nil), untouched, StartPlanOptions{})
	reporter.ReportStatus(op.ID(), progrep.OperationStatusCompleted)
	drainChannel(ch)

	reporter.Stop(context.Background())

	reports := drainChannel(ch)
	require.Len(t, reports, 1)

	for _, o := range reports[0].Operations {
		assert.Equal(t, progrep.OperationStatusCompleted, o.Status)
	}
}

func TestAI_Stop_ReportsAllPlans(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op1 := createConfigMapOp("cm1")
	startTestPlan(reporter, buildTestPlan([]*Operation{op1}, nil), nil, StartPlanOptions{})
	reporter.ReportStatus(op1.ID(), progrep.OperationStatusFailed)

	op2 := createConfigMapOp("cm2")
	startTestPlan(reporter, buildTestPlan([]*Operation{op2}, nil), nil, StartPlanOptions{})
	reporter.ReportStatus(op2.ID(), progrep.OperationStatusCompleted)
	drainChannel(ch)

	reporter.Stop(context.Background())

	reports := drainChannel(ch)
	require.Len(t, reports, 1)
	require.Equal(t, []string{op1.ID(), "2/" + op2.ID()}, operationIDs(reports[0].Operations))
	assert.Equal(t, progrep.OperationStatusFailed, reports[0].Operations[0].Status)
	assert.Equal(t, progrep.OperationStatusCompleted, reports[0].Operations[1].Status)
}

func TestAI_Stop_SendsFinalReport(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 64)
	reporter := NewLegacyProgressReporter(ch)

	op := createConfigMapOp("cm1")
	startTestPlan(reporter, buildTestPlan([]*Operation{op}, nil), nil, StartPlanOptions{})
	reporter.ReportStatus(op.ID(), progrep.OperationStatusCompleted)

	drainChannel(ch)

	reporter.Stop(context.Background())

	reports := drainChannel(ch)
	require.Len(t, reports, 1, "Stop should send exactly one final report")

	finalOps := reports[0].Operations
	require.Len(t, finalOps, 1)
	assert.Equal(t, progrep.OperationStatusCompleted, finalOps[0].Status)
}

func TestAI_Stop_SkipsOnCanceledContext(t *testing.T) {
	ch := make(chan progrep.ProgressReport, 1)
	ch <- progrep.ProgressReport{}

	reporter := NewLegacyProgressReporter(ch)

	startTestPlan(reporter, buildTestPlan([]*Operation{createConfigMapOp("cm1")}, nil), nil, StartPlanOptions{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	reporter.Stop(ctx)

	assert.Len(t, ch, 1)
}
