package resourceinfo

import (
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/util"
)

func ValidateLocal(releaseNamespace string, transformedResources []*resource.InstallableResource) error {
	if err := validateNoDuplicates(releaseNamespace, transformedResources); err != nil {
		return fmt.Errorf("validate for no duplicated resources: %w", err)
	}

	return nil
}

func ValidateRemote(releaseName, releaseNamespace string, installableResourceInfos []*InstallableResourceInfo, forceAdoption bool) error {
	if !forceAdoption {
		if err := validateAdoptableResources(releaseName, releaseNamespace, installableResourceInfos); err != nil {
			return fmt.Errorf("validate adoptable resources: %w", err)
		}
	}

	return nil
}

func validateNoDuplicates(releaseNamespace string, transformedResources []*resource.InstallableResource) error {
	for _, res := range transformedResources {
		if resource.IsReleaseNamespace(res.Unstruct.GetName(), res.Unstruct.GroupVersionKind(), releaseNamespace) {
			return fmt.Errorf("release namespace %q cannot be deployed as part of the release", res.Unstruct.GetName())
		}
	}

	duplicates := lo.FindDuplicatesBy(transformedResources, func(instRes *resource.InstallableResource) string {
		return instRes.ID()
	})

	duplicatedIDHumans := lo.Map(duplicates, func(instRes *resource.InstallableResource, _ int) string {
		return instRes.IDHuman()
	})

	return fmt.Errorf("duplicated resources found: %s", strings.Join(duplicatedIDHumans, ", "))
}

func validateAdoptableResources(releaseName, releaseNamespace string, resourceInfos []*InstallableResourceInfo) error {
	var errs []error
	for _, info := range resourceInfos {
		if info.GetResult == nil {
			continue
		}

		if adoptable, nonAdoptableReason := resource.AdoptableBy(info.LocalResource.ResourceMeta, releaseName, releaseNamespace); !adoptable {
			errs = append(errs, fmt.Errorf("resource %q is not adoptable: %s", info.IDHuman(), nonAdoptableReason))
		}
	}

	return util.Multierrorf("adoption validation failed", errs)
}
