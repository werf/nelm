package opertn

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/rls"
	"helm.sh/helm/v3/pkg/werf/rlshistor"
)

var _ Operation = (*CreatePendingReleaseOperation)(nil)

const TypeCreatePendingReleaseOperation = "create-pending-release"

func NewCreatePendingReleaseOperation(
	rel *rls.Release,
	deployType common.DeployType,
	history rlshistor.Historier,
) *CreatePendingReleaseOperation {
	return &CreatePendingReleaseOperation{
		deployType: deployType,
		release:    rel,
		history:    history,
	}
}

type CreatePendingReleaseOperation struct {
	deployType common.DeployType
	release    *rls.Release
	history    rlshistor.Historier
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
