package dependency

import (
	"github.com/werf/nelm/internal/resource/matcher"
)

func NewInternalDependency(matchNames, matchNamespaces, matchGroups, matchVersions, matchKinds []string, opts InternalDependencyOptions) *InternalDependency {
	var resourceState ResourceState
	if opts.ResourceState == "" {
		resourceState = ResourceStatePresent
	} else {
		resourceState = opts.ResourceState
	}

	resMatcher := matcher.NewResourceMatcher(matchNames, matchNamespaces, matchGroups, matchVersions, matchKinds, matcher.ResourceMatcherOptions{
		DefaultNamespace: opts.DefaultNamespace,
	})

	return &InternalDependency{
		ResourceMatcher: resMatcher,
		ResourceState:   resourceState,
	}
}

type InternalDependencyOptions struct {
	DefaultNamespace string
	ResourceState    ResourceState
}

type InternalDependency struct {
	*matcher.ResourceMatcher
	ResourceState ResourceState
}
