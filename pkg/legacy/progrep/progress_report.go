package progrep

import "k8s.io/apimachinery/pkg/runtime/schema"

// ProgressReport contains stage reports ordered chronologically; the last element is the
// currently active stage.
type ProgressReport struct {
	StageReports []StageReport
}

// StageReport contains ALL operations in the plan -- from the very first report, every
// operation is present (initially as Pending).
type StageReport struct {
	Operations []Operation
}

type Operation struct {
	ObjectRef

	Type       OperationType
	Status     OperationStatus
	WaitingFor []ObjectRef
}

type OperationType string

const (
	OperationTypeCreate         OperationType = "Create"
	OperationTypeUpdate         OperationType = "Update"
	OperationTypeDelete         OperationType = "Delete"
	OperationTypeApply          OperationType = "Apply"
	OperationTypeRecreate       OperationType = "Recreate"
	OperationTypeTrackReadiness OperationType = "TrackReadiness"
	OperationTypeTrackPresence  OperationType = "TrackPresence"
	OperationTypeTrackAbsence   OperationType = "TrackAbsence"
)

type OperationStatus string

const (
	OperationStatusPending     OperationStatus = "Pending"
	OperationStatusProgressing OperationStatus = "Progressing"
	OperationStatusCompleted   OperationStatus = "Completed"
	OperationStatusFailed      OperationStatus = "Failed"
)

type ObjectRef struct {
	schema.GroupVersionKind

	Name      string
	Namespace string
}
