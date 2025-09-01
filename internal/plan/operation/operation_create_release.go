package operation

import (
	helmrelease "github.com/werf/3p-helm/pkg/release"
)

const (
	OperationTypeCreateRelease    = "create-release"
	OperationVersionCreateRelease = 1
)

var _ OperationConfig = (*OperationConfigCreateRelease)(nil)

type OperationConfigCreateRelease struct {
	Release *helmrelease.Release
}

func (c *OperationConfigCreateRelease) ID() string {
	return c.Release.ID()
}

func (c *OperationConfigCreateRelease) IDHuman() string {
	return c.Release.IDHuman()
}
