package operation

import (
	"context"
	"fmt"

	"github.com/werf/nelm/internal/release"
)

var _ Operation = (*SucceedReleaseOperation)(nil)

const TypeSucceedReleaseOperation = "succeed-release"

func NewSucceedReleaseOperation(
	rel *release.Release,
	history release.Historier,
) *SucceedReleaseOperation {
	return &SucceedReleaseOperation{
		release: rel,
		history: history,
	}
}

type SucceedReleaseOperation struct {
	release *release.Release
	history release.Historier
	status  Status
}

func (o *SucceedReleaseOperation) Execute(ctx context.Context) error {
	o.release.Succeed()

	if err := o.history.UpdateRelease(ctx, o.release); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error updating release: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *SucceedReleaseOperation) ID() string {
	return TypeSucceedReleaseOperation + "/" + o.release.ID()
}

func (o *SucceedReleaseOperation) HumanID() string {
	return "succeed release: " + o.release.HumanID()
}

func (o *SucceedReleaseOperation) Status() Status {
	return o.status
}

func (o *SucceedReleaseOperation) Type() Type {
	return TypeSucceedReleaseOperation
}

func (o *SucceedReleaseOperation) Empty() bool {
	return false
}
