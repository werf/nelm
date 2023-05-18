package plan

import (
	"helm.sh/helm/v3/pkg/release"
)

func NewOperationUpdateReleases() *OperationUpdateReleases {
	return &OperationUpdateReleases{}
}

type OperationUpdateReleases struct {
	Releases []*release.Release
}

func (o *OperationUpdateReleases) Type() string {
	return "update-releases"
}

func (o *OperationUpdateReleases) AddReleases(releases ...*release.Release) *OperationUpdateReleases {
	o.Releases = append(o.Releases, releases...)
	return o
}

func (o *OperationUpdateReleases) ResourcesWillBeCreatedOrUpdated() bool {
	return false
}

func (o *OperationUpdateReleases) ResourcesWillBeDeleted() bool {
	return false
}

func (o *OperationUpdateReleases) ResourcesWillBeTracked() bool {
	return false
}
