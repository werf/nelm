package operation

import (
	"github.com/werf/nelm/internal/resource"
)

const (
	OperationTypeUpdate    OperationType    = "update"
	OperationVersionUpdate OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigUpdate)(nil)

type OperationConfigUpdate struct {
	ResourceSpec *resource.ResourceSpec
}

func (c *OperationConfigUpdate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigUpdate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}
