package operation

import (
	"github.com/werf/nelm/internal/resource"
)

const (
	OperationTypeRecreate    OperationType    = "recreate"
	OperationVersionRecreate OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigRecreate)(nil)

type OperationConfigRecreate struct {
	ResourceSpec  *resource.ResourceSpec
	ForceReplicas *int
}

func (c *OperationConfigRecreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigRecreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}
