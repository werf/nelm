package stages

import (
	"github.com/werf/nelm/internal/helm/pkg/kube"
	"github.com/werf/nelm/internal/helm/pkg/phases/stages/externaldeps"
)

type Stage struct {
	Weight               int
	ExternalDependencies externaldeps.ExternalDependencyList
	DesiredResources     kube.ResourceList
	Result               *kube.Result
}
