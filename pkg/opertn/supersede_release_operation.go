package opertn

import (
	"context"
	"fmt"

	"nelm.sh/nelm/pkg/rls"
	"nelm.sh/nelm/pkg/rlshistor"
)

var _ Operation = (*SupersedeReleaseOperation)(nil)

const TypeSupersedeReleaseOperation = "supersede-release"

func NewSupersedeReleaseOperation(
	rel *rls.Release,
	history rlshistor.Historier,
) *SupersedeReleaseOperation {
	return &SupersedeReleaseOperation{
		release: rel,
		history: history,
	}
}

type SupersedeReleaseOperation struct {
	release *rls.Release
	history rlshistor.Historier
	status  Status
}

func (o *SupersedeReleaseOperation) Execute(ctx context.Context) error {
	o.release.Supersede()

	if err := o.history.UpdateRelease(ctx, o.release); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error updating release: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *SupersedeReleaseOperation) ID() string {
	return TypeSupersedeReleaseOperation + "/" + o.release.ID()
}

func (o *SupersedeReleaseOperation) HumanID() string {
	return "supersede release: " + o.release.HumanID()
}

func (o *SupersedeReleaseOperation) Status() Status {
	return o.status
}

func (o *SupersedeReleaseOperation) Type() Type {
	return TypeSupersedeReleaseOperation
}

func (o *SupersedeReleaseOperation) Empty() bool {
	return false
}
