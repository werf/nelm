package opertn

import (
	"context"
	"fmt"

	"nelm.sh/nelm/pkg/kubeclnt"
	"nelm.sh/nelm/pkg/resrcid"
)

var _ Operation = (*DeleteResourceOperation)(nil)

const TypeDeleteResourceOperation = "delete"

func NewDeleteResourceOperation(
	resource *resrcid.ResourceID,
	kubeClient kubeclnt.KubeClienter,
) *DeleteResourceOperation {
	return &DeleteResourceOperation{
		resource:   resource,
		kubeClient: kubeClient,
	}
}

type DeleteResourceOperation struct {
	resource   *resrcid.ResourceID
	kubeClient kubeclnt.KubeClienter
	status     Status
}

func (o *DeleteResourceOperation) Execute(ctx context.Context) error {
	if err := o.kubeClient.Delete(ctx, o.resource); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error deleting resource: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *DeleteResourceOperation) ID() string {
	return TypeDeleteResourceOperation + "/" + o.resource.ID()
}

func (o *DeleteResourceOperation) HumanID() string {
	return "delete resource: " + o.resource.HumanID()
}

func (o *DeleteResourceOperation) Status() Status {
	return o.status
}

func (o *DeleteResourceOperation) Type() Type {
	return TypeDeleteResourceOperation
}

func (o *DeleteResourceOperation) Empty() bool {
	return false
}
