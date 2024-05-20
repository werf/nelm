package resrcid

func ResourceIDsSortHandler(id1, id2 *ResourceID) bool {
	kind1 := id1.GroupVersionKind().Kind
	kind2 := id2.GroupVersionKind().Kind
	if kind1 != kind2 {
		return kind1 < kind2
	}

	group1 := id1.GroupVersionKind().Group
	group2 := id2.GroupVersionKind().Group
	if group1 != group2 {
		return group1 < group2
	}

	version1 := id1.GroupVersionKind().Version
	version2 := id2.GroupVersionKind().Version
	if version1 != version2 {
		return version1 < version2
	}

	namespace1 := id1.Namespace()
	namespace2 := id2.Namespace()
	if namespace1 != namespace2 {
		return namespace1 < namespace2
	}

	return id1.Name() < id2.Name()
}
