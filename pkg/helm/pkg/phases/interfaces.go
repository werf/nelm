package phases

import (
	"github.com/werf/nelm/pkg/helm/pkg/kube"
	"github.com/werf/nelm/pkg/helm/pkg/phases/stages"
)

type Splitter interface {
	Split(resources kube.ResourceList) (stages.SortedStageList, error)
}

type ExternalDepsGenerator interface {
	Generate(stages stages.SortedStageList) error
}
