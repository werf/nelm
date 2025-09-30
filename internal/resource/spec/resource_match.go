package spec

type ResourceMatcher struct {
	Names      []string
	Namespaces []string
	Groups     []string
	Versions   []string
	Kinds      []string
}

func (s *ResourceMatcher) Match(resMeta *ResourceMeta) bool {
	var nameMatch bool
	if len(s.Names) == 0 {
		nameMatch = true
	} else {
		for _, name := range s.Names {
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
	if len(s.Namespaces) == 0 {
		namespaceMatch = true
	} else {
		for _, namespace := range s.Namespaces {
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
	if len(s.Groups) == 0 {
		groupMatch = true
	} else {
		for _, group := range s.Groups {
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
	if len(s.Versions) == 0 {
		versionMatch = true
	} else {
		for _, version := range s.Versions {
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
	if len(s.Kinds) == 0 {
		kindMatch = true
	} else {
		for _, kind := range s.Kinds {
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
