package operation

import "github.com/werf/nelm/internal/resource/id"

const (
	OperationTypeUpdate    = "update"
	OperationVersionUpdate = 1
)

var _ OperationConfig = (*OperationConfigUpdate)(nil)

type OperationConfigUpdate struct {
	ResourceSpec *id.ResourceSpec
}

func (c *OperationConfigUpdate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigUpdate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}
