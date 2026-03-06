package externaldeps

import (
	"github.com/werf/3p-helm/pkg/kube"
)

type ExternalDependencyList []*ExternalDependency

func (l ExternalDependencyList) AsResourceList() kube.ResourceList {
	resourceList := kube.ResourceList{}
	for _, extDep := range l {
		resourceList = append(resourceList, extDep.Info)
	}

	return resourceList
}
