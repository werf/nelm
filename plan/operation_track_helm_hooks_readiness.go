package plan

import "helm.sh/helm/v3/pkg/werf/resource"

func NewOperationTrackHelmHooksReadiness() *OperationTrackHelmHooksReadiness {
	return &OperationTrackHelmHooksReadiness{}
}

type OperationTrackHelmHooksReadiness struct {
	Targets []*resource.HelmHook
}

func (o *OperationTrackHelmHooksReadiness) Type() string {
	return "track-helm-hooks-readiness"
}

func (o *OperationTrackHelmHooksReadiness) AddTargets(targets ...*resource.HelmHook) *OperationTrackHelmHooksReadiness {
	o.Targets = append(o.Targets, targets...)
	return o
}

func (o *OperationTrackHelmHooksReadiness) ResourcesWillBeCreatedOrUpdated() bool {
	return false
}

func (o *OperationTrackHelmHooksReadiness) ResourcesWillBeDeleted() bool {
	return false
}

func (o *OperationTrackHelmHooksReadiness) ResourcesWillBeTracked() bool {
	return true
}
