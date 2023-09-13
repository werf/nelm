package resrcid

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/werf/utls"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func NewResourceID(name, namespace string, gvk schema.GroupVersionKind, opts ResourceIDOptions) *ResourceID {
	return &ResourceID{
		name:             name,
		namespace:        namespace,
		gvk:              gvk,
		defaultNamespace: opts.DefaultNamespace,
		filePath:         opts.FilePath,
		mapper:           opts.Mapper,
	}
}

func NewResourceIDFromUnstruct(unstruct *unstructured.Unstructured, opts ResourceIDOptions) *ResourceID {
	return NewResourceID(unstruct.GetName(), unstruct.GetNamespace(), unstruct.GroupVersionKind(), opts)
}

type ResourceIDOptions struct {
	DefaultNamespace string
	FilePath         string
	Mapper           meta.ResettableRESTMapper
}

func NewResourceIDFromID(id string, opts ResourceIDOptions) *ResourceID {
	split := strings.SplitN(id, ":", 5)
	lo.Must0(len(split) == 5)

	return NewResourceID(split[4], split[0], schema.GroupVersionKind{
		Group:   split[1],
		Version: split[2],
		Kind:    split[3],
	}, opts)
}

type ResourceID struct {
	name             string
	namespace        string
	gvk              schema.GroupVersionKind
	defaultNamespace string
	filePath         string
	mapper           meta.ResettableRESTMapper
}

func (i *ResourceID) Name() string {
	return i.name
}

func (i *ResourceID) Namespace() string {
	return utls.FallbackNamespace(i.namespace, i.defaultNamespace)
}

func (i *ResourceID) Namespaced() (namespaced bool, err error) {
	if i.mapper == nil {
		panic("don't call Namespaced() without mapper")
	}

	mapping, err := i.mapper.RESTMapping(i.gvk.GroupKind(), i.gvk.Version)
	if err != nil {
		return false, fmt.Errorf("error getting resource mapping for %q: %w", i.HumanID(), err)
	}

	return mapping.Scope == meta.RESTScopeNamespace, nil
}

func (i *ResourceID) GroupVersionKind() schema.GroupVersionKind {
	return i.gvk
}

func (i *ResourceID) GroupVersionResource() (schema.GroupVersionResource, error) {
	gvk := i.GroupVersionKind()
	mapping, err := i.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("error getting resource mapping for %q: %w", i.HumanID(), err)
	}

	return mapping.Resource, nil
}

func (i *ResourceID) FilePath() string {
	return i.filePath
}

func (i *ResourceID) ID() string {
	return fmt.Sprintf("%s:%s:%s:%s:%s", i.Namespace(), i.gvk.Group, i.gvk.Version, i.gvk.Kind, i.name)
}

func (i *ResourceID) HumanID() string {
	if i.namespace != i.defaultNamespace && i.namespace != "" {
		return fmt.Sprintf("%s/%s/%s", i.namespace, i.gvk.Kind, i.Name())
	}

	return fmt.Sprintf("%s/%s", i.gvk.Kind, i.Name())
}
