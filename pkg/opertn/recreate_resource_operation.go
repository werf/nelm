package opertn

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/werf/kubedog/pkg/trackers/dyntracker"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcid"
)

var _ Operation = (*RecreateResourceOperation)(nil)

const (
	TypeRecreateResourceOperation          = "recreate"
	TypeExtraPostRecreateResourceOperation = "extra-post-recreate"
)

func NewRecreateResourceOperation(
	resource *resrcid.ResourceID,
	unstruct *unstructured.Unstructured,
	absenceTaskState *util.Concurrent[*statestore.AbsenceTaskState],
	kubeClient kubeclnt.KubeClienter,
	dynamicClient dynamic.Interface,
	mapper meta.ResettableRESTMapper,
	opts RecreateResourceOperationOptions,
) *RecreateResourceOperation {
	return &RecreateResourceOperation{
		resource:                resource,
		unstruct:                unstruct,
		taskState:               absenceTaskState,
		kubeClient:              kubeClient,
		dynamicClient:           dynamicClient,
		mapper:                  mapper,
		manageableBy:            opts.ManageableBy,
		forceReplicas:           opts.ForceReplicas,
		deletionTrackTimeout:    opts.DeletionTrackTimeout,
		deletionTrackPollPeriod: opts.DeletionTrackPollPeriod,
		extraPost:               opts.ExtraPost,
	}
}

type RecreateResourceOperationOptions struct {
	ManageableBy            resrc.ManageableBy
	ForceReplicas           *int
	DeletionTrackTimeout    time.Duration
	DeletionTrackPollPeriod time.Duration
	ExtraPost               bool
}

type RecreateResourceOperation struct {
	resource                *resrcid.ResourceID
	unstruct                *unstructured.Unstructured
	taskState               *util.Concurrent[*statestore.AbsenceTaskState]
	kubeClient              kubeclnt.KubeClienter
	dynamicClient           dynamic.Interface
	mapper                  meta.ResettableRESTMapper
	manageableBy            resrc.ManageableBy
	forceReplicas           *int
	deletionTrackTimeout    time.Duration
	deletionTrackPollPeriod time.Duration
	extraPost               bool

	status Status
}

func (o *RecreateResourceOperation) Execute(ctx context.Context) error {
	if err := o.kubeClient.Delete(ctx, o.resource, kubeclnt.KubeClientDeleteOptions{}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error deleting resource: %w", err)
	}

	tracker := dyntracker.NewDynamicAbsenceTracker(o.taskState, o.dynamicClient, o.mapper, dyntracker.DynamicAbsenceTrackerOptions{
		Timeout:    o.deletionTrackTimeout,
		PollPeriod: o.deletionTrackPollPeriod,
	})

	if err := tracker.Track(ctx); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("track resource absence: %w", err)
	}

	if _, err := o.kubeClient.Create(ctx, o.resource, o.unstruct, kubeclnt.KubeClientCreateOptions{
		ForceReplicas: o.forceReplicas,
	}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error creating resource: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *RecreateResourceOperation) ID() string {
	if o.extraPost {
		return TypeExtraPostRecreateResourceOperation + "/" + o.resource.ID()
	}

	return TypeRecreateResourceOperation + "/" + o.resource.ID()
}

func (o *RecreateResourceOperation) HumanID() string {
	return "recreate resource: " + o.resource.HumanID()
}

func (o *RecreateResourceOperation) Status() Status {
	return o.status
}

func (o *RecreateResourceOperation) Type() Type {
	if o.extraPost {
		return TypeExtraPostRecreateResourceOperation
	}

	return TypeRecreateResourceOperation
}

func (o *RecreateResourceOperation) Empty() bool {
	return false
}
