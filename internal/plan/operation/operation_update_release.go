package operation

import (
	helmrelease "github.com/werf/3p-helm/pkg/release"
)

const (
	OperationTypeUpdateRelease    = "update-release"
	OperationVersionUpdateRelease = 1
)

var _ OperationConfig = (*OperationConfigUpdateRelease)(nil)

type OperationConfigUpdateRelease struct {
	Release *helmrelease.Release
}

func (c *OperationConfigUpdateRelease) ID() string {
	return c.Release.ID()
}

func (c *OperationConfigUpdateRelease) IDHuman() string {
	return c.Release.IDHuman()
}
