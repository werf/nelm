package plan

import (
	"fmt"
	"strings"
)

type OperationCategory string

const (
	OperationCategoryMeta     OperationCategory = "meta"
	OperationCategoryResource OperationCategory = "resource"
	OperationCategoryTrack    OperationCategory = "track"
	OperationCategoryRelease  OperationCategory = "release"
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

type OperationIteration int

type Operation struct {
	Type      OperationType
	Version   OperationVersion
	Category  OperationCategory
	Iteration OperationIteration
	Status    OperationStatus
	Config    OperationConfig
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
