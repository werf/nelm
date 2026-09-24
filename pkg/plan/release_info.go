package plan

import (
	"context"
	"fmt"

	"github.com/werf/nelm/pkg/common"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	"github.com/werf/nelm/pkg/release"
)

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

type ReleaseType string

// Data class, which stores all info to make a decision on what to do with the release revision
// in the plan.
type ReleaseInfo struct {
	Revision release.Revision          `json:"revision"`
	Release  *release.VersionedRelease `json:"release"`

	Must                   ReleaseType `json:"must"`
	MustFailOnFailedDeploy bool        `json:"mustFailOnFailedDeploy"`
}

// Build ReleaseInfos from Releases that we got from the cluster. Here we actually decide on what to
// do with each release revision. Compute here as much as you can: Release shouldn't be used for
// decision making (its just a JSON representation of a Helm release) and BuildPlan is complex
// enough already.
func BuildReleaseInfos(ctx context.Context, deployType common.DeployType, prevReleases []helmrel.Accessor, newRel helmrel.Accessor) ([]*ReleaseInfo, error) {
	var infos []*ReleaseInfo
	switch deployType {
	case common.DeployTypeInitial, common.DeployTypeInstall:
		infos = append(infos, &ReleaseInfo{
			Must:                   ReleaseTypeInstall,
			MustFailOnFailedDeploy: true,
			Release:                &release.VersionedRelease{Accessor: newRel},
			Revision:               revisionFromAccessor(newRel),
		})

		for _, rel := range prevReleases {
			if rel.Status() == helmreleasecommon.StatusDeployed.String() {
				infos = append(infos, &ReleaseInfo{
					Must:     ReleaseTypeSupersede,
					Release:  &release.VersionedRelease{Accessor: rel},
					Revision: revisionFromAccessor(rel),
				})
			}
		}
	case common.DeployTypeUpgrade:
		infos = append(infos, &ReleaseInfo{
			Must:                   ReleaseTypeUpgrade,
			MustFailOnFailedDeploy: true,
			Release:                &release.VersionedRelease{Accessor: newRel},
			Revision:               revisionFromAccessor(newRel),
		})

		for _, rel := range prevReleases {
			if rel.Status() == helmreleasecommon.StatusDeployed.String() {
				infos = append(infos, &ReleaseInfo{
					Must:     ReleaseTypeSupersede,
					Release:  &release.VersionedRelease{Accessor: rel},
					Revision: revisionFromAccessor(rel),
				})
			}
		}
	case common.DeployTypeRollback:
		infos = append(infos, &ReleaseInfo{
			Must:                   ReleaseTypeRollback,
			MustFailOnFailedDeploy: true,
			Release:                &release.VersionedRelease{Accessor: newRel},
			Revision:               revisionFromAccessor(newRel),
		})

		for _, rel := range prevReleases {
			if rel.Status() == helmreleasecommon.StatusDeployed.String() {
				infos = append(infos, &ReleaseInfo{
					Must:     ReleaseTypeSupersede,
					Release:  &release.VersionedRelease{Accessor: rel},
					Revision: revisionFromAccessor(rel),
				})
			}
		}
	case common.DeployTypeUninstall:
		return nil, fmt.Errorf("build release infos: uninstall release infos must be built with BuildUninstallReleaseInfos")
	default:
		panic("unexpected deploy type")
	}

	return infos, nil
}

// Build ReleaseInfos for an uninstall. Only the last revision needs a release body, since it is the
// only one whose status is rewritten; older revisions are dropped by name and version alone.
func BuildUninstallReleaseInfos(ctx context.Context, revisions []release.Revision, lastRel helmrel.Accessor) ([]*ReleaseInfo, error) {
	if len(revisions) == 0 {
		return nil, nil
	}

	if lastRel == nil {
		return nil, fmt.Errorf("build uninstall release infos: no release body for the last revision")
	}

	infos := make([]*ReleaseInfo, 0, len(revisions))
	for i, revision := range revisions {
		if i < len(revisions)-1 {
			infos = append(infos, &ReleaseInfo{
				Must:     ReleaseTypeDelete,
				Revision: revision,
			})

			continue
		}

		infos = append(infos, &ReleaseInfo{
			Must:                   ReleaseTypeUninstall,
			MustFailOnFailedDeploy: true,
			Release:                &release.VersionedRelease{Accessor: lastRel},
			Revision:               revision,
		})
	}

	return infos, nil
}

func revisionFromAccessor(rel helmrel.Accessor) release.Revision {
	return release.Revision{
		Name:      rel.Name(),
		Namespace: rel.Namespace(),
		Status:    rel.Status(),
		Version:   rel.Version(),
	}
}
