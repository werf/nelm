package plan

type Operation interface {
	Type() string
	ResourcesWillBeCreatedOrUpdated() bool
	ResourcesWillBeDeleted() bool
	ResourcesWillBeTracked() bool
}
