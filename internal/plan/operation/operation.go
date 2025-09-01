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
	OperationStatusPending   Status = "pending"
	OperationStatusCompleted Status = "completed"
	OperationStatusFailed    Status = "failed"
)

type Operation struct {
	Type      OperationType
	Version   OperationVersion
	Iteration OperationIteration
	Status    OperationStatus
	Config    OperationConfig
}

func (o *Operation) ID() string {
	return fmt.Sprintf("%s/%s/%s/%s", o.Type, o.Version, o.Iteration, o.Config.ID())
}

func (o *Operation) IDHuman() string {
	id := fmt.Sprintf("%s: ", strings.ReplaceAll(string(o.Type), "-", " "))
	id += o.Config.IDHuman()
	if o.Iteration > 0 {
		id += fmt.Sprintf(" (%d)", o.Iteration)
	}

	return id
}
