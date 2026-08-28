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
	OperationTypeNoOp           OperationType = "NoOp"
	OperationTypeTrackReadiness OperationType = "TrackReadiness"
	OperationTypeTrackPresence  OperationType = "TrackPresence"
	OperationTypeTrackAbsence   OperationType = "TrackAbsence"
)

type OperationType string

type OperationStatus string

// ProgressReport contains stage reports ordered chronologically; the last element is the
// currently active stage.
type ProgressReport struct {
	StageReports []StageReport `json:"stageReports"`
}

// StageReport contains ALL operations in the plan -- from the very first report, every
// operation is present (initially as Pending). A stage of a plan that describes a complete
// desired state, such as an install or a rollback plan, additionally lists the resources that
// plan leaves untouched, as NoOp with status Completed. Each stage computes that set from its
// own plan. A failure plan acts upon a few resources only, so its stage lists just its own
// operations.
type StageReport struct {
	Operations []Operation `json:"operations"`
}

type Operation struct {
	OperationRef

	Status     OperationStatus `json:"status"`
	WaitingFor []OperationRef  `json:"waitingFor"`
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
