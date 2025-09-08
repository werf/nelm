package dependency

import (
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource/matcher"
)

func NewInternalDependency(matchNames, matchNamespaces, matchGroups, matchVersions, matchKinds []string, matchState common.ResourceState) *InternalDependency {
	resMatcher := matcher.NewResourceMatcher(matchNames, matchNamespaces, matchGroups, matchVersions, matchKinds, matcher.ResourceMatcherOptions{})

	return &InternalDependency{
		ResourceMatcher: resMatcher,
		ResourceState:   matchState,
	}
}

type InternalDependency struct {
	*matcher.ResourceMatcher

	ResourceState common.ResourceState
}
