package dependency

type ResourceState string

const (
	ResourceStateAbsent  ResourceState = "absent"
	ResourceStatePresent ResourceState = "present"
	ResourceStateReady   ResourceState = "ready"
)
