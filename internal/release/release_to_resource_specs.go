package release

import (
	"fmt"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/nelm/internal/resource"
)

func ReleaseToResourceSpecs(rel *helmrelease.Release, releaseNamespace string) ([]*resource.ResourceSpec, error) {
	var resources []*resource.ResourceSpec
	for _, manifest := range releaseutil.SplitManifests(rel.UnstoredManifest) {
		if res, err := resource.NewResourceSpecFromManifest(manifest, releaseNamespace, resource.ResourceSpecOptions{
			StoreAs: resource.StoreAsNone,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from unstored manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	for _, manifest := range releaseutil.SplitManifests(rel.Manifest) {
		if res, err := resource.NewResourceSpecFromManifest(manifest, releaseNamespace, resource.ResourceSpecOptions{
			StoreAs: resource.StoreAsRegular,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from regular manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	for _, hook := range rel.Hooks {
		if res, err := resource.NewResourceSpecFromManifest(hook.Manifest, releaseNamespace, resource.ResourceSpecOptions{
			StoreAs: resource.StoreAsHook,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from hook manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	return resources, nil
}
