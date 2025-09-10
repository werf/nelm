package operation

import "github.com/werf/nelm/internal/resource/id"

const (
	OperationTypeTrackAbsence    OperationType    = "track-absence"
	OperationVersionTrackAbsence OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigTrackAbsence)(nil)

type OperationConfigTrackAbsence struct {
	ResourceMeta *id.ResourceMeta
}

func (c *OperationConfigTrackAbsence) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackAbsence) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}
