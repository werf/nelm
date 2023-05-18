package resource

import (
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ Referencer = (*Reference)(nil)

func NewReference(name, namespace string, groupVersionKind schema.GroupVersionKind) *Reference {
	return &Reference{
		name:             name,
		namespace:        namespace,
		groupVersionKind: groupVersionKind,
	}
}

type Reference struct {
	name             string
	namespace        string
	groupVersionKind schema.GroupVersionKind
}

func (r *Reference) Validate() error {
	return nil
}

func (r *Reference) Name() string {
	return r.name
}

func (r *Reference) Namespace() string {
	return r.namespace
}

func (r *Reference) GroupVersionKind() schema.GroupVersionKind {
	return r.groupVersionKind
}

func (r *Reference) Matches(other Referencer) bool {
	return r.String() == other.String()
}

func (r *Reference) String() string {
	apiVersion, kind := r.GroupVersionKind().ToAPIVersionAndKind()

	var resultParts []string
	for _, part := range []string{apiVersion, kind, r.Namespace(), r.Name()} {
		if part == "" {
			continue
		}

		resultParts = append(resultParts, part)
	}

	return strings.Join(resultParts, "/")
}

func BuildReferencesFromHeads(heads ...*Head) []*Reference {
	var result []*Reference
	for _, head := range heads {
		result = append(result, &Reference{
			name:             head.Name,
			namespace:        head.Namespace,
			groupVersionKind: head.GroupVersionKind(),
		})
	}

	return result
}

func BuildReferencesFromResources(resources ...*baseResource) []*Reference {
	var result []*Reference
	for _, resource := range resources {
		result = append(result, &Reference{
			name:             resource.Name(),
			namespace:        resource.Namespace(),
			groupVersionKind: resource.GroupVersionKind(),
		})
	}

	return result
}

func DiffReferencerLists(source []Referencer, target []Referencer) (removed []Referencer, added []Referencer, kept []Referencer) {
	keptMap := map[string]Referencer{}

firstLoop:
	for _, src := range source {
		for _, t := range target {
			if src.Matches(t) {
				keptMap[src.String()] = src
				continue firstLoop
			}
		}

		removed = append(removed, src)
	}

secondLoop:
	for _, t := range target {
		for _, src := range source {
			if t.Matches(src) {
				if _, found := keptMap[t.String()]; !found {
					keptMap[t.String()] = t
				}
				continue secondLoop
			}
		}

		added = append(added, t)
	}

	for _, ref := range keptMap {
		kept = append(kept, ref)
	}

	return removed, added, kept
}
