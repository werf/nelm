package plan

import (
	"context"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/pkg/common"
)

type ReleaseType string

const (
	// No-op
	ReleaseTypeNone ReleaseType = "none"
	// First release revision is to be installed
	ReleaseTypeInstall ReleaseType = "install"
	// New release revision is to be installed as an upgrade over the previous one
	ReleaseTypeUpgrade ReleaseType = "upgrade"
	// New release revision is to be installed, based on one of the previous revisions
	ReleaseTypeRollback ReleaseType = "rollback"
	// One of the previous revisions is to be superseded by a successful release
	ReleaseTypeSupersede ReleaseType = "supersede"
	// Release is to be uninstalled as a whole, with its resources
	ReleaseTypeUninstall ReleaseType = "uninstall"
	// Release revision is to be dropped/deleted (its resources are untouched)
	ReleaseTypeDelete ReleaseType = "delete"
)

// Data class, which stores all info to make a decision on what to do with the release revision
// in the plan.
type ReleaseInfo struct {
	Release *helmrelease.Release `json:"release"`

	Must                   ReleaseType `json:"must"`
	MustFailOnFailedDeploy bool        `json:"mustFailOnFailedDeploy"`
}

// Build ReleaseInfos from Releases that we got from the cluster. Here we actually decide on what to
// do with each release revision. Compute here as much as you can: Release shouldn't be used for
// decision making (its just a JSON representation of a Helm release) and BuildPlan is complex
// enough already.
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
