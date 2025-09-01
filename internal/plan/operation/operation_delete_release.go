package operation

import "github.com/werf/3p-helm/pkg/release"

const (
	OperationTypeDeleteRelease    = "delete-release"
	OperationVersionDeleteRelease = 1
)

var _ OperationConfig = (*OperationConfigDeleteRelease)(nil)

type OperationConfigDeleteRelease struct {
	ReleaseName      string
	ReleaseNamespace string
	ReleaseRevision  int
}

func (c *OperationConfigDeleteRelease) ID() string {
	return release.ReleaseID(c.ReleaseNamespace, c.ReleaseName, c.ReleaseRevision)
}

func (c *OperationConfigDeleteRelease) IDHuman() string {
	return release.ReleaseIDHuman(c.ReleaseNamespace, c.ReleaseName, c.ReleaseRevision)
}
