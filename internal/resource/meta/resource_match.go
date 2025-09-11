package meta

type ResourceMatcherOptions struct{}

func NewResourceMatcher(names, namespaces, groups, versions, kinds []string, opts ResourceMatcherOptions) *ResourceMatcher {
	return &ResourceMatcher{
		names:      names,
		namespaces: namespaces,
		groups:     groups,
		versions:   versions,
		kinds:      kinds,
	}
}

type ResourceMatcher struct {
	names      []string
	namespaces []string
	groups     []string
	versions   []string
	kinds      []string
}

func (s *ResourceMatcher) Match(resMeta *ResourceMeta) bool {
	var nameMatch bool
	if len(s.names) == 0 {
		nameMatch = true
	} else {
		for _, name := range s.names {
			if resMeta.Name == name {
				nameMatch = true
				break
			}
		}
	}
	if !nameMatch {
		return false
	}

	var namespaceMatch bool
	if len(s.namespaces) == 0 {
		namespaceMatch = true
	} else {
		for _, namespace := range s.namespaces {
			if resMeta.Namespace == namespace {
				namespaceMatch = true
				break
			}
		}
	}
	if !namespaceMatch {
		return false
	}

	var groupMatch bool
	if len(s.groups) == 0 {
		groupMatch = true
	} else {
		for _, group := range s.groups {
			if resMeta.GroupVersionKind.Group == group {
				groupMatch = true
				break
			}
		}
	}
	if !groupMatch {
		return false
	}

	var versionMatch bool
	if len(s.versions) == 0 {
		versionMatch = true
	} else {
		for _, version := range s.versions {
			if resMeta.GroupVersionKind.Version == version {
				versionMatch = true
				break
			}
		}
	}
	if !versionMatch {
		return false
	}

	var kindMatch bool
	if len(s.kinds) == 0 {
		kindMatch = true
	} else {
		for _, kind := range s.kinds {
			if resMeta.GroupVersionKind.Kind == kind {
				kindMatch = true
				break
			}
		}
	}
	if !kindMatch {
		return false
	}

	return true
}
