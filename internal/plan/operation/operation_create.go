package operation

import "github.com/werf/nelm/internal/resource/id"

const (
	OperationTypeCreate    = "create"
	OperationVersionCreate = 1
)

var _ OperationConfig = (*OperationConfigCreate)(nil)

type OperationConfigCreate struct {
	ResourceSpec  *id.ResourceSpec
	ForceReplicas *int
}

func (c *OperationConfigCreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigCreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}
