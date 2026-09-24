package release

import (
	"github.com/samber/lo"

	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
)

type Revision struct {
	Name      string
	Namespace string
	Status    string
	Version   int
}

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
