package plan

import (
	"fmt"
	"strings"

	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/internal/util"
	"github.com/werf/nelm/pkg/common"
)

func ValidateRemote(releaseName, releaseNamespace string, installableResourceInfos []*InstallableResourceInfo, forceAdoption bool) error {
	if !forceAdoption {
		if err := validateAdoptableResources(releaseName, releaseNamespace, installableResourceInfos); err != nil {
			return fmt.Errorf("validate adoptable resources: %w", err)
		}
	}

	return nil
}

func validateAdoptableResources(releaseName, releaseNamespace string, resourceInfos []*InstallableResourceInfo) error {
	var errs []error
	for _, info := range resourceInfos {
		if info.GetResult == nil {
			continue
		}

		if info.LocalResource.Ownership == common.OwnershipAnyone {
			continue
		}

		if adoptable, nonAdoptableReason := adoptableBy(info.LocalResource.ResourceMeta, releaseName, releaseNamespace); !adoptable {
			errs = append(errs, fmt.Errorf("resource %q is not adoptable: %s", info.IDHuman(), nonAdoptableReason))
		}
	}

	return util.Multierrorf("adoption validation failed", errs)
}

func adoptableBy(meta *spec.ResourceMeta, releaseName, releaseNamespace string) (adoptable bool, nonAdoptableReason string) {
	nonAdoptableReasons := []string{}

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternReleaseName); found {
		if value != releaseName {
			nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseName))
		}
	} else {
		nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, common.AnnotationKeyHumanReleaseName, releaseName))
	}

	if key, value, found := spec.FindAnnotationOrLabelByKeyPattern(meta.Annotations, common.AnnotationKeyPatternReleaseNamespace); found {
		if value != releaseNamespace {
			nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseNamespace))
		}
	} else {
		nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, common.AnnotationKeyHumanReleaseNamespace, releaseNamespace))
	}

	nonAdoptableReason = strings.Join(nonAdoptableReasons, ", ")

	return len(nonAdoptableReasons) == 0, nonAdoptableReason
}
