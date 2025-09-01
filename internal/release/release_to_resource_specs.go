package release

import (
	"fmt"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/nelm/internal/resource/id"
)

func ReleaseToResourceSpecs(rel *helmrelease.Release, releaseNamespace string) ([]*id.ResourceSpec, error) {
	var resources []*id.ResourceSpec
	for _, manifest := range releaseutil.SplitManifests(rel.UnstoredManifest) {
		if res, err := id.NewResourceSpecFromManifest(manifest, releaseNamespace, id.ResourceSpecOptions{
			StoreAs: id.StoreAsNone,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from unstored manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	for _, manifest := range releaseutil.SplitManifests(rel.Manifest) {
		if res, err := id.NewResourceSpecFromManifest(manifest, releaseNamespace, id.ResourceSpecOptions{
			StoreAs: id.StoreAsRegular,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from regular manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	for _, hook := range rel.Hooks {
		if res, err := id.NewResourceSpecFromManifest(hook.Manifest, releaseNamespace, id.ResourceSpecOptions{
			StoreAs: id.StoreAsHook,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from hook manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	return resources, nil
}
