package progrep

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	OperationCategoryMeta     OperationCategory = "meta"
	OperationCategoryResource OperationCategory = "resource"
	OperationCategoryTrack    OperationCategory = "track"
	OperationCategoryRelease  OperationCategory = "release"

	OperationStatusPending     OperationStatus = "Pending"
	OperationStatusProgressing OperationStatus = "Progressing"
	OperationStatusCompleted   OperationStatus = "Completed"
	OperationStatusFailed      OperationStatus = "Failed"
	OperationStatusCanceled    OperationStatus = "Canceled"

	OperationTypeCreate         OperationType = "Create"
	OperationTypeUpdate         OperationType = "Update"
	OperationTypeDelete         OperationType = "Delete"
	OperationTypeApply          OperationType = "Apply"
	OperationTypeRecreate       OperationType = "Recreate"
	OperationTypeNoOp           OperationType = "NoOp"
	OperationTypeTrackReadiness OperationType = "TrackReadiness"
	OperationTypeTrackPresence  OperationType = "TrackPresence"
	OperationTypeTrackAbsence   OperationType = "TrackAbsence"
	OperationTypeStageStart     OperationType = "StageStart"
	OperationTypeStageEnd       OperationType = "StageEnd"
	OperationTypeCreateRelease  OperationType = "CreateRelease"
	OperationTypeUpdateRelease  OperationType = "UpdateRelease"
	OperationTypeDeleteRelease  OperationType = "DeleteRelease"
)

type OperationCategory string

type OperationType string

type OperationStatus string

// ProgressReport lists ALL operations of every plan executed so far, in execution order: from
// the very first report every operation of the current plan is present (initially as Pending),
// and operations of later plans (e.g. a failure plan) are appended after the earlier ones. The
// plans form a single graph: the root operations of a later plan depend on the final operations
// of the plan before it. Operations of a finished plan that were never started are Canceled. The first plan describes the complete desired state of the release, so
// the resources it leaves untouched are listed too, as NoOp with status Completed, at the
// beginning of the slice. Later plans, e.g. a failure plan, never add untouched resources.
//
// An untouched resource is one the plan has no operations for. NoOp/Completed means only that
// nothing was done to the resource during the release: the resource may be absent from the
// cluster (e.g. creation skipped by a resource policy) or differ from the chart (e.g. update
// skipped by a resource policy). It is not a statement about the resource being present or
// in the desired state.
type ProgressReport struct {
	Operations []Operation `json:"operations"`
}

// Operation ID is unique within a report and is referenced by DependsOn of other operations.
// Operations of the first plan carry the plan's own operation ID; operations of every following
// plan are prefixed with the plan's ordinal number, e.g. "2/apply/1/0/...". Meta and release
// operations have an empty ObjectRef.
type Operation struct {
	OperationRef

	ID        string            `json:"id"`
	Category  OperationCategory `json:"category"`
	Status    OperationStatus   `json:"status"`
	DependsOn []string          `json:"dependsOn"`
}

type OperationRef struct {
	ObjectRef

	Type      OperationType `json:"type"`
	Iteration int           `json:"iteration"`
}

type ObjectRef struct {
	schema.GroupVersionKind

	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}
