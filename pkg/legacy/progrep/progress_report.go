package progrep

import "k8s.io/apimachinery/pkg/runtime/schema"

const (
	OperationStatusPending     OperationStatus = "Pending"
	OperationStatusProgressing OperationStatus = "Progressing"
	OperationStatusCompleted   OperationStatus = "Completed"
	OperationStatusFailed      OperationStatus = "Failed"

	OperationTypeCreate         OperationType = "Create"
	OperationTypeUpdate         OperationType = "Update"
	OperationTypeDelete         OperationType = "Delete"
	OperationTypeApply          OperationType = "Apply"
	OperationTypeRecreate       OperationType = "Recreate"
	OperationTypeTrackReadiness OperationType = "TrackReadiness"
	OperationTypeTrackPresence  OperationType = "TrackPresence"
	OperationTypeTrackAbsence   OperationType = "TrackAbsence"
)

type OperationType string

type OperationStatus string

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

	Status     OperationStatus
	Type       OperationType
	WaitingFor []ObjectRef
}

type ObjectRef struct {
	schema.GroupVersionKind

	Name      string
	Namespace string
}
