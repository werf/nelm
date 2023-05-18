package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewOperationRecreate() *OperationRecreate {
	return &OperationRecreate{}
}

type OperationRecreate struct {
	Targets []resource.Resourcer
}

func (o *OperationRecreate) Type() string {
	return "recreate"
}

func (o *OperationRecreate) AddTargets(target ...resource.Resourcer) *OperationRecreate {
	o.Targets = append(o.Targets, target...)
	return o
}

func (o *OperationRecreate) ResourcesWillBeCreatedOrUpdated() bool {
	return true
}

func (o *OperationRecreate) ResourcesWillBeDeleted() bool {
	return true
}

func (o *OperationRecreate) ResourcesWillBeTracked() bool {
	return false
}
