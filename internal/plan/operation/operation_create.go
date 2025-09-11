package operation

import (
	"github.com/werf/nelm/internal/resource"
)

const (
	OperationTypeCreate    OperationType    = "create"
	OperationVersionCreate OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigCreate)(nil)

type OperationConfigCreate struct {
	ResourceSpec  *resource.ResourceSpec
	ForceReplicas *int
}

func (c *OperationConfigCreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigCreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}
