package dependency

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/internal/resource/id"
)

func NewExternalDependency(name, namespace string, gvk schema.GroupVersionKind, opts ExternalDependencyOptions) *ExternalDependency {
	resID := id.NewResourceID(name, namespace, gvk, id.ResourceIDOptions{
		DefaultNamespace: opts.DefaultNamespace,
		FilePath:         opts.FilePath,
		Mapper:           opts.Mapper,
	})

	return &ExternalDependency{
		ResourceID: resID,
	}
}

type ExternalDependencyOptions struct {
	DefaultNamespace string
	FilePath         string
	Mapper           meta.ResettableRESTMapper
}

type ExternalDependency struct {
	*id.ResourceID
}
