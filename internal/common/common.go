package common

import (
	"fmt"

	"github.com/Masterminds/sprig/v3"
	"github.com/samber/lo"
)

var (
	Brand   = "Nelm"
	Version = "0.0.0"
)

const (
	DefaultFieldManager     = "helm"
	KubectlEditFieldManager = "kubectl-edit"
	OldFieldManagerPrefix   = "werf"
	StagePrefix             = "stage"
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
	StageFinal             Stage = "final"               // succeed pending release, supersede previous release
)

var StagesOrdered = []Stage{
	StageInit,
	StagePrePreUninstall,
	StagePrePreInstall,
	StagePreInstall,
	StagePreUninstall,
	StageInstall,
	StageUninstall,
	StagePostInstall,
	StagePostUninstall,
	StagePostPostInstall,
	StagePostPostUninstall,
	StageFinal,
}

func StagesSortHandler(stage1, stage2 Stage) bool {
	index1 := lo.IndexOf(StagesOrdered, stage1)
	index2 := lo.IndexOf(StagesOrdered, stage2)

	return index1 < index2
}

func SubStageWeighted(stage Stage, weight int) Stage {
	return Stage(fmt.Sprintf("%s/weight:%d", stage, weight))
}

type On string

const (
	InstallOnInstall  On = "install"
	InstallOnUpgrade  On = "upgrade"
	InstallOnRollback On = "rollback"
	InstallOnDelete   On = "delete"
	InstallOnTest     On = "test"
)

type ResourceState string

const (
	ResourceStateAbsent  ResourceState = "absent"
	ResourceStatePresent ResourceState = "present"
	ResourceStateReady   ResourceState = "ready"
)

type StoreAs string

const (
	StoreAsNone    StoreAs = "none"
	StoreAsHook    StoreAs = "hook"
	StoreAsRegular StoreAs = "regular"
)

var OrderedStoreAs = []StoreAs{StoreAsNone, StoreAsHook, StoreAsRegular}

var SprigFuncs = sprig.TxtFuncMap()
