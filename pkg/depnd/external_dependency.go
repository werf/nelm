package depnd

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/nelm/resrcid"
)

func NewExternalDependency(name, namespace string, gvk schema.GroupVersionKind, opts ExternalDependencyOptions) *ExternalDependency {
	resID := resrcid.NewResourceID(name, namespace, gvk, resrcid.ResourceIDOptions{
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
	*resrcid.ResourceID
}
