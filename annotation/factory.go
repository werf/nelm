package annotation

func AnnotationFactory(key, value string) Annotationer {
	if AnnotationKeyPatternExternalDependencyNamespace.MatchString(key) {
		return NewAnnotationExternalDependencyNamespace(key, value)
	} else if AnnotationKeyPatternExternalDependencyResource.MatchString(key) {
		return NewAnnotationExternalDependencyResource(key, value)
	} else if AnnotationKeyPatternFailMode.MatchString(key) {
		return NewAnnotationFailMode(key, value)
	} else if AnnotationKeyPatternFailuresAllowedPerReplica.MatchString(key) {
		return NewAnnotationFailuresAllowedPerReplica(key, value)
	} else if AnnotationKeyPatternHook.MatchString(key) {
		return NewAnnotationHook(key, value)
	} else if AnnotationKeyPatternHookDeletePolicy.MatchString(key) {
		return NewAnnotationHookDeletePolicy(key, value)
	} else if AnnotationKeyPatternHookWeight.MatchString(key) {
		return NewAnnotationHookWeight(key, value)
	} else if AnnotationKeyPatternIgnoreReadinessProbeFailsFor.MatchString(key) {
		return NewAnnotationIgnoreReadinessProbeFailsFor(key, value)
	} else if AnnotationKeyPatternLogRegex.MatchString(key) {
		return NewAnnotationLogRegex(key, value)
	} else if AnnotationKeyPatternLogRegexFor.MatchString(key) {
		return NewAnnotationLogRegexFor(key, value)
	} else if AnnotationKeyPatternNoActivityTimeout.MatchString(key) {
		return NewAnnotationNoActivityTimeout(key, value)
	} else if AnnotationKeyPatternReleaseName.MatchString(key) {
		return NewAnnotationReleaseName(key, value)
	} else if AnnotationKeyPatternReleaseNamespace.MatchString(key) {
		return NewAnnotationReleaseNamespace(key, value)
	} else if AnnotationKeyPatternReplicasOnCreation.MatchString(key) {
		return NewAnnotationReplicasOnCreation(key, value)
	} else if AnnotationKeyPatternResourcePolicy.MatchString(key) {
		return NewAnnotationResourcePolicy(key, value)
	} else if AnnotationKeyPatternShowLogsOnlyForContainers.MatchString(key) {
		return NewAnnotationShowLogsOnlyForContainers(key, value)
	} else if AnnotationKeyPatternShowServiceMessages.MatchString(key) {
		return NewAnnotationShowServiceMessages(key, value)
	} else if AnnotationKeyPatternSkipLogs.MatchString(key) {
		return NewAnnotationSkipLogs(key, value)
	} else if AnnotationKeyPatternSkipLogsForContainers.MatchString(key) {
		return NewAnnotationSkipLogsForContainers(key, value)
	} else if AnnotationKeyPatternTrackTerminationMode.MatchString(key) {
		return NewAnnotationTrackTerminationMode(key, value)
	} else if AnnotationKeyPatternWeight.MatchString(key) {
		return NewAnnotationWeight(key, value)
	}

	return NewAnnotationUnknown(key, value)
}
