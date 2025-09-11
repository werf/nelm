package resource

import (
	"context"
	"fmt"
)

func BuildTransformedResourceSpecs(ctx context.Context, releaseNamespace string, resources []*ResourceSpec, transformers []ResourceTransformer) ([]*ResourceSpec, error) {
	var transformedResources []*ResourceSpec
	for _, transformer := range transformers {
		for _, res := range resources {
			if matched, err := transformer.Match(ctx, &ResourceTransformerResourceInfo{
				Obj: res.Unstruct,
			}); err != nil {
				return nil, fmt.Errorf("match resource by %q: %w", transformer.Type(), err)
			} else if !matched {
				transformedResources = append(transformedResources, res)
				continue
			}

			newObjs, err := transformer.Transform(ctx, &ResourceTransformerResourceInfo{
				Obj: res.Unstruct,
			})
			if err != nil {
				return nil, fmt.Errorf("transform resource by %q: %w", transformer.Type(), err)
			}

			for _, newObj := range newObjs {
				newRes := NewResourceSpec(newObj, releaseNamespace, ResourceSpecOptions{
					StoreAs:  res.StoreAs,
					FilePath: res.FilePath,
				})

				transformedResources = append(transformedResources, newRes)
			}
		}
	}

	return transformedResources, nil
}
