package depnd

import (
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
