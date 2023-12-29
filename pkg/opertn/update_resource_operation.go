package opertn

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"nelm.sh/nelm/pkg/kubeclnt"
	"nelm.sh/nelm/pkg/resrc"
	"nelm.sh/nelm/pkg/resrcid"
)

var _ Operation = (*UpdateResourceOperation)(nil)

const TypeUpdateResourceOperation = "update"

func NewUpdateResourceOperation(
	resource *resrcid.ResourceID,
	unstruct *unstructured.Unstructured,
	kubeClient kubeclnt.KubeClienter,
	opts UpdateResourceOperationOptions,
) (*UpdateResourceOperation, error) {
	return &UpdateResourceOperation{
		resource:     resource,
		unstruct:     unstruct,
		kubeClient:   kubeClient,
		manageableBy: opts.ManageableBy,
	}, nil
}

type UpdateResourceOperationOptions struct {
	ManageableBy resrc.ManageableBy
}

type UpdateResourceOperation struct {
	resource     *resrcid.ResourceID
	unstruct     *unstructured.Unstructured
	kubeClient   kubeclnt.KubeClienter
	manageableBy resrc.ManageableBy
	status       Status
}

func (o *UpdateResourceOperation) Execute(ctx context.Context) error {
	if _, err := o.kubeClient.Apply(ctx, o.resource, o.unstruct, kubeclnt.KubeClientApplyOptions{}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error applying resource: %w", err)
	}
	o.status = StatusCompleted

	return nil
}

func (o *UpdateResourceOperation) ID() string {
	return TypeUpdateResourceOperation + "/" + o.resource.ID()
}

func (o *UpdateResourceOperation) HumanID() string {
	return "update resource: " + o.resource.HumanID()
}

func (o *UpdateResourceOperation) Status() Status {
	return o.status
}

func (o *UpdateResourceOperation) Type() Type {
	return TypeUpdateResourceOperation
}

func (o *UpdateResourceOperation) Empty() bool {
	return false
}
