package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewOperationTrackUnmanagedResourcesReadiness() *OperationTrackUnmanagedResourcesReadiness {
	return &OperationTrackUnmanagedResourcesReadiness{}
}

type OperationTrackUnmanagedResourcesReadiness struct {
	Targets []*resource.UnmanagedResource
}

func (o *OperationTrackUnmanagedResourcesReadiness) Type() string {
	return "track-unmanaged-resources-readiness"
}

func (o *OperationTrackUnmanagedResourcesReadiness) AddTargets(targets ...*resource.UnmanagedResource) *OperationTrackUnmanagedResourcesReadiness {
	o.Targets = append(o.Targets, targets...)
	return o
}

func (o *OperationTrackUnmanagedResourcesReadiness) ResourcesWillBeCreatedOrUpdated() bool {
	return false
}

func (o *OperationTrackUnmanagedResourcesReadiness) ResourcesWillBeDeleted() bool {
	return false
}

func (o *OperationTrackUnmanagedResourcesReadiness) ResourcesWillBeTracked() bool {
	return true
}
