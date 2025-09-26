package resource

import (
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/werf/nelm/internal/resource/spec"
)

func ValidateLocal(releaseNamespace string, transformedResources []*InstallableResource) error {
	if err := validateNoDuplicates(releaseNamespace, transformedResources); err != nil {
		return fmt.Errorf("validate for no duplicated resources: %w", err)
	}

	return nil
}

func validateNoDuplicates(releaseNamespace string, transformedResources []*InstallableResource) error {
	for _, res := range transformedResources {
		if spec.IsReleaseNamespace(res.Unstruct.GetName(), res.Unstruct.GroupVersionKind(), releaseNamespace) {
			return fmt.Errorf("release namespace %q cannot be deployed as part of the release", res.Unstruct.GetName())
		}
	}

	duplicates := lo.FindDuplicatesBy(transformedResources, func(instRes *InstallableResource) string {
		return instRes.ID()
	})

	if len(duplicates) == 0 {
		return nil
	}

	duplicatedIDHumans := lo.Map(duplicates, func(instRes *InstallableResource, _ int) string {
		return instRes.IDHuman()
	})

	return fmt.Errorf("duplicated resources found: %s", strings.Join(duplicatedIDHumans, ", "))
}
