package annotation

import (
	"fmt"
	"regexp"
	"strings"
)

var AnnotationKeyPatternLogRegexFor = regexp.MustCompile(`^werf.io/log-regex-for-(?P<container>.+)$`)

var _ Annotationer = (*AnnotationLogRegexFor)(nil)

func NewAnnotationLogRegexFor(key, value string) *AnnotationLogRegexFor {
	return &AnnotationLogRegexFor{
		baseAnnotation: newBaseAnnotation(key, value),
	}
}

type AnnotationLogRegexFor struct{ *baseAnnotation }

func (a *AnnotationLogRegexFor) Validate() error {
	keyMatches := AnnotationKeyPatternLogRegexFor.FindStringSubmatch(a.Key())
	if keyMatches == nil {
		return fmt.Errorf("unexpected annotation %q", a.Key())
	}

	containerSubexpIndex := AnnotationKeyPatternLogRegexFor.SubexpIndex("container")
	if containerSubexpIndex == -1 {
		return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternLogRegexFor.String(), a.Key())
	}

	if len(keyMatches) < containerSubexpIndex+1 {
		return fmt.Errorf("can't parse container name for annotation %q", a.Key())
	}

	// TODO(ilya-lesikov): validate container name

	if strings.TrimSpace(a.Value()) == "" {
		return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", a.Value(), a.Key())
	}

	if _, err := regexp.Compile(strings.TrimSpace(a.Value())); err != nil {
		return fmt.Errorf("invalid value %q for annotation %q, expected valid regular expression", a.Value(), a.Key())
	}

	return nil
}

func (a *AnnotationLogRegexFor) ForContainer() string {
	matches := AnnotationKeyPatternLogRegexFor.FindStringSubmatch(a.Key())
	containerSubexpIndex := AnnotationKeyPatternLogRegexFor.SubexpIndex("container")
	return matches[containerSubexpIndex]
}

func (a *AnnotationLogRegexFor) LogRegex() *regexp.Regexp {
	return regexp.MustCompile(strings.TrimSpace(a.Value()))
}
