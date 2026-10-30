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
		{human: LabelKeyHumanManagedBy, pattern: LabelKeyPatternManagedBy},
		{human: AnnotationKeyHumanReleaseName, pattern: AnnotationKeyPatternReleaseName},
		{human: AnnotationKeyHumanReleaseNamespace, pattern: AnnotationKeyPatternReleaseNamespace},
		{human: AnnotationKeyHumanHook, pattern: AnnotationKeyPatternHook},
		{human: AnnotationKeyHumanResourcePolicy, pattern: AnnotationKeyPatternResourcePolicy},
		{human: AnnotationKeyHumanWerfResourcePolicy, pattern: AnnotationKeyPatternWerfResourcePolicy},
		{human: AnnotationKeyHumanDeletePolicy, pattern: AnnotationKeyPatternDeletePolicy},
		{human: AnnotationKeyHumanHookDeletePolicy, pattern: AnnotationKeyPatternHookDeletePolicy},
		{human: AnnotationKeyHumanReplicasOnCreation, pattern: AnnotationKeyPatternReplicasOnCreation},
		{human: AnnotationKeyHumanFailMode, pattern: AnnotationKeyPatternFailMode},
		{human: AnnotationKeyHumanFailuresAllowedPerReplica, pattern: AnnotationKeyPatternFailuresAllowedPerReplica},
		{human: AnnotationKeyHumanIgnoreReadinessProbeFailsFor, pattern: AnnotationKeyPatternIgnoreReadinessProbeFailsFor},
		{human: AnnotationKeyHumanLogRegex, pattern: AnnotationKeyPatternLogRegex},
		{human: AnnotationKeyHumanLogRegexSkip, pattern: AnnotationKeyPatternLogRegexSkip},
		{human: AnnotationKeyHumanLogRegexFor, pattern: AnnotationKeyPatternLogRegexFor},
		{human: AnnotationKeyHumanSkipLogRegexFor, pattern: AnnotationKeyPatternSkipLogRegexFor},
		{human: AnnotationKeyHumanNoActivityTimeout, pattern: AnnotationKeyPatternNoActivityTimeout},
		{human: AnnotationKeyHumanShowLogsOnlyForContainers, pattern: AnnotationKeyPatternShowLogsOnlyForContainers},
		{human: AnnotationKeyHumanShowServiceMessages, pattern: AnnotationKeyPatternShowServiceMessages},
		{human: AnnotationKeyHumanShowLogsOnlyForNumberOfReplicas, pattern: AnnotationKeyPatternShowLogsOnlyForNumberOfReplicas},
		{human: AnnotationKeyHumanSkipLogs, pattern: AnnotationKeyPatternSkipLogs},
		{human: AnnotationKeyHumanSkipLogsForContainers, pattern: AnnotationKeyPatternSkipLogsForContainers},
		{human: AnnotationKeyHumanTrackTerminationMode, pattern: AnnotationKeyPatternTrackTerminationMode},
		{human: AnnotationKeyHumanWeight, pattern: AnnotationKeyPatternWeight},
		{human: AnnotationKeyHumanHookWeight, pattern: AnnotationKeyPatternHookWeight},
		{human: AnnotationKeyHumanDeployDependency, pattern: AnnotationKeyPatternDeployDependency},
		{human: AnnotationKeyHumanDeleteDependency, pattern: AnnotationKeyPatternDeleteDependency},
		{human: AnnotationKeyHumanSensitive, pattern: AnnotationKeyPatternSensitive},
		{human: AnnotationKeyHumanSensitivePaths, pattern: AnnotationKeyPatternSensitivePaths},
		{human: AnnotationKeyHumanDeployOn, pattern: AnnotationKeyPatternDeployOn},
		{human: AnnotationKeyHumanOwnership, pattern: AnnotationKeyPatternOwnership},
		{human: AnnotationKeyHumanDeletePropagation, pattern: AnnotationKeyPatternDeletePropagation},
		{human: AnnotationKeyHumanTriggersRelease, pattern: AnnotationKeyPatternTriggersRelease},
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
