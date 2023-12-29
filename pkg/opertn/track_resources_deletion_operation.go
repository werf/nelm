package opertn

import (
	"context"
	"fmt"
	"time"

	"github.com/werf/nelm/resrcid"
	"github.com/werf/nelm/resrctracker"
)

var _ Operation = (*TrackResourcesDeletionOperation)(nil)

const TypeTrackResourcesDeletionOperation = "track-resources-deletion"

func NewTrackResourcesDeletionOperation(
	name string,
	humanName string,
	tracker resrctracker.ResourceTrackerer,
	opts TrackResourcesDeletionOperationOptions,
) *TrackResourcesDeletionOperation {
	return &TrackResourcesDeletionOperation{
		name:               name,
		humanName:          humanName,
		tracker:            tracker,
		timeout:            opts.Timeout,
		showProgressPeriod: opts.ShowProgressPeriod,
	}
}

type TrackResourcesDeletionOperationOptions struct {
	Timeout            time.Duration
	ShowProgressPeriod time.Duration
}

type TrackResourcesDeletionOperation struct {
	name               string
	humanName          string
	resources          []*resrcid.ResourceID
	tracker            resrctracker.ResourceTrackerer
	timeout            time.Duration
	showProgressPeriod time.Duration
	status             Status
}

func (o *TrackResourcesDeletionOperation) AddResource(resource *resrcid.ResourceID) *TrackResourcesDeletionOperation {
	o.resources = append(o.resources, resource)
	return o
}

func (o *TrackResourcesDeletionOperation) Execute(ctx context.Context) error {
	if o.Empty() {
		o.status = StatusCompleted
		return nil
	}

	for _, res := range o.resources {
		if err := o.tracker.AddResourceToTrackDeletion(res); err != nil {
			o.status = StatusFailed
			return fmt.Errorf("error adding resource to track deletion: %w", err)
		}
	}

	if err := o.tracker.TrackDeletion(ctx, resrctracker.TrackDeletionOptions{
		Timeout:            o.timeout,
		ShowProgressPeriod: o.showProgressPeriod,
	}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error tracking resources deletion: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *TrackResourcesDeletionOperation) ID() string {
	return TypeTrackResourcesDeletionOperation + "/" + o.name
}

func (o *TrackResourcesDeletionOperation) HumanID() string {
	return "track resources deletion: " + o.humanName
}

func (o *TrackResourcesDeletionOperation) Status() Status {
	return o.status
}

func (o *TrackResourcesDeletionOperation) Type() Type {
	return TypeTrackResourcesDeletionOperation
}

func (o *TrackResourcesDeletionOperation) Empty() bool {
	return len(o.resources) == 0
}
