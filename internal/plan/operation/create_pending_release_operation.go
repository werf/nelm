package operation

import (
	"context"
	"fmt"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/release"
)

var _ Operation = (*CreatePendingReleaseOperation)(nil)

const TypeCreatePendingReleaseOperation = "create-pending-release"

func NewCreatePendingReleaseOperation(
	rel *release.Release,
	deployType common.DeployType,
	history release.Historier,
) *CreatePendingReleaseOperation {
	return &CreatePendingReleaseOperation{
		deployType: deployType,
		release:    rel,
		history:    history,
	}
}

type CreatePendingReleaseOperation struct {
	deployType common.DeployType
	release    *release.Release
	history    release.Historier
	status     Status
}

func (o *CreatePendingReleaseOperation) Execute(ctx context.Context) error {
	o.release.Pend(o.deployType)

	if err := o.history.CreateRelease(ctx, o.release); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error creating release: %w", err)
	}
	o.status = StatusCompleted

	return nil
}

func (o *CreatePendingReleaseOperation) ID() string {
	return TypeCreatePendingReleaseOperation + "/" + o.release.ID()
}

func (o *CreatePendingReleaseOperation) HumanID() string {
	return "create pending release: " + o.release.HumanID()
}

func (o *CreatePendingReleaseOperation) Status() Status {
	return o.status
}

func (o *CreatePendingReleaseOperation) Type() Type {
	return TypeCreatePendingReleaseOperation
}

func (o *CreatePendingReleaseOperation) Empty() bool {
	return false
}
