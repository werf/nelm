package plan

import (
	"context"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/common"
)

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

func BuildReleaseInfos(ctx context.Context, deployType common.DeployType, prevReleases []*helmrelease.Release, newRel *helmrelease.Release) ([]*ReleaseInfo, error) {
	var infos []*ReleaseInfo
	switch deployType {
	case common.DeployTypeInitial, common.DeployTypeInstall:
		infos = append(infos, &ReleaseInfo{
			Release:                newRel,
			Must:                   ReleaseTypeInstall,
			MustFailOnFailedDeploy: true,
		})

		for _, rel := range prevReleases {
			if rel.Info.Status == helmrelease.StatusDeployed {
				infos = append(infos, &ReleaseInfo{
					Release: rel,
					Must:    ReleaseTypeSupersede,
				})
			}
		}
	case common.DeployTypeUpgrade:
		infos = append(infos, &ReleaseInfo{
			Release:                newRel,
			Must:                   ReleaseTypeUpgrade,
			MustFailOnFailedDeploy: true,
		})

		for _, rel := range prevReleases {
			if rel.Info.Status == helmrelease.StatusDeployed {
				infos = append(infos, &ReleaseInfo{
					Release: rel,
					Must:    ReleaseTypeSupersede,
				})
			}
		}
	case common.DeployTypeRollback:
		infos = append(infos, &ReleaseInfo{
			Release:                newRel,
			Must:                   ReleaseTypeRollback,
			MustFailOnFailedDeploy: true,
		})

		for _, rel := range prevReleases {
			if rel.Info.Status == helmrelease.StatusDeployed {
				infos = append(infos, &ReleaseInfo{
					Release: rel,
					Must:    ReleaseTypeSupersede,
				})
			}
		}
	case common.DeployTypeUninstall:
		for i := 0; i < len(prevReleases); i++ {
			var (
				releaseType        ReleaseType
				failOnFailedDeploy bool
			)

			if i == len(prevReleases)-1 {
				releaseType = ReleaseTypeUninstall
				failOnFailedDeploy = true
			} else {
				releaseType = ReleaseTypeDelete
			}

			infos = append(infos, &ReleaseInfo{
				Release:                prevReleases[i],
				Must:                   releaseType,
				MustFailOnFailedDeploy: failOnFailedDeploy,
			})
		}
	default:
		panic("unexpected deploy type")
	}

	return infos, nil
}
