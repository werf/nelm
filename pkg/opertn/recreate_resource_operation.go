package opertn

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcid"
	"github.com/werf/nelm/pkg/resrctracker"
)

var _ Operation = (*RecreateResourceOperation)(nil)

const TypeRecreateResourceOperation = "recreate"

func NewRecreateResourceOperation(
	resource *resrcid.ResourceID,
	unstruct *unstructured.Unstructured,
	kubeClient kubeclnt.KubeClienter,
	tracker resrctracker.ResourceTrackerer,
	opts RecreateResourceOperationOptions,
) *RecreateResourceOperation {
	return &RecreateResourceOperation{
		resource:        resource,
		unstruct:        unstruct,
		kubeClient:      kubeClient,
		tracker:         tracker,
		manageableBy:    opts.ManageableBy,
		forceReplicas:   opts.ForceReplicas,
		deletionTimeout: opts.DeletionTimeout,
	}
}

type RecreateResourceOperationOptions struct {
	ManageableBy    resrc.ManageableBy
	ForceReplicas   *int
	DeletionTimeout time.Duration
}

type RecreateResourceOperation struct {
	resource           *resrcid.ResourceID
	unstruct           *unstructured.Unstructured
	kubeClient         kubeclnt.KubeClienter
	tracker            resrctracker.ResourceTrackerer
	manageableBy       resrc.ManageableBy
	addServiceMetadata bool
	forceReplicas      *int
	deletionTimeout    time.Duration
	status             Status
}

func (o *RecreateResourceOperation) Execute(ctx context.Context) error {
	if err := o.kubeClient.Delete(ctx, o.resource); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error deleting resource: %w", err)
	}

	if err := o.tracker.WaitDeletion(ctx, o.resource, resrctracker.WaitDeletionOptions{
		Timeout: o.deletionTimeout,
	}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error waiting for resource deletion: %w", err)
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
	return TypeRecreateResourceOperation + "/" + o.resource.ID()
}

func (o *RecreateResourceOperation) HumanID() string {
	return "recreate resource: " + o.resource.HumanID()
}

func (o *RecreateResourceOperation) Status() Status {
	return o.status
}

func (o *RecreateResourceOperation) Type() Type {
	return TypeRecreateResourceOperation
}

func (o *RecreateResourceOperation) Empty() bool {
	return false
}
