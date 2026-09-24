package release

import (
	"github.com/samber/lo"

	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
)

type Revision struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Version   int    `json:"version"`
}

// DeployedRevisions expects revisions sorted by ascending Version.
func DeployedRevisions(revisions []Revision) []Revision {
	_, lastUninstalledIndex, lastUninstalledFound := lo.FindLastIndexOf(revisions, func(r Revision) bool {
		return r.Status == helmreleasecommon.StatusUninstalled.String() ||
			r.Status == helmreleasecommon.StatusUninstalling.String()
	})

	revisionsSinceUninstalled := revisions
	if lastUninstalledFound {
		if lastUninstalledIndex == len(revisions)-1 {
			return nil
		}

		revisionsSinceUninstalled = revisions[lastUninstalledIndex+1:]
	}

	return lo.Filter(revisionsSinceUninstalled, func(r Revision, _ int) bool {
		return r.Status == helmreleasecommon.StatusDeployed.String() ||
			r.Status == helmreleasecommon.StatusSuperseded.String()
	})
}
