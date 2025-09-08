package dependency

import (
	"github.com/werf/nelm/internal/resource/id"
)

func NewExternalDependency(meta *id.ResourceMeta) *ExternalDependency {
	return &ExternalDependency{
		ResourceMeta: meta,
	}
}

type ExternalDependency struct {
	*id.ResourceMeta
}
