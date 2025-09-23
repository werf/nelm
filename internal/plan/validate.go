package plan

import (
	"fmt"
	"strings"

	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/meta"
	"github.com/werf/nelm/internal/util"
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

		if info.LocalResource.Ownership == common.OwnershipEveryone {
			continue
		}

		if adoptable, nonAdoptableReason := adoptableBy(info.LocalResource.ResourceMeta, releaseName, releaseNamespace); !adoptable {
			errs = append(errs, fmt.Errorf("resource %q is not adoptable: %s", info.IDHuman(), nonAdoptableReason))
		}
	}

	return util.Multierrorf("adoption validation failed", errs)
}

func adoptableBy(meta *meta.ResourceMeta, releaseName, releaseNamespace string) (adoptable bool, nonAdoptableReason string) {
	nonAdoptableReasons := []string{}

	if key, value, found := resource.FindAnnotationOrLabelByKeyPattern(meta.Annotations, resource.AnnotationKeyPatternReleaseName); found {
		if value != releaseName {
			nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseName))
		}
	} else {
		nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, resource.AnnotationKeyHumanReleaseName, releaseName))
	}

	if key, value, found := resource.FindAnnotationOrLabelByKeyPattern(meta.Annotations, resource.AnnotationKeyPatternReleaseNamespace); found {
		if value != releaseNamespace {
			nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation "%s=%s" must have value %q`, key, value, releaseNamespace))
		}
	} else {
		nonAdoptableReasons = append(nonAdoptableReasons, fmt.Sprintf(`annotation %q not found, must be set to %q`, resource.AnnotationKeyHumanReleaseNamespace, releaseNamespace))
	}

	nonAdoptableReason = strings.Join(nonAdoptableReasons, ", ")

	return len(nonAdoptableReasons) == 0, nonAdoptableReason
}
