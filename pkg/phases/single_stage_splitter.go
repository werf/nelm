package phases

import (
	"fmt"

	"github.com/werf/3p-helm/pkg/kube"
	"github.com/werf/3p-helm/pkg/phases/stages"
	"k8s.io/cli-runtime/pkg/resource"
)

type SingleStageSplitter struct{}

func (s *SingleStageSplitter) Split(resources kube.ResourceList) (stages.SortedStageList, error) {
	stage := &stages.Stage{}

	if err := resources.Visit(func(res *resource.Info, err error) error {
		if err != nil {
			return err
		}

		stage.DesiredResources.Append(res)

		return nil
	}); err != nil {
		return nil, fmt.Errorf("error visiting resources list: %w", err)
	}

	return stages.SortedStageList{stage}, nil
}
