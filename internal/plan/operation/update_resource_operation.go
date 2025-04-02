package operation

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
)

var _ Operation = (*UpdateResourceOperation)(nil)

const (
	TypeUpdateResourceOperation          = "update"
	TypeExtraPostUpdateResourceOperation = "extra-post-update"
)

func NewUpdateResourceOperation(
	resource *id.ResourceID,
	unstruct *unstructured.Unstructured,
	kubeClient kube.KubeClienter,
	opts UpdateResourceOperationOptions,
) (*UpdateResourceOperation, error) {
	return &UpdateResourceOperation{
		resource:     resource,
		unstruct:     unstruct,
		kubeClient:   kubeClient,
		manageableBy: opts.ManageableBy,
		extraPost:    opts.ExtraPost,
	}, nil
}

type UpdateResourceOperationOptions struct {
	ManageableBy resource.ManageableBy
	ExtraPost    bool
}

type UpdateResourceOperation struct {
	resource     *id.ResourceID
	unstruct     *unstructured.Unstructured
	kubeClient   kube.KubeClienter
	manageableBy resource.ManageableBy
	extraPost    bool
	status       Status
}

func (o *UpdateResourceOperation) Execute(ctx context.Context) error {
	if _, err := o.kubeClient.Apply(ctx, o.resource, o.unstruct, kube.KubeClientApplyOptions{}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error applying resource: %w", err)
	}
	o.status = StatusCompleted

	return nil
}

func (o *UpdateResourceOperation) ID() string {
	if o.extraPost {
		return TypeExtraPostUpdateResourceOperation + "/" + o.resource.ID()
	}

	return TypeUpdateResourceOperation + "/" + o.resource.ID()
}

func (o *UpdateResourceOperation) HumanID() string {
	return "update resource: " + o.resource.HumanID()
}

func (o *UpdateResourceOperation) Status() Status {
	return o.status
}

func (o *UpdateResourceOperation) Type() Type {
	if o.extraPost {
		return TypeExtraPostUpdateResourceOperation
	}

	return TypeUpdateResourceOperation
}

func (o *UpdateResourceOperation) Empty() bool {
	return false
}
