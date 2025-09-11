package operation

import (
	"fmt"
	"strings"
)

type (
	OperationType      string
	OperationVersion   int
	OperationIteration int
	OperationStatus    string
	OperationConfig    interface {
		ID() string
		IDHuman() string
	}
)

const (
	OperationStatusPending   OperationStatus = "pending"
	OperationStatusCompleted OperationStatus = "completed"
	OperationStatusFailed    OperationStatus = "failed"
)

type Operation struct {
	Type      OperationType
	Version   OperationVersion
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
	return fmt.Sprintf("%s/%s/%s/%s", t, version, iteration, configID)
}

func OperationIDHuman(t OperationType, iteration OperationIteration, configIDHuman string) string {
	id := fmt.Sprintf("%s: ", strings.ReplaceAll(string(t), "-", " "))
	id += configIDHuman
	if iteration > 0 {
		id += fmt.Sprintf(" (%d)", iteration)
	}

	return id
}
