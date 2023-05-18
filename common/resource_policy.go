package common

type ResourcePolicy string

const (
	ResourcePolicyNone ResourcePolicy = ""
	ResourcePolicyKeep ResourcePolicy = "keep"
)

var DefaultResourcePolicy = ResourcePolicyNone

var ResourcePolicies = []ResourcePolicy{
	ResourcePolicyNone,
	ResourcePolicyKeep,
}
