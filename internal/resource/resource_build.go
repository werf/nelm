package resource

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
	"github.com/werf/nelm/internal/resource/meta"
)

type BuildResourcesOptions struct {
	Mapper meta.ResettableRESTMapper
}

func BuildResources(ctx context.Context, deployType common.DeployType, releaseNamespace string, prevRel, newRel *helmrelease.Release, patchers []ResourcePatcher, opts BuildResourcesOptions) ([]*InstallableResource, []*DeletableResource, error) {
	var prevRelResSpecs []*meta.ResourceSpec
	if prevRel != nil {
		if resSpecs, err := release.ReleaseToResourceSpecs(prevRel, releaseNamespace); err != nil {
			return nil, nil, fmt.Errorf("convert previous release to resource specs: %w", err)
		} else {
			prevRelResSpecs = resSpecs
		}
	}

	var prevRelDelResources []*DeletableResource
	for _, resSpec := range prevRelResSpecs {
		var stage common.Stage
		if deployType == common.DeployTypeUninstall {
			stage = common.StageUninstall
		} else {
			stage = common.StagePrePreUninstall
		}

		deletableRes := NewDeletableResource(resSpec.ResourceMeta, releaseNamespace, stage, DeletableResourceOptions{})
		prevRelDelResources = append(prevRelDelResources, deletableRes)
	}

	var prevRelInstResources []*InstallableResource
	for _, resSpec := range prevRelResSpecs {
		installableResources, err := NewInstallableResource(resSpec, deployType, releaseNamespace, InstallableResourceOptions{
			Mapper: opts.Mapper,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("construct installable resource: %w", err)
		}

		prevRelInstResources = append(prevRelInstResources, installableResources...)
	}

	var newRelResSpecs []*meta.ResourceSpec
	if newRel != nil {
		if resSpecs, err := release.ReleaseToResourceSpecs(newRel, releaseNamespace); err != nil {
			return nil, nil, fmt.Errorf("convert new release to resource specs: %w", err)
		} else {
			newRelResSpecs = resSpecs
		}
	}

	var newRelInstResources []*InstallableResource
	for _, resSpec := range newRelResSpecs {
		installableResources, err := NewInstallableResource(resSpec, deployType, releaseNamespace, InstallableResourceOptions{
			Mapper: opts.Mapper,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("construct installable resource: %w", err)
		}

		newRelInstResources = append(newRelInstResources, installableResources...)
	}

	var filteredPrevRelInstResources []*InstallableResource
	if deployType == common.DeployTypeUninstall {
		filteredPrevRelInstResources = lo.Filter(prevRelInstResources, func(instRes *InstallableResource, _ int) bool {
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

	delResources := lo.Filter(prevRelDelResources, func(delRes *DeletableResource, _ int) bool {
		if _, isInstallable := lo.Find(filteredPrevRelInstResources, func(instRes *InstallableResource) bool {
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

	filteredNewRelInstResources := lo.Filter(newRelInstResources, func(instRes *InstallableResource, _ int) bool {
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

	var instResources []*InstallableResource
	for _, r := range append(filteredPrevRelInstResources, filteredNewRelInstResources...) {
		instReses := []*InstallableResource{r}

		var deepCopied bool
		for _, patcher := range patchers {
			var newInstReses []*InstallableResource
			for _, instRes := range instReses {
				if matched, err := patcher.Match(ctx, &ResourcePatcherResourceInfo{
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

				patchedObj, err := patcher.Patch(ctx, &ResourcePatcherResourceInfo{
					Obj:       unstruct,
					Ownership: instRes.Ownership,
				})
				if err != nil {
					return nil, nil, fmt.Errorf("patch deployable resource by %q: %w", patcher.Type(), err)
				}

				resSpec := meta.NewResourceSpec(patchedObj, releaseNamespace, meta.ResourceSpecOptions{
					StoreAs:  instRes.StoreAs,
					FilePath: instRes.FilePath,
				})

				if rs, err := NewInstallableResource(resSpec, deployType, releaseNamespace, InstallableResourceOptions{
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
		return meta.ResourceSpecSortHandler(instResources[i].ResourceSpec, instResources[j].ResourceSpec)
	})

	sort.SliceStable(delResources, func(i, j int) bool {
		return meta.ResourceMetaSortHandler(delResources[i].ResourceMeta, delResources[j].ResourceMeta)
	})

	return instResources, delResources, nil
}
