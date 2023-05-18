package plan

import "helm.sh/helm/v3/pkg/release"

func NewOperationCreateReleases() *OperationCreateReleases {
	return &OperationCreateReleases{}
}

type OperationCreateReleases struct {
	Releases []*release.Release
}

func (o *OperationCreateReleases) Type() string {
	return "create-releases"
}

func (o *OperationCreateReleases) AddReleases(releases ...*release.Release) *OperationCreateReleases {
	o.Releases = append(o.Releases, releases...)
	return o
}

func (o *OperationCreateReleases) ResourcesWillBeCreatedOrUpdated() bool {
	return false
}

func (o *OperationCreateReleases) ResourcesWillBeDeleted() bool {
	return false
}

func (o *OperationCreateReleases) ResourcesWillBeTracked() bool {
	return false
}
