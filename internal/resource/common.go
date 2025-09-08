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
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/plan/dependency"
	"github.com/werf/nelm/internal/resource/id"
	"github.com/werf/nelm/internal/util"
)

var (
	labelKeyHumanManagedBy   = "app.kubernetes.io/managed-by"
	labelKeyPatternManagedBy = regexp.MustCompile(`^app.kubernetes.io/managed-by$`)

	annotationKeyHumanReleaseName   = "meta.helm.sh/release-name"
	annotationKeyPatternReleaseName = regexp.MustCompile(`^meta.helm.sh/release-name$`)

	annotationKeyHumanReleaseNamespace   = "meta.helm.sh/release-namespace"
	annotationKeyPatternReleaseNamespace = regexp.MustCompile(`^meta.helm.sh/release-namespace$`)

	annotationKeyHumanHook   = "helm.sh/hook"
	annotationKeyPatternHook = regexp.MustCompile(`^helm.sh/hook$`)

	annotationKeyHumanResourcePolicy   = "helm.sh/resource-policy"
	annotationKeyPatternResourcePolicy = regexp.MustCompile(`^helm.sh/resource-policy$`)

	annotationKeyHumanDeletePolicy   = "werf.io/delete-policy"
	annotationKeyPatternDeletePolicy = regexp.MustCompile(`^werf.io/delete-policy$`)

	annotationKeyHumanHookDeletePolicy   = "helm.sh/hook-delete-policy"
	annotationKeyPatternHookDeletePolicy = regexp.MustCompile(`^helm.sh/hook-delete-policy$`)

	annotationKeyHumanReplicasOnCreation   = "werf.io/replicas-on-creation"
	annotationKeyPatternReplicasOnCreation = regexp.MustCompile(`^werf.io/replicas-on-creation$`)

	annotationKeyHumanFailMode   = "werf.io/fail-mode"
	annotationKeyPatternFailMode = regexp.MustCompile(`^werf.io/fail-mode$`)

	annotationKeyHumanFailuresAllowedPerReplica   = "werf.io/failures-allowed-per-replica"
	annotationKeyPatternFailuresAllowedPerReplica = regexp.MustCompile(`^werf.io/failures-allowed-per-replica$`)

	annotationKeyHumanIgnoreReadinessProbeFailsFor   = "werf.io/ignore-readiness-probe-fails-for-<container>"
	annotationKeyPatternIgnoreReadinessProbeFailsFor = regexp.MustCompile(`^werf.io/ignore-readiness-probe-fails-for-(?P<container>.+)$`)

	annotationKeyHumanLogRegex   = "werf.io/log-regex"
	annotationKeyPatternLogRegex = regexp.MustCompile(`^werf.io/log-regex$`)

	annotationKeyHumanLogRegexFor   = "werf.io/log-regex-for-<container>"
	annotationKeyPatternLogRegexFor = regexp.MustCompile(`^werf.io/log-regex-for-(?P<container>.+)$`)

	annotationKeyHumanNoActivityTimeout   = "werf.io/no-activity-timeout"
	annotationKeyPatternNoActivityTimeout = regexp.MustCompile(`^werf.io/no-activity-timeout$`)

	annotationKeyHumanShowLogsOnlyForContainers   = "werf.io/show-logs-only-for-containers"
	annotationKeyPatternShowLogsOnlyForContainers = regexp.MustCompile(`^werf.io/show-logs-only-for-containers$`)

	annotationKeyHumanShowServiceMessages   = "werf.io/show-service-messages"
	annotationKeyPatternShowServiceMessages = regexp.MustCompile(`^werf.io/show-service-messages$`)

	annotationKeyHumanShowLogsOnlyForNumberOfReplicas   = "werf.io/show-logs-only-for-number-of-replicas"
	annotationKeyPatternShowLogsOnlyForNumberOfReplicas = regexp.MustCompile(`^werf.io/show-logs-only-for-number-of-replicas$`)

	annotationKeyHumanSkipLogs   = "werf.io/skip-logs"
	annotationKeyPatternSkipLogs = regexp.MustCompile(`^werf.io/skip-logs$`)

	annotationKeyHumanSkipLogsForContainers   = "werf.io/skip-logs-for-containers"
	annotationKeyPatternSkipLogsForContainers = regexp.MustCompile(`^werf.io/skip-logs-for-containers$`)

	annotationKeyHumanTrackTerminationMode   = "werf.io/track-termination-mode"
	annotationKeyPatternTrackTerminationMode = regexp.MustCompile(`^werf.io/track-termination-mode$`)

	annotationKeyHumanWeight   = "werf.io/weight"
	annotationKeyPatternWeight = regexp.MustCompile(`^werf.io/weight$`)

	annotationKeyHumanHookWeight   = "helm.sh/hook-weight"
	annotationKeyPatternHookWeight = regexp.MustCompile(`^helm.sh/hook-weight$`)

	annotationKeyHumanDeployDependency   = "werf.io/deploy-dependency-<name>"
	annotationKeyPatternDeployDependency = regexp.MustCompile(`^werf.io/deploy-dependency-(?P<id>.+)$`)

	// TODO(v2): get rid
	annotationKeyHumanDependency   = "<name>.dependency.werf.io"
	annotationKeyPatternDependency = regexp.MustCompile(`^(?P<id>.+).dependency.werf.io$`)

	annotationKeyHumanExternalDependency   = "<name>.external-dependency.werf.io"
	annotationKeyPatternExternalDependency = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io$`)

	annotationKeyHumanLegacyExternalDependencyResource   = "<name>.external-dependency.werf.io/resource"
	annotationKeyPatternLegacyExternalDependencyResource = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/resource$`)

	annotationKeyHumanLegacyExternalDependencyNamespace   = "<name>.external-dependency.werf.io/namespace"
	annotationKeyPatternLegacyExternalDependencyNamespace = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/namespace$`)

	annotationKeyHumanSensitive   = "werf.io/sensitive"
	annotationKeyPatternSensitive = regexp.MustCompile(`^werf.io/sensitive$`)

	annotationKeyHumanSensitivePaths   = "werf.io/sensitive-paths"
	annotationKeyPatternSensitivePaths = regexp.MustCompile(`^werf.io/sensitive-paths$`)

	annotationKeyHumanDeployOn   = "werf.io/deploy-on"
	annotationKeyPatternDeployOn = regexp.MustCompile(`^werf.io/deploy-on$`)

	annotationKeyHumanOwnership   = "werf.io/ownership"
	annotationKeyPatternOwnership = regexp.MustCompile(`^werf.io/ownership$`)
)

func ValidateResourcePolicy(meta *id.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternResourcePolicy); found {
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

func KeepOnDelete(meta *id.ResourceMeta, releaseNamespace string) bool {
	if IsReleaseNamespace(meta.Name, meta.GroupVersionKind, releaseNamespace) {
		return true
	}

	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternResourcePolicy)
	if !found {
		return false
	}

	return value == "keep"
}

func Orphaned(meta *id.ResourceMeta, releaseName, releaseNamespace string) bool {
	if _, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternReleaseName); !found || value != releaseName {
		return true
	}

	if _, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternReleaseNamespace); !found || value != releaseNamespace {
		return true
	}

	if _, value, found := FindAnnotationOrLabelByKeyPattern(meta.Labels, labelKeyPatternManagedBy); !found || value != "Helm" {
		return true
	}

	return false
}

func AdoptableBy(meta *id.ResourceMeta, releaseName, releaseNamespace string) (adoptable bool, nonAdoptableReason string) {
	nonAdoptableReasons := []string{}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternReleaseName); found {
		if value != releaseName {
			nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseName))
		}
	} else {
		nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, annotationKeyHumanReleaseName, releaseName))
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternReleaseNamespace); found {
		if value != releaseNamespace {
			nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseNamespace))
		}
	} else {
		nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, annotationKeyHumanReleaseNamespace, releaseNamespace))
	}

	nonAdoptableReason = strings.Join(nonAdoptableReasons, ", ")

	return len(nonAdoptableReasons) == 0, nonAdoptableReason
}

func validateHook(meta *id.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternHook); found {
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

func validateWeight(meta *id.ResourceMeta) error {
	if IsHook(meta.Annotations) {
		if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternHookWeight); found {
			if value == "" {
				return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
			}

			if _, err := strconv.Atoi(value); err != nil {
				return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
			}
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternWeight); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		}
	}

	return nil
}

func validateDeletePolicy(meta *id.ResourceMeta) error {
	annotations := meta.Annotations

	if IsHook(annotations) {
		if key, value, found := FindAnnotationOrLabelByKeyPattern(annotations, annotationKeyPatternHookDeletePolicy); found && value != "" {
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

	if key, value, found := FindAnnotationOrLabelByKeyPattern(annotations, annotationKeyPatternDeletePolicy); found && value != "" {
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

func validateReplicasOnCreation(meta *id.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternReplicasOnCreation); found {
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

func validateTrack(meta *id.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternFailMode); found {
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

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternFailuresAllowedPerReplica); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if failures, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		} else if failures < 0 {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-negative integer value", value, key)
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternIgnoreReadinessProbeFailsFor); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			containerSubexpIndex := annotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
			if containerSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", annotationKeyPatternIgnoreReadinessProbeFailsFor.String(), key)
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

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternLogRegex); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		if _, err := regexp.Compile(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected valid regexp", value, key)
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternLogRegexFor); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternLogRegexFor.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			containerSubexpIndex := annotationKeyPatternLogRegexFor.SubexpIndex("container")
			if containerSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", annotationKeyPatternLogRegexFor.String(), key)
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

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternNoActivityTimeout); found {
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

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternShowLogsOnlyForContainers); found {
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

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternShowServiceMessages); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternShowLogsOnlyForNumberOfReplicas); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if replicas, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		} else if replicas < 0 {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-negative integer value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternSkipLogs); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternSkipLogsForContainers); found {
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

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternTrackTerminationMode); found {
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

func validateDeployDependencies(meta *id.ResourceMeta) error {
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternDeployDependency); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternDeployDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := annotationKeyPatternDeployDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", annotationKeyPatternDeployDependency.String(), key)
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

func validateInternalDependencies(meta *id.ResourceMeta) error {
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternDependency); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := annotationKeyPatternDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", annotationKeyPatternDependency.String(), key)
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

func validateExternalDependencies(meta *id.ResourceMeta) error {
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternExternalDependency); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternExternalDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := annotationKeyPatternExternalDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", annotationKeyPatternExternalDependency.String(), key)
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

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternLegacyExternalDependencyResource); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternLegacyExternalDependencyResource.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := annotationKeyPatternLegacyExternalDependencyResource.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", annotationKeyPatternLegacyExternalDependencyResource.String(), key)
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

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternLegacyExternalDependencyNamespace); found {
		for key, value := range annotations {
			keyMatches := annotationKeyPatternLegacyExternalDependencyNamespace.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := annotationKeyPatternLegacyExternalDependencyNamespace.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", annotationKeyPatternLegacyExternalDependencyNamespace.String(), key)
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

func validateSensitive(meta *id.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternSensitive); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternSensitivePaths); found {
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

func validateDeployOn(meta *id.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternDeployOn); found {
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

func validateOwnership(meta *id.ResourceMeta) error {
	if key, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternOwnership); found {
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

func recreate(meta *id.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicyBeforeCreation)
}

func defaultReplicasOnCreation(meta *id.ResourceMeta, releaseNamespace string) *int {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternReplicasOnCreation)
	if !found {
		return nil
	}

	return lo.ToPtr(lo.Must(strconv.Atoi(value)))
}

func failMode(meta *id.ResourceMeta) multitrack.FailMode {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternFailMode)
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
	_, value, found := FindAnnotationOrLabelByKeyPattern(unstruct.GetAnnotations(), annotationKeyPatternFailuresAllowedPerReplica)
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

func ignoreReadinessProbeFailsForContainers(meta *id.ResourceMeta) map[string]time.Duration {
	annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternIgnoreReadinessProbeFailsFor)
	if !found {
		return nil
	}

	durationByContainer := map[string]time.Duration{}
	for key, value := range annotations {
		keyMatches := annotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(key)
		containerSubexpIndex := annotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
		container := keyMatches[containerSubexpIndex]
		duration := lo.Must(time.ParseDuration(value))
		durationByContainer[container] = duration
	}

	return durationByContainer
}

func logRegex(meta *id.ResourceMeta) *regexp.Regexp {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternLogRegex)
	if !found {
		return nil
	}

	return regexp.MustCompile(value)
}

func logRegexesForContainers(meta *id.ResourceMeta) map[string]*regexp.Regexp {
	annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternLogRegexFor)
	if !found {
		return nil
	}

	regexByContainer := map[string]*regexp.Regexp{}
	for key, value := range annotations {
		keyMatches := annotationKeyPatternLogRegexFor.FindStringSubmatch(key)
		containerSubexpIndex := annotationKeyPatternLogRegexFor.SubexpIndex("container")
		container := keyMatches[containerSubexpIndex]
		regexByContainer[container] = regexp.MustCompile(value)
	}

	return regexByContainer
}

func noActivityTimeout(meta *id.ResourceMeta) *time.Duration {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternNoActivityTimeout)
	if !found {
		return nil
	}

	return lo.ToPtr(lo.Must(time.ParseDuration(value)))
}

func showLogsOnlyForContainers(meta *id.ResourceMeta) []string {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternShowLogsOnlyForContainers)
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

func showServiceMessages(meta *id.ResourceMeta) bool {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternShowServiceMessages)
	if !found {
		return false
	}

	return lo.Must(strconv.ParseBool(value))
}

func showLogsOnlyForNumberOfReplicas(meta *id.ResourceMeta) int {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternShowLogsOnlyForNumberOfReplicas)
	if !found {
		return 1
	}

	return lo.Must(strconv.Atoi(value))
}

func skipLogs(meta *id.ResourceMeta) bool {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternSkipLogs)
	if !found {
		return false
	}

	return lo.Must(strconv.ParseBool(value))
}

func skipLogsForContainers(meta *id.ResourceMeta) []string {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternSkipLogsForContainers)
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

func trackTerminationMode(meta *id.ResourceMeta) multitrack.TrackTerminationMode {
	_, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternTrackTerminationMode)
	if !found {
		return multitrack.WaitUntilResourceReady
	}

	return multitrack.TrackTerminationMode(value)
}

func deleteOnSucceeded(meta *id.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicySucceeded)
}

func deleteOnFailed(meta *id.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicyFailed)
}

func deployConditions(meta *id.ResourceMeta) map[common.On][]common.Stage {
	if IsCRD(meta.GroupVersionKind.GroupKind()) {
		return map[common.On][]common.Stage{
			common.InstallOnInstall:  []common.Stage{common.StagePrePreInstall},
			common.InstallOnUpgrade:  []common.Stage{common.StagePrePreInstall},
			common.InstallOnRollback: []common.Stage{common.StagePrePreInstall},
		}
	}

	if generalConditions := deployConditionsForAnnotation(meta, annotationKeyPatternDeployOn); len(generalConditions) > 0 {
		return generalConditions
	}

	if IsHook(meta.Annotations) {
		if conditions := deployConditionsForAnnotation(meta, annotationKeyPatternHook); len(conditions) > 0 {
			return conditions
		}
	}

	return map[common.On][]common.Stage{
		common.InstallOnInstall:  []common.Stage{common.StageInstall},
		common.InstallOnUpgrade:  []common.Stage{common.StageInstall},
		common.InstallOnRollback: []common.Stage{common.StageInstall},
	}
}

func deployConditionsForAnnotation(meta *id.ResourceMeta, annoPattern *regexp.Regexp) map[common.On][]common.Stage {
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

func ownership(meta *id.ResourceMeta, releaseNamespace string) common.Ownership {
	if IsReleaseNamespace(meta.Name, meta.GroupVersionKind, releaseNamespace) {
		return common.OwnershipEveryone
	}

	if _, value, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternOwnership); found {
		return common.Ownership(value)
	}

	if IsHook(meta.Annotations) {
		return common.OwnershipEveryone
	}

	return common.OwnershipRelease
}

func weight(meta *id.ResourceMeta, hasManualInternalDeps bool) *int {
	if hasManualInternalDeps {
		return nil
	}

	if IsCRD(meta.GroupVersionKind.GroupKind()) {
		return lo.ToPtr(0)
	}

	var weightValue string
	if IsHook(meta.Annotations) {
		_, hookWeightValue, hookWeightFound := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternHookWeight)

		_, generalWeightValue, weightFound := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternWeight)

		if !hookWeightFound && !weightFound {
			return lo.ToPtr(0)
		} else if weightFound {
			weightValue = generalWeightValue
		} else {
			weightValue = hookWeightValue
		}
	} else {
		var found bool
		_, weightValue, found = FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternWeight)
		if !found {
			return lo.ToPtr(0)
		}
	}

	return lo.ToPtr(lo.Must(strconv.Atoi(weightValue)))
}

func deletePolicies(meta *id.ResourceMeta) []common.DeletePolicy {
	var deletePolicies []common.DeletePolicy
	if IsHook(meta.Annotations) {
		_, hookDeletePolicies, hookDeletePoliciesFound := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternHookDeletePolicy)

		_, generalDeletePolicies, generalDeletePoliciesFound := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternDeletePolicy)

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
		if _, generalDeletePolicies, found := FindAnnotationOrLabelByKeyPattern(meta.Annotations, annotationKeyPatternDeletePolicy); found {
			for _, deletePolicy := range strings.Split(generalDeletePolicies, ",") {
				deletePolicy = strings.TrimSpace(deletePolicy)
				deletePolicies = append(deletePolicies, common.DeletePolicy(deletePolicy))
			}
		}
	}

	return deletePolicies
}

func manualInternalDependencies(meta *id.ResourceMeta) []*dependency.InternalDependency {
	if IsCRD(meta.GroupVersionKind.GroupKind()) {
		return nil
	}

	deps := map[string]*dependency.InternalDependency{}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternDependency); found {
		for key, value := range annotations {
			matches := annotationKeyPatternDependency.FindStringSubmatch(key)
			idSubexpIndex := annotationKeyPatternDependency.SubexpIndex("id")
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

			dep := dependency.NewInternalDependency(
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

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternDeployDependency); found {
		for key, value := range annotations {
			matches := annotationKeyPatternDeployDependency.FindStringSubmatch(key)
			idSubexpIndex := annotationKeyPatternDeployDependency.SubexpIndex("id")
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

			dep := dependency.NewInternalDependency(
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

func externalDependencies(meta *id.ResourceMeta, releaseNamespace string, mapper meta.ResettableRESTMapper) ([]*dependency.ExternalDependency, error) {
	if IsCRD(meta.GroupVersionKind.GroupKind()) {
		return nil, nil
	}

	deps := externalDeps(meta, releaseNamespace)

	legacyExtDeps := map[string]*dependency.ExternalDependency{}
	// Pretend that we don't have any external dependencies when we don't have cluster access, since we need cluster access to map GVR to GVK.
	if mapper != nil {
		var err error
		legacyExtDeps, err = legacyExternalDeps(meta, releaseNamespace, mapper)
		if err != nil {
			return nil, fmt.Errorf("get legacy external dependencies: %w", err)
		}
	}

	duplResult := lo.Values(lo.Assign(legacyExtDeps, deps))
	uniqResult := lo.UniqBy(duplResult, func(d *dependency.ExternalDependency) string {
		return d.ID()
	})

	return uniqResult, nil
}

func externalDeps(meta *id.ResourceMeta, releaseNamespace string) map[string]*dependency.ExternalDependency {
	deps := map[string]*dependency.ExternalDependency{}
	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternExternalDependency); found {
		for key, value := range annotations {
			matches := annotationKeyPatternExternalDependency.FindStringSubmatch(key)
			idSubexpIndex := annotationKeyPatternExternalDependency.SubexpIndex("id")
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

			resMeta := id.NewResourceMeta(depName, depNamespace, releaseNamespace, "", gvk, nil, nil)
			dep := dependency.NewExternalDependency(resMeta)

			deps[depID] = dep
		}
	}

	return deps
}

// TODO(v2): get rid of legacy external deps
func legacyExternalDeps(meta *id.ResourceMeta, releaseNamespace string, mapper meta.ResettableRESTMapper) (map[string]*dependency.ExternalDependency, error) {
	deps := map[string]*dependency.ExternalDependency{}

	type DepInfo struct {
		Name      string
		Namespace string
		Type      string
	}
	extDepInfos := map[string]*DepInfo{}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternLegacyExternalDependencyResource); found {
		for key, value := range annotations {
			matches := annotationKeyPatternLegacyExternalDependencyResource.FindStringSubmatch(key)
			idSubexpIndex := annotationKeyPatternLegacyExternalDependencyResource.SubexpIndex("id")
			extDepID := matches[idSubexpIndex]
			extDepType := strings.Split(value, "/")[0]
			extDepName := strings.Split(value, "/")[1]

			extDepInfos[extDepID] = &DepInfo{
				Name: extDepName,
				Type: extDepType,
			}
		}
	}

	if annotations, found := FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, annotationKeyPatternLegacyExternalDependencyNamespace); found {
		for key, value := range annotations {
			matches := annotationKeyPatternLegacyExternalDependencyNamespace.FindStringSubmatch(key)
			idSubexpIndex := annotationKeyPatternLegacyExternalDependencyNamespace.SubexpIndex("id")
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

		resMeta := id.NewResourceMeta(extDepInfo.Name, extDepInfo.Namespace, releaseNamespace, "", gvk, nil, nil)
		dep := dependency.NewExternalDependency(resMeta)

		deps[extDepID] = dep
	}

	return deps, nil
}
