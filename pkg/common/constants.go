package common

const DefaultFieldManager = "helm"
const KubectlEditFieldManager = "kubectl-edit"
const OldFieldManagerPrefix = "werf"

type DeployType string

const (
	// Activated for the first revision of the release.
	DeployTypeInitial DeployType = "Initial"
	// Activated when no successful revision found. But for the very first revision
	// DeployTypeInitial is used instead.
	DeployTypeInstall DeployType = "Install"
	// Activated when a successful revision found.
	DeployTypeUpgrade  DeployType = "Upgrade"
	DeployTypeRollback DeployType = "Rollback"
)

type DeletePolicy string

const (
	DeletePolicySucceeded      DeletePolicy = "succeeded"
	DeletePolicyFailed         DeletePolicy = "failed"
	DeletePolicyBeforeCreation DeletePolicy = "before-creation"
)
