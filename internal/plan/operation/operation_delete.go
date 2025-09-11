package operation

import "github.com/werf/nelm/internal/resource/meta"

const (
	OperationTypeDelete    OperationType    = "delete"
	OperationVersionDelete OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigDelete)(nil)

type OperationConfigDelete struct {
	ResourceMeta *meta.ResourceMeta
}

func (c *OperationConfigDelete) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigDelete) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}
