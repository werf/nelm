package stages

import (
	"github.com/werf/nelm/pkg/helm/pkg/kube"
	"github.com/werf/nelm/pkg/helm/pkg/phases/stages/externaldeps"
)

type Stage struct {
	Weight               int
	ExternalDependencies externaldeps.ExternalDependencyList
	DesiredResources     kube.ResourceList
	Result               *kube.Result
}
