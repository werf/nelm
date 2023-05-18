package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewOperationTrackExternalDependenciesReadiness() *OperationTrackExternalDependenciesReadiness {
	return &OperationTrackExternalDependenciesReadiness{}
}

type OperationTrackExternalDependenciesReadiness struct {
	Targets []*resource.ExternalDependency
}

func (o *OperationTrackExternalDependenciesReadiness) Type() string {
	return "track-external-dependencies-readiness"
}

func (o *OperationTrackExternalDependenciesReadiness) AddTargets(targets ...*resource.ExternalDependency) *OperationTrackExternalDependenciesReadiness {
	o.Targets = append(o.Targets, targets...)
	return o
}

func (o *OperationTrackExternalDependenciesReadiness) ResourcesWillBeCreatedOrUpdated() bool {
	return false
}

func (o *OperationTrackExternalDependenciesReadiness) ResourcesWillBeDeleted() bool {
	return false
}

func (o *OperationTrackExternalDependenciesReadiness) ResourcesWillBeTracked() bool {
	return true
}
