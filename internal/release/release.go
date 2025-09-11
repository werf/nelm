package release

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"sigs.k8s.io/yaml"

	"github.com/werf/3p-helm/pkg/chartutil"
	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource"
)

type ReleaseOptions struct {
	InfoAnnotations map[string]string
	Labels          map[string]string
	Notes           string
}

func NewRelease(name, namespace string, revision int, deployType common.DeployType, resources []*resource.ResourceSpec, opts ReleaseOptions) (*helmrelease.Release, error) {
	if err := chartutil.ValidateReleaseName(name); err != nil {
		return nil, fmt.Errorf("release name %q is not valid: %w", name, err)
	}

	if opts.InfoAnnotations == nil {
		opts.InfoAnnotations = map[string]string{}
	}

	if opts.Labels == nil {
		opts.Labels = map[string]string{}
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
		return resource.ResourceSpecSortHandler(resources[i], resources[j])
	})

	var unstoredResources []string
	var regularResources []string
	var hookResources []*helmrelease.Hook
	for _, res := range resources {
		switch res.StoreAs {
		case resource.StoreAsHook:
			manifest, err := resourceSpecToManifest(name, namespace, revision, res)
			if err != nil {
				return nil, fmt.Errorf("convert resource spec to manifest: %w", err)
			}

			hookResources = append(hookResources, &helmrelease.Hook{
				Manifest: manifest,
			})
		case resource.StoreAsRegular:
			manifest, err := resourceSpecToManifest(name, namespace, revision, res)
			if err != nil {
				return nil, fmt.Errorf("convert resource spec to manifest: %w", err)
			}

			regularResources = append(regularResources, manifest)
		case resource.StoreAsNone:
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
		Manifest:         strings.Join(regularResources, "\n---\n"),
		Hooks:            hookResources,
		Version:          revision,
		Namespace:        namespace,
		Labels:           opts.Labels,
		UnstoredManifest: strings.Join(unstoredResources, "\n---\n"),
	}, nil
}

func resourceSpecToManifest(name string, namespace string, revision int, res *resource.ResourceSpec) (string, error) {
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
