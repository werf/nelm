package action

import "fmt"

type ReleaseNotFoundError struct {
	ReleaseName      string
	ReleaseNamespace string
}

func (e *ReleaseNotFoundError) Error() string {
	return fmt.Sprintf("release %q (namespace %q) not found", e.ReleaseName, e.ReleaseNamespace)
}

type ReleaseRevisionNotFoundError struct {
	ReleaseName      string
	ReleaseNamespace string
	Revision         int
}

func (e *ReleaseRevisionNotFoundError) Error() string {
	return fmt.Sprintf("revision %d of release %q (namespace %q) not found", e.Revision, e.ReleaseName, e.ReleaseNamespace)
}
