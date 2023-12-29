package depnd

import "helm.sh/helm/v3/pkg/werf/resrcmatcher"

func NewInternalDependency(matchNames, matchNamespaces, matchGroups, matchVersions, matchKinds []string, opts InternalDependencyOptions) *InternalDependency {
	resMatcher := resrcmatcher.NewResourceMatcher(matchNames, matchNamespaces, matchGroups, matchVersions, matchKinds, resrcmatcher.ResourceMatcherOptions{
		DefaultNamespace: opts.DefaultNamespace,
	})

	return &InternalDependency{
		ResourceMatcher: resMatcher,
	}
}

type InternalDependencyOptions struct {
	DefaultNamespace string
}

type InternalDependency struct {
	*resrcmatcher.ResourceMatcher
}
