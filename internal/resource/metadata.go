package resource

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ohler55/ojg/jp"
	"github.com/samber/lo"
	v1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource/meta"
	"github.com/werf/nelm/internal/util"
)

var (
	LabelKeyHumanManagedBy   = "app.kubernetes.io/managed-by"
	LabelKeyPatternManagedBy = regexp.MustCompile(`^app.kubernetes.io/managed-by$`)

	AnnotationKeyHumanReleaseName   = "meta.helm.sh/release-name"
	AnnotationKeyPatternReleaseName = regexp.MustCompile(`^meta.helm.sh/release-name$`)

	AnnotationKeyHumanReleaseNamespace   = "meta.helm.sh/release-namespace"
	AnnotationKeyPatternReleaseNamespace = regexp.MustCompile(`^meta.helm.sh/release-namespace$`)

	AnnotationKeyHumanHook   = "helm.sh/hook"
	AnnotationKeyPatternHook = regexp.MustCompile(`^helm.sh/hook$`)

	AnnotationKeyHumanResourcePolicy   = "helm.sh/resource-policy"
	AnnotationKeyPatternResourcePolicy = regexp.MustCompile(`^helm.sh/resource-policy$`)

	AnnotationKeyHumanDeletePolicy   = "werf.io/delete-policy"
	AnnotationKeyPatternDeletePolicy = regexp.MustCompile(`^werf.io/delete-policy$`)

	AnnotationKeyHumanHookDeletePolicy   = "helm.sh/hook-delete-policy"
	AnnotationKeyPatternHookDeletePolicy = regexp.MustCompile(`^helm.sh/hook-delete-policy$`)

	AnnotationKeyHumanReplicasOnCreation   = "werf.io/replicas-on-creation"
	AnnotationKeyPatternReplicasOnCreation = regexp.MustCompile(`^werf.io/replicas-on-creation$`)

	AnnotationKeyHumanFailMode   = "werf.io/fail-mode"
	AnnotationKeyPatternFailMode = regexp.MustCompile(`^werf.io/fail-mode$`)

	AnnotationKeyHumanFailuresAllowedPerReplica   = "werf.io/failures-allowed-per-replica"
	AnnotationKeyPatternFailuresAllowedPerReplica = regexp.MustCompile(`^werf.io/failures-allowed-per-replica$`)

	AnnotationKeyHumanIgnoreReadinessProbeFailsFor   = "werf.io/ignore-readiness-probe-fails-for-<container>"
	AnnotationKeyPatternIgnoreReadinessProbeFailsFor = regexp.MustCompile(`^werf.io/ignore-readiness-probe-fails-for-(?P<container>.+)$`)

	AnnotationKeyHumanLogRegex   = "werf.io/log-regex"
	AnnotationKeyPatternLogRegex = regexp.MustCompile(`^werf.io/log-regex$`)

	AnnotationKeyHumanLogRegexFor   = "werf.io/log-regex-for-<container>"
	AnnotationKeyPatternLogRegexFor = regexp.MustCompile(`^werf.io/log-regex-for-(?P<container>.+)$`)

	AnnotationKeyHumanNoActivityTimeout   = "werf.io/no-activity-timeout"
	AnnotationKeyPatternNoActivityTimeout = regexp.MustCompile(`^werf.io/no-activity-timeout$`)

	AnnotationKeyHumanShowLogsOnlyForContainers   = "werf.io/show-logs-only-for-containers"
	AnnotationKeyPatternShowLogsOnlyForContainers = regexp.MustCompile(`^werf.io/show-logs-only-for-containers$`)

	AnnotationKeyHumanShowServiceMessages   = "werf.io/show-service-messages"
	AnnotationKeyPatternShowServiceMessages = regexp.MustCompile(`^werf.io/show-service-messages$`)

	AnnotationKeyHumanShowLogsOnlyForNumberOfReplicas   = "werf.io/show-logs-only-for-number-of-replicas"
	AnnotationKeyPatternShowLogsOnlyForNumberOfReplicas = regexp.MustCompile(`^werf.io/show-logs-only-for-number-of-replicas$`)

	AnnotationKeyHumanSkipLogs   = "werf.io/skip-logs"
	AnnotationKeyPatternSkipLogs = regexp.MustCompile(`^werf.io/skip-logs$`)

	AnnotationKeyHumanSkipLogsForContainers   = "werf.io/skip-logs-for-containers"
	AnnotationKeyPatternSkipLogsForContainers = regexp.MustCompile(`^werf.io/skip-logs-for-containers$`)

	AnnotationKeyHumanTrackTerminationMode   = "werf.io/track-termination-mode"
	AnnotationKeyPatternTrackTerminationMode = regexp.MustCompile(`^werf.io/track-termination-mode$`)

	AnnotationKeyHumanWeight   = "werf.io/weight"
	AnnotationKeyPatternWeight = regexp.MustCompile(`^werf.io/weight$`)

	AnnotationKeyHumanHookWeight   = "helm.sh/hook-weight"
	AnnotationKeyPatternHookWeight = regexp.MustCompile(`^helm.sh/hook-weight$`)

	AnnotationKeyHumanDeployDependency   = "werf.io/deploy-dependency-<name>"
	AnnotationKeyPatternDeployDependency = regexp.MustCompile(`^werf.io/deploy-dependency-(?P<id>.+)$`)

	// TODO(v2): get rid
	AnnotationKeyHumanDependency   = "<name>.dependency.werf.io"
	AnnotationKeyPatternDependency = regexp.MustCompile(`^(?P<id>.+).dependency.werf.io$`)

	AnnotationKeyHumanExternalDependency   = "<name>.external-dependency.werf.io"
	AnnotationKeyPatternExternalDependency = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io$`)

	AnnotationKeyHumanLegacyExternalDependencyResource   = "<name>.external-dependency.werf.io/resource"
	AnnotationKeyPatternLegacyExternalDependencyResource = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/resource$`)

	AnnotationKeyHumanLegacyExternalDependencyNamespace   = "<name>.external-dependency.werf.io/namespace"
	AnnotationKeyPatternLegacyExternalDependencyNamespace = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/namespace$`)

	AnnotationKeyHumanSensitive   = "werf.io/sensitive"
	AnnotationKeyPatternSensitive = regexp.MustCompile(`^werf.io/sensitive$`)

	AnnotationKeyHumanSensitivePaths   = "werf.io/sensitive-paths"
	AnnotationKeyPatternSensitivePaths = regexp.MustCompile(`^werf.io/sensitive-paths$`)

	AnnotationKeyHumanDeployOn   = "werf.io/deploy-on"
	AnnotationKeyPatternDeployOn = regexp.MustCompile(`^werf.io/deploy-on$`)

	AnnotationKeyHumanOwnership   = "werf.io/ownership"
	AnnotationKeyPatternOwnership = regexp.MustCompile(`^werf.io/ownership$`)
)

func ValidateResourcePolicy(meta *meta.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternResourcePolicy); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch value {
		case "keep":
		default:
			return fmt.Errorf("invalid unknown value %q for annotation %q", value, key)
		}
	}

	return nil
}

func KeepOnDelete(meta *meta.ResourceMeta, releaseNamespace string) bool {
	if IsReleaseNamespace(meta.Name, meta.GroupVersionKind, releaseNamespace) {
		return true
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternResourcePolicy)
	if !found {
		return false
	}

	return value == "keep"
}

func Orphaned(meta *meta.ResourceMeta, releaseName, releaseNamespace string) bool {
	if _, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternReleaseName); !found || value != releaseName {
		return true
	}

	if _, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternReleaseNamespace); !found || value != releaseNamespace {
		return true
	}

	if _, value, found := FindAnnotationOrLabelByKeyPattern(meta.Labels, LabelKeyPatternManagedBy); !found || value != "Helm" {
		return true
	}

	return false
}

func AdoptableBy(meta *meta.ResourceMeta, releaseName, releaseNamespace string) (adoptable bool, nonAdoptableReason string) {
	nonAdoptableReasons := []string{}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternReleaseName); found {
		if value != releaseName {
			nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseName))
		}
	} else {
		nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, AnnotationKeyHumanReleaseName, releaseName))
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternReleaseNamespace); found {
		if value != releaseNamespace {
			nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseNamespace))
		}
	} else {
		nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, AnnotationKeyHumanReleaseNamespace, releaseNamespace))
	}

	nonAdoptableReason = strings.Join(nonAdoptableReasons, ", ")

	return len(nonAdoptableReasons) == 0, nonAdoptableReason
}

func validateHook(meta *meta.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternHook); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		for _, hookType := range strings.Split(value, ",") {
			hookType = strings.TrimSpace(hookType)
			if hookType == "" {
				return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
			}

			switch hookType {
			case string(helmrelease.HookPreInstall),
				string(helmrelease.HookPostInstall),
				string(helmrelease.HookPreUpgrade),
				string(helmrelease.HookPostUpgrade),
				string(helmrelease.HookPreRollback),
				string(helmrelease.HookPostRollback),
				string(helmrelease.HookPreDelete),
				string(helmrelease.HookPostDelete),
				string(helmrelease.HookTest),
				"test-success":
			default:
				return fmt.Errorf("value %q for annotation %q is not supported", value, key)
			}
		}
	}

	return nil
}

func validateWeight(meta *meta.ResourceMeta) error {
	if IsHook(meta.Annotations) {
		if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternHookWeight); found {
			if value == "" {
				return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
			}

			if _, err := strconv.Atoi(value); err != nil {
				return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternWeight); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		}
	}

	return nil
}

func validateDeletePolicy(meta *meta.ResourceMeta) error {
	annotations := meta.Annotations

	if IsHook(annotations) {
		if key, value, found := FindAnnotationOrLabelByKeyPattern(annotations, AnnotationKeyPatternHookDeletePolicy); found && value != "" {
			for _, hookDeletePolicy := range strings.Split(value, ",") {
				hookDeletePolicy = strings.TrimSpace(hookDeletePolicy)
				if hookDeletePolicy == "" {
					return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
				}

				switch hookDeletePolicy {
				case string(helmrelease.HookSucceeded),
					string(helmrelease.HookFailed),
					string(helmrelease.HookBeforeHookCreation):
				default:
					return fmt.Errorf("value %q for annotation %q is not supported", value, key)
				}
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(annotations, AnnotationKeyPatternDeletePolicy); found && value != "" {
		for _, deletePolicy := range strings.Split(value, ",") {
			deletePolicy = strings.TrimSpace(deletePolicy)
			if deletePolicy == "" {
				return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
			}

			switch deletePolicy {
			case string(common.DeletePolicySucceeded),
				string(common.DeletePolicyFailed),
				string(common.DeletePolicyBeforeCreation):
			default:
				return fmt.Errorf("value %q for annotation %q is not supported", value, key)
			}
		}
	}

	return nil
}

func validateReplicasOnCreation(meta *meta.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternReplicasOnCreation); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty numeric value", value, key)
		}

		replicas, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, value must be a number", value, key)
		}

		if replicas < 0 {
			return fmt.Errorf("invalid value %q for annotation %q, value must be a positive number or zero", value, key)
		}
	}

	return nil
}

func validateTrack(meta *meta.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternFailMode); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch value {
		case string(multitrack.IgnoreAndContinueDeployProcess):
		case string(multitrack.FailWholeDeployProcessImmediately):
		case string(multitrack.HopeUntilEndOfDeployProcess):
		default:
			return fmt.Errorf("invalid unknown value %q for annotation %q", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternFailuresAllowedPerReplica); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if failures, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		} else if failures < 0 {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-negative integer value", value, key)
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternIgnoreReadinessProbeFailsFor); found {
		for key, value := range annotations {
			keyMatches := AnnotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			containerSubexpIndex := AnnotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
			if containerSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternIgnoreReadinessProbeFailsFor.String(), key)
			}

			if len(keyMatches) < containerSubexpIndex+1 {
				return fmt.Errorf("can't parse container name for annotation %q", key)
			}

			if value == "" {
				return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", value, key)
			}

			duration, err := time.ParseDuration(value)
			if err != nil {
				return fmt.Errorf("invalid value %q for annotation %q, expected valid duration", value, key)
			}

			if math.Signbit(duration.Seconds()) {
				return fmt.Errorf("invalid value %q for annotation %q, expected positive duration value", value, key)
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternLogRegex); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		if _, err := regexp.Compile(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected valid regexp", value, key)
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternLogRegexFor); found {
		for key, value := range annotations {
			keyMatches := AnnotationKeyPatternLogRegexFor.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			containerSubexpIndex := AnnotationKeyPatternLogRegexFor.SubexpIndex("container")
			if containerSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternLogRegexFor.String(), key)
			}

			if len(keyMatches) < containerSubexpIndex+1 {
				return fmt.Errorf("can't parse container name for annotation %q", key)
			}

			if value == "" {
				return fmt.Errorf("invalid value %q for annotation %q, expected non-empty value", value, key)
			}

			if _, err := regexp.Compile(value); err != nil {
				return fmt.Errorf("invalid value %q for annotation %q, expected valid regular expression", value, key)
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternNoActivityTimeout); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty duration value", value, key)
		}

		duration, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected valid duration", value, key)
		}

		if duration.Seconds() < 0 {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-negative duration value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternShowLogsOnlyForContainers); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		if strings.Contains(value, ",") {
			for _, container := range strings.Split(value, ",") {
				container = strings.TrimSpace(container)
				if container == "" {
					return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
				}
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternShowServiceMessages); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternShowLogsOnlyForNumberOfReplicas); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if replicas, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		} else if replicas < 0 {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-negative integer value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternSkipLogs); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternSkipLogsForContainers); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		if strings.Contains(value, ",") {
			for _, container := range strings.Split(value, ",") {
				container = strings.TrimSpace(container)
				if container == "" {
					return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
				}
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternTrackTerminationMode); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch value {
		case string(multitrack.WaitUntilResourceReady):
		case string(multitrack.NonBlocking):
		default:
			return fmt.Errorf("invalid unknown value %q for annotation %q", value, key)
		}
	}

	return nil
}

func validateDeployDependencies(meta *meta.ResourceMeta) error {
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternDeployDependency); found {
		for key, value := range annotations {
			keyMatches := AnnotationKeyPatternDeployDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := AnnotationKeyPatternDeployDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternDeployDependency.String(), key)
			}

			if len(keyMatches) < idSubexpIndex+1 {
				return fmt.Errorf("can't parse deploy dependency id from annotation key %q", key)
			}

			if value == "" {
				return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
			}

			properties, err := util.ParseProperties(context.TODO(), value)
			if err != nil {
				return fmt.Errorf("invalid value %q for annotation %q: %w", err)
			}

			if !lo.Some(lo.Keys(properties), []string{"group", "version", "kind", "name", "namespace"}) {
				return fmt.Errorf("invalid value %q for annotation %q, target not specified", value, key)
			}

			if _, found := properties["state"]; !found {
				return fmt.Errorf(`invalid value %q for annotation %q, "state" property must be set`, value, key)
			}

			for propKey, propVal := range properties {
				switch propKey {
				case "group", "version", "kind", "name", "namespace":
					switch pv := propVal.(type) {
					case string:
						if pv == "" {
							return fmt.Errorf("invalid value %q for property %q, expected non-empty string value", pv, propKey)
						}
					case bool:
						return fmt.Errorf("invalid boolean value %q for property %q, expected string value", pv, propKey)
					default:
						panic(fmt.Sprintf("unexpected type %T for property %q", pv, propKey))
					}
				case "state":
					switch pv := propVal.(type) {
					case string:
						switch pv {
						case "present", "ready":
						case "":
							return fmt.Errorf("invalid value %q for property %q, expected non-empty string value", pv, propKey)
						default:
							return fmt.Errorf("unknown value %q for property %q", pv, propKey)
						}
					case bool:
						return fmt.Errorf("invalid boolean value %q for property %q, expected string value", pv, propKey)
					default:
						panic(fmt.Sprintf("unexpected type %T for property %q", pv, propKey))
					}
				default:
					return fmt.Errorf("unknown property %q in value of annotation %q", propKey, key)
				}
			}
		}
	}

	return nil
}

func validateInternalDependencies(meta *meta.ResourceMeta) error {
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternDependency); found {
		for key, value := range annotations {
			keyMatches := AnnotationKeyPatternDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := AnnotationKeyPatternDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternDependency.String(), key)
			}

			if len(keyMatches) < idSubexpIndex+1 {
				return fmt.Errorf("can't parse dependency id from annotation key %q", key)
			}

			if value != "" {
				valueElems := strings.Split(value, ":")

				if len(valueElems) != 3 && len(valueElems) != 4 {
					return fmt.Errorf(`invalid format of value %q for annotation %q, should be: apiVersion:kind[:namespace]:name or empty`, value, key)
				}
			}
		}
	}

	return nil
}

func validateExternalDependencies(meta *meta.ResourceMeta) error {
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternExternalDependency); found {
		for key, value := range annotations {
			keyMatches := AnnotationKeyPatternExternalDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := AnnotationKeyPatternExternalDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternExternalDependency.String(), key)
			}

			if len(keyMatches) < idSubexpIndex+1 {
				return fmt.Errorf("can't parse external dependency id from annotation key %q", key)
			}

			valueElems := strings.Split(value, ":")

			if len(valueElems) != 3 && len(valueElems) != 4 {
				return fmt.Errorf(`invalid format of value %q for annotation %q, should be: apiVersion:kind[:namespace]:name`, value, key)
			}
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternLegacyExternalDependencyResource); found {
		for key, value := range annotations {
			keyMatches := AnnotationKeyPatternLegacyExternalDependencyResource.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := AnnotationKeyPatternLegacyExternalDependencyResource.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternLegacyExternalDependencyResource.String(), key)
			}

			if len(keyMatches) < idSubexpIndex+1 {
				return fmt.Errorf("can't parse external dependency id from annotation key %q", key)
			}

			valueElems := strings.Split(value, "/")

			if len(valueElems) != 2 {
				return fmt.Errorf(`invalid format of value %q for annotation %q, should be: type/name`, value, key)
			}

			switch valueElems[0] {
			case "":
				return fmt.Errorf("value %q of annotation %q can't have empty resource type", value, key)
			case "all":
				return fmt.Errorf(`"all" resource type in value %q of annotation %q is not allowed`, value, key)
			}

			resourceTypeParts := strings.Split(valueElems[0], ".")
			for _, part := range resourceTypeParts {
				if part == "" {
					return fmt.Errorf("resource type in value %q of annotation %q should have dots (.) delimiting only non-empty resource.version.group", value, key)
				}
			}

			if valueElems[1] == "" {
				return fmt.Errorf("in value %q of annotation %q resource name can't be empty", value, key)
			}
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternLegacyExternalDependencyNamespace); found {
		for key, value := range annotations {
			keyMatches := AnnotationKeyPatternLegacyExternalDependencyNamespace.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := AnnotationKeyPatternLegacyExternalDependencyNamespace.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", AnnotationKeyPatternLegacyExternalDependencyNamespace.String(), key)
			}

			if len(keyMatches) < idSubexpIndex+1 {
				return fmt.Errorf("can't parse external dependency id from annotation key %q", key)
			}

			if value == "" {
				return fmt.Errorf("invalid value %q for annotation %q, value must not be empty", value, key)
			}
		}
	}

	return nil
}

func validateSensitive(meta *meta.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternSensitive); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternSensitivePaths); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty comma-separated list of JSONPath strings", value, key)
		}

		paths := ParseSensitivePaths(value)
		if len(paths) == 0 {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty comma-separated list of JSONPath strings", value, key)
		}
		for _, path := range paths {
			if strings.TrimSpace(path) == "" {
				return fmt.Errorf("invalid value %q for annotation %q, JSONPath cannot be empty", value, key)
			}

			if _, err := jp.ParseString(path); err != nil {
				return fmt.Errorf("invalid JSONPath expression %q in annotation %q: %v", path, key, err)
			}
		}
	}

	return nil
}

func validateDeployOn(meta *meta.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternDeployOn); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		for _, on := range strings.Split(value, ",") {
			on = strings.TrimSpace(on)
			if on == "" {
				return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
			}

			switch value {
			case string(helmrelease.HookPreInstall),
				string(helmrelease.HookPostInstall),
				string(helmrelease.HookPreUpgrade),
				string(helmrelease.HookPostUpgrade),
				string(helmrelease.HookPreRollback),
				string(helmrelease.HookPostRollback),
				string(helmrelease.HookPreDelete),
				string(helmrelease.HookPostDelete),
				string(helmrelease.HookTest),
				string(helmrelease.HookInstall),
				string(helmrelease.HookUpgrade),
				string(helmrelease.HookRollback),
				string(helmrelease.HookDelete),
				"test-success":
			default:
				return fmt.Errorf("value %q for annotation %q is not supported", value, key)
			}
		}
	}

	return nil
}

func validateOwnership(meta *meta.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternOwnership); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch common.Ownership(value) {
		case common.OwnershipEveryone:
		case common.OwnershipRelease:
		default:
			return fmt.Errorf("invalid unknown value %q for annotation %q", value, key)
		}
	}

	return nil
}

func recreate(meta *meta.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicyBeforeCreation)
}

func defaultReplicasOnCreation(meta *meta.ResourceMeta, releaseNamespace string) *int {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternReplicasOnCreation)
	if !found {
		return nil
	}

	return lo.ToPtr(lo.Must(strconv.Atoi(value)))
}

func failMode(meta *meta.ResourceMeta) multitrack.FailMode {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternFailMode)
	if !found {
		return multitrack.FailWholeDeployProcessImmediately
	}

	return multitrack.FailMode(value)
}

func failuresAllowed(unstruct *unstructured.Unstructured) int {
	gk := unstruct.GroupVersionKind().GroupKind()

	if gk == (schema.GroupKind{Group: "batch", Kind: "Job"}) {
		return 0
	}

	if restartPolicy, found, err := unstructured.NestedString(unstruct.UnstructuredContent(), "spec", "template", "spec", "restartPolicy"); err == nil && found && restartPolicy == string(v1.RestartPolicyNever) {
		return 0
	}

	var failuresAllowed int
	_, value, found := FindAnnotationOrLabelByKeyPattern(unstruct.GetAnnotations(), AnnotationKeyPatternFailuresAllowedPerReplica)
	if found {
		failuresAllowed = lo.Must(strconv.Atoi(value))
	} else {
		switch gk {
		case schema.GroupKind{Group: "apps", Kind: "Deployment"},
			schema.GroupKind{Group: "extensions", Kind: "Deployment"},
			schema.GroupKind{Group: "apps", Kind: "DaemonSet"},
			schema.GroupKind{Group: "extensions", Kind: "DaemonSet"},
			schema.GroupKind{Group: "flagger.app", Kind: "Canary"},
			schema.GroupKind{Group: "apps", Kind: "StatefulSet"}:
			failuresAllowed = 1
		default:
			return 0
		}
	}

	if replicas, found, err := unstructured.NestedInt64(unstruct.UnstructuredContent(), "spec", "replicas"); err == nil && found {
		failuresAllowed = int(replicas) * failuresAllowed
	}

	return failuresAllowed
}

func ignoreReadinessProbeFailsForContainers(meta *meta.ResourceMeta) map[string]time.Duration {
	annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternIgnoreReadinessProbeFailsFor)
	if !found {
		return nil
	}

	durationByContainer := map[string]time.Duration{}
	for key, value := range annotations {
		keyMatches := AnnotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(key)
		containerSubexpIndex := AnnotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
		container := keyMatches[containerSubexpIndex]
		duration := lo.Must(time.ParseDuration(value))
		durationByContainer[container] = duration
	}

	return durationByContainer
}

func logRegex(meta *meta.ResourceMeta) *regexp.Regexp {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternLogRegex)
	if !found {
		return nil
	}

	return regexp.MustCompile(value)
}

func logRegexesForContainers(meta *meta.ResourceMeta) map[string]*regexp.Regexp {
	annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternLogRegexFor)
	if !found {
		return nil
	}

	regexByContainer := map[string]*regexp.Regexp{}
	for key, value := range annotations {
		keyMatches := AnnotationKeyPatternLogRegexFor.FindStringSubmatch(key)
		containerSubexpIndex := AnnotationKeyPatternLogRegexFor.SubexpIndex("container")
		container := keyMatches[containerSubexpIndex]
		regexByContainer[container] = regexp.MustCompile(value)
	}

	return regexByContainer
}

func noActivityTimeout(meta *meta.ResourceMeta) time.Duration {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternNoActivityTimeout)
	if !found {
		return 4 * time.Minute
	}

	return lo.Must(time.ParseDuration(value))
}

func showLogsOnlyForContainers(meta *meta.ResourceMeta) []string {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternShowLogsOnlyForContainers)
	if !found {
		return nil
	}

	var containers []string
	for _, container := range strings.Split(value, ",") {
		container = strings.TrimSpace(container)
		containers = append(containers, strings.TrimSpace(container))
	}

	return containers
}

func showServiceMessages(meta *meta.ResourceMeta) bool {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternShowServiceMessages)
	if !found {
		return false
	}

	return lo.Must(strconv.ParseBool(value))
}

func showLogsOnlyForNumberOfReplicas(meta *meta.ResourceMeta) int {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternShowLogsOnlyForNumberOfReplicas)
	if !found {
		return 1
	}

	return lo.Must(strconv.Atoi(value))
}

func skipLogs(meta *meta.ResourceMeta) bool {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternSkipLogs)
	if !found {
		return false
	}

	return lo.Must(strconv.ParseBool(value))
}

func skipLogsForContainers(meta *meta.ResourceMeta) []string {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternSkipLogsForContainers)
	if !found {
		return nil
	}

	var containers []string
	for _, container := range strings.Split(value, ",") {
		container = strings.TrimSpace(container)
		containers = append(containers, strings.TrimSpace(container))
	}

	return containers
}

func trackTerminationMode(meta *meta.ResourceMeta) multitrack.TrackTerminationMode {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternTrackTerminationMode)
	if !found {
		return multitrack.WaitUntilResourceReady
	}

	return multitrack.TrackTerminationMode(value)
}

func deleteOnSucceeded(meta *meta.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicySucceeded)
}

func deleteOnFailed(meta *meta.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicyFailed)
}

func deployConditions(meta *meta.ResourceMeta) map[common.On][]common.Stage {
	if IsCRD(meta.GroupVersionKind.GroupKind()) {
		return map[common.On][]common.Stage{
			common.InstallOnInstall:  []common.Stage{common.StagePrePreInstall},
			common.InstallOnUpgrade:  []common.Stage{common.StagePrePreInstall},
			common.InstallOnRollback: []common.Stage{common.StagePrePreInstall},
		}
	}

	if generalConditions := deployConditionsForAnnotation(meta, AnnotationKeyPatternDeployOn); len(generalConditions) > 0 {
		return generalConditions
	}

	if IsHook(meta.Annotations) {
		if conditions := deployConditionsForAnnotation(meta, AnnotationKeyPatternHook); len(conditions) > 0 {
			return conditions
		}
	}

	return map[common.On][]common.Stage{
		common.InstallOnInstall:  []common.Stage{common.StageInstall},
		common.InstallOnUpgrade:  []common.Stage{common.StageInstall},
		common.InstallOnRollback: []common.Stage{common.StageInstall},
	}
}

func deployConditionsForAnnotation(meta *meta.ResourceMeta, annoPattern *regexp.Regexp) map[common.On][]common.Stage {
	key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annoPattern)
	if !found {
		return nil
	}

	valConditions := lo.Map(strings.Split(value, ","), func(p string, _ int) string {
		return strings.TrimSpace(p)
	})

	var result map[common.On][]common.Stage
	for _, valCondition := range valConditions {
		switch valCondition {
		case string(helmrelease.HookPreInstall):
			result[common.InstallOnInstall] = append(result[common.InstallOnInstall], common.StagePreInstall)
		case string(helmrelease.HookPostInstall):
			result[common.InstallOnInstall] = append(result[common.InstallOnInstall], common.StagePostInstall)
		case string(helmrelease.HookPreUpgrade):
			result[common.InstallOnUpgrade] = append(result[common.InstallOnUpgrade], common.StagePreInstall)
		case string(helmrelease.HookPostUpgrade):
			result[common.InstallOnUpgrade] = append(result[common.InstallOnUpgrade], common.StagePostInstall)
		case string(helmrelease.HookPreRollback):
			result[common.InstallOnRollback] = append(result[common.InstallOnRollback], common.StagePreInstall)
		case string(helmrelease.HookPostRollback):
			result[common.InstallOnRollback] = append(result[common.InstallOnRollback], common.StagePostInstall)
		case string(helmrelease.HookPreDelete):
			result[common.InstallOnDelete] = append(result[common.InstallOnDelete], common.StagePreInstall)
		case string(helmrelease.HookPostDelete):
			result[common.InstallOnDelete] = append(result[common.InstallOnDelete], common.StagePostInstall)
		case string(helmrelease.HookTest), "test-success":
			result[common.InstallOnTest] = append(result[common.InstallOnTest], common.StageInstall)
		default:
			panic(fmt.Sprintf("unknown value %q for %s", value, key))
		}
	}

	for on := range result {
		sort.SliceStable(result[on], func(i, j int) bool {
			return common.StagesSortHandler(result[on][i], result[on][j])
		})
	}

	return result
}

func ownership(meta *meta.ResourceMeta, releaseNamespace string) common.Ownership {
	if IsReleaseNamespace(meta.Name, meta.GroupVersionKind, releaseNamespace) {
		return common.OwnershipEveryone
	}

	if _, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternOwnership); found {
		return common.Ownership(value)
	}

	if IsHook(meta.Annotations) {
		return common.OwnershipEveryone
	}

	return common.OwnershipRelease
}

func weight(meta *meta.ResourceMeta, hasManualInternalDeps bool) *int {
	if hasManualInternalDeps {
		return nil
	}

	if IsCRD(meta.GroupVersionKind.GroupKind()) {
		return lo.ToPtr(0)
	}

	var weightValue string
	if IsHook(meta.Annotations) {
		_, hookWeightValue, hookWeightFound := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternHookWeight)

		_, generalWeightValue, weightFound := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternWeight)

		if !hookWeightFound && !weightFound {
			return lo.ToPtr(0)
		} else if weightFound {
			weightValue = generalWeightValue
		} else {
			weightValue = hookWeightValue
		}
	} else {
		var found bool
		_, weightValue, found = FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternWeight)
		if !found {
			return lo.ToPtr(0)
		}
	}

	return lo.ToPtr(lo.Must(strconv.Atoi(weightValue)))
}

func deletePolicies(meta *meta.ResourceMeta) []common.DeletePolicy {
	var deletePolicies []common.DeletePolicy
	if IsHook(meta.Annotations) {
		_, hookDeletePolicies, hookDeletePoliciesFound := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternHookDeletePolicy)

		_, generalDeletePolicies, generalDeletePoliciesFound := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternDeletePolicy)

		if !hookDeletePoliciesFound && !generalDeletePoliciesFound {
			deletePolicies = append(deletePolicies, common.DeletePolicyBeforeCreation)
		} else if generalDeletePoliciesFound {
			for _, deletePolicy := range strings.Split(generalDeletePolicies, ",") {
				deletePolicy = strings.TrimSpace(deletePolicy)
				deletePolicies = append(deletePolicies, common.DeletePolicy(deletePolicy))
			}
		} else {
			for _, deletePolicy := range strings.Split(hookDeletePolicies, ",") {
				deletePolicy = strings.TrimSpace(deletePolicy)

				switch deletePolicy {
				case string(helmrelease.HookSucceeded):
					deletePolicies = append(deletePolicies, common.DeletePolicySucceeded)
				case string(helmrelease.HookFailed):
					deletePolicies = append(deletePolicies, common.DeletePolicyFailed)
				case string(helmrelease.HookBeforeHookCreation):
					deletePolicies = append(deletePolicies, common.DeletePolicyBeforeCreation)
				}
			}
		}
	} else {
		if _, generalDeletePolicies, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, AnnotationKeyPatternDeletePolicy); found {
			for _, deletePolicy := range strings.Split(generalDeletePolicies, ",") {
				deletePolicy = strings.TrimSpace(deletePolicy)
				deletePolicies = append(deletePolicies, common.DeletePolicy(deletePolicy))
			}
		}
	}

	return deletePolicies
}

func manualInternalDependencies(meta *meta.ResourceMeta) []*InternalDependency {
	if IsCRD(meta.GroupVersionKind.GroupKind()) {
		return nil
	}

	deps := map[string]*InternalDependency{}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternDependency); found {
		for key, value := range annotations {
			matches := AnnotationKeyPatternDependency.FindStringSubmatch(key)
			idSubexpIndex := AnnotationKeyPatternDependency.SubexpIndex("id")
			depID := matches[idSubexpIndex]
			valParts := strings.Split(value, ":")
			depApiVersionParts := strings.SplitN(valParts[0], "/", 2)

			var gvk schema.GroupVersionKind
			if len(depApiVersionParts) == 1 {
				gvk = schema.GroupVersionKind{
					Version: depApiVersionParts[0],
					Kind:    valParts[1],
				}
			} else {
				gvk = schema.GroupVersionKind{
					Group:   depApiVersionParts[0],
					Version: depApiVersionParts[1],
					Kind:    valParts[1],
				}
			}

			var depNamespace string
			if len(valParts) == 4 {
				depNamespace = valParts[2]
			}

			depName := valParts[len(valParts)-1]

			dep := NewInternalDependency(
				[]string{depName},
				[]string{depNamespace},
				[]string{gvk.Group},
				[]string{gvk.Version},
				[]string{gvk.Kind},
				common.ResourceStatePresent,
			)
			deps[depID] = dep
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, AnnotationKeyPatternDeployDependency); found {
		for key, value := range annotations {
			matches := AnnotationKeyPatternDeployDependency.FindStringSubmatch(key)
			idSubexpIndex := AnnotationKeyPatternDeployDependency.SubexpIndex("id")
			depID := matches[idSubexpIndex]
			properties := lo.Must(util.ParseProperties(context.TODO(), value))

			var depNames []string
			if depName, found := properties["name"]; found {
				depNames = []string{depName.(string)}
			}

			var depNamespaces []string
			if depNamespace, found := properties["namespace"]; found {
				depNamespaces = []string{depNamespace.(string)}
			}

			var depGroups []string
			if depGroup, found := properties["group"]; found {
				depGroups = []string{depGroup.(string)}
			}

			var depVersions []string
			if depVersion, found := properties["version"]; found {
				depVersions = []string{depVersion.(string)}
			}

			var depKinds []string
			if depKind, found := properties["kind"]; found {
				depKinds = []string{depKind.(string)}
			}

			var depState common.ResourceState
			if s := properties["state"].(string); s != "" {
				depState = common.ResourceState(s)
			} else {
				depState = common.ResourceStatePresent
			}

			dep := NewInternalDependency(
				depNames,
				depNamespaces,
				depGroups,
				depVersions,
				depKinds,
				depState,
			)
			deps[depID] = dep
		}
	}

	return lo.Values(deps)
}

func externalDependencies(meta *meta.ResourceMeta, releaseNamespace string, mapper apimeta.ResettableRESTMapper) ([]*ExternalDependency, error) {
	if IsCRD(meta.GroupVersionKind.GroupKind()) {
		return nil, nil
	}

	deps := externalDeps(meta, releaseNamespace)

	legacyExtDeps := map[string]*ExternalDependency{}
	// Pretend that we don't have any external dependencies when we don't have cluster access, since we need cluster access to map GVR to GVK.
	if mapper != nil {
		var err error
		legacyExtDeps, err = legacyExternalDeps(meta, releaseNamespace, mapper)
		if err != nil {
			return nil, fmt.Errorf("get legacy external dependencies: %w", err)
		}
	}

	duplResult := lo.Values(lo.Assign(legacyExtDeps, deps))
	uniqResult := lo.UniqBy(duplResult, func(d *ExternalDependency) string {
		return d.ID()
	})

	return uniqResult, nil
}

func externalDeps(resMeta *meta.ResourceMeta, releaseNamespace string) map[string]*ExternalDependency {
	deps := map[string]*ExternalDependency{}
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(resMeta.Annotations, AnnotationKeyPatternExternalDependency); found {
		for key, value := range annotations {
			matches := AnnotationKeyPatternExternalDependency.FindStringSubmatch(key)
			idSubexpIndex := AnnotationKeyPatternExternalDependency.SubexpIndex("id")
			depID := matches[idSubexpIndex]
			valParts := strings.Split(value, ":")
			depApiVersionParts := strings.SplitN(valParts[0], "/", 2)

			var gvk schema.GroupVersionKind
			if len(depApiVersionParts) == 1 {
				gvk = schema.GroupVersionKind{
					Version: depApiVersionParts[0],
					Kind:    valParts[1],
				}
			} else {
				gvk = schema.GroupVersionKind{
					Group:   depApiVersionParts[0],
					Version: depApiVersionParts[1],
					Kind:    valParts[1],
				}
			}

			var depNamespace string
			if len(valParts) == 4 {
				depNamespace = valParts[2]
			}

			depName := valParts[len(valParts)-1]

			resMeta := meta.NewResourceMeta(depName, depNamespace, releaseNamespace, "", gvk, nil, nil)
			dep := &ExternalDependency{
				ResourceMeta: resMeta,
			}

			deps[depID] = dep
		}
	}

	return deps
}

// TODO(v2): get rid of legacy external deps
func legacyExternalDeps(resMeta *meta.ResourceMeta, releaseNamespace string, mapper apimeta.ResettableRESTMapper) (map[string]*ExternalDependency, error) {
	deps := map[string]*ExternalDependency{}

	type DepInfo struct {
		Name      string
		Namespace string
		Type      string
	}
	extDepInfos := map[string]*DepInfo{}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(resMeta.Annotations, AnnotationKeyPatternLegacyExternalDependencyResource); found {
		for key, value := range annotations {
			matches := AnnotationKeyPatternLegacyExternalDependencyResource.FindStringSubmatch(key)
			idSubexpIndex := AnnotationKeyPatternLegacyExternalDependencyResource.SubexpIndex("id")
			extDepID := matches[idSubexpIndex]
			extDepType := strings.Split(value, "/")[0]
			extDepName := strings.Split(value, "/")[1]

			extDepInfos[extDepID] = &DepInfo{
				Name: extDepName,
				Type: extDepType,
			}
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(resMeta.Annotations, AnnotationKeyPatternLegacyExternalDependencyNamespace); found {
		for key, value := range annotations {
			matches := AnnotationKeyPatternLegacyExternalDependencyNamespace.FindStringSubmatch(key)
			idSubexpIndex := AnnotationKeyPatternLegacyExternalDependencyNamespace.SubexpIndex("id")
			extDepID := matches[idSubexpIndex]
			extDepNamespace := value

			if extDepInfo, hasKey := extDepInfos[extDepID]; hasKey {
				extDepInfo.Namespace = extDepNamespace
			}
		}
	}

	for extDepID, extDepInfo := range extDepInfos {
		gvk, err := ParseKubectlResourceStringtoGVK(extDepInfo.Type, mapper)
		if err != nil {
			return nil, fmt.Errorf("parse external dependency resource type %q for dependency %q (namespace: %q): %w", extDepInfo.Type, extDepInfo.Name, extDepInfo.Namespace, err)
		}

		resMeta := meta.NewResourceMeta(extDepInfo.Name, extDepInfo.Namespace, releaseNamespace, "", gvk, nil, nil)
		dep := &ExternalDependency{
			ResourceMeta: resMeta,
		}

		deps[extDepID] = dep
	}

	return deps, nil
}
