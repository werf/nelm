package operation

import (
	"context"
	"fmt"

	"github.com/werf/nelm/internal/release"
)

var _ Operation = (*DeleteReleaseOperation)(nil)

const TypeDeleteReleaseOperation = "delete-release"

func NewDeleteReleaseOperation(
	rel *release.Release,
	history release.Historier,
) *DeleteReleaseOperation {
	return &DeleteReleaseOperation{
		release: rel,
		history: history,
	}
}

type DeleteReleaseOperation struct {
	release *release.Release
	history release.Historier
	status  Status
}

func (o *DeleteReleaseOperation) Execute(ctx context.Context) error {
	if err := o.history.DeleteRelease(ctx, o.release); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error deleting release: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *DeleteReleaseOperation) ID() string {
	return TypeDeleteReleaseOperation + "/" + o.release.ID()
}

func (o *DeleteReleaseOperation) HumanID() string {
	return "delete release: " + o.release.HumanID()
}

func (o *DeleteReleaseOperation) Status() Status {
	return o.status
}

func (o *DeleteReleaseOperation) Type() Type {
	return TypeDeleteReleaseOperation
}

func (o *DeleteReleaseOperation) Empty() bool {
	return false
}
