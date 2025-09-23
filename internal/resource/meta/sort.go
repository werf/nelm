package meta

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
