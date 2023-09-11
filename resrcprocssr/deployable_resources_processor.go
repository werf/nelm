package resrcprocssr

import (
	"context"
	"fmt"
	"strings"

	"github.com/samber/lo"
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/kubeclnt"
	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/resrc"
	"helm.sh/helm/v3/pkg/werf/resrcid"
	"helm.sh/helm/v3/pkg/werf/resrcinfo"
	"helm.sh/helm/v3/pkg/werf/resrcpatcher"
	"helm.sh/helm/v3/pkg/werf/resrctransfrmr"
	"helm.sh/helm/v3/pkg/werf/utls"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
)

func NewDeployableResourcesProcessor(
	deployType common.DeployType,
	releaseName string,
	releaseNamespace *resrc.ReleaseNamespace,
	standaloneCRDs []*resrc.StandaloneCRD,
	hookResources []*resrc.HookResource,
	generalResources []*resrc.GeneralResource,
	prevReleaseGeneralResources []*resrc.GeneralResource,
	kubeClient kubeclnt.KubeClienter,
	mapper meta.ResettableRESTMapper,
	discoveryClient discovery.CachedDiscoveryInterface,
	opts DeployableResourcesProcessorOptions,
) *DeployableResourcesProcessor {
	return &DeployableResourcesProcessor{
		deployType:                        deployType,
		releaseName:                       releaseName,
		releaseNamespace:                  releaseNamespace,
		standaloneCRDs:                    standaloneCRDs,
		hookResources:                     hookResources,
		generalResources:                  generalResources,
		prevRelGeneralResources:           prevReleaseGeneralResources,
		kubeClient:                        kubeClient,
		mapper:                            mapper,
		discoveryClient:                   discoveryClient,
		networkParallelism:                lo.Max([]int{opts.NetworkParallelism, 1}),
		hookResourceTransformers:          opts.HookResourceTransformers,
		generalResourceTransformers:       opts.GeneralResourceTransformers,
		releasableHookResourcePatchers:    opts.ReleasableHookResourcePatchers,
		releasableGeneralResourcePatchers: opts.ReleasableGeneralResourcePatchers,
		deployableStandaloneCRDsPatchers:  opts.DeployableStandaloneCRDsPatchers,
		deployableHookResourcePatchers:    opts.DeployableHookResourcePatchers,
		deployableGeneralResourcePatchers: opts.DeployableGeneralResourcePatchers,
	}
}

type DeployableResourcesProcessorOptions struct {
	NetworkParallelism                int
	HookResourceTransformers          []resrctransfrmr.ResourceTransformer
	GeneralResourceTransformers       []resrctransfrmr.ResourceTransformer
	ReleasableHookResourcePatchers    []resrcpatcher.ResourcePatcher
	ReleasableGeneralResourcePatchers []resrcpatcher.ResourcePatcher
	DeployableStandaloneCRDsPatchers  []resrcpatcher.ResourcePatcher
	DeployableHookResourcePatchers    []resrcpatcher.ResourcePatcher
	DeployableGeneralResourcePatchers []resrcpatcher.ResourcePatcher
}

type DeployableResourcesProcessor struct {
	deployType              common.DeployType
	releaseName             string
	releaseNamespace        *resrc.ReleaseNamespace
	standaloneCRDs          []*resrc.StandaloneCRD
	hookResources           []*resrc.HookResource
	generalResources        []*resrc.GeneralResource
	prevRelGeneralResources []*resrc.GeneralResource
	kubeClient              kubeclnt.KubeClienter
	mapper                  meta.ResettableRESTMapper
	discoveryClient         discovery.CachedDiscoveryInterface
	networkParallelism      int

	hookResourceTransformers    []resrctransfrmr.ResourceTransformer
	generalResourceTransformers []resrctransfrmr.ResourceTransformer

	releasableHookResourcePatchers    []resrcpatcher.ResourcePatcher
	releasableGeneralResourcePatchers []resrcpatcher.ResourcePatcher

	deployableStandaloneCRDsPatchers  []resrcpatcher.ResourcePatcher
	deployableHookResourcePatchers    []resrcpatcher.ResourcePatcher
	deployableGeneralResourcePatchers []resrcpatcher.ResourcePatcher

	releasableHookResources    []*resrc.HookResource
	releasableGeneralResources []*resrc.GeneralResource

	deployableReleaseNamespace *resrc.ReleaseNamespace
	deployableStandaloneCRDs   []*resrc.StandaloneCRD
	deployableHookResources    []*resrc.HookResource
	deployableGeneralResources []*resrc.GeneralResource

	deployableReleaseNamespaceInfo         *resrcinfo.DeployableReleaseNamespaceInfo
	deployableStandaloneCRDsInfos          []*resrcinfo.DeployableStandaloneCRDInfo
	deployableHookResourcesInfos           []*resrcinfo.DeployableHookResourceInfo
	deployableGeneralResourcesInfos        []*resrcinfo.DeployableGeneralResourceInfo
	deployablePrevRelGeneralResourcesInfos []*resrcinfo.DeployablePrevReleaseGeneralResourceInfo
}

func (p *DeployableResourcesProcessor) Process(ctx context.Context) error {
	if err := p.transformHookResources(ctx); err != nil {
		return fmt.Errorf("error transforming hook resources: %w", err)
	}

	if err := p.transformGeneralResources(ctx); err != nil {
		return fmt.Errorf("error transforming general resources: %w", err)
	}

	if err := p.validateResources(); err != nil {
		return fmt.Errorf("error validating resources: %w", err)
	}

	if err := p.validateNoDuplicates(); err != nil {
		return fmt.Errorf("error validating for no duplicated resources: %w", err)
	}

	if err := p.buildReleasableHookResources(ctx); err != nil {
		return fmt.Errorf("error building releasable hook resources: %w", err)
	}

	if err := p.buildReleasableGeneralResources(ctx); err != nil {
		return fmt.Errorf("error building releasable general resources: %w", err)
	}

	if err := p.validateReleasableResources(); err != nil {
		return fmt.Errorf("error validating releasable resources: %w", err)
	}

	if err := p.buildDeployableStandaloneCRDs(ctx); err != nil {
		return fmt.Errorf("error building deployable standalone crds: %w", err)
	}

	p.deployableReleaseNamespace = p.releaseNamespace

	if err := p.buildDeployableHookResources(ctx); err != nil {
		return fmt.Errorf("error building deployable hook resources: %w", err)
	}

	if err := p.buildDeployableGeneralResources(ctx); err != nil {
		return fmt.Errorf("error building deployable general resources: %w", err)
	}

	if err := p.validateDeployableResources(); err != nil {
		return fmt.Errorf("error validating deployable resources: %w", err)
	}

	if err := p.buildDeployableResourceInfos(ctx); err != nil {
		return fmt.Errorf("error building deployable resource infos: %w", err)
	}

	if err := p.validateAdoptableResources(); err != nil {
		return fmt.Errorf("error validating adoptable resources: %w", err)
	}

	log.Default.TraceStruct(ctx, p, "Resources processor:")

	return nil
}

func (p *DeployableResourcesProcessor) ReleasableHookResources() []*resrc.HookResource {
	return p.releasableHookResources
}

func (p *DeployableResourcesProcessor) ReleasableGeneralResources() []*resrc.GeneralResource {
	return p.releasableGeneralResources
}

func (p *DeployableResourcesProcessor) DeployableReleaseNamespaceInfo() *resrcinfo.DeployableReleaseNamespaceInfo {
	return p.deployableReleaseNamespaceInfo
}

func (p *DeployableResourcesProcessor) DeployableStandaloneCRDsInfos() []*resrcinfo.DeployableStandaloneCRDInfo {
	return p.deployableStandaloneCRDsInfos
}

func (p *DeployableResourcesProcessor) DeployableHookResourcesInfos() []*resrcinfo.DeployableHookResourceInfo {
	return p.deployableHookResourcesInfos
}

func (p *DeployableResourcesProcessor) DeployableGeneralResourcesInfos() []*resrcinfo.DeployableGeneralResourceInfo {
	return p.deployableGeneralResourcesInfos
}

func (p *DeployableResourcesProcessor) DeployablePrevReleaseGeneralResourcesInfos() []*resrcinfo.DeployablePrevReleaseGeneralResourceInfo {
	return p.deployablePrevRelGeneralResourcesInfos
}

func (p *DeployableResourcesProcessor) transformHookResources(ctx context.Context) error {
	for _, resTransformer := range p.hookResourceTransformers {
		var transformedResources []*resrc.HookResource

		for _, res := range p.hookResources {
			if matched, err := resTransformer.Match(ctx, &resrctransfrmr.ResourceInfo{
				Obj:          res.Unstructured(),
				Type:         resrc.TypeHookResource,
				ManageableBy: res.ManageableBy(),
			}); err != nil {
				return fmt.Errorf("error matching hook resource %q for transformation by %q: %w", res.HumanID(), resTransformer.Type(), err)
			} else if !matched {
				transformedResources = append(transformedResources, res)
				continue
			}

			newObjs, err := resTransformer.Transform(ctx, &resrctransfrmr.ResourceInfo{
				Obj:          res.Unstructured(),
				Type:         resrc.TypeHookResource,
				ManageableBy: res.ManageableBy(),
			})
			if err != nil {
				return fmt.Errorf("error transforming hook resource %q by %q: %w", res.HumanID(), resTransformer.Type(), err)
			}

			for _, newObj := range newObjs {
				newRes := resrc.NewHookResource(newObj, resrc.HookResourceOptions{
					FilePath:         res.FilePath(),
					DefaultNamespace: p.releaseNamespace.Name(),
					Mapper:           p.mapper,
					DiscoveryClient:  p.discoveryClient,
				})
				transformedResources = append(transformedResources, newRes)
			}
		}

		p.hookResources = transformedResources
	}

	return nil
}

func (p *DeployableResourcesProcessor) transformGeneralResources(ctx context.Context) error {
	for _, resTransformer := range p.generalResourceTransformers {
		var transformedResources []*resrc.GeneralResource

		for _, res := range p.generalResources {
			if matched, err := resTransformer.Match(ctx, &resrctransfrmr.ResourceInfo{
				Obj:          res.Unstructured(),
				Type:         resrc.TypeGeneralResource,
				ManageableBy: res.ManageableBy(),
			}); err != nil {
				return fmt.Errorf("error matching general resource %q for transformation by %q: %w", res.HumanID(), resTransformer.Type(), err)
			} else if !matched {
				transformedResources = append(transformedResources, res)
				continue
			}

			newObjs, err := resTransformer.Transform(ctx, &resrctransfrmr.ResourceInfo{
				Obj:          res.Unstructured(),
				Type:         resrc.TypeGeneralResource,
				ManageableBy: res.ManageableBy(),
			})
			if err != nil {
				return fmt.Errorf("error transforming general resource %q by %q: %w", res.HumanID(), resTransformer.Type(), err)
			}

			for _, newObj := range newObjs {
				newRes := resrc.NewGeneralResource(newObj, resrc.GeneralResourceOptions{
					FilePath:         res.FilePath(),
					DefaultNamespace: p.releaseNamespace.Name(),
					Mapper:           p.mapper,
					DiscoveryClient:  p.discoveryClient,
				})
				transformedResources = append(transformedResources, newRes)
			}
		}

		p.generalResources = transformedResources
	}

	return nil
}

func (p *DeployableResourcesProcessor) buildReleasableHookResources(ctx context.Context) error {
	p.releasableHookResources = p.hookResources

	for _, resPatcher := range p.releasableHookResourcePatchers {
		var patchedResources []*resrc.HookResource

		for _, res := range p.releasableHookResources {
			if matched, err := resPatcher.Match(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured(),
				Type:         resrc.TypeHookResource,
				ManageableBy: res.ManageableBy(),
			}); err != nil {
				return fmt.Errorf("error matching hook resource %q for patching by %q: %w", res.HumanID(), resPatcher.Type(), err)
			} else if !matched {
				patchedResources = append(patchedResources, res)
				continue
			}

			patchedObj, err := resPatcher.Patch(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured().DeepCopy(),
				Type:         resrc.TypeHookResource,
				ManageableBy: res.ManageableBy(),
			})
			if err != nil {
				return fmt.Errorf("error patching hook resource %q by %q: %w", res.HumanID(), resPatcher.Type(), err)
			}

			patchedRes := resrc.NewHookResource(patchedObj, resrc.HookResourceOptions{
				FilePath:         res.FilePath(),
				DefaultNamespace: p.releaseNamespace.Name(),
				Mapper:           p.mapper,
				DiscoveryClient:  p.discoveryClient,
			})
			patchedResources = append(patchedResources, patchedRes)
		}

		p.releasableHookResources = patchedResources
	}

	return nil
}

func (p *DeployableResourcesProcessor) buildReleasableGeneralResources(ctx context.Context) error {
	p.releasableGeneralResources = p.generalResources

	for _, resPatcher := range p.releasableGeneralResourcePatchers {
		var patchedResources []*resrc.GeneralResource

		for _, res := range p.releasableGeneralResources {
			if matched, err := resPatcher.Match(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured(),
				Type:         resrc.TypeGeneralResource,
				ManageableBy: res.ManageableBy(),
			}); err != nil {
				return fmt.Errorf("error matching general resource %q for patching by %q: %w", res.HumanID(), resPatcher.Type(), err)
			} else if !matched {
				patchedResources = append(patchedResources, res)
				continue
			}

			patchedObj, err := resPatcher.Patch(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured().DeepCopy(),
				Type:         resrc.TypeGeneralResource,
				ManageableBy: res.ManageableBy(),
			})
			if err != nil {
				return fmt.Errorf("error patching general resource %q by %q: %w", res.HumanID(), resPatcher.Type(), err)
			}

			patchedRes := resrc.NewGeneralResource(patchedObj, resrc.GeneralResourceOptions{
				FilePath:         res.FilePath(),
				DefaultNamespace: p.releaseNamespace.Name(),
				Mapper:           p.mapper,
				DiscoveryClient:  p.discoveryClient,
			})
			patchedResources = append(patchedResources, patchedRes)
		}

		p.releasableGeneralResources = patchedResources
	}

	return nil
}

func (p *DeployableResourcesProcessor) buildDeployableStandaloneCRDs(ctx context.Context) error {
	p.deployableStandaloneCRDs = p.standaloneCRDs

	for _, resPatcher := range p.deployableStandaloneCRDsPatchers {
		var patchedResources []*resrc.StandaloneCRD

		for _, res := range p.deployableStandaloneCRDs {
			if matched, err := resPatcher.Match(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured(),
				Type:         resrc.TypeHookResource,
				ManageableBy: res.ManageableBy(),
			}); err != nil {
				return fmt.Errorf("error matching deployable standalone crd %q for patching by %q: %w", res.HumanID(), resPatcher.Type(), err)
			} else if !matched {
				patchedResources = append(patchedResources, res)
				continue
			}

			patchedObj, err := resPatcher.Patch(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured().DeepCopy(),
				Type:         resrc.TypeStandaloneCRD,
				ManageableBy: res.ManageableBy(),
			})
			if err != nil {
				return fmt.Errorf("error patching deployable standalone crd %q by %q: %w", res.HumanID(), resPatcher.Type(), err)
			}

			patchedRes := resrc.NewStandaloneCRD(patchedObj, resrc.StandaloneCRDOptions{
				FilePath:         res.FilePath(),
				DefaultNamespace: p.releaseNamespace.Name(),
				Mapper:           p.mapper,
			})
			patchedResources = append(patchedResources, patchedRes)
		}

		p.deployableStandaloneCRDs = patchedResources
	}

	return nil
}

func (p *DeployableResourcesProcessor) buildDeployableHookResources(ctx context.Context) error {
	deployableHookResources := lo.Filter(p.hookResources, func(res *resrc.HookResource, _ int) bool {
		switch p.deployType {
		case common.DeployTypeInitial, common.DeployTypeInstall:
			return res.OnPreInstall() || res.OnPostInstall()
		case common.DeployTypeUpgrade:
			return res.OnPreUpgrade() || res.OnPostUpgrade()
		}

		return false
	})

	p.deployableHookResources = deployableHookResources

	for _, resPatcher := range p.deployableHookResourcePatchers {
		var patchedResources []*resrc.HookResource

		for _, res := range p.deployableHookResources {
			if matched, err := resPatcher.Match(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured(),
				Type:         resrc.TypeHookResource,
				ManageableBy: res.ManageableBy(),
			}); err != nil {
				return fmt.Errorf("error matching deployable hook resource %q for patching by %q: %w", res.HumanID(), resPatcher.Type(), err)
			} else if !matched {
				patchedResources = append(patchedResources, res)
				continue
			}

			patchedObj, err := resPatcher.Patch(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured().DeepCopy(),
				Type:         resrc.TypeHookResource,
				ManageableBy: res.ManageableBy(),
			})
			if err != nil {
				return fmt.Errorf("error patching deployable hook resource %q by %q: %w", res.HumanID(), resPatcher.Type(), err)
			}

			patchedRes := resrc.NewHookResource(patchedObj, resrc.HookResourceOptions{
				FilePath:         res.FilePath(),
				DefaultNamespace: p.releaseNamespace.Name(),
				Mapper:           p.mapper,
				DiscoveryClient:  p.discoveryClient,
			})
			patchedResources = append(patchedResources, patchedRes)
		}

		p.deployableHookResources = patchedResources
	}

	return nil
}

func (p *DeployableResourcesProcessor) buildDeployableGeneralResources(ctx context.Context) error {
	p.deployableGeneralResources = p.generalResources

	for _, resPatcher := range p.deployableGeneralResourcePatchers {
		var patchedResources []*resrc.GeneralResource

		for _, res := range p.deployableGeneralResources {
			if matched, err := resPatcher.Match(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured(),
				Type:         resrc.TypeGeneralResource,
				ManageableBy: res.ManageableBy(),
			}); err != nil {
				return fmt.Errorf("error matching deployable general resource %q for patching by %q: %w", res.HumanID(), resPatcher.Type(), err)
			} else if !matched {
				patchedResources = append(patchedResources, res)
				continue
			}

			patchedObj, err := resPatcher.Patch(ctx, &resrcpatcher.ResourceInfo{
				Obj:          res.Unstructured().DeepCopy(),
				Type:         resrc.TypeGeneralResource,
				ManageableBy: res.ManageableBy(),
			})
			if err != nil {
				return fmt.Errorf("error patching deployable general resource %q by %q: %w", res.HumanID(), resPatcher.Type(), err)
			}

			patchedRes := resrc.NewGeneralResource(patchedObj, resrc.GeneralResourceOptions{
				FilePath:         res.FilePath(),
				DefaultNamespace: p.releaseNamespace.Name(),
				Mapper:           p.mapper,
				DiscoveryClient:  p.discoveryClient,
			})
			patchedResources = append(patchedResources, patchedRes)
		}

		p.deployableGeneralResources = patchedResources
	}

	return nil
}

func (p *DeployableResourcesProcessor) buildDeployableResourceInfos(ctx context.Context) error {
	var err error
	p.deployableReleaseNamespaceInfo, p.deployableStandaloneCRDsInfos, p.deployableHookResourcesInfos, p.deployableGeneralResourcesInfos, p.deployablePrevRelGeneralResourcesInfos, err = resrcinfo.BuildDeployableResourceInfos(
		ctx,
		p.releaseName,
		p.deployableReleaseNamespace,
		p.deployableStandaloneCRDs,
		p.deployableHookResources,
		p.deployableGeneralResources,
		p.prevRelGeneralResources,
		p.kubeClient,
		p.mapper,
		p.networkParallelism,
	)
	if err != nil {
		return fmt.Errorf("error building deployable resource infos: %w", err)
	}

	return nil
}

func (p *DeployableResourcesProcessor) validateResources() error {
	var errs []error

	if err := p.releaseNamespace.Validate(); err != nil {
		errs = append(errs, err)
	}

	for _, res := range p.standaloneCRDs {
		if err := res.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	for _, res := range p.hookResources {
		if err := res.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	for _, res := range p.generalResources {
		if err := res.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	return utls.Multierrorf("resources validation failed", errs)
}

func (p *DeployableResourcesProcessor) validateReleasableResources() error {
	var errs []error

	for _, res := range p.releasableHookResources {
		if err := res.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	for _, res := range p.releasableGeneralResources {
		if err := res.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	return utls.Multierrorf("releasable resources validation failed", errs)
}

func (p *DeployableResourcesProcessor) validateDeployableResources() error {
	var errs []error

	if err := p.deployableReleaseNamespace.Validate(); err != nil {
		errs = append(errs, err)
	}

	for _, res := range p.deployableStandaloneCRDs {
		if err := res.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	for _, res := range p.deployableHookResources {
		if err := res.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	for _, res := range p.deployableGeneralResources {
		if err := res.Validate(); err != nil {
			errs = append(errs, err)
		}
	}

	return utls.Multierrorf("deployable resources validation failed", errs)
}

func (p *DeployableResourcesProcessor) validateNoDuplicates() error {
	resources := []*resrcid.ResourceID{
		p.releaseNamespace.ResourceID,
	}

	for _, res := range p.standaloneCRDs {
		resources = append(resources, res.ResourceID)
	}

	for _, res := range p.hookResources {
		resources = append(resources, res.ResourceID)
	}

	for _, res := range p.generalResources {
		resources = append(resources, res.ResourceID)
	}

	resourceIDs := lo.Map(resources, func(res *resrcid.ResourceID, _ int) string {
		return res.ID()
	})

	duplicatedIDs := lo.FindDuplicates(resourceIDs)

	if len(duplicatedIDs) > 0 {
		duplicatedResources := lo.Filter(resources, func(resID *resrcid.ResourceID, _ int) bool {
			_, found := lo.Find(duplicatedIDs, func(id string) bool {
				return id == resID.ID()
			})
			return found
		})

		duplicatedHumanIDs := lo.Map(duplicatedResources, func(res *resrcid.ResourceID, _ int) string {
			return res.HumanID()
		})

		return fmt.Errorf("duplicated resources found: %s", strings.Join(duplicatedHumanIDs, ", "))
	}

	return nil
}

func (p *DeployableResourcesProcessor) validateAdoptableResources() error {
	var errs []error
	for _, genResInfo := range p.deployableGeneralResourcesInfos {
		if genResInfo.LiveResource() == nil {
			continue
		}

		if adoptable, nonAdoptableReason := genResInfo.LiveResource().AdoptableBy(p.releaseName, p.releaseNamespace.Name()); !adoptable {
			errs = append(errs, fmt.Errorf("resource %q is not adoptable: %s", genResInfo.HumanID(), nonAdoptableReason))
		}
	}

	return utls.Multierrorf("adoption validation failed", errs)
}
