package spec

import "github.com/werf/nelm/pkg/common"

func ResourceSpecSortHandler(r1, r2 *ResourceSpec) bool {
	sortAs1 := r1.StoreAs
	sortAs2 := r2.StoreAs

	// TODO(major): sorted based on sortAs for compatibility. In future should just probably sort
	// like this: first CRDs (any type), then helm.sh/hook hooks, then the rest
	if sortAs1 != sortAs2 {
		if sortAs1 == common.StoreAsNone {
			return true
		} else if sortAs1 == common.StoreAsHook && sortAs2 != common.StoreAsNone {
			return true
		} else {
			return false
		}
	}

	return ResourceMetaSortHandler(r1.ResourceMeta, r2.ResourceMeta)
}

func ResourceMetaSortHandler(r1, r2 *ResourceMeta) bool {
	kind1 := r1.GroupVersionKind.Kind
	kind2 := r2.GroupVersionKind.Kind

	if kind1 != kind2 {
		return kind1 < kind2
	}

	group1 := r1.GroupVersionKind.Group
	group2 := r2.GroupVersionKind.Group

	if group1 != group2 {
		return group1 < group2
	}

	version1 := r1.GroupVersionKind.Version
	version2 := r2.GroupVersionKind.Version

	if version1 != version2 {
		return version1 < version2
	}

	namespace1 := r1.Namespace
	namespace2 := r2.Namespace

	if namespace1 != namespace2 {
		return namespace1 < namespace2
	}

	return r1.Name < r2.Name
}
