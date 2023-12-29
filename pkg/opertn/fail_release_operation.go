package opertn

import (
	"context"
	"fmt"

	"github.com/werf/nelm/pkg/rls"
	"github.com/werf/nelm/pkg/rlshistor"
)

var _ Operation = (*FailReleaseOperation)(nil)

const TypeFailReleaseOperation = "fail-release"

func NewFailReleaseOperation(
	rel *rls.Release,
	history rlshistor.Historier,
) *FailReleaseOperation {
	return &FailReleaseOperation{
		release: rel,
		history: history,
	}
}

type FailReleaseOperation struct {
	release *rls.Release
	history rlshistor.Historier
	status  Status
}

func (o *FailReleaseOperation) Execute(ctx context.Context) error {
	o.release.Fail()

	if err := o.history.UpdateRelease(ctx, o.release); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error updating release: %w", err)
	}
	o.status = StatusCompleted

	return nil
}

func (o *FailReleaseOperation) ID() string {
	return TypeFailReleaseOperation + "/" + o.release.ID()
}

func (o *FailReleaseOperation) HumanID() string {
	return "fail release: " + o.release.HumanID()
}

func (o *FailReleaseOperation) Status() Status {
	return o.status
}

func (o *FailReleaseOperation) Type() Type {
	return TypeFailReleaseOperation
}

func (o *FailReleaseOperation) Empty() bool {
	return false
}
