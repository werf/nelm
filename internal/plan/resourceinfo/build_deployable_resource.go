package resourceinfo

import (
	"context"
	"fmt"
	"sort"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/release"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/id"
)

type BuildDeployableResourcesOptions struct {
	Mapper meta.ResettableRESTMapper
}

func BuildDeployableResources(ctx context.Context, deployType common.DeployType, releaseNamespace string, prevRel, newRel *helmrelease.Release, patchers []resource.ResourcePatcher, opts BuildDeployableResourcesOptions) ([]*resource.InstallableResource, []*resource.DeletableResource, error) {
	var prevRelResSpecs []*id.ResourceSpec
	if prevRel != nil {
		if resSpecs, err := release.ReleaseToResourceSpecs(prevRel, releaseNamespace); err != nil {
			return nil, nil, fmt.Errorf("convert previous release to resource specs: %w", err)
		} else {
			prevRelResSpecs = resSpecs
		}
	}

	var prevRelDelResources []*resource.DeletableResource
	for _, resSpec := range prevRelResSpecs {
		var stage common.Stage
		if deployType == common.DeployTypeUninstall {
			stage = common.StageUninstall
		} else {
			stage = common.StagePrePreUninstall
		}

		deletableRes := resource.NewDeletableResource(resSpec.ResourceMeta, releaseNamespace, stage, resource.DeletableResourceOptions{})
		prevRelDelResources = append(prevRelDelResources, deletableRes)
	}

	var prevRelInstResources []*resource.InstallableResource
	for _, resSpec := range prevRelResSpecs {
		installableResources, err := resource.NewInstallableResource(resSpec, deployType, releaseNamespace, resource.InstallableResourceOptions{
			Mapper: opts.Mapper,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("construct installable resource: %w", err)
		}

		prevRelInstResources = append(prevRelInstResources, installableResources...)
	}

	var newRelResSpecs []*id.ResourceSpec
	if newRel != nil {
		if resSpecs, err := release.ReleaseToResourceSpecs(newRel, releaseNamespace); err != nil {
			return nil, nil, fmt.Errorf("convert new release to resource specs: %w", err)
		} else {
			newRelResSpecs = resSpecs
		}
	}

	var newRelInstResources []*resource.InstallableResource
	for _, resSpec := range newRelResSpecs {
		installableResources, err := resource.NewInstallableResource(resSpec, deployType, releaseNamespace, resource.InstallableResourceOptions{
			Mapper: opts.Mapper,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("construct installable resource: %w", err)
		}

		newRelInstResources = append(newRelInstResources, installableResources...)
	}

	var filteredPrevRelInstResources []*resource.InstallableResource
	if deployType == common.DeployTypeUninstall {
		filteredPrevRelInstResources = lo.Filter(prevRelInstResources, func(instRes *resource.InstallableResource, _ int) bool {
			if len(instRes.DeployConditions) == 0 {
				return false
			}

			conds, found := instRes.DeployConditions[common.InstallOnDelete]
			if !found {
				return false
			}

			return len(conds) > 0
		})
	}

	delResources := lo.Filter(prevRelDelResources, func(delRes *resource.DeletableResource, _ int) bool {
		if _, isInstallable := lo.Find(filteredPrevRelInstResources, func(instRes *resource.InstallableResource) bool {
			return instRes.ID() == delRes.ID()
		}); isInstallable {
			return false
		}

		if delRes.KeepOnDelete {
			return false
		}

		switch delRes.Ownership {
		case common.OwnershipRelease:
			return true
		case common.OwnershipEveryone:
			return false
		default:
			panic("unexpected ownership")
		}
	})

	filteredNewRelInstResources := lo.Filter(newRelInstResources, func(instRes *resource.InstallableResource, _ int) bool {
		if len(instRes.DeployConditions) == 0 {
			return false
		}

		switch deployType {
		case common.DeployTypeInitial, common.DeployTypeInstall:
			conds, found := instRes.DeployConditions[common.InstallOnInstall]
			if !found {
				return false
			}

			return len(conds) > 0
		case common.DeployTypeUpgrade:
			conds, found := instRes.DeployConditions[common.InstallOnUpgrade]
			if !found {
				return false
			}

			return len(conds) > 0
		case common.DeployTypeRollback:
			conds, found := instRes.DeployConditions[common.InstallOnRollback]
			if !found {
				return false
			}

			return len(conds) > 0
		default:
			panic("unexpected deploy type")
		}
	})

	var instResources []*resource.InstallableResource
	for _, r := range append(filteredPrevRelInstResources, filteredNewRelInstResources...) {
		instReses := []*resource.InstallableResource{r}

		var deepCopied bool
		for _, patcher := range patchers {
			var newInstReses []*resource.InstallableResource
			for _, instRes := range instReses {
				if matched, err := patcher.Match(ctx, &resource.ResourcePatcherResourceInfo{
					Obj:       instRes.Unstruct,
					Ownership: instRes.Ownership,
				}); err != nil {
					return nil, nil, fmt.Errorf("match deployable resource for patching by %q: %w", patcher.Type(), err)
				} else if !matched {
					continue
				}

				var unstruct *unstructured.Unstructured
				if deepCopied {
					unstruct = instRes.Unstruct
				} else {
					unstruct = instRes.Unstruct.DeepCopy()
					deepCopied = true
				}

				patchedObj, err := patcher.Patch(ctx, &resource.ResourcePatcherResourceInfo{
					Obj:       unstruct,
					Ownership: instRes.Ownership,
				})
				if err != nil {
					return nil, nil, fmt.Errorf("patch deployable resource by %q: %w", patcher.Type(), err)
				}

				resSpec := id.NewResourceSpec(patchedObj, releaseNamespace, id.ResourceSpecOptions{
					StoreAs:  instRes.StoreAs,
					FilePath: instRes.FilePath,
				})

				if rs, err := resource.NewInstallableResource(resSpec, deployType, releaseNamespace, resource.InstallableResourceOptions{
					Mapper: opts.Mapper,
				}); err != nil {
					return nil, nil, fmt.Errorf("construct deployable resource from patched object by %q: %w", patcher.Type(), err)
				} else {
					newInstReses = append(newInstReses, rs...)
				}
			}

			instReses = newInstReses
		}

		instResources = append(instResources, instReses...)
	}

	sort.SliceStable(instResources, func(i, j int) bool {
		return id.ResourceSpecSortHandler(instResources[i].ResourceSpec, instResources[j].ResourceSpec)
	})

	sort.SliceStable(delResources, func(i, j int) bool {
		return id.ResourceMetaSortHandler(delResources[i].ResourceMeta, delResources[j].ResourceMeta)
	})

	return instResources, delResources, nil
}
