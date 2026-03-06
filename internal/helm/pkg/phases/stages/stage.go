package stages

import (
	"github.com/werf/3p-helm/pkg/kube"
	"github.com/werf/3p-helm/pkg/phases/stages/externaldeps"
)

type Stage struct {
	Weight               int
	ExternalDependencies externaldeps.ExternalDependencyList
	DesiredResources     kube.ResourceList
	Result               *kube.Result
}
