package common

import (
	"github.com/Masterminds/sprig/v3"
)

var (
	Brand   = "Nelm"
	Version = "0.0.0"
)

const (
	DefaultFieldManager     = "helm"
	KubectlEditFieldManager = "kubectl-edit"
	OldFieldManagerPrefix   = "werf"
	StageStartSuffix        = "start"
	StageEndSuffix          = "end"
)

type DeployType string

const (
	// Activated for the first revision of the release.
	DeployTypeInitial DeployType = "Initial"
	// Activated when no successful revision found. But for the very first revision
	// DeployTypeInitial is used instead.
	DeployTypeInstall DeployType = "Install"
	// Activated when a successful revision found.
	DeployTypeUpgrade   DeployType = "Upgrade"
	DeployTypeRollback  DeployType = "Rollback"
	DeployTypeUninstall DeployType = "Uninstall"
)

type DeletePolicy string

const (
	DeletePolicySucceeded      DeletePolicy = "succeeded"
	DeletePolicyFailed         DeletePolicy = "failed"
	DeletePolicyBeforeCreation DeletePolicy = "before-creation"
)

type Ownership string

const (
	OwnershipEveryone Ownership = "everyone"
	OwnershipRelease  Ownership = "release"
)

type Stage string

const (
	StageInit              Stage = "init"              // create pending release
	StagePrePreUninstall   Stage = "pre-pre-uninstall" // uninstall previous release resources
	StagePrePreInstall     Stage = "pre-pre-install"   // install crd
	StagePreInstall        Stage = "pre-install"       // install pre-hooks
	StagePreUninstall      Stage = "pre-uninstall"     // cleanup pre-hooks
	StageInstall           Stage = "install"           // install resources
	StageUninstall         Stage = "uninstall"         // cleanup resources
	StagePostInstall       Stage = "post-install"      // install post-hooks
	StagePostUninstall     Stage = "post-uninstall"    // cleanup post-hooks
	StagePostPostInstall   Stage = "post-post-install"
	StagePostPostUninstall Stage = "post-post-uninstall" // uninstall crd, webhook
	StageFinal             Stage = "final"               // succeed pending release
)

type On string

const (
	InstallOnInstall  On = "install"
	InstallOnUpgrade  On = "upgrade"
	InstallOnRollback On = "rollback"
	InstallOnDelete   On = "delete"
	InstallOnTest     On = "test"
)

var SprigFuncs = sprig.TxtFuncMap()
