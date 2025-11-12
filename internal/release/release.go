package release

import (
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
	"strings"
	"unicode"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/yaml"

	helmchart "github.com/werf/3p-helm/pkg/chart"
	"github.com/werf/3p-helm/pkg/chartutil"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/3p-helm/pkg/releaseutil"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
)

type ReleaseOptions struct {
	InfoAnnotations map[string]string
	Labels          map[string]string
	Notes           string
}

func NewRelease(name, namespace string, revision int, deployType common.DeployType, resources []*spec.ResourceSpec, chart *helmchart.Chart, releaseConfig map[string]interface{}, opts ReleaseOptions) (*helmrelease.Release, error) {
	if err := chartutil.ValidateReleaseName(name); err != nil {
		return nil, fmt.Errorf("release name %q is not valid: %w", name, err)
	}

	var status helmrelease.Status
	switch deployType {
	case common.DeployTypeInitial,
		common.DeployTypeInstall:
		status = helmrelease.StatusPendingInstall
	case common.DeployTypeUpgrade:
		status = helmrelease.StatusPendingUpgrade
	case common.DeployTypeRollback:
		status = helmrelease.StatusPendingRollback
	case common.DeployTypeUninstall:
		status = helmrelease.StatusUninstalling
	default:
		panic("unexpected deploy type")
	}

	sort.SliceStable(resources, func(i, j int) bool {
		return spec.ResourceSpecSortHandler(resources[i], resources[j])
	})

	var (
		unstoredResources []string
		regularResources  []string
		hookResources     []*helmrelease.Hook
	)

	for _, res := range resources {
		switch res.StoreAs {
		case common.StoreAsHook:
			manifest, err := resourceSpecToManifest(name, namespace, revision, res)
			if err != nil {
				return nil, fmt.Errorf("convert resource spec to manifest: %w", err)
			}

			hook, err := releaseutil.HookManifestToHook(manifest, res.FilePath)
			if err != nil {
				return nil, fmt.Errorf("convert hook manifest to hook: %w", err)
			}

			hookResources = append(hookResources, hook)
		case common.StoreAsRegular:
			manifest, err := resourceSpecToManifest(name, namespace, revision, res)
			if err != nil {
				return nil, fmt.Errorf("convert resource spec to manifest: %w", err)
			}

			regularResources = append(regularResources, manifest)
		case common.StoreAsNone:
			manifest, err := resourceSpecToManifest(name, namespace, revision, res)
			if err != nil {
				return nil, fmt.Errorf("convert resource spec to manifest: %w", err)
			}

			unstoredResources = append(unstoredResources, manifest)
		default:
			panic(fmt.Sprintf("unknown resource store type %q", res.StoreAs))
		}
	}

	opts.Notes = strings.TrimRightFunc(opts.Notes, unicode.IsSpace)

	return &helmrelease.Release{
		Name: name,
		Info: &helmrelease.Info{
			Status:      status,
			Notes:       opts.Notes,
			Annotations: opts.InfoAnnotations,
		},
		Chart:            chart,
		Config:           releaseConfig,
		Manifest:         strings.Join(regularResources, "\n---\n"),
		Hooks:            hookResources,
		Version:          revision,
		Namespace:        namespace,
		Labels:           opts.Labels,
		UnstoredManifest: strings.Join(unstoredResources, "\n---\n"),
	}, nil
}

func IsReleaseUpToDate(oldRel, newRel *helmrelease.Release) (bool, error) {
	if oldRel == nil {
		return false, nil
	}

	cmpOpts := cmp.Options{
		cmpopts.EquateEmpty(),
	}

	if oldRel.Info.Status != helmrelease.StatusDeployed ||
		oldRel.Info.Notes != newRel.Info.Notes ||
		!cmp.Equal(oldRel.Config, newRel.Config, cmpOpts) {
		return false, nil
	}

	oldHookResourcesHash := fnv.New32a()
	for _, oldHook := range oldRel.Hooks {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(oldHook.Manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return false, fmt.Errorf("decode old hook: %w", err)
		}

		unstruct := cleanUnstruct(obj.(*unstructured.Unstructured))

		if err := writeUnstructHash(unstruct, oldHookResourcesHash); err != nil {
			return false, fmt.Errorf("write old hook hash: %w", err)
		}
	}

	newHookResourcesHash := fnv.New32a()
	for _, newHook := range newRel.Hooks {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(newHook.Manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return false, fmt.Errorf("decode new hook: %w", err)
		}

		unstruct := cleanUnstruct(obj.(*unstructured.Unstructured))

		if err := writeUnstructHash(unstruct, newHookResourcesHash); err != nil {
			return false, fmt.Errorf("write new hook hash: %w", err)
		}
	}

	if oldHookResourcesHash.Sum32() != newHookResourcesHash.Sum32() {
		return false, nil
	}

	oldRelManifests := releaseutil.SplitManifestsToSlice(oldRel.Manifest)

	oldRegularResourcesHash := fnv.New32a()
	for _, manifest := range oldRelManifests {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return false, fmt.Errorf("decode old regular resource: %w", err)
		}

		unstruct := cleanUnstruct(obj.(*unstructured.Unstructured))

		if err := writeUnstructHash(unstruct, oldRegularResourcesHash); err != nil {
			return false, fmt.Errorf("write old regular resource hash: %w", err)
		}
	}

	newRelManifests := releaseutil.SplitManifestsToSlice(newRel.Manifest)

	newRegularResourcesHash := fnv.New32a()
	for _, manifest := range newRelManifests {
		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(manifest), nil, &unstructured.Unstructured{})
		if err != nil {
			return false, fmt.Errorf("decode new regular resource: %w", err)
		}

		unstruct := cleanUnstruct(obj.(*unstructured.Unstructured))

		if err := writeUnstructHash(unstruct, newRegularResourcesHash); err != nil {
			return false, fmt.Errorf("write new regular resource hash: %w", err)
		}
	}

	if oldRegularResourcesHash.Sum32() != newRegularResourcesHash.Sum32() {
		return false, nil
	}

	return true, nil
}

func ReleaseToResourceSpecs(rel *helmrelease.Release, releaseNamespace string) ([]*spec.ResourceSpec, error) {
	var resources []*spec.ResourceSpec
	for _, manifest := range releaseutil.SplitManifestsToSlice(rel.UnstoredManifest) {
		if res, err := spec.NewResourceSpecFromManifest(manifest, releaseNamespace, spec.ResourceSpecOptions{
			StoreAs: common.StoreAsNone,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from unstored manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	for _, manifest := range releaseutil.SplitManifestsToSlice(rel.Manifest) {
		if res, err := spec.NewResourceSpecFromManifest(manifest, releaseNamespace, spec.ResourceSpecOptions{
			StoreAs: common.StoreAsRegular,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from regular manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	for _, hook := range rel.Hooks {
		if res, err := spec.NewResourceSpecFromManifest(hook.Manifest, releaseNamespace, spec.ResourceSpecOptions{
			StoreAs: common.StoreAsHook,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from hook manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	return resources, nil
}

func resourceSpecToManifest(name, namespace string, revision int, res *spec.ResourceSpec) (string, error) {
	manifestByte, err := yaml.Marshal(res.Unstruct.UnstructuredContent())
	if err != nil {
		return "", fmt.Errorf("marshal resource %q for release %q (namespace: %q, revision: %d): %w", res.IDHuman(), name, namespace, revision, err)
	}

	var manifest string
	if res.FilePath != "" {
		manifest = fmt.Sprintf("# Source: %s\n%s", res.FilePath, string(manifestByte))
	} else {
		manifest = string(manifestByte)
	}

	return manifest, nil
}

func writeUnstructHash(unstruct *unstructured.Unstructured, hash hash.Hash32) error {
	if b, err := unstruct.MarshalJSON(); err != nil {
		return fmt.Errorf("unmarshal resource: %w", err)
	} else {
		hash.Write(b)
	}

	return nil
}

func cleanUnstruct(unstruct *unstructured.Unstructured) *unstructured.Unstructured {
	return resource.CleanUnstruct(unstruct, resource.CleanUnstructOptions{
		CleanManagedFields:      true,
		CleanReleaseAnnosLabels: true,
		CleanRuntimeData:        true,
		CleanWerfIoRuntimeAnnos: true,
	})
}
