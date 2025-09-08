package resourceinfo

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
)

func BuildReleasableResourceSpecs(ctx context.Context, releaseNamespace string, transformedResources []*id.ResourceSpec, patchers []resource.ResourcePatcher) ([]*id.ResourceSpec, error) {
	var releasableResources []*id.ResourceSpec

	for _, res := range transformedResources {
		releasableRes := res

		var deepCopied bool
		for _, resPatcher := range patchers {
			if matched, err := resPatcher.Match(ctx, &resource.ResourcePatcherResourceInfo{
				Obj: releasableRes.Unstruct,
				// FIXME(ilya-lesikov): get rid of ownership for releasable resources
				Ownership: "",
			}); err != nil {
				return nil, fmt.Errorf("match resource for patching by %q: %w", resPatcher.Type(), err)
			} else if !matched {
				continue
			}

			var unstruct *unstructured.Unstructured
			if deepCopied {
				unstruct = releasableRes.Unstruct
			} else {
				unstruct = releasableRes.Unstruct.DeepCopy()
				deepCopied = true
			}

			patchedObj, err := resPatcher.Patch(ctx, &resource.ResourcePatcherResourceInfo{
				Obj: unstruct,
				// FIXME(ilya-lesikov): get rid of ownership for releasable resources
				Ownership: "",
			})
			if err != nil {
				return nil, fmt.Errorf("patch resource by %q: %w", resPatcher.Type(), err)
			}

			releasableRes = id.NewResourceSpec(patchedObj, releaseNamespace, id.ResourceSpecOptions{
				StoreAs:  res.StoreAs,
				FilePath: res.FilePath,
			})
		}

		releasableResources = append(releasableResources, releasableRes)
	}

	sort.SliceStable(releasableResources, func(i, j int) bool {
		return id.ResourceSpecSortHandler(releasableResources[i], releasableResources[j])
	})

	return releasableResources, nil
}
