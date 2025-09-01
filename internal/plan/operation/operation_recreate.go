package operation

import "github.com/werf/nelm/internal/resource/id"

const (
	OperationTypeRecreate    = "recreate"
	OperationVersionRecreate = 1
)

var _ OperationConfig = (*OperationConfigRecreate)(nil)

type OperationConfigRecreate struct {
	ResourceSpec  *id.ResourceSpec
	ForceReplicas *int
}

func (c *OperationConfigRecreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigRecreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}
