package operation

import "github.com/werf/nelm/internal/resource/id"

const (
	OperationTypeApply    = "apply"
	OperationVersionApply = 1
)

var _ OperationConfig = (*OperationConfigApply)(nil)

type OperationConfigApply struct {
	ResourceSpec *id.ResourceSpec
}

func (c *OperationConfigApply) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigApply) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}
