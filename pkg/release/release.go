package release

import (
	"context"
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

	"github.com/werf/nelm/pkg/common"
	v3chart "github.com/werf/nelm/pkg/helm/intern/chart/v3"
	v2release "github.com/werf/nelm/pkg/helm/intern/release/v2"
	v2releaseutil "github.com/werf/nelm/pkg/helm/intern/release/v2/util"
	chart "github.com/werf/nelm/pkg/helm/pkg/chart"
	helmchart "github.com/werf/nelm/pkg/helm/pkg/chart/v2"
	chartv2util "github.com/werf/nelm/pkg/helm/pkg/chart/v2/util"
	helmrel "github.com/werf/nelm/pkg/helm/pkg/release"
	helmreleasecommon "github.com/werf/nelm/pkg/helm/pkg/release/common"
	helmrelease "github.com/werf/nelm/pkg/helm/pkg/release/v1"
	releaseutil "github.com/werf/nelm/pkg/helm/pkg/release/v1/util"
	"github.com/werf/nelm/pkg/resource/spec"
	"github.com/werf/nelm/pkg/util"
)

type ReleaseOptions struct {
	InfoAnnotations map[string]string
	Labels          map[string]string
	Notes           string
}

// Check if the new Release is up-to-date compared to the old Release. It doesn't check any
// resources of the release in the cluster, just compares Release objects.
func IsReleaseUpToDate(oldRel, newRel helmrel.Accessor) (bool, error) {
	if oldRel == nil {
		return false, nil
	}

	cmpOpts := cmp.Options{
		cmpopts.EquateEmpty(),
	}

	if oldRel.Status() != helmreleasecommon.StatusDeployed.String() ||
		oldRel.Notes() != newRel.Notes() ||
		!cmp.Equal(oldRel.Config(), newRel.Config(), cmpOpts) {
		return false, nil
	}

	oldHookResourcesHash := fnv.New32a()
	for _, oldHook := range oldRel.Hooks() {
		hookAcc, err := helmrel.NewHookAccessor(oldHook)
		if err != nil {
			return false, fmt.Errorf("get old hook accessor: %w", err)
		}

		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(hookAcc.Manifest()), nil, &unstructured.Unstructured{})
		if err != nil {
			return false, fmt.Errorf("decode old hook: %w", err)
		}

		unstruct := cleanUnstruct(obj.(*unstructured.Unstructured))

		if err := writeUnstructHash(unstruct, oldHookResourcesHash); err != nil {
			return false, fmt.Errorf("write old hook hash: %w", err)
		}
	}

	newHookResourcesHash := fnv.New32a()
	for _, newHook := range newRel.Hooks() {
		hookAcc, err := helmrel.NewHookAccessor(newHook)
		if err != nil {
			return false, fmt.Errorf("get new hook accessor: %w", err)
		}

		obj, _, err := scheme.Codecs.UniversalDecoder().Decode([]byte(hookAcc.Manifest()), nil, &unstructured.Unstructured{})
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

	oldRelManifests := util.SplitManifests(oldRel.Manifest())

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

	newRelManifests := util.SplitManifests(newRel.Manifest())

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

// Construct Helm release.
func NewRelease(name, namespace string, revision int, deployType common.DeployType, resources []*spec.ResourceSpec, chrt chart.Accessor, releaseConfig map[string]interface{}, opts ReleaseOptions) (helmrel.Accessor, error) {
	if err := chartv2util.ValidateReleaseName(name); err != nil {
		return nil, fmt.Errorf("release name %q is not valid: %w", name, err)
	}

	var status helmreleasecommon.Status
	switch deployType {
	case common.DeployTypeInitial,
		common.DeployTypeInstall:
		status = helmreleasecommon.StatusPendingInstall
	case common.DeployTypeUpgrade:
		status = helmreleasecommon.StatusPendingUpgrade
	case common.DeployTypeRollback:
		status = helmreleasecommon.StatusPendingRollback
	case common.DeployTypeUninstall:
		status = helmreleasecommon.StatusUninstalling
	default:
		panic("unexpected deploy type")
	}

	sort.SliceStable(resources, func(i, j int) bool {
		return spec.ResourceSpecSortHandler(resources[i], resources[j])
	})

	_, isV3 := chrt.Charter().(*v3chart.Chart)

	var (
		unstoredResources []string
		regularResources  []string
		v1HookResources   []*helmrelease.Hook
		v2HookResources   []*v2release.Hook
	)

	for _, res := range resources {
		switch res.StoreAs {
		case common.StoreAsHook:
			manifest, err := resourceSpecToManifest(name, namespace, revision, res)
			if err != nil {
				return nil, fmt.Errorf("convert resource spec to manifest: %w", err)
			}

			if isV3 {
				hook, err := v2releaseutil.HookManifestToHook(manifest, res.FilePath)
				if err != nil {
					return nil, fmt.Errorf("convert hook manifest to hook: %w", err)
				}

				v2HookResources = append(v2HookResources, hook)
			} else {
				hook, err := releaseutil.HookManifestToHook(manifest, res.FilePath)
				if err != nil {
					return nil, fmt.Errorf("convert hook manifest to hook: %w", err)
				}

				v1HookResources = append(v1HookResources, hook)
			}
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

	var releaser helmrel.Releaser
	switch chartObj := chrt.Charter().(type) {
	case *helmchart.Chart:
		releaser = &helmrelease.Release{
			Name: name,
			Info: &helmrelease.Info{
				Status:      status,
				Notes:       opts.Notes,
				Annotations: opts.InfoAnnotations,
			},
			Chart:            chartObj,
			Config:           releaseConfig,
			Manifest:         strings.Join(regularResources, "\n---\n"),
			Hooks:            v1HookResources,
			Version:          revision,
			Namespace:        namespace,
			Labels:           opts.Labels,
			UnstoredManifest: strings.Join(unstoredResources, "\n---\n"),
		}
	case *v3chart.Chart:
		releaser = &v2release.Release{
			Name: name,
			Info: &v2release.Info{
				Status: status,
				Notes:  opts.Notes,
			},
			Chart:            chartObj,
			Config:           releaseConfig,
			Manifest:         strings.Join(regularResources, "\n---\n"),
			Hooks:            v2HookResources,
			Version:          revision,
			Namespace:        namespace,
			Labels:           opts.Labels,
			UnstoredManifest: strings.Join(unstoredResources, "\n---\n"),
		}
	default:
		return nil, fmt.Errorf("unexpected chart type: %T", chrt.Charter())
	}

	acc, err := helmrel.NewAccessor(releaser)
	if err != nil {
		return nil, fmt.Errorf("wrap release: %w", err)
	}

	return acc, nil
}

// Constructs ResourceSpecs from a Release object.
func ReleaseToResourceSpecs(ctx context.Context, rel helmrel.Accessor, releaseNamespace string, noCleanNullFields bool) ([]*spec.ResourceSpec, error) {
	var resources []*spec.ResourceSpec
	for _, manifest := range util.SplitManifests(rel.UnstoredManifest()) {
		if res, err := spec.NewResourceSpecFromManifest(ctx, manifest, releaseNamespace, spec.ResourceSpecOptions{
			StoreAs:                         common.StoreAsNone,
			LegacyNoCleanNullFields:         noCleanNullFields,
			DropInvalidAnnotationsAndLabels: true,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from unstored manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	for _, manifest := range util.SplitManifests(rel.Manifest()) {
		if res, err := spec.NewResourceSpecFromManifest(ctx, manifest, releaseNamespace, spec.ResourceSpecOptions{
			StoreAs:                         common.StoreAsRegular,
			LegacyNoCleanNullFields:         noCleanNullFields,
			DropInvalidAnnotationsAndLabels: true,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from regular manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	for _, hook := range rel.Hooks() {
		hookAcc, err := helmrel.NewHookAccessor(hook)
		if err != nil {
			return nil, fmt.Errorf("get hook accessor: %w", err)
		}

		if res, err := spec.NewResourceSpecFromManifest(ctx, hookAcc.Manifest(), releaseNamespace, spec.ResourceSpecOptions{
			StoreAs:                         common.StoreAsHook,
			LegacyNoCleanNullFields:         noCleanNullFields,
			DropInvalidAnnotationsAndLabels: true,
		}); err != nil {
			return nil, fmt.Errorf("construct resource spec from hook manifest: %w", err)
		} else {
			resources = append(resources, res)
		}
	}

	return resources, nil
}

func cleanUnstruct(unstruct *unstructured.Unstructured) *unstructured.Unstructured {
	return spec.CleanUnstruct(unstruct, spec.CleanUnstructOptions{
		CleanManagedFields:      true,
		CleanReleaseAnnosLabels: true,
		CleanRuntimeData:        true,
		CleanWerfIoRuntimeAnnos: true,
	})
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
