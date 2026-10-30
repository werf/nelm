package common

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKeyPatternsMatchOnlyTheirKey(t *testing.T) {
	tests := []struct {
		human   string
		pattern *regexp.Regexp
	}{
		{LabelKeyHumanManagedBy, LabelKeyPatternManagedBy},
		{AnnotationKeyHumanReleaseName, AnnotationKeyPatternReleaseName},
		{AnnotationKeyHumanReleaseNamespace, AnnotationKeyPatternReleaseNamespace},
		{AnnotationKeyHumanHook, AnnotationKeyPatternHook},
		{AnnotationKeyHumanResourcePolicy, AnnotationKeyPatternResourcePolicy},
		{AnnotationKeyHumanWerfResourcePolicy, AnnotationKeyPatternWerfResourcePolicy},
		{AnnotationKeyHumanDeletePolicy, AnnotationKeyPatternDeletePolicy},
		{AnnotationKeyHumanHookDeletePolicy, AnnotationKeyPatternHookDeletePolicy},
		{AnnotationKeyHumanReplicasOnCreation, AnnotationKeyPatternReplicasOnCreation},
		{AnnotationKeyHumanFailMode, AnnotationKeyPatternFailMode},
		{AnnotationKeyHumanFailuresAllowedPerReplica, AnnotationKeyPatternFailuresAllowedPerReplica},
		{AnnotationKeyHumanIgnoreReadinessProbeFailsFor, AnnotationKeyPatternIgnoreReadinessProbeFailsFor},
		{AnnotationKeyHumanLogRegex, AnnotationKeyPatternLogRegex},
		{AnnotationKeyHumanLogRegexSkip, AnnotationKeyPatternLogRegexSkip},
		{AnnotationKeyHumanLogRegexFor, AnnotationKeyPatternLogRegexFor},
		{AnnotationKeyHumanSkipLogRegexFor, AnnotationKeyPatternSkipLogRegexFor},
		{AnnotationKeyHumanNoActivityTimeout, AnnotationKeyPatternNoActivityTimeout},
		{AnnotationKeyHumanShowLogsOnlyForContainers, AnnotationKeyPatternShowLogsOnlyForContainers},
		{AnnotationKeyHumanShowServiceMessages, AnnotationKeyPatternShowServiceMessages},
		{AnnotationKeyHumanShowLogsOnlyForNumberOfReplicas, AnnotationKeyPatternShowLogsOnlyForNumberOfReplicas},
		{AnnotationKeyHumanSkipLogs, AnnotationKeyPatternSkipLogs},
		{AnnotationKeyHumanSkipLogsForContainers, AnnotationKeyPatternSkipLogsForContainers},
		{AnnotationKeyHumanTrackTerminationMode, AnnotationKeyPatternTrackTerminationMode},
		{AnnotationKeyHumanWeight, AnnotationKeyPatternWeight},
		{AnnotationKeyHumanHookWeight, AnnotationKeyPatternHookWeight},
		{AnnotationKeyHumanDeployDependency, AnnotationKeyPatternDeployDependency},
		{AnnotationKeyHumanDeleteDependency, AnnotationKeyPatternDeleteDependency},
		{AnnotationKeyHumanSensitive, AnnotationKeyPatternSensitive},
		{AnnotationKeyHumanSensitivePaths, AnnotationKeyPatternSensitivePaths},
		{AnnotationKeyHumanDeployOn, AnnotationKeyPatternDeployOn},
		{AnnotationKeyHumanOwnership, AnnotationKeyPatternOwnership},
		{AnnotationKeyHumanDeletePropagation, AnnotationKeyPatternDeletePropagation},
		{AnnotationKeyHumanTriggersRelease, AnnotationKeyPatternTriggersRelease},
	}

	placeholders := strings.NewReplacer("<container>", "app", "<name>", "db")

	for _, tt := range tests {
		t.Run(tt.human, func(t *testing.T) {
			key := placeholders.Replace(tt.human)
			assert.True(t, tt.pattern.MatchString(key), "pattern %q must match %q", tt.pattern, key)

			for i, char := range key {
				if char != '.' {
					continue
				}

				lookalikeKey := key[:i] + "-" + key[i+1:]
				assert.False(t, tt.pattern.MatchString(lookalikeKey), "pattern %q must not match %q", tt.pattern, lookalikeKey)
			}
		})
	}
}
