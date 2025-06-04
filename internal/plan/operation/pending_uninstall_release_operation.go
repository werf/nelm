package operation

import (
	"context"
	"fmt"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/release"
)

var _ Operation = (*PendingUninstallReleaseOperation)(nil)

const TypePendingUninstallReleaseOperation = "pending-uninstall-release"

func NewPendingUninstallReleaseOperation(
	rel *release.Release,
	history release.Historier,
) *PendingUninstallReleaseOperation {
	return &PendingUninstallReleaseOperation{
		release: rel,
		history: history,
	}
}

type PendingUninstallReleaseOperation struct {
	release *release.Release
	history release.Historier
	status  Status
}

func (o *PendingUninstallReleaseOperation) Execute(ctx context.Context) error {
	o.release.Pend(common.DeployTypeUninstall)

	if err := o.history.UpdateRelease(ctx, o.release); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error updating release: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *PendingUninstallReleaseOperation) ID() string {
	return TypePendingUninstallReleaseOperation + "/" + o.release.ID()
}

func (o *PendingUninstallReleaseOperation) HumanID() string {
	return "pending uninstall release: " + o.release.HumanID()
}

func (o *PendingUninstallReleaseOperation) Status() Status {
	return o.status
}

func (o *PendingUninstallReleaseOperation) Type() Type {
	return TypePendingUninstallReleaseOperation
}

func (o *PendingUninstallReleaseOperation) Empty() bool {
	return false
}
