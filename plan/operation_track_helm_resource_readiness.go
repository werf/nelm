package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewOperationTrackHelmResourcesReadiness() *OperationTrackHelmResourcesReadiness {
	return &OperationTrackHelmResourcesReadiness{}
}

type OperationTrackHelmResourcesReadiness struct {
	Targets []*resource.HelmResource
}

func (o *OperationTrackHelmResourcesReadiness) Type() string {
	return "track-helm-resources-readiness"
}

func (o *OperationTrackHelmResourcesReadiness) AddTargets(targets ...*resource.HelmResource) *OperationTrackHelmResourcesReadiness {
	o.Targets = append(o.Targets, targets...)
	return o
}

func (o *OperationTrackHelmResourcesReadiness) ResourcesWillBeCreatedOrUpdated() bool {
	return false
}

func (o *OperationTrackHelmResourcesReadiness) ResourcesWillBeDeleted() bool {
	return false
}

func (o *OperationTrackHelmResourcesReadiness) ResourcesWillBeTracked() bool {
	return true
}
