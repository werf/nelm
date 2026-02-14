package plan

import (
	"encoding/json"
	"fmt"
	"strings"
)

type OperationCategory string

const (
	// Does nothing. Used for things like  grouping.
	OperationCategoryMeta OperationCategory = "meta"
	// Operations that mutate Kubernetes resources in the cluster.
	OperationCategoryResource OperationCategory = "resource"
	// Operations that track resources in the cluster. Never mutate anything.
	OperationCategoryTrack OperationCategory = "track"
	// Operations that mutate Helm releases in the cluster.
	OperationCategoryRelease OperationCategory = "release"
)

type OperationType string

const (
	OperationTypeApply          OperationType = "apply"
	OperationTypeCreate         OperationType = "create"
	OperationTypeCreateRelease  OperationType = "create-release"
	OperationTypeDelete         OperationType = "delete"
	OperationTypeDeleteRelease  OperationType = "delete-release"
	OperationTypeNoop           OperationType = "noop"
	OperationTypeRecreate       OperationType = "recreate"
	OperationTypeTrackAbsence   OperationType = "track-absence"
	OperationTypeTrackPresence  OperationType = "track-presence"
	OperationTypeTrackReadiness OperationType = "track-readiness"
	OperationTypeUpdate         OperationType = "update"
	OperationTypeUpdateRelease  OperationType = "update-release"
)

// Used to handle breaking changes in the Operation struct.
type OperationVersion int

const (
	OperationVersionApply          OperationVersion = 1
	OperationVersionCreate         OperationVersion = 1
	OperationVersionCreateRelease  OperationVersion = 1
	OperationVersionDelete         OperationVersion = 1
	OperationVersionDeleteRelease  OperationVersion = 1
	OperationVersionNoop           OperationVersion = 1
	OperationVersionRecreate       OperationVersion = 1
	OperationVersionTrackAbsence   OperationVersion = 1
	OperationVersionTrackPresence  OperationVersion = 1
	OperationVersionTrackReadiness OperationVersion = 1
	OperationVersionUpdate         OperationVersion = 1
	OperationVersionUpdateRelease  OperationVersion = 1
)

type OperationStatus string

const (
	OperationStatusUnknown   OperationStatus = ""
	OperationStatusPending   OperationStatus = "pending"
	OperationStatusCompleted OperationStatus = "completed"
	OperationStatusFailed    OperationStatus = "failed"
)

// Helps to avoid operation ID collisions. Since you can't have two operations with the same ID in
// the graph, you can increment the iteration to get a new unique ID for the operation. The higher
// the iteration, the later in the plan/graph the operation should appear.
type OperationIteration int

// Represents an operation on a resource, such as create, update, track readiness, etc. The
// operation ID must be unique: you can't have two operations with the same ID in the plan/graph.
// Operation must be easily serializable.
type Operation struct {
	Type      OperationType      `json:"type"`
	Version   OperationVersion   `json:"version"`
	Category  OperationCategory  `json:"category"`
	Iteration OperationIteration `json:"iteration"`
	Status    OperationStatus    `json:"status"`
	Config    OperationConfig    `json:"config"`
}

func (o *Operation) ID() string {
	return OperationID(o.Type, o.Version, o.Iteration, o.Config.ID())
}

func (o *Operation) IDHuman() string {
	return OperationIDHuman(o.Type, o.Iteration, o.Config.IDHuman())
}

func OperationID(t OperationType, version OperationVersion, iteration OperationIteration, configID string) string {
	return fmt.Sprintf("%s/%d/%d/%s", t, version, iteration, configID)
}

func OperationIDHuman(t OperationType, iteration OperationIteration, configIDHuman string) string {
	id := fmt.Sprintf("%s: ", strings.ReplaceAll(string(t), "-", " "))

	id += configIDHuman
	if iteration > 0 {
		id += fmt.Sprintf(" (iteration=%d)", iteration)
	}

	return id
}

func (o *Operation) UnmarshalJSON(data []byte) error {
	type Alias Operation

	aux := &struct {
		*Alias

		Config json.RawMessage `json:"config"`
	}{
		Alias: (*Alias)(o),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("unmarshal operation json: %w", err)
	}

	switch o.Type {
	case OperationTypeNoop:
		o.Config = &OperationConfigNoop{}
	case OperationTypeCreate:
		o.Config = &OperationConfigCreate{}
	case OperationTypeRecreate:
		o.Config = &OperationConfigRecreate{}
	case OperationTypeUpdate:
		o.Config = &OperationConfigUpdate{}
	case OperationTypeApply:
		o.Config = &OperationConfigApply{}
	case OperationTypeDelete:
		o.Config = &OperationConfigDelete{}
	case OperationTypeTrackReadiness:
		o.Config = &OperationConfigTrackReadiness{}
	case OperationTypeTrackPresence:
		o.Config = &OperationConfigTrackPresence{}
	case OperationTypeTrackAbsence:
		o.Config = &OperationConfigTrackAbsence{}
	case OperationTypeCreateRelease:
		o.Config = &OperationConfigCreateRelease{}
	case OperationTypeUpdateRelease:
		o.Config = &OperationConfigUpdateRelease{}
	case OperationTypeDeleteRelease:
		o.Config = &OperationConfigDeleteRelease{}
	default:
		return fmt.Errorf("unknown operation type: %s", o.Type)
	}

	if err := json.Unmarshal(aux.Config, &o.Config); err != nil {
		return fmt.Errorf("unmarshal %s operation config: %w", o.Type, err)
	}

	return nil
}
