package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewOperationDelete() *OperationDelete {
	return &OperationDelete{}
}

type OperationDelete struct {
	Targets []resource.Referencer
}

func (o *OperationDelete) Type() string {
	return "delete"
}

func (o *OperationDelete) AddTargets(target ...resource.Referencer) *OperationDelete {
	o.Targets = append(o.Targets, target...)
	return o
}

func (o *OperationDelete) ResourcesWillBeCreatedOrUpdated() bool {
	return false
}

func (o *OperationDelete) ResourcesWillBeDeleted() bool {
	return true
}

func (o *OperationDelete) ResourcesWillBeTracked() bool {
	return false
}
