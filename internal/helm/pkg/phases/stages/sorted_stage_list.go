package stages

import (
	"github.com/werf/3p-helm/pkg/kube"
)

type SortedStageList []*Stage

func (l SortedStageList) Len() int {
	return len(l)
}

func (l SortedStageList) Less(i, j int) bool {
	return l[i].Weight < l[j].Weight
}

func (l SortedStageList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l SortedStageList) StageByWeight(weight int) *Stage {
	for _, stg := range l {
		if stg.Weight == weight {
			return stg
		}
	}

	return nil
}

func (l SortedStageList) MergedCreatedResources() kube.ResourceList {
	return l.MergedCreatedResourcesInStagesRange(0, len(l)-1)
}

func (l SortedStageList) MergedCreatedResourcesInStagesRange(first, last int) kube.ResourceList {
	created := kube.ResourceList{}
	for i := first; i <= last; i++ {
		stg := l[i]

		if stg.Result == nil {
			continue
		}

		created.Merge(stg.Result.Created)
	}

	return created
}

func (l SortedStageList) MergedUpdatedResources() kube.ResourceList {
	return l.MergedUpdatedResourcesInStagesRange(0, len(l)-1)
}

func (l SortedStageList) MergedUpdatedResourcesInStagesRange(first, last int) kube.ResourceList {
	updated := kube.ResourceList{}
	for i := first; i <= last; i++ {
		stg := l[i]

		if stg.Result == nil {
			continue
		}

		updated.Merge(stg.Result.Updated)
	}

	return updated
}

func (l SortedStageList) MergedDeletedResources() kube.ResourceList {
	return l.MergedDeletedResourcesInStagesRange(0, len(l)-1)
}

func (l SortedStageList) MergedDeletedResourcesInStagesRange(first, last int) kube.ResourceList {
	deleted := kube.ResourceList{}
	for i := first; i <= last; i++ {
		stg := l[i]

		if stg.Result == nil {
			continue
		}

		deleted.Merge(stg.Result.Deleted)
	}

	return deleted
}

func (l SortedStageList) MergedDesiredResources() kube.ResourceList {
	return l.MergedDesiredResourcesInStagesRange(0, len(l)-1)
}

func (l SortedStageList) MergedDesiredResourcesInStagesRange(first, last int) kube.ResourceList {
	resources := kube.ResourceList{}
	for i := first; i <= last; i++ {
		stg := l[i]
		resources.Merge(stg.DesiredResources)
	}

	return resources
}
