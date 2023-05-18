package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewOperationTrackDeletion() *OperationTrackDeletion {
	return &OperationTrackDeletion{}
}

type OperationTrackDeletion struct {
	Targets []resource.Referencer
}

func (o *OperationTrackDeletion) Type() string {
	return "track-deletion"
}

func (o *OperationTrackDeletion) AddTargets(target ...resource.Referencer) *OperationTrackDeletion {
	o.Targets = append(o.Targets, target...)
	return o
}

func (o *OperationTrackDeletion) ResourcesWillBeCreatedOrUpdated() bool {
	return false
}

func (o *OperationTrackDeletion) ResourcesWillBeDeleted() bool {
	return false
}

func (o *OperationTrackDeletion) ResourcesWillBeTracked() bool {
	return true
}
