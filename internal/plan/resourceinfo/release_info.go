package resourceinfo

import helmrelease "github.com/werf/3p-helm/pkg/release"

type ReleaseType string

const (
	ReleaseTypeNone      ReleaseType = "none"
	ReleaseTypeInstall   ReleaseType = "install"
	ReleaseTypeUpgrade   ReleaseType = "upgrade"
	ReleaseTypeRollback  ReleaseType = "rollback"
	ReleaseTypeSupersede ReleaseType = "supersede"
	ReleaseTypeUninstall ReleaseType = "uninstall"
	ReleaseTypeDelete    ReleaseType = "delete"
)

type ReleaseInfo struct {
	Release *helmrelease.Release

	Must                   ReleaseType
	MustFailOnFailedDeploy bool
}
