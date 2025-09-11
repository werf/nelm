package operation

import (
	"github.com/werf/nelm/internal/resource"
)

const (
	OperationTypeApply    OperationType    = "apply"
	OperationVersionApply OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigApply)(nil)

type OperationConfigApply struct {
	ResourceSpec *resource.ResourceSpec
}

func (c *OperationConfigApply) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigApply) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}
