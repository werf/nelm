package annotation

import (
	"fmt"
	"regexp"
	"strings"
)

var AnnotationKeyPatternReleaseNamespace = regexp.MustCompile(`^meta.helm.sh/release-namespace$`)

var _ Annotationer = (*AnnotationReleaseNamespace)(nil)

func NewAnnotationReleaseNamespace(key, value string) *AnnotationReleaseNamespace {
	return &AnnotationReleaseNamespace{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationReleaseNamespace struct{ *baseAnnotation }

func (a *AnnotationReleaseNamespace) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, value must not be empty", a.Value(), a.Key())
	}

	// TODO(ilya-lesikov): validate namespace name

	return nil
}

func (a *AnnotationReleaseNamespace) ReleaseNamespace() string {
	return strings.TrimSpace(a.Value())
}
