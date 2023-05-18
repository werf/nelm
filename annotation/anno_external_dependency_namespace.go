package annotation

import (
	"fmt"
	"regexp"
	"strings"
)

var AnnotationKeyPatternExternalDependencyNamespace = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/namespace$`)

var _ Annotationer = (*AnnotationExternalDependencyNamespace)(nil)

func NewAnnotationExternalDependencyNamespace(key, value string) *AnnotationExternalDependencyNamespace {
	return &AnnotationExternalDependencyNamespace{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationExternalDependencyNamespace struct{ *baseAnnotation }

func (a *AnnotationExternalDependencyNamespace) Validate() error {
	keyMatches := AnnotationKeyPatternExternalDependencyNamespace.FindStringSubmatch(a.Key())
	if keyMatches == nil {
		return fmt.Errorf("unexpected annotation %q", a.Key())
	}

	idSubexpIndex := AnnotationKeyPatternExternalDependencyNamespace.SubexpIndex("id")
	if idSubexpIndex == -1 {
		return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternExternalDependencyNamespace.String(), a.Key())
	}

	if len(keyMatches) < idSubexpIndex+1 {
		return fmt.Errorf("can't parse external dependency id for annotation %q", a.Key())
	}

	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, value must not be empty", a.Value(), a.Key())
	}

	// TODO(ilya-lesikov): validate namespace name

	return nil
}

func (a *AnnotationExternalDependencyNamespace) ExternalDependencyId() string {
	matches := AnnotationKeyPatternExternalDependencyNamespace.FindStringSubmatch(strings.TrimSpace(a.Value()))
	idSubexpIndex := AnnotationKeyPatternExternalDependencyNamespace.SubexpIndex("id")
	return matches[idSubexpIndex]
}

func (a *AnnotationExternalDependencyNamespace) ExternalDependencyNamespace() string {
	return strings.TrimSpace(a.Value())
}
