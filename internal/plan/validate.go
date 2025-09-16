package plan

import (
	"fmt"

	"github.com/werf/nelm/internal/resource"
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

		if adoptable, nonAdoptableReason := resource.AdoptableBy(info.LocalResource.ResourceMeta, releaseName, releaseNamespace); !adoptable {
			errs = append(errs, fmt.Errorf("resource %q is not adoptable: %s", info.IDHuman(), nonAdoptableReason))
		}
	}

	return util.Multierrorf("adoption validation failed", errs)
}
