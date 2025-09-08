package operation

import "github.com/werf/nelm/internal/resource/id"

const (
	OperationTypeDelete    = "delete"
	OperationVersionDelete = 1
)

var _ OperationConfig = (*OperationConfigDelete)(nil)

type OperationConfigDelete struct {
	ResourceMeta *id.ResourceMeta
}

func (c *OperationConfigDelete) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigDelete) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}
