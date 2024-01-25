package opertn

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"github.com/werf/kubedog/pkg/trackers/dyntracker"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/pkg/resrcid"
)

var _ Operation = (*TrackResourcePresenceOperation)(nil)

const TypeTrackResourcePresenceOperation = "track-resource-presence"

func NewTrackResourcePresenceOperation(
	resource *resrcid.ResourceID,
	taskState *util.Concurrent[*statestore.PresenceTaskState],
	dynamicClient dynamic.Interface,
	mapper meta.ResettableRESTMapper,
	opts TrackResourcePresenceOperationOptions,
) *TrackResourcePresenceOperation {
	return &TrackResourcePresenceOperation{
		resource:      resource,
		taskState:     taskState,
		dynamicClient: dynamicClient,
		mapper:        mapper,
		timeout:       opts.Timeout,
		pollPeriod:    opts.PollPeriod,
	}
}

type TrackResourcePresenceOperationOptions struct {
	Timeout    time.Duration
	PollPeriod time.Duration
}

type TrackResourcePresenceOperation struct {
	resource      *resrcid.ResourceID
	taskState     *util.Concurrent[*statestore.PresenceTaskState]
	dynamicClient dynamic.Interface
	mapper        meta.ResettableRESTMapper
	timeout       time.Duration
	pollPeriod    time.Duration

	status Status
}

func (o *TrackResourcePresenceOperation) Execute(ctx context.Context) error {
	tracker := dyntracker.NewDynamicPresenceTracker(o.taskState, o.dynamicClient, o.mapper, dyntracker.DynamicPresenceTrackerOptions{
		Timeout:    o.timeout,
		PollPeriod: o.pollPeriod,
	})

	if err := tracker.Track(ctx); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("track resource presence: %w", err)
	}

	o.status = StatusCompleted
	return nil
}

func (o *TrackResourcePresenceOperation) ID() string {
	return TypeTrackResourcePresenceOperation + "/" + o.resource.ID()
}

func (o *TrackResourcePresenceOperation) HumanID() string {
	return "track resource presence: " + o.resource.HumanID()
}

func (o *TrackResourcePresenceOperation) Status() Status {
	return o.status
}

func (o *TrackResourcePresenceOperation) Type() Type {
	return TypeTrackResourcePresenceOperation
}

func (o *TrackResourcePresenceOperation) Empty() bool {
	return false
}
