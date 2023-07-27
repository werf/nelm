package common

type DeletePolicy string

const (
	DeletePolicySucceeded      DeletePolicy = "succeeded"
	DeletePolicyFailed         DeletePolicy = "failed"
	DeletePolicyBeforeCreation DeletePolicy = "before-creation"
)
