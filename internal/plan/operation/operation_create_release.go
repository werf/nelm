package operation

import (
	helmrelease "github.com/werf/3p-helm/pkg/release"
)

const (
	OperationTypeCreateRelease    OperationType    = "create-release"
	OperationVersionCreateRelease OperationVersion = 1
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
