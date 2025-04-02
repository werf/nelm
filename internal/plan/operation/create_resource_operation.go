package operation

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
)

var _ Operation = (*CreateResourceOperation)(nil)

const (
	TypeCreateResourceOperation          = "create"
	TypeExtraPostCreateResourceOperation = "extra-post-create"
)

func NewCreateResourceOperation(
	resource *id.ResourceID,
	unstruct *unstructured.Unstructured,
	kubeClient kube.KubeClienter,
	opts CreateResourceOperationOptions,
) *CreateResourceOperation {
	return &CreateResourceOperation{
		resource:      resource,
		unstruct:      unstruct,
		kubeClient:    kubeClient,
		manageableBy:  opts.ManageableBy,
		extraPost:     opts.ExtraPost,
		forceReplicas: opts.ForceReplicas,
	}
}

type CreateResourceOperationOptions struct {
	ManageableBy  resource.ManageableBy
	ForceReplicas *int
	ExtraPost     bool
}

type CreateResourceOperation struct {
	resource      *id.ResourceID
	unstruct      *unstructured.Unstructured
	kubeClient    kube.KubeClienter
	manageableBy  resource.ManageableBy
	forceReplicas *int
	extraPost     bool
	status        Status
}

func (o *CreateResourceOperation) Execute(ctx context.Context) error {
	if _, err := o.kubeClient.Create(ctx, o.resource, o.unstruct, kube.KubeClientCreateOptions{
		ForceReplicas: o.forceReplicas,
	}); err != nil {
		if errors.IsAlreadyExists(err) {
			if _, err := o.kubeClient.Apply(ctx, o.resource, o.unstruct, kube.KubeClientApplyOptions{}); err != nil {
				o.status = StatusFailed
				return fmt.Errorf("error applying resource: %w", err)
			}
		}

		o.status = StatusFailed
		return fmt.Errorf("error creating resource: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *CreateResourceOperation) ID() string {
	if o.extraPost {
		return TypeExtraPostCreateResourceOperation + "/" + o.resource.ID()
	}

	return TypeCreateResourceOperation + "/" + o.resource.ID()
}

func (o *CreateResourceOperation) HumanID() string {
	return "create resource: " + o.resource.HumanID()
}

func (o *CreateResourceOperation) Status() Status {
	return o.status
}

func (o *CreateResourceOperation) Type() Type {
	if o.extraPost {
		return TypeExtraPostCreateResourceOperation
	}

	return TypeCreateResourceOperation
}

func (o *CreateResourceOperation) Empty() bool {
	return false
}
