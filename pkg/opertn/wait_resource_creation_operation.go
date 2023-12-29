package opertn

import (
	"context"
	"fmt"
	"time"

	"nelm.sh/nelm/pkg/resrcid"
	"nelm.sh/nelm/pkg/resrctracker"
)

var _ Operation = (*WaitResourceCreationOperation)(nil)

const TypeWaitResourceCreationOperation = "wait-resource-creation"

func NewWaitResourceCreationOperation(
	resource *resrcid.ResourceID,
	tracker resrctracker.ResourceTrackerer,
	opts WaitResourceCreationOperationOptions,
) *WaitResourceCreationOperation {
	return &WaitResourceCreationOperation{
		resource: resource,
		tracker:  tracker,
		timeout:  opts.Timeout,
	}
}

type WaitResourceCreationOperationOptions struct {
	Timeout time.Duration
}

type WaitResourceCreationOperation struct {
	resource *resrcid.ResourceID
	tracker  resrctracker.ResourceTrackerer
	timeout  time.Duration
	status   Status
}

func (o *WaitResourceCreationOperation) Execute(ctx context.Context) error {
	if err := o.tracker.WaitCreation(ctx, o.resource, resrctracker.WaitCreationOptions{
		Timeout: o.timeout,
	}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error waiting for resource creation: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *WaitResourceCreationOperation) ID() string {
	return TypeWaitResourceCreationOperation + "/" + o.resource.ID()
}

func (o *WaitResourceCreationOperation) HumanID() string {
	return "wait resource creation: " + o.resource.HumanID()
}

func (o *WaitResourceCreationOperation) Status() Status {
	return o.status
}

func (o *WaitResourceCreationOperation) Type() Type {
	return TypeWaitResourceCreationOperation
}

func (o *WaitResourceCreationOperation) Empty() bool {
	return false
}
