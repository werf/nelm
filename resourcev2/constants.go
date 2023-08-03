package resourcev2

type ResourceScope string

const (
	ResourceScopeNamespace ResourceScope = "namespace"
	ResourceScopeCluster   ResourceScope = "cluster"
	ResourceScopeUnknown   ResourceScope = "unknown"
)
