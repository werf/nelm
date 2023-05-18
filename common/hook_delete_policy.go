package common

type HelmHookDeletePolicy string

const (
	HelmHookDeletePolicyBeforeCreation          HelmHookDeletePolicy = "before-hook-creation"
	HelmHookDeletePolicyAfterSuceededCompletion HelmHookDeletePolicy = "hook-succeeded"
	HelmHookDeletePolicyAfterFailedCompletion   HelmHookDeletePolicy = "hook-failed"
)

var DefaultHookPolicy = HelmHookDeletePolicyBeforeCreation

var HookDeletePolicies = []HelmHookDeletePolicy{
	HelmHookDeletePolicyBeforeCreation,
	HelmHookDeletePolicyAfterSuceededCompletion,
	HelmHookDeletePolicyAfterFailedCompletion,
}
