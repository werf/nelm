package depnd

import (
	"github.com/werf/nelm/internal/resrcmatcher"
)

func NewInternalDependency(matchNames, matchNamespaces, matchGroups, matchVersions, matchKinds []string, opts InternalDependencyOptions) *InternalDependency {
	var resourceState ResourceState
	if opts.ResourceState == "" {
		resourceState = ResourceStatePresent
	} else {
		resourceState = opts.ResourceState
	}

	resMatcher := resrcmatcher.NewResourceMatcher(matchNames, matchNamespaces, matchGroups, matchVersions, matchKinds, resrcmatcher.ResourceMatcherOptions{
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
	*resrcmatcher.ResourceMatcher
	ResourceState ResourceState
}
