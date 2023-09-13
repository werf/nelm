package resrcmatcher

import (
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"helm.sh/helm/v3/pkg/werf/utls"
)

func NewResourceMatcher(names, namespaces, groups, versions, kinds []string, opts ResourceMatcherOptions) *ResourceMatcher {
	var nses []string
	for _, ns := range namespaces {
		nses = append(nses, utls.FallbackNamespace(ns, opts.DefaultNamespace))
	}

	return &ResourceMatcher{
		names:      names,
		namespaces: nses,
		groups:     groups,
		versions:   versions,
		kinds:      kinds,
	}
}

type ResourceMatcherOptions struct {
	DefaultNamespace string
}

type ResourceMatcher struct {
	names      []string
	namespaces []string
	groups     []string
	versions   []string
	kinds      []string
}

func (s *ResourceMatcher) Match(resource *resrcid.ResourceID) bool {
	var nameMatch bool
	if len(s.names) == 0 {
		nameMatch = true
	} else {
		for _, name := range s.names {
			if resource.Name() == name {
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
			if resource.Namespace() == namespace {
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
			if resource.GroupVersionKind().Group == group {
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
			if resource.GroupVersionKind().Version == version {
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
			if resource.GroupVersionKind().Kind == kind {
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
