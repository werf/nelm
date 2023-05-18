package common

type HelmHookType string

const (
	HelmHookTypePreInstall   HelmHookType = "pre-install"
	HelmHookTypePostInstall  HelmHookType = "post-install"
	HelmHookTypePreUpgrade   HelmHookType = "pre-upgrade"
	HelmHookTypePostUpgrade  HelmHookType = "post-upgrade"
	HelmHookTypePreRollback  HelmHookType = "pre-rollback"
	HelmHookTypePostRollback HelmHookType = "post-rollback"
	HelmHookTypePreDelete    HelmHookType = "pre-delete"
	HelmHookTypePostDelete   HelmHookType = "post-delete"
	HelmHookTypeTest         HelmHookType = "test"
	HelmHookTypeTestLegacy   HelmHookType = "test-success" // Helm 2 hooks compatibility
)

var HelmHookTypes = []HelmHookType{
	HelmHookTypePreInstall,
	HelmHookTypePostInstall,
	HelmHookTypePreUpgrade,
	HelmHookTypePostUpgrade,
	HelmHookTypePreRollback,
	HelmHookTypePostRollback,
	HelmHookTypePreDelete,
	HelmHookTypePostDelete,
	HelmHookTypeTest,
	HelmHookTypeTestLegacy,
}
