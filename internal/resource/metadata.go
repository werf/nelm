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
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	apiv1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
)

func ValidateResourcePolicy(meta *spec.ResourceMeta) error {
	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternResourcePolicy); found {
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

func KeepOnDelete(meta *spec.ResourceMeta, releaseNamespace string) bool {
	if spec.IsReleaseNamespace(meta.Name, meta.GroupVersionKind, releaseNamespace) {
		return true
	}

	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternResourcePolicy)
	if !found {
		return false
	}

	return value == "keep"
}

func validateHook(meta *spec.ResourceMeta) error {
	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternHook); found {
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

func validateWeight(meta *spec.ResourceMeta) error {
	if spec.IsHook(meta.Annotations) {
		if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternHookWeight); found {
			if value == "" {
				return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
			}

			if _, err := strconv.Atoi(value); err != nil {
				return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
			}
		}
	}

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternWeight); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		}
	}

	return nil
}

func validateDeletePolicy(meta *spec.ResourceMeta) error {
	annotations := meta.Annotations

	if spec.IsHook(annotations) {
		if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(annotations, common.AnnotationKeyPatternHookDeletePolicy); found && value != "" {
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

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(annotations, common.AnnotationKeyPatternDeletePolicy); found && value != "" {
		for _, deletePolicy := range strings.Split(value, ",") {
			deletePolicy = strings.TrimSpace(deletePolicy)
			if deletePolicy == "" {
				return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
			}

			switch deletePolicy {
			case string(common.DeletePolicySucceeded),
				string(common.DeletePolicyFailed),
				string(common.DeletePolicyBeforeCreation),
				string(common.DeletePolicyBeforeCreationIfImmutable):
			default:
				return fmt.Errorf("value %q for annotation %q is not supported", value, key)
			}
		}
	}

	return nil
}

func validateReplicasOnCreation(meta *spec.ResourceMeta) error {
	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternReplicasOnCreation); found {
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

func validateTrack(meta *spec.ResourceMeta) error {
	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternFailMode); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch value {
		case string(multitrack.IgnoreAndContinueDeployProcess):
		case string(multitrack.FailWholeDeployProcessImmediately):
		case string(multitrack.LegacyHopeUntilEndOfDeployProcess):
		default:
			return fmt.Errorf("invalid unknown value %q for annotation %q", value, key)
		}
	}

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternFailuresAllowedPerReplica); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if failures, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		} else if failures < 0 {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-negative integer value", value, key)
		}
	}

	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternIgnoreReadinessProbeFailsFor); found {
		for key, value := range annotations {
			keyMatches := common.AnnotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			containerSubexpIndex := common.AnnotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
			if containerSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", common.AnnotationKeyPatternIgnoreReadinessProbeFailsFor.String(), key)
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

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternLogRegex); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		if _, err := regexp.Compile(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected valid regexp", value, key)
		}
	}

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternLogRegexSkip); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		if _, err := regexp.Compile(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected valid regexp", value, key)
		}
	}

	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternLogRegexFor); found {
		for key, value := range annotations {
			keyMatches := common.AnnotationKeyPatternLogRegexFor.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			containerSubexpIndex := common.AnnotationKeyPatternLogRegexFor.SubexpIndex("container")
			if containerSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", common.AnnotationKeyPatternLogRegexFor.String(), key)
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

	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternSkipLogRegexFor); found {
		for key, value := range annotations {
			keyMatches := common.AnnotationKeyPatternSkipLogRegexFor.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			containerSubexpIndex := common.AnnotationKeyPatternSkipLogRegexFor.SubexpIndex("container")
			if containerSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", common.AnnotationKeyPatternSkipLogRegexFor.String(), key)
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

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternNoActivityTimeout); found {
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

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternShowLogsOnlyForContainers); found {
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

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternShowServiceMessages); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternShowLogsOnlyForNumberOfReplicas); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty integer value", value, key)
		}

		if replicas, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected integer value", value, key)
		} else if replicas < 0 {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-negative integer value", value, key)
		}
	}

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternSkipLogs); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternSkipLogsForContainers); found {
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

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternTrackTerminationMode); found {
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

func validateDeployDependencies(meta *spec.ResourceMeta) error {
	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDeployDependency); found {
		for key, value := range annotations {
			keyMatches := common.AnnotationKeyPatternDeployDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := common.AnnotationKeyPatternDeployDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", common.AnnotationKeyPatternDeployDependency.String(), key)
			}

			if len(keyMatches) < idSubexpIndex+1 {
				return fmt.Errorf("can't parse deploy dependency id from annotation key %q", key)
			}

			if value == "" {
				return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
			}

			properties, err := util.ParseProperties(context.TODO(), value)
			if err != nil {
				return fmt.Errorf("invalid value %q for annotation %q: %w", value, key, err)
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
						return fmt.Errorf("invalid boolean value %t for property %q, expected string value", pv, propKey)
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
						return fmt.Errorf("invalid boolean value %t for property %q, expected string value", pv, propKey)
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

func validateInternalDependencies(meta *spec.ResourceMeta) error {
	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDependency); found {
		for key, value := range annotations {
			keyMatches := common.AnnotationKeyPatternDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := common.AnnotationKeyPatternDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", common.AnnotationKeyPatternDependency.String(), key)
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

func validateExternalDependencies(meta *spec.ResourceMeta) error {
	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternExternalDependency); found {
		for key, value := range annotations {
			keyMatches := common.AnnotationKeyPatternExternalDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := common.AnnotationKeyPatternExternalDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", common.AnnotationKeyPatternExternalDependency.String(), key)
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

	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternLegacyExternalDependencyResource); found {
		for key, value := range annotations {
			keyMatches := common.AnnotationKeyPatternLegacyExternalDependencyResource.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := common.AnnotationKeyPatternLegacyExternalDependencyResource.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", common.AnnotationKeyPatternLegacyExternalDependencyResource.String(), key)
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

	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternLegacyExternalDependencyNamespace); found {
		for key, value := range annotations {
			keyMatches := common.AnnotationKeyPatternLegacyExternalDependencyNamespace.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := common.AnnotationKeyPatternLegacyExternalDependencyNamespace.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", common.AnnotationKeyPatternLegacyExternalDependencyNamespace.String(), key)
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

func validateSensitive(meta *spec.ResourceMeta) error {
	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternSensitive); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty boolean value", value, key)
		}

		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid value %q for annotation %q, expected boolean value", value, key)
		}
	}

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternSensitivePaths); found {
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
				return fmt.Errorf("invalid JSONPath expression %q in annotation %q: %w", path, key, err)
			}
		}
	}

	return nil
}

func validateDeployOn(meta *spec.ResourceMeta) error {
	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDeployOn); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		for _, on := range strings.Split(value, ",") {
			on = strings.TrimSpace(on)
			if on == "" {
				return fmt.Errorf("invalid value %q for annotation %q, one of the comma-separated values is empty", value, key)
			}

			switch on {
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

func validateOwnership(meta *spec.ResourceMeta) error {
	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternOwnership); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch common.Ownership(value) {
		case common.OwnershipAnyone:
		case common.OwnershipRelease:
		default:
			return fmt.Errorf("invalid unknown value %q for annotation %q", value, key)
		}
	}

	return nil
}

func validateDeletePropagation(meta *spec.ResourceMeta) error {
	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDeletePropagation); found {
		if value == "" {
			return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
		}

		switch apiv1.DeletionPropagation(value) {
		case apiv1.DeletePropagationForeground, apiv1.DeletePropagationBackground, apiv1.DeletePropagationOrphan:
		default:
			return fmt.Errorf("invalid unknown value %q for annotation %q", value, key)
		}
	}

	return nil
}

func validateDeleteDependencies(meta *spec.ResourceMeta) error {
	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDeleteDependency); found {
		for key, value := range annotations {
			keyMatches := common.AnnotationKeyPatternDeleteDependency.FindStringSubmatch(key)
			if keyMatches == nil {
				return fmt.Errorf("invalid key for annotation %q", key)
			}

			idSubexpIndex := common.AnnotationKeyPatternDeleteDependency.SubexpIndex("id")
			if idSubexpIndex == -1 {
				return fmt.Errorf("invalid regexp pattern %q for annotation %q", common.AnnotationKeyPatternDeleteDependency.String(), key)
			}

			if len(keyMatches) < idSubexpIndex+1 {
				return fmt.Errorf("can't parse delete dependency id from annotation key %q", key)
			}

			if value == "" {
				return fmt.Errorf("invalid value %q for annotation %q, expected non-empty string value", value, key)
			}

			properties, err := util.ParseProperties(context.TODO(), value)
			if err != nil {
				return fmt.Errorf("invalid value %q for annotation %q: %w", value, key, err)
			}

			if !lo.Some(lo.Keys(properties), []string{"group", "version", "kind", "name", "namespace"}) {
				return fmt.Errorf("invalid value %q for annotation %q, target not specified", value, key)
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
						return fmt.Errorf("invalid boolean value %t for property %q, expected string value", pv, propKey)
					default:
						panic(fmt.Sprintf("unexpected type %T for property %q", pv, propKey))
					}
				case "state":
					switch pv := propVal.(type) {
					case string:
						switch pv {
						case "absent":
						case "":
							return fmt.Errorf("invalid value %q for property %q, expected non-empty string value", pv, propKey)
						default:
							return fmt.Errorf("unknown value %q for property %q", pv, propKey)
						}
					case bool:
						return fmt.Errorf("invalid boolean value %t for property %q, expected string value", pv, propKey)
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

func recreate(meta *spec.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicyBeforeCreation)
}

func recreateOnImmutable(meta *spec.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicyBeforeCreationIfImmutable)
}

func defaultReplicasOnCreation(meta *spec.ResourceMeta, releaseNamespace string) *int {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternReplicasOnCreation)
	if !found {
		return nil
	}

	return lo.ToPtr(lo.Must(strconv.Atoi(value)))
}

func failMode(meta *spec.ResourceMeta) multitrack.FailMode {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternFailMode)
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

	if restartPolicy, found, err := unstructured.NestedString(unstruct.UnstructuredContent(), "spec", "template", "spec", "restartPolicy"); err == nil && found && restartPolicy == string(corev1.RestartPolicyNever) {
		return 0
	}

	var failuresAllowed int

	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(unstruct.GetAnnotations(), common.AnnotationKeyPatternFailuresAllowedPerReplica)
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

func ignoreReadinessProbeFailsForContainers(meta *spec.ResourceMeta) map[string]time.Duration {
	annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternIgnoreReadinessProbeFailsFor)
	if !found {
		return nil
	}

	durationByContainer := map[string]time.Duration{}
	for key, value := range annotations {
		keyMatches := common.AnnotationKeyPatternIgnoreReadinessProbeFailsFor.FindStringSubmatch(key)
		containerSubexpIndex := common.AnnotationKeyPatternIgnoreReadinessProbeFailsFor.SubexpIndex("container")
		container := keyMatches[containerSubexpIndex]
		duration := lo.Must(time.ParseDuration(value))
		durationByContainer[container] = duration
	}

	return durationByContainer
}

func logRegex(meta *spec.ResourceMeta) *regexp.Regexp {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternLogRegex)
	if !found {
		return nil
	}

	return regexp.MustCompile(value)
}

func skipLogRegex(meta *spec.ResourceMeta) *regexp.Regexp {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternLogRegexSkip)
	if !found {
		return nil
	}

	return regexp.MustCompile(value)
}

func logRegexesForContainers(meta *spec.ResourceMeta) map[string]*regexp.Regexp {
	annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternLogRegexFor)
	if !found {
		return nil
	}

	regexByContainer := map[string]*regexp.Regexp{}
	for key, value := range annotations {
		keyMatches := common.AnnotationKeyPatternLogRegexFor.FindStringSubmatch(key)
		containerSubexpIndex := common.AnnotationKeyPatternLogRegexFor.SubexpIndex("container")
		container := keyMatches[containerSubexpIndex]
		regexByContainer[container] = regexp.MustCompile(value)
	}

	return regexByContainer
}

func skipLogRegexesForContainers(meta *spec.ResourceMeta) map[string]*regexp.Regexp {
	annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternSkipLogRegexFor)
	if !found {
		return nil
	}

	regexByContainer := map[string]*regexp.Regexp{}
	for key, value := range annotations {
		keyMatches := common.AnnotationKeyPatternSkipLogRegexFor.FindStringSubmatch(key)
		containerSubexpIndex := common.AnnotationKeyPatternSkipLogRegexFor.SubexpIndex("container")
		container := keyMatches[containerSubexpIndex]
		regexByContainer[container] = regexp.MustCompile(value)
	}

	return regexByContainer
}

func noActivityTimeout(meta *spec.ResourceMeta) time.Duration {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternNoActivityTimeout)
	if !found {
		return 4 * time.Minute
	}

	return lo.Must(time.ParseDuration(value))
}

func showLogsOnlyForContainers(meta *spec.ResourceMeta) []string {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternShowLogsOnlyForContainers)
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

func showServiceMessages(meta *spec.ResourceMeta) bool {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternShowServiceMessages)
	if !found {
		return false
	}

	return lo.Must(strconv.ParseBool(value))
}

func showLogsOnlyForNumberOfReplicas(meta *spec.ResourceMeta) int {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternShowLogsOnlyForNumberOfReplicas)
	if !found {
		return 1
	}

	return lo.Must(strconv.Atoi(value))
}

func skipLogs(meta *spec.ResourceMeta) bool {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternSkipLogs)
	if !found {
		return false
	}

	return lo.Must(strconv.ParseBool(value))
}

func skipLogsForContainers(meta *spec.ResourceMeta) []string {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternSkipLogsForContainers)
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

func trackTerminationMode(meta *spec.ResourceMeta) multitrack.TrackTerminationMode {
	_, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternTrackTerminationMode)
	if !found {
		return multitrack.WaitUntilResourceReady
	}

	return multitrack.TrackTerminationMode(value)
}

func deleteOnSucceeded(meta *spec.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicySucceeded)
}

func deleteOnFailed(meta *spec.ResourceMeta) bool {
	deletePolicies := deletePolicies(meta)
	return lo.Contains(deletePolicies, common.DeletePolicyFailed)
}

func deployConditions(meta *spec.ResourceMeta, hasManualInternalDeps bool) map[common.On][]common.Stage {
	if conditions := deployConditionsForAnnotation(meta, common.AnnotationKeyPatternDeployOn); len(conditions) > 0 {
		if spec.IsCRD(meta.GroupVersionKind.GroupKind()) {
			for on := range conditions {
				conditions[on] = []common.Stage{common.StagePrePreInstall}
			}
		} else if spec.IsWebhook(meta.GroupVersionKind.GroupKind()) && !hasManualInternalDeps {
			for on := range conditions {
				conditions[on] = []common.Stage{common.StagePostPostInstall}
			}
		}

		return conditions
	}

	if spec.IsHook(meta.Annotations) {
		if conditions := deployConditionsForAnnotation(meta, common.AnnotationKeyPatternHook); len(conditions) > 0 {
			if spec.IsCRD(meta.GroupVersionKind.GroupKind()) {
				for on := range conditions {
					conditions[on] = []common.Stage{common.StagePrePreInstall}
				}
			} else if spec.IsWebhook(meta.GroupVersionKind.GroupKind()) && !hasManualInternalDeps {
				for on := range conditions {
					conditions[on] = []common.Stage{common.StagePostPostInstall}
				}
			}

			return conditions
		}
	}

	if spec.IsCRD(meta.GroupVersionKind.GroupKind()) {
		return map[common.On][]common.Stage{
			common.InstallOnInstall:  {common.StagePrePreInstall},
			common.InstallOnUpgrade:  {common.StagePrePreInstall},
			common.InstallOnRollback: {common.StagePrePreInstall},
		}
	} else if spec.IsWebhook(meta.GroupVersionKind.GroupKind()) && !hasManualInternalDeps {
		return map[common.On][]common.Stage{
			common.InstallOnInstall:  {common.StagePostPostInstall},
			common.InstallOnUpgrade:  {common.StagePostPostInstall},
			common.InstallOnRollback: {common.StagePostPostInstall},
		}
	}

	return map[common.On][]common.Stage{
		common.InstallOnInstall:  {common.StageInstall},
		common.InstallOnUpgrade:  {common.StageInstall},
		common.InstallOnRollback: {common.StageInstall},
	}
}

func deployConditionsForAnnotation(meta *spec.ResourceMeta, annoPattern *regexp.Regexp) map[common.On][]common.Stage {
	key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, annoPattern)
	if !found {
		return nil
	}

	valConditions := lo.Map(strings.Split(value, ","), func(p string, _ int) string {
		return strings.TrimSpace(p)
	})

	result := map[common.On][]common.Stage{}
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
		case string(helmrelease.HookInstall):
			result[common.InstallOnInstall] = append(result[common.InstallOnInstall], common.StageInstall)
		case string(helmrelease.HookUpgrade):
			result[common.InstallOnUpgrade] = append(result[common.InstallOnUpgrade], common.StageInstall)
		case string(helmrelease.HookRollback):
			result[common.InstallOnRollback] = append(result[common.InstallOnRollback], common.StageInstall)
		case string(helmrelease.HookDelete):
			result[common.InstallOnDelete] = append(result[common.InstallOnDelete], common.StageInstall)
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

func ownership(meta *spec.ResourceMeta, releaseNamespace string, storeAs common.StoreAs) common.Ownership {
	if spec.IsReleaseNamespace(meta.Name, meta.GroupVersionKind, releaseNamespace) ||
		storeAs == common.StoreAsNone {
		return common.OwnershipAnyone
	}

	if _, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternOwnership); found {
		return common.Ownership(value)
	}

	if spec.IsHook(meta.Annotations) {
		return common.OwnershipAnyone
	}

	switch storeAs {
	case common.StoreAsHook:
		return common.OwnershipAnyone
	case common.StoreAsRegular:
		return common.OwnershipRelease
	default:
		panic("unexpected storeAs value")
	}
}

func deletePropagation(meta *spec.ResourceMeta, defaultDeletePropagation apiv1.DeletionPropagation) apiv1.DeletionPropagation {
	if _, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDeletePropagation); found {
		return apiv1.DeletionPropagation(value)
	}

	if defaultDeletePropagation != "" {
		return defaultDeletePropagation
	}

	return common.DefaultDeletePropagation
}

func weight(meta *spec.ResourceMeta, hasManualInternalDeps bool) *int {
	if hasManualInternalDeps {
		return nil
	}

	if spec.IsCRD(meta.GroupVersionKind.GroupKind()) {
		return lo.ToPtr(0)
	}

	var weightValue string
	if spec.IsHook(meta.Annotations) {
		_, hookWeightValue, hookWeightFound := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternHookWeight)

		_, generalWeightValue, weightFound := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternWeight)

		if !hookWeightFound && !weightFound {
			return lo.ToPtr(0)
		} else if weightFound {
			weightValue = generalWeightValue
		} else {
			weightValue = hookWeightValue
		}
	} else {
		var found bool

		_, weightValue, found = spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternWeight)
		if !found {
			return lo.ToPtr(0)
		}
	}

	return lo.ToPtr(lo.Must(strconv.Atoi(weightValue)))
}

func deletePolicies(meta *spec.ResourceMeta) []common.DeletePolicy {
	var deletePolicies []common.DeletePolicy
	if spec.IsHook(meta.Annotations) {
		_, hookDeletePolicies, hookDeletePoliciesFound := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternHookDeletePolicy)

		_, generalDeletePolicies, generalDeletePoliciesFound := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDeletePolicy)

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
		if _, generalDeletePolicies, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDeletePolicy); found {
			for _, deletePolicy := range strings.Split(generalDeletePolicies, ",") {
				deletePolicy = strings.TrimSpace(deletePolicy)
				deletePolicies = append(deletePolicies, common.DeletePolicy(deletePolicy))
			}
		}

		if len(deletePolicies) == 0 {
			if meta.GroupVersionKind.GroupKind() == (schema.GroupKind{Kind: "Job", Group: "batch"}) {
				deletePolicies = append(deletePolicies, common.DeletePolicyBeforeCreationIfImmutable)
			}
		}
	}

	return deletePolicies
}

func manualInternalDeployDependencies(meta *spec.ResourceMeta) []*InternalDependency {
	if spec.IsCRD(meta.GroupVersionKind.GroupKind()) {
		return nil
	}

	deps := map[string]*InternalDependency{}

	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDependency); found {
		for key, value := range annotations {
			matches := common.AnnotationKeyPatternDependency.FindStringSubmatch(key)
			idSubexpIndex := common.AnnotationKeyPatternDependency.SubexpIndex("id")
			depID := matches[idSubexpIndex]
			valParts := strings.Split(value, ":")
			depAPIVersionParts := strings.SplitN(valParts[0], "/", 2)

			var gvk schema.GroupVersionKind
			if len(depAPIVersionParts) == 1 {
				gvk = schema.GroupVersionKind{
					Version: depAPIVersionParts[0],
					Kind:    valParts[1],
				}
			} else {
				gvk = schema.GroupVersionKind{
					Group:   depAPIVersionParts[0],
					Version: depAPIVersionParts[1],
					Kind:    valParts[1],
				}
			}

			var depNamespace string
			if len(valParts) == 4 {
				depNamespace = valParts[2]
			}

			depName := valParts[len(valParts)-1]

			dep := &InternalDependency{
				ResourceMatcher: &spec.ResourceMatcher{
					Names:      []string{depName},
					Namespaces: []string{depNamespace},
					Groups:     []string{gvk.Group},
					Versions:   []string{gvk.Version},
					Kinds:      []string{gvk.Kind},
				},
				ResourceState: common.ResourceStatePresent,
			}
			deps[depID] = dep
		}
	}

	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDeployDependency); found {
		for key, value := range annotations {
			matches := common.AnnotationKeyPatternDeployDependency.FindStringSubmatch(key)
			idSubexpIndex := common.AnnotationKeyPatternDeployDependency.SubexpIndex("id")
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

			dep := &InternalDependency{
				ResourceMatcher: &spec.ResourceMatcher{
					Names:      depNames,
					Namespaces: depNamespaces,
					Groups:     depGroups,
					Versions:   depVersions,
					Kinds:      depKinds,
				},
				ResourceState: depState,
			}
			deps[depID] = dep
		}
	}

	return lo.Values(deps)
}

func manualInternalDeleteDependencies(meta *spec.ResourceMeta) []*InternalDependency {
	if spec.IsCRD(meta.GroupVersionKind.GroupKind()) {
		// TODO: Should we remove it?
		return nil
	}

	deps := map[string]*InternalDependency{}

	// TODO: Maybe it is better to move to new func
	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(meta.Annotations, common.AnnotationKeyPatternDeleteDependency); found {
		for key, value := range annotations {
			matches := common.AnnotationKeyPatternDeleteDependency.FindStringSubmatch(key)
			idSubexpIndex := common.AnnotationKeyPatternDeleteDependency.SubexpIndex("id")
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

			dep := &InternalDependency{
				ResourceMatcher: &spec.ResourceMatcher{
					Names:      depNames,
					Namespaces: depNamespaces,
					Groups:     depGroups,
					Versions:   depVersions,
					Kinds:      depKinds,
				},
				ResourceState: depState,
			}
			deps[depID] = dep
		}
	}

	return lo.Values(deps)
}

func externalDependencies(meta *spec.ResourceMeta, releaseNamespace string, clientFactory kube.ClientFactorier, remote bool) ([]*ExternalDependency, error) {
	if spec.IsCRD(meta.GroupVersionKind.GroupKind()) {
		return nil, nil
	}

	deps := externalDeps(meta, releaseNamespace)

	legacyExtDeps := map[string]*ExternalDependency{}
	// Pretend that we don't have any external dependencies when we don't have cluster access, since we need cluster access to map GVR to GVK.
	if remote {
		var err error

		legacyExtDeps, err = legacyExternalDeps(meta, releaseNamespace, clientFactory.Mapper())
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

func externalDeps(resMeta *spec.ResourceMeta, releaseNamespace string) map[string]*ExternalDependency {
	deps := map[string]*ExternalDependency{}
	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(resMeta.Annotations, common.AnnotationKeyPatternExternalDependency); found {
		for key, value := range annotations {
			matches := common.AnnotationKeyPatternExternalDependency.FindStringSubmatch(key)
			idSubexpIndex := common.AnnotationKeyPatternExternalDependency.SubexpIndex("id")
			depID := matches[idSubexpIndex]
			valParts := strings.Split(value, ":")
			depAPIVersionParts := strings.SplitN(valParts[0], "/", 2)

			var gvk schema.GroupVersionKind
			if len(depAPIVersionParts) == 1 {
				gvk = schema.GroupVersionKind{
					Version: depAPIVersionParts[0],
					Kind:    valParts[1],
				}
			} else {
				gvk = schema.GroupVersionKind{
					Group:   depAPIVersionParts[0],
					Version: depAPIVersionParts[1],
					Kind:    valParts[1],
				}
			}

			var depNamespace string
			if len(valParts) == 4 {
				depNamespace = valParts[2]
			}

			depName := valParts[len(valParts)-1]

			resMeta := spec.NewResourceMeta(depName, depNamespace, releaseNamespace, "", gvk, nil, nil)
			dep := &ExternalDependency{
				ResourceMeta: resMeta,
			}

			deps[depID] = dep
		}
	}

	return deps
}

// TODO(v2): get rid of legacy external deps
func legacyExternalDeps(resMeta *spec.ResourceMeta, releaseNamespace string, mapper apimeta.ResettableRESTMapper) (map[string]*ExternalDependency, error) {
	deps := map[string]*ExternalDependency{}

	type DepInfo struct {
		Name      string
		Namespace string
		Type      string
	}

	extDepInfos := map[string]*DepInfo{}

	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(resMeta.Annotations, common.AnnotationKeyPatternLegacyExternalDependencyResource); found {
		for key, value := range annotations {
			matches := common.AnnotationKeyPatternLegacyExternalDependencyResource.FindStringSubmatch(key)
			idSubexpIndex := common.AnnotationKeyPatternLegacyExternalDependencyResource.SubexpIndex("id")
			extDepID := matches[idSubexpIndex]
			extDepType := strings.Split(value, "/")[0]
			extDepName := strings.Split(value, "/")[1]

			extDepInfos[extDepID] = &DepInfo{
				Name: extDepName,
				Type: extDepType,
			}
		}
	}

	if annotations, found := spec.FindAnnotationsOrLabelsByKeyPattern(resMeta.Annotations, common.AnnotationKeyPatternLegacyExternalDependencyNamespace); found {
		for key, value := range annotations {
			matches := common.AnnotationKeyPatternLegacyExternalDependencyNamespace.FindStringSubmatch(key)
			idSubexpIndex := common.AnnotationKeyPatternLegacyExternalDependencyNamespace.SubexpIndex("id")
			extDepID := matches[idSubexpIndex]
			extDepNamespace := value

			if extDepInfo, hasKey := extDepInfos[extDepID]; hasKey {
				extDepInfo.Namespace = extDepNamespace
			}
		}
	}

	for extDepID, extDepInfo := range extDepInfos {
		gvk, err := spec.ParseKubectlResourceStringtoGVK(extDepInfo.Type, mapper)
		if err != nil {
			return nil, fmt.Errorf("parse external dependency resource type %q for dependency %q (namespace: %q): %w", extDepInfo.Type, extDepInfo.Name, extDepInfo.Namespace, err)
		}

		resMeta := spec.NewResourceMeta(extDepInfo.Name, extDepInfo.Namespace, releaseNamespace, "", gvk, nil, nil)
		dep := &ExternalDependency{
			ResourceMeta: resMeta,
		}

		deps[extDepID] = dep
	}

	return deps, nil
}
