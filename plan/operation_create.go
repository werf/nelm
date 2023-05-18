package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewOperationCreate() *OperationCreate {
	return &OperationCreate{}
}

type OperationCreate struct {
	Targets []resource.Resourcer
}

func (o *OperationCreate) Type() string {
	return "create"
}

func (o *OperationCreate) AddTargets(target ...resource.Resourcer) *OperationCreate {
	o.Targets = append(o.Targets, target...)
	return o
}

func (o *OperationCreate) ResourcesWillBeCreatedOrUpdated() bool {
	return true
}

func (o *OperationCreate) ResourcesWillBeDeleted() bool {
	return false
}

func (o *OperationCreate) ResourcesWillBeTracked() bool {
	return false
}
