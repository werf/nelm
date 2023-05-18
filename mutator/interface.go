package mutator

import (
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/resource"
)

type RuntimeResourceMutator interface {
	Mutate(resource resource.Resourcer, operationType common.ClientOperationType) (resource.Resourcer, error)
}
