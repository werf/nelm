package operation

import (
	"context"
	"fmt"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource/id"
)

var _ Operation = (*DeleteResourceOperation)(nil)

const (
	TypeDeleteResourceOperation          = "delete"
	TypeExtraPostDeleteResourceOperation = "extra-post-delete"
)

func NewDeleteResourceOperation(
	resource *id.ResourceID,
	kubeClient kube.KubeClienter,
	opts DeleteResourceOperationOptions,
) *DeleteResourceOperation {
	return &DeleteResourceOperation{
		resource:   resource,
		kubeClient: kubeClient,
		extraPost:  opts.ExtraPost,
	}
}

type DeleteResourceOperationOptions struct {
	ExtraPost bool
}

type DeleteResourceOperation struct {
	resource   *id.ResourceID
	kubeClient kube.KubeClienter
	extraPost  bool
	status     Status
}

func (o *DeleteResourceOperation) Execute(ctx context.Context) error {
	if err := o.kubeClient.Delete(ctx, o.resource, kube.KubeClientDeleteOptions{}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error deleting resource: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *DeleteResourceOperation) ID() string {
	if o.extraPost {
		return TypeExtraPostDeleteResourceOperation + "/" + o.resource.ID()
	}

	return TypeDeleteResourceOperation + "/" + o.resource.ID()
}

func (o *DeleteResourceOperation) HumanID() string {
	return "delete resource: " + o.resource.HumanID()
}

func (o *DeleteResourceOperation) Status() Status {
	return o.status
}

func (o *DeleteResourceOperation) Type() Type {
	if o.extraPost {
		return TypeExtraPostDeleteResourceOperation
	}

	return TypeDeleteResourceOperation
}

func (o *DeleteResourceOperation) Empty() bool {
	return false
}
