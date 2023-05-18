package annotation

import (
	"fmt"
	"regexp"
	"strings"
)

var AnnotationKeyPatternReleaseName = regexp.MustCompile(`^meta.helm.sh/release-name$`)

var _ Annotationer = (*AnnotationReleaseName)(nil)

func NewAnnotationReleaseName(key, value string) *AnnotationReleaseName {
	return &AnnotationReleaseName{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationReleaseName struct{ *baseAnnotation }

func (a *AnnotationReleaseName) Validate() error {
	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, value must not be empty", a.Value(), a.Key())
	}

	// TODO(ilya-lesikov): validate release name

	return nil
}

func (a *AnnotationReleaseName) ReleaseName() string {
	return strings.TrimSpace(a.Value())
}
