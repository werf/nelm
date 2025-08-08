package operation

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"

	"github.com/werf/kubedog/pkg/informer"
	"github.com/werf/kubedog/pkg/trackers/dyntracker"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/internal/resource/id"
)

var _ Operation = (*TrackResourceAbsenceOperation)(nil)

const TypeTrackResourceAbsenceOperation = "track-resource-absence"

func NewTrackResourceAbsenceOperation(
	resource *id.ResourceID,
	taskState *util.Concurrent[*statestore.AbsenceTaskState],
	informerFactory *util.Concurrent[*informer.InformerFactory],
	dynamicClient dynamic.Interface,
	mapper meta.ResettableRESTMapper,
	opts TrackResourceAbsenceOperationOptions,
) *TrackResourceAbsenceOperation {
	return &TrackResourceAbsenceOperation{
		resource:        resource,
		taskState:       taskState,
		informerFactory: informerFactory,
		dynamicClient:   dynamicClient,
		mapper:          mapper,
		timeout:         opts.Timeout,
		pollPeriod:      opts.PollPeriod,
	}
}

type TrackResourceAbsenceOperationOptions struct {
	Timeout    time.Duration
	PollPeriod time.Duration
}

type TrackResourceAbsenceOperation struct {
	resource        *id.ResourceID
	taskState       *util.Concurrent[*statestore.AbsenceTaskState]
	informerFactory *util.Concurrent[*informer.InformerFactory]
	dynamicClient   dynamic.Interface
	mapper          meta.ResettableRESTMapper
	timeout         time.Duration
	pollPeriod      time.Duration

	status Status
}

func (o *TrackResourceAbsenceOperation) Execute(ctx context.Context) error {
	tracker := dyntracker.NewDynamicAbsenceTracker(o.taskState, o.informerFactory, o.dynamicClient, o.mapper, dyntracker.DynamicAbsenceTrackerOptions{
		Timeout:    o.timeout,
		PollPeriod: o.pollPeriod,
	})

	if err := tracker.Track(ctx); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("track resource absence: %w", err)
	}

	o.status = StatusCompleted
	return nil
}

func (o *TrackResourceAbsenceOperation) ID() string {
	return TypeTrackResourceAbsenceOperation + "/" + o.resource.ID()
}

func (o *TrackResourceAbsenceOperation) HumanID() string {
	return "track resource absence: " + o.resource.HumanID()
}

func (o *TrackResourceAbsenceOperation) Status() Status {
	return o.status
}

func (o *TrackResourceAbsenceOperation) Type() Type {
	return TypeTrackResourceAbsenceOperation
}

func (o *TrackResourceAbsenceOperation) Empty() bool {
	return false
}
