package resourceinfo

import (
	"context"
	"fmt"

	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
)

func BuildTransformedResourceSpecs(ctx context.Context, releaseNamespace string, resources []*id.ResourceSpec, transformers []resource.ResourceTransformer) ([]*id.ResourceSpec, error) {
	var transformedResources []*id.ResourceSpec
	for _, transformer := range transformers {
		for _, res := range resources {
			if matched, err := transformer.Match(ctx, &resource.ResourceTransformerResourceInfo{
				Obj: res.Unstruct,
			}); err != nil {
				return nil, fmt.Errorf("match resource by %q: %w", transformer.Type(), err)
			} else if !matched {
				transformedResources = append(transformedResources, res)
				continue
			}

			newObjs, err := transformer.Transform(ctx, &resource.ResourceTransformerResourceInfo{
				Obj: res.Unstruct,
			})
			if err != nil {
				return nil, fmt.Errorf("transform resource by %q: %w", transformer.Type(), err)
			}

			for _, newObj := range newObjs {
				newRes := id.NewResourceSpec(newObj, releaseNamespace, id.ResourceSpecOptions{
					StoreAs:  res.StoreAs,
					FilePath: res.FilePath,
				})

				transformedResources = append(transformedResources, newRes)
			}
		}
	}

	return transformedResources, nil
}
