package operation

import "github.com/werf/nelm/internal/resource/meta"

const (
	OperationTypeTrackPresence    OperationType    = "track-presence"
	OperationVersionTrackPresence OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigTrackPresence)(nil)

type OperationConfigTrackPresence struct {
	ResourceMeta *meta.ResourceMeta
}

func (c *OperationConfigTrackPresence) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackPresence) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}
