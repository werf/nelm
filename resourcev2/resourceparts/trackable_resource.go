package resourceparts

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/werf/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

var annotationKeyPatternFailMode = regexp.MustCompile(`^werf.io/fail-mode$`)
var annotationKeyPatternFailuresAllowedPerReplica = regexp.MustCompile(`^werf.io/failures-allowed-per-replica$`)
var annotationKeyPatternIgnoreReadinessProbeFailsFor = regexp.MustCompile(`^werf.io/ignore-readiness-probe-fails-for-(?P<container>.+)$`)
var annotationKeyPatternLogRegex = regexp.MustCompile(`^werf.io/log-regex$`)
var annotationKeyPatternLogRegexFor = regexp.MustCompile(`^werf.io/log-regex-for-(?P<container>.+)$`)
var annotationKeyPatternNoActivityTimeout = regexp.MustCompile(`^werf.io/no-activity-timeout$`)
var annotationKeyPatternShowLogsOnlyForContainers = regexp.MustCompile(`^werf.io/show-logs-only-for-containers$`)
var annotationKeyPatternShowServiceMessages = regexp.MustCompile(`^werf.io/show-service-messages$`)
var annotationKeyPatternSkipLogs = regexp.MustCompile(`^werf.io/skip-logs$`)
var annotationKeyPatternSkipLogsForContainers = regexp.MustCompile(`^werf.io/skip-logs-for-containers$`)
var annotationKeyPatternTrackTerminationMode = regexp.MustCompile(`^werf.io/track-termination-mode$`)

func NewTrackableResource(opts NewTrackableResourceOptions) *TrackableResource {
	return &TrackableResource{
		unstructured: opts.Unstructured,
	}
}

type NewTrackableResourceOptions struct {
	Unstructured *unstructured.Unstructured
}

type TrackableResource struct {
	unstructured *unstructured.Unstructured
}

func (r *TrackableResource) Validate() error {
	if r.unstructured == nil {
		return nil
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternFailMode); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch value {
		case string(multitrack.IgnoreAndContinueDeployProcess):
		case string(multitrack.FailWholeDeployProcessImmediately):
		case string(multitrack.HopeUntilEndOfDeployProcess):
		default:
			return errors.NewValidationError("invalid unknown value %q for annotation %q", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternFailuresAllowedPerReplica); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if failures, err := strconv.Atoi(value); err != nil {
			return errors.NewValidationError("invalid value %q for annotation %q, expected integer value", value, key)
		} else if failures < 0 {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-negative integer value", value, key)
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternIgnoreReadinessProbeFailsFor); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(key)
			if keyMatches == nil {
				return errors.NewValidationError("invalid key for annotation %q", key)
			}

			containerSubexpIndex := annotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
			if containerSubexpIndex == -1 {
				return errors.NewValidationError("invalid regexp pattern %q for annotation %q", annotationKeyPatternIgnoreReadinessProbeFailsFor.String(), key)
			}

			if len(keyMatches) < containerSubexpIndex+1 {
				return errors.NewValidationError("can't parse container name for annotation %q", key)
			}

			// TODO(ilya-lesikov): validate container name

			if value == "" {
				return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty value", value, key)
			}

			duration, err := time.ParseDuration(value)
			if err != nil {
				return errors.NewValidationError("invalid value %q for annotation %q, expected valid duration", value, key)
			}

			if math.Signbit(duration.Seconds()) {
				return errors.NewValidationError("invalid value %q for annotation %q, expected positive duration value", value, key)
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternLogRegex); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		if _, err := regexp.Compile(value); err != nil {
			return errors.NewValidationError("invalid value %q for annotation %q, expected valid regexp", value, key)
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternLogRegexFor); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternLogRegexFor.FindStringSubmatch(key)
			if keyMatches == nil {
				return errors.NewValidationError("invalid key for annotation %q", key)
			}

			containerSubexpIndex := annotationKeyPatternLogRegexFor.SubexpIndex("container")
			if containerSubexpIndex == -1 {
				return errors.NewValidationError("invalid regexp pattern %q for annotation %q", annotationKeyPatternLogRegexFor.String(), key)
			}

			if len(keyMatches) < containerSubexpIndex+1 {
				return errors.NewValidationError("can't parse container name for annotation %q", key)
			}

			// TODO(ilya-lesikov): validate container name

			if value == "" {
				return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty value", value, key)
			}

			if _, err := regexp.Compile(value); err != nil {
				return errors.NewValidationError("invalid value %q for annotation %q, expected valid regular expression", value, key)
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternNoActivityTimeout); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty duration value", value, key)
		}

		duration, err := time.ParseDuration(value)
		if err != nil {
			return errors.NewValidationError("invalid value %q for annotation %q, expected valid duration", value, key)
		}

		if duration.Seconds() < 0 {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-negative duration value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternShowLogsOnlyForContainers); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		if strings.Contains(value, ",") {
			for _, container := range strings.Split(value, ",") {
				container = strings.TrimSpace(container)
				if container == "" {

					return errors.NewValidationError("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
				}

				// TODO(ilya-lesikov): should be valid container name
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternShowServiceMessages); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return errors.NewValidationError("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternSkipLogs); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return errors.NewValidationError("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternSkipLogsForContainers); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		if strings.Contains(value, ",") {
			for _, container := range strings.Split(value, ",") {
				container = strings.TrimSpace(container)
				if container == "" {
					return errors.NewValidationError("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
				}

				// TODO(ilya-lesikov): validate container name
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternTrackTerminationMode); found {
		if value == "" {
			return errors.NewValidationError("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch value {
		case string(multitrack.WaitUntilResourceReady):
		case string(multitrack.NonBlocking):
		default:
			return errors.NewValidationError("invalid unknown value %q for annotation %q", value, key)
		}
	}

	return nil
}

func (r *TrackableResource) FailMode() multitrack.FailMode {
	if r.unstructured == nil {
		return multitrack.FailWholeDeployProcessImmediately
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternFailMode)
	if !found {
		return multitrack.FailWholeDeployProcessImmediately
	}

	return multitrack.FailMode(value)
}

func (r *TrackableResource) FailuresAllowed() int {
	if r.unstructured == nil {
		return 1
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternFailuresAllowedPerReplica)
	var failuresAllowed int
	if found {
		failuresAllowed, _ = strconv.Atoi(value)
	} else {
		failuresAllowed = 1
	}

	replicas, replicasFound, _ := unstructured.NestedInt64(r.unstructured.UnstructuredContent(), "spec", "replicas")

	if replicasFound {
		return int(replicas) * failuresAllowed
	} else {
		return failuresAllowed
	}
}

func (r *TrackableResource) IgnoreReadinessProbeFailsForContainers() (durationByContainer map[string]time.Duration) {
	if r.unstructured == nil {
		return nil
	}

	annotations, found := FindAnnotationsOrLabelsByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternIgnoreReadinessProbeFailsFor)
	if !found {
		return nil
	}

	for key, value := range annotations {
		keyMatches := annotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(key)
		containerSubexpIndex := annotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
		container := keyMatches[containerSubexpIndex]
		duration, _ := time.ParseDuration(value)

		durationByContainer[container] = duration
	}

	return durationByContainer
}

func (r *TrackableResource) LogRegex() *regexp.Regexp {
	if r.unstructured == nil {
		return nil
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternLogRegex)
	if !found {
		return nil
	}

	return regexp.MustCompile(value)
}

func (r *TrackableResource) LogRegexesForContainers() (regexByContainer map[string]*regexp.Regexp) {
	if r.unstructured == nil {
		return nil
	}

	annotations, found := FindAnnotationsOrLabelsByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternLogRegexFor)
	if !found {
		return nil
	}

	for key, value := range annotations {
		keyMatches := annotationKeyPatternLogRegexFor.FindStringSubmatch(key)
		containerSubexpIndex := annotationKeyPatternLogRegexFor.SubexpIndex("container")
		container := keyMatches[containerSubexpIndex]
		regexByContainer[container] = regexp.MustCompile(value)
	}

	return regexByContainer
}

func (r *TrackableResource) NoActivityTimeout() *time.Duration {
	if r.unstructured == nil {
		return nil
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternNoActivityTimeout)
	if !found {
		return nil
	}

	timeout, _ := time.ParseDuration(value)

	return &timeout
}

func (r *TrackableResource) ShowLogsOnlyForContainers() []string {
	if r.unstructured == nil {
		return nil
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternShowLogsOnlyForContainers)
	if !found {
		return nil
	}

	var containers []string
	for _, container := range strings.Split(value, ",") {
		containers = append(containers, strings.TrimSpace(container))
	}

	return containers
}

func (r *TrackableResource) ShowServiceMessages() bool {
	if r.unstructured == nil {
		return false
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternShowServiceMessages)
	if !found {
		return false
	}

	showServiceMessages, _ := strconv.ParseBool(value)

	return showServiceMessages
}

func (r *TrackableResource) SkipLogs() bool {
	if r.unstructured == nil {
		return false
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternSkipLogs)
	if !found {
		return false
	}

	skipLogs, _ := strconv.ParseBool(value)

	return skipLogs
}

func (r *TrackableResource) SkipLogsForContainers() []string {
	if r.unstructured == nil {
		return nil
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternSkipLogsForContainers)
	if !found {
		return nil
	}

	var containers []string
	for _, container := range strings.Split(value, ",") {
		containers = append(containers, strings.TrimSpace(container))
	}

	return containers
}

func (r *TrackableResource) TrackTerminationMode() multitrack.TrackTerminationMode {
	if r.unstructured == nil {
		return multitrack.WaitUntilResourceReady
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(r.unstructured.GetAnnotations(), annotationKeyPatternTrackTerminationMode)
	if !found {
		return multitrack.WaitUntilResourceReady
	}

	return multitrack.TrackTerminationMode(value)
}
