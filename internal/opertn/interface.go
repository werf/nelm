package opertn

import "context"

type Operation interface {
	Execute(ctx context.Context) error
	ID() string
	HumanID() string
	Status() Status
	Type() Type
	Empty() bool
}

type Status string

const (
	StatusUnknown   Status = ""
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Type string
