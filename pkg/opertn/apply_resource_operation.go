package opertn

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/pkg/kubeclnt"
	"github.com/werf/nelm/pkg/resrc"
	"github.com/werf/nelm/pkg/resrcid"
)

var _ Operation = (*ApplyResourceOperation)(nil)

const (
	TypeApplyResourceOperation          = "apply"
	TypeExtraPostApplyResourceOperation = "extra-post-apply"
)

func NewApplyResourceOperation(
	resource *resrcid.ResourceID,
	unstruct *unstructured.Unstructured,
	kubeClient kubeclnt.KubeClienter,
	opts ApplyResourceOperationOptions,
) (*ApplyResourceOperation, error) {
	return &ApplyResourceOperation{
		resource:     resource,
		unstruct:     unstruct,
		kubeClient:   kubeClient,
		manageableBy: opts.ManageableBy,
		extraPost:    opts.ExtraPost,
	}, nil
}

type ApplyResourceOperationOptions struct {
	ManageableBy resrc.ManageableBy
	ExtraPost    bool
}

type ApplyResourceOperation struct {
	resource     *resrcid.ResourceID
	unstruct     *unstructured.Unstructured
	kubeClient   kubeclnt.KubeClienter
	manageableBy resrc.ManageableBy
	extraPost    bool
	status       Status
}

func (o *ApplyResourceOperation) Execute(ctx context.Context) error {
	if _, err := o.kubeClient.Apply(ctx, o.resource, o.unstruct, kubeclnt.KubeClientApplyOptions{}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error applying resource: %w", err)
	}
	o.status = StatusCompleted

	return nil
}

func (o *ApplyResourceOperation) ID() string {
	if o.extraPost {
		return TypeExtraPostApplyResourceOperation + "/" + o.resource.ID()
	}

	return TypeApplyResourceOperation + "/" + o.resource.ID()
}

func (o *ApplyResourceOperation) HumanID() string {
	return "apply resource: " + o.resource.HumanID()
}

func (o *ApplyResourceOperation) Status() Status {
	return o.status
}

func (o *ApplyResourceOperation) Type() Type {
	if o.extraPost {
		return TypeExtraPostApplyResourceOperation
	}

	return TypeApplyResourceOperation
}

func (o *ApplyResourceOperation) Empty() bool {
	return false
}
