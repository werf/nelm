package opertn

import "context"

var _ Operation = (*StageOperation)(nil)

const TypeStageOperation = "stage"

func NewStageOperation(name string) *StageOperation {
	return &StageOperation{
		name: name,
	}
}

type StageOperation struct {
	name   string
	status Status
}

func (o *StageOperation) Execute(ctx context.Context) error {
	o.status = StatusCompleted
	return nil
}

func (o *StageOperation) ID() string {
	return o.name
}

func (o *StageOperation) HumanID() string {
	return o.name
}

func (o *StageOperation) Status() Status {
	return o.status
}

func (o *StageOperation) Type() Type {
	return TypeStageOperation
}

func (o *StageOperation) Empty() bool {
	return true
}
