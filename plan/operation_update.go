package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewOperationUpdate() *OperationUpdate {
	return &OperationUpdate{}
}

type OperationUpdate struct {
	Targets []resource.Resourcer
}

func (o *OperationUpdate) Type() string {
	return "update"
}

func (o *OperationUpdate) AddTargets(target ...resource.Resourcer) *OperationUpdate {
	o.Targets = append(o.Targets, target...)
	return o
}

func (o *OperationUpdate) ResourcesWillBeCreatedOrUpdated() bool {
	return true
}

func (o *OperationUpdate) ResourcesWillBeDeleted() bool {
	return false
}

func (o *OperationUpdate) ResourcesWillBeTracked() bool {
	return false
}
