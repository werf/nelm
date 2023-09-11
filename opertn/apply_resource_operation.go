package opertn

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/werf/kubeclnt"
	"helm.sh/helm/v3/pkg/werf/resrc"
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ Operation = (*ApplyResourceOperation)(nil)

const TypeApplyResourceOperation = "apply"

func NewApplyResourceOperation(
	resource *resrcid.ResourceID,
	unstruct *unstructured.Unstructured,
	kubeClient kubeclnt.KubeClienter,
	opts ApplyResourceOperationOptions,
) (*ApplyResourceOperation, error) {
	return &ApplyResourceOperation{
		resource:            resource,
		unstruct:            unstruct,
		kubeClient:          kubeClient,
		manageableBy:        opts.ManageableBy,
		repairManagedFields: opts.RepairManagedFields,
	}, nil
}

type ApplyResourceOperationOptions struct {
	ManageableBy        resrc.ManageableBy
	RepairManagedFields bool
}

type ApplyResourceOperation struct {
	resource            *resrcid.ResourceID
	unstruct            *unstructured.Unstructured
	kubeClient          kubeclnt.KubeClienter
	manageableBy        resrc.ManageableBy
	repairManagedFields bool
	status              Status
}

func (o *ApplyResourceOperation) Execute(ctx context.Context) error {
	if o.repairManagedFields {
		if err := doRepairManagedFields(ctx, o.resource, o.kubeClient); err != nil {
			o.status = StatusFailed
			return fmt.Errorf("error repairing managed fields: %w", err)
		}
	}

	if _, err := o.kubeClient.Apply(ctx, o.resource, o.unstruct, kubeclnt.KubeClientApplyOptions{}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error applying resource: %w", err)
	}
	o.status = StatusCompleted

	return nil
}

func (o *ApplyResourceOperation) ID() string {
	return TypeApplyResourceOperation + "/" + o.resource.ID()
}

func (o *ApplyResourceOperation) HumanID() string {
	return "apply resource: " + o.resource.HumanID()
}

func (o *ApplyResourceOperation) Status() Status {
	return o.status
}

func (o *ApplyResourceOperation) Type() Type {
	return TypeApplyResourceOperation
}

func (o *ApplyResourceOperation) Empty() bool {
	return false
}
