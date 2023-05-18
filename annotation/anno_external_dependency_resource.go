package annotation

import (
	"fmt"
	"regexp"
	"strings"
)

var AnnotationKeyPatternExternalDependencyResource = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/resource$`)

var _ Annotationer = (*AnnotationExternalDependencyResource)(nil)

func NewAnnotationExternalDependencyResource(key, value string) *AnnotationExternalDependencyResource {
	return &AnnotationExternalDependencyResource{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationExternalDependencyResource struct{ *baseAnnotation }

func (a *AnnotationExternalDependencyResource) Validate() error {
	keyMatches := AnnotationKeyPatternExternalDependencyResource.FindStringSubmatch(a.Key())
	if keyMatches == nil {
		return fmt.Errorf("unexpected annotation %q", a.Key())
	}

	idSubexpIndex := AnnotationKeyPatternExternalDependencyResource.SubexpIndex("id")
	if idSubexpIndex == -1 {
		return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternExternalDependencyResource.String(), a.Key())
	}

	if len(keyMatches) < idSubexpIndex+1 {
		return fmt.Errorf("can't parse external dependency id for annotation %q", a.Key())
	}

	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, value must not be empty", a.Value(), a.Key())
	}

	valueElems := strings.Split(strings.TrimSpace(a.Value()), "/")

	if len(valueElems) != 2 {
		return fmt.Errorf("wrong annotation %q value format, should be: type/name", a.Key())
	}

	switch valueElems[0] {
	case "":
		return fmt.Errorf("in annotation %q resource type cannot be empty", a.Key())
	case "all":
		return fmt.Errorf("\"all\" resource type is not allowed in annotation %q", a.Key())
	}

	resourceTypeParts := strings.Split(valueElems[0], ".")
	for _, part := range resourceTypeParts {
		if part == "" {
			return fmt.Errorf("resource type in annotation %q should have dots (.) delimiting only non-empty resource.version.group: %s", a.Key(), a.Value())
		}
	}

	if valueElems[1] == "" {
		return fmt.Errorf("in annotation %q resource name can't be empty", a.Key())
	}

	return nil
}

func (a *AnnotationExternalDependencyResource) ExternalDependencyId() string {
	matches := AnnotationKeyPatternExternalDependencyResource.FindStringSubmatch(strings.TrimSpace(a.Key()))
	idSubexpIndex := AnnotationKeyPatternExternalDependencyResource.SubexpIndex("id")
	return matches[idSubexpIndex]
}

func (a *AnnotationExternalDependencyResource) ExternalDependencyResourceType() string {
	return strings.Split(strings.TrimSpace(a.Value()), "/")[0]
}

func (a *AnnotationExternalDependencyResource) ExternalDependencyResourceName() string {
	return strings.Split(strings.TrimSpace(a.Value()), "/")[1]
}
