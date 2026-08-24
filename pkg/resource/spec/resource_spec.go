package spec

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/werf/nelm/pkg/common"
)

// Contains all generic information about the resource, e.g. its name, namespace, GVK and its spec.
// Enough to create or update resources, as well as delete, get, etc. Use it when ResourceMeta is
// not enough and you also need the actual resource spec.
type ResourceSpec struct {
	*ResourceMeta `json:"resourceMeta"`

	Unstruct *unstructured.Unstructured `json:"unstruct"`
	StoreAs  common.StoreAs             `json:"storeAs"`
}

func NewResourceSpec(unstruct *unstructured.Unstructured, releaseNamespace string, opts ResourceSpecOptions) *ResourceSpec {
	unstruct = unstruct.DeepCopy()
	unstruct.Object = cleanNulls(unstruct.Object).(map[string]interface{})

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
		StoreAs:      opts.StoreAs,
		Unstruct:     unstruct,
	}
}

func NewResourceSpecFromManifest(ctx context.Context, manifest, releaseNamespace string, opts ResourceSpecOptions) (*ResourceSpec, error) {
	if opts.FilePath == "" && strings.HasPrefix(manifest, "# Source: ") {
		firstLine := strings.TrimSpace(strings.Split(manifest, "\n")[0])
		opts.FilePath = strings.TrimPrefix(firstLine, "# Source: ")
	}

	obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
	if err != nil {
		return nil, fmt.Errorf("decode resource (file: %q): %w", opts.FilePath, err)
	}

	unstruct := obj.(*unstructured.Unstructured)

	if opts.DropInvalidAnnotationsAndLabels {
		unstruct.SetAnnotations(stripInvalidEntries(ctx, opts.FilePath, unstruct.Object, "metadata", "annotations"))
		unstruct.SetLabels(stripInvalidEntries(ctx, opts.FilePath, unstruct.Object, "metadata", "labels"))
	} else if err := validateMetadataStringMaps(unstruct); err != nil {
		return nil, fmt.Errorf("decode resource (file: %q): %w", opts.FilePath, err)
	}

	return NewResourceSpec(unstruct, releaseNamespace, opts), nil
}

func (s *ResourceSpec) SetAnnotations(annotations map[string]string) {
	s.Unstruct.SetAnnotations(annotations)
	s.Annotations = annotations
}

func (s *ResourceSpec) SetLabels(labels map[string]string) {
	s.Unstruct.SetLabels(labels)
	s.Labels = labels
}

type ResourceSpecOptions struct {
	DropInvalidAnnotationsAndLabels bool
	FilePath                        string
	StoreAs                         common.StoreAs
}

// Patch ResourceSpecs to make them releasable, after which they can be saved into the Helm release.
// Don't try to add/delete/expand specs here, use transformers in BuildTransformedResourceSpecs
// instead.
func BuildPatchedResourceSpecs(ctx context.Context, releaseNamespace string, transformedResources []*ResourceSpec, patchers []ResourcePatcher) ([]*ResourceSpec, error) {
	var releasableResources []*ResourceSpec

	for _, res := range transformedResources {
		releasableRes := res

		var deepCopied bool
		for _, resPatcher := range patchers {
			if matched, err := resPatcher.Match(ctx, &ResourcePatcherResourceInfo{
				Obj: releasableRes.Unstruct,
				// TODO: get rid of ownership for releasable resources
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
				// TODO: get rid of ownership for releasable resources
				Ownership: "",
			})
			if err != nil {
				return nil, fmt.Errorf("patch resource by %q: %w", resPatcher.Type(), err)
			}

			releasableRes = NewResourceSpec(patchedObj, releaseNamespace, ResourceSpecOptions{
				FilePath: res.FilePath,
				StoreAs:  res.StoreAs,
			})
		}

		releasableResources = append(releasableResources, releasableRes)
	}

	sort.SliceStable(releasableResources, func(i, j int) bool {
		return ResourceSpecSortHandler(releasableResources[i], releasableResources[j])
	})

	return releasableResources, nil
}

// Patch ResourceSpecs with render patches, i.e. right after the chart is rendered. Unlike diff
// patches, the result is what gets released and applied to the cluster, so a patch must not change
// the resource identity. StoreAs is re-derived, because a patch can add or remove the Helm hook
// annotation, except for StoreAsNone, which is not releasable at all and must survive patching.
func BuildRenderPatchedResourceSpecs(ctx context.Context, releaseNamespace string, resources []*ResourceSpec, patches []*CompiledPatch) ([]*ResourceSpec, error) {
	if len(patches) == 0 {
		return resources, nil
	}

	patchedResources := make([]*ResourceSpec, 0, len(resources))

	for _, res := range resources {
		patchedUnstruct := res.Unstruct
		patchedMeta := res.ResourceMeta

		for i, patch := range patches {
			// There is no live object at render time to take the true namespace from, and
			// namespaced resources without an explicit namespace end up in the release namespace,
			// so cluster-scoped resources are indistinguishable from them here.
			namespace := lo.Ternary(patchedUnstruct.GetNamespace() == "", releaseNamespace, patchedUnstruct.GetNamespace())

			if !patch.Match(patchedMeta, namespace) {
				continue
			}

			out, err := patch.transform(ctx, patchedUnstruct)
			if err != nil {
				return nil, fmt.Errorf("apply render patches to resource %q: patch #%d: %w", res.IDHuman(), i+1, err)
			}

			if err := validateMetadataStringMaps(out); err != nil {
				return nil, fmt.Errorf("apply render patches to resource %q: patch #%d: %w", res.IDHuman(), i+1, err)
			}

			patchedUnstruct = out
			patchedMeta = NewResourceMetaFromUnstructured(patchedUnstruct, releaseNamespace, res.FilePath)
		}

		if err := validateSameResourceIdentity(res.Unstruct, patchedUnstruct, releaseNamespace); err != nil {
			return nil, fmt.Errorf("apply render patches to resource %q: %w", res.IDHuman(), err)
		}

		patchedResources = append(patchedResources, NewResourceSpec(patchedUnstruct, releaseNamespace, ResourceSpecOptions{
			FilePath: res.FilePath,
			StoreAs:  lo.Ternary(res.StoreAs == common.StoreAsNone, common.StoreAsNone, common.StoreAs("")),
		}))
	}

	return patchedResources, nil
}

// Transforms ResourceSpecs, which means specs can be added, deleted, expanded (like Lists). If you
// just need to modify specs, use patchers in BuildReleasableResourceSpecs instead.
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
					FilePath: res.FilePath,
					StoreAs:  res.StoreAs,
				})

				transfResources = append(transfResources, newRes)
			}
		}

		transformedResources = transfResources
	}

	return transformedResources, nil
}

// Annotations and labels are read via apimachinery accessors, which silently discard non-string
// values, so anything but a string map must be rejected before the resource is used.
func validateMetadataStringMaps(unstruct *unstructured.Unstructured) error {
	for _, field := range []string{"annotations", "labels"} {
		if _, _, err := unstructured.NestedNullCoercingStringMap(unstruct.Object, "metadata", field); err != nil {
			return fmt.Errorf("validate resource metadata: %w", err)
		}
	}

	return nil
}

func validateSameResourceIdentity(original, patched *unstructured.Unstructured, releaseNamespace string) error {
	if patched.GetAPIVersion() == "" || patched.GetKind() == "" || patched.GetName() == "" {
		return fmt.Errorf("patch output is not a resource: apiVersion, kind or name is missing")
	}

	originalNamespace := lo.Ternary(original.GetNamespace() == "", releaseNamespace, original.GetNamespace())
	patchedNamespace := lo.Ternary(patched.GetNamespace() == "", releaseNamespace, patched.GetNamespace())

	if original.GetAPIVersion() == patched.GetAPIVersion() &&
		original.GetKind() == patched.GetKind() &&
		original.GetName() == patched.GetName() &&
		originalNamespace == patchedNamespace {
		return nil
	}

	return fmt.Errorf(
		"patch changed resource identity to apiVersion %q, kind %q, name %q, namespace %q, which is not allowed",
		patched.GetAPIVersion(), patched.GetKind(), patched.GetName(), patchedNamespace,
	)
}
