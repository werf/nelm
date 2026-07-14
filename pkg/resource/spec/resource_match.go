package spec

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var
// once regardless of how many resources it is matched against.
regexpCache sync.Map // string -> *regexp.Regexp

// ResourceMatcher matches resources by metadata. Fields AND together; values
// within a field OR together; an empty field matches everything. String values
// use the /regex/ convention: a bare value is an exact match (case-insensitive
// for groups/versions/kinds, case-sensitive for names/namespaces/charts), a
// value wrapped in slashes is an anchored regexp. Labels and annotations are
// exact key=value and must all match. Charts matches a resource's originating
// (sub)chart alias or its full chart-path.
type ResourceMatcher struct {
	Names       []string          `json:"names,omitempty"`
	Namespaces  []string          `json:"namespaces,omitempty"`
	Groups      []string          `json:"groups,omitempty"`
	Versions    []string          `json:"versions,omitempty"`
	Kinds       []string          `json:"kinds,omitempty"`
	Charts      []string          `json:"charts,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

func (s *ResourceMatcher) Match(resMeta *ResourceMeta) bool {
	return matchStrings(s.Kinds, resMeta.GroupVersionKind.Kind, true) &&
		matchStrings(s.Names, resMeta.Name, false) &&
		matchStrings(s.Namespaces, resMeta.Namespace, false) &&
		matchStrings(s.Groups, resMeta.GroupVersionKind.Group, true) &&
		matchStrings(s.Versions, resMeta.GroupVersionKind.Version, true) &&
		matchCharts(s.Charts, resMeta.FilePath) &&
		matchKeyValues(s.Labels, resMeta.Labels) &&
		matchKeyValues(s.Annotations, resMeta.Annotations)
}

// Validate compiles every /regex/ value and returns the first error, so callers
// that need fail-closed behavior can reject a bad matcher before matching.
func (s *ResourceMatcher) Validate() error {
	for _, group := range [][]string{s.Names, s.Namespaces, s.Groups, s.Versions, s.Kinds, s.Charts} {
		for _, value := range group {
			if isRegexPattern(value) {
				if _, err := compileMatchString(value); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func matchCharts(patterns []string, filePath string) bool {
	if len(patterns) == 0 {
		return true
	}

	charts := chartSegments(filePath)
	if len(charts) == 0 {
		return false
	}

	for _, pattern := range patterns {
		for _, chart := range charts {
			if matchString(pattern, chart, false) {
				return true
			}
		}
	}

	return false
}

func matchStrings(patterns []string, value string, caseInsensitive bool) bool {
	if len(patterns) == 0 {
		return true
	}

	for _, pattern := range patterns {
		if matchString(pattern, value, caseInsensitive) {
			return true
		}
	}

	return false
}

func matchString(pattern, value string, caseInsensitive bool) bool {
	if isRegexPattern(pattern) {
		re, err := compileMatchString(pattern)
		if err != nil {
			return false
		}

		return re.MatchString(value)
	}

	if caseInsensitive {
		return strings.EqualFold(value, pattern)
	}

	return value == pattern
}

// chartSegments returns the chart match candidates for a rendered FilePath: for
// each "templates/" boundary, both the chart alias (the preceding segment) and
// the full chart-path. Standalone-CRD paths (no "templates/") fall back to the
// leading segment. Uses chart aliases, not upstream names.
func chartSegments(filePath string) []string {
	if filePath == "" {
		return nil
	}

	segments := strings.Split(filePath, "/")

	var charts []string
	for i, seg := range segments {
		if seg == "templates" && i > 0 {
			charts = append(charts, segments[i-1])
			charts = append(charts, strings.Join(segments[:i], "/"))
		}
	}

	if len(charts) == 0 {
		charts = append(charts, segments[0])
	}

	return charts
}

// compileMatchString compiles a /…/ regexp value (anchored), caching the result.
// The caller must ensure value is a regexp pattern (see isRegexPattern).
func compileMatchString(value string) (*regexp.Regexp, error) {
	if cached, ok := regexpCache.Load(value); ok {
		return cached.(*regexp.Regexp), nil
	}

	re, err := regexp.Compile(`\A(?:` + value[1:len(value)-1] + `)\z`)
	if err != nil {
		return nil, fmt.Errorf("compile regexp %q: %w", value, err)
	}

	regexpCache.Store(value, re)

	return re, nil
}

// isRegexPattern reports whether a matcher value uses the /…/ regexp convention.
func isRegexPattern(value string) bool {
	return len(value) >= 2 && strings.HasPrefix(value, "/") && strings.HasSuffix(value, "/")
}

func matchKeyValues(want, have map[string]string) bool {
	for k, v := range want {
		if got, ok := have[k]; !ok || got != v {
			return false
		}
	}

	return true
}
