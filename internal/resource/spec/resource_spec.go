package spec

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/featgate"
)

type ResourceSpec struct {
	*ResourceMeta

	Unstruct *unstructured.Unstructured
	StoreAs  common.StoreAs
}

type ResourceSpecOptions struct {
	FilePath                string
	LegacyNoCleanNullFields bool // TODO(v2): always clean
	StoreAs                 common.StoreAs
}

func NewResourceSpec(unstruct *unstructured.Unstructured, releaseNamespace string, opts ResourceSpecOptions) *ResourceSpec {
	unstruct = CleanUnstruct(unstruct, CleanUnstructOptions{
		CleanNullFields: (featgate.FeatGatePreviewV2.Enabled() || featgate.FeatCleanNullFields.Enabled()) && !opts.LegacyNoCleanNullFields,
	})

	if opts.StoreAs == "" {
		if IsHook(unstruct.GetAnnotations()) {
			opts.StoreAs = common.StoreAsHook
		} else {
			opts.StoreAs = common.StoreAsRegular
		}
	}

	if releaseNamespace == unstruct.GetNamespace() {
		unstruct.SetNamespace("")
	}

	return &ResourceSpec{
		ResourceMeta: NewResourceMetaFromUnstructured(unstruct, releaseNamespace, opts.FilePath),
		Unstruct:     unstruct,
		StoreAs:      opts.StoreAs,
	}
}

func NewResourceSpecFromManifest(manifest, releaseNamespace string, opts ResourceSpecOptions) (*ResourceSpec, error) {
	if opts.FilePath == "" && strings.HasPrefix(manifest, "# Source: ") {
		firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
		opts.FilePath = strings.TrimPrefix(firstLine, "# Source: ")
	}

	obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
	if err != nil {
		return nil, fmt.Errorf("decode resource (file: %q): %w", opts.FilePath, err)
	}

	return NewResourceSpec(obj.(*unstructured.Unstructured), releaseNamespace, opts), nil
}

func (s *ResourceSpec) SetAnnotations(annotations map[string]string) {
	s.Unstruct.SetAnnotations(annotations)
	s.Annotations = annotations
}

func (s *ResourceSpec) SetLabels(labels map[string]string) {
	s.Unstruct.SetLabels(labels)
	s.Labels = labels
}

func BuildTransformedResourceSpecs(ctx context.Context, releaseNamespace string, resources []*ResourceSpec, transformers []ResourceTransformer) ([]*ResourceSpec, error) {
	transformedResources := resources
	for _, transformer := range transformers {
		var transfResources []*ResourceSpec
		for _, res := range transformedResources {
			if matched, err := transformer.Match(ctx, &ResourceTransformerResourceInfo{
				Obj: res.Unstruct,
			}); err != nil {
				return nil, fmt.Errorf("match resource by %q: %w", transformer.Type(), err)
			} else if !matched {
				transfResources = append(transfResources, res)
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

				transfResources = append(transfResources, newRes)
			}
		}

		transformedResources = transfResources
	}

	return transformedResources, nil
}

func BuildReleasableResourceSpecs(ctx context.Context, releaseNamespace string, transformedResources []*ResourceSpec, patchers []ResourcePatcher) ([]*ResourceSpec, error) {
	var releasableResources []*ResourceSpec

	for _, res := range transformedResources {
		releasableRes := res

		var deepCopied bool
		for _, resPatcher := range patchers {
			if matched, err := resPatcher.Match(ctx, &ResourcePatcherResourceInfo{
				Obj: releasableRes.Unstruct,
				// TODO(ilya-lesikov): get rid of ownership for releasable resources
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

			patchedObj, err := resPatcher.Patch(ctx, &ResourcePatcherResourceInfo{
				Obj: unstruct,
				// TODO(ilya-lesikov): get rid of ownership for releasable resources
				Ownership: "",
			})
			if err != nil {
				return nil, fmt.Errorf("patch resource by %q: %w", resPatcher.Type(), err)
			}

			releasableRes = NewResourceSpec(patchedObj, releaseNamespace, ResourceSpecOptions{
				StoreAs:  res.StoreAs,
				FilePath: res.FilePath,
			})
		}

		releasableResources = append(releasableResources, releasableRes)
	}

	sort.SliceStable(releasableResources, func(i, j int) bool {
		return ResourceSpecSortHandler(releasableResources[i], releasableResources[j])
	})

	return releasableResources, nil
}
