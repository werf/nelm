package resource

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/samber/lo"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/resource/meta"
)

type BuildResourcesOptions struct {
	Mapper apimeta.ResettableRESTMapper
}

func BuildResources(ctx context.Context, deployType common.DeployType, releaseNamespace string, prevRelResSpecs, newRelResSpecs []*ResourceSpec, patchers []ResourcePatcher, opts BuildResourcesOptions) ([]*InstallableResource, []*DeletableResource, error) {
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

				resSpec := NewResourceSpec(patchedObj, releaseNamespace, ResourceSpecOptions{
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
		return ResourceSpecSortHandler(instResources[i].ResourceSpec, instResources[j].ResourceSpec)
	})

	sort.SliceStable(delResources, func(i, j int) bool {
		return meta.ResourceMetaSortHandler(delResources[i].ResourceMeta, delResources[j].ResourceMeta)
	})

	return instResources, delResources, nil
}

type InstallableResourceOptions struct {
	Mapper apimeta.ResettableRESTMapper
}

func NewInstallableResource(res *ResourceSpec, deployType common.DeployType, releaseNamespace string, opts InstallableResourceOptions) ([]*InstallableResource, error) {
	if err := validateHook(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate hook configuration: %w", err)
	}

	if err := validateReplicasOnCreation(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate replicas on creation: %w", err)
	}

	if err := validateDeletePolicy(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate delete policy: %w", err)
	}

	if err := ValidateResourcePolicy(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate resource policy: %w", err)
	}

	if err := validateTrack(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate track annotations: %w", err)
	}

	if err := validateWeight(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate weight: %w", err)
	}

	if err := validateDeployDependencies(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate deploy dependencies: %w", err)
	}

	if err := validateInternalDependencies(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate internal dependencies: %w", err)
	}

	if err := validateExternalDependencies(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate external dependencies: %w", err)
	}

	if err := validateSensitive(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate sensitive: %w", err)
	}

	if err := validateDeployOn(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate deploy stage: %w", err)
	}

	if err := validateOwnership(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate ownership: %w", err)
	}

	extDeps, err := externalDependencies(res.ResourceMeta, releaseNamespace, opts.Mapper)
	if err != nil {
		return nil, fmt.Errorf("get external dependencies: %w", err)
	}

	deplConditions := deployConditions(res.ResourceMeta)
	manIntDeps := manualInternalDependencies(res.ResourceMeta)

	var stages []common.Stage
	switch deployType {
	case common.DeployTypeInitial, common.DeployTypeInstall:
		stages = deplConditions[common.InstallOnInstall]
	case common.DeployTypeUpgrade:
		stages = deplConditions[common.InstallOnUpgrade]
	case common.DeployTypeRollback:
		stages = deplConditions[common.InstallOnRollback]
	case common.DeployTypeUninstall:
		stages = deplConditions[common.InstallOnDelete]
	}

	var instResources []*InstallableResource
	for _, stage := range stages {
		instResources = append(instResources, &InstallableResource{
			ResourceSpec:                           res,
			Recreate:                               recreate(res.ResourceMeta),
			DefaultReplicasOnCreation:              defaultReplicasOnCreation(res.ResourceMeta, releaseNamespace),
			Ownership:                              ownership(res.ResourceMeta, releaseNamespace),
			DeleteOnSucceeded:                      deleteOnSucceeded(res.ResourceMeta),
			DeleteOnFailed:                         deleteOnFailed(res.ResourceMeta),
			KeepOnDelete:                           KeepOnDelete(res.ResourceMeta, releaseNamespace),
			FailMode:                               failMode(res.ResourceMeta),
			FailuresAllowed:                        failuresAllowed(res.Unstruct),
			IgnoreReadinessProbeFailsForContainers: ignoreReadinessProbeFailsForContainers(res.ResourceMeta),
			LogRegex:                               logRegex(res.ResourceMeta),
			LogRegexesForContainers:                logRegexesForContainers(res.ResourceMeta),
			NoActivityTimeout:                      noActivityTimeout(res.ResourceMeta),
			ShowLogsOnlyForContainers:              showLogsOnlyForContainers(res.ResourceMeta),
			ShowServiceMessages:                    showServiceMessages(res.ResourceMeta),
			ShowLogsOnlyForNumberOfReplicas:        showLogsOnlyForNumberOfReplicas(res.ResourceMeta),
			SkipLogs:                               skipLogs(res.ResourceMeta),
			SkipLogsForContainers:                  skipLogsForContainers(res.ResourceMeta),
			TrackTerminationMode:                   trackTerminationMode(res.ResourceMeta),
			Weight:                                 weight(res.ResourceMeta, len(manIntDeps) > 0),
			ManualInternalDependencies:             manIntDeps,
			AutoInternalDependencies:               DetectInternalDependencies(res.Unstruct),
			ExternalDependencies:                   extDeps,
			DeployConditions:                       deplConditions,
			Stage:                                  stage,
		})
	}

	return instResources, nil
}

type InstallableResource struct {
	*ResourceSpec

	Ownership                              common.Ownership
	Recreate                               bool
	DefaultReplicasOnCreation              *int
	DeleteOnSucceeded                      bool
	DeleteOnFailed                         bool
	KeepOnDelete                           bool
	FailMode                               multitrack.FailMode
	FailuresAllowed                        int
	IgnoreReadinessProbeFailsForContainers map[string]time.Duration
	LogRegex                               *regexp.Regexp
	LogRegexesForContainers                map[string]*regexp.Regexp
	NoActivityTimeout                      time.Duration
	ShowLogsOnlyForContainers              []string
	ShowServiceMessages                    bool
	ShowLogsOnlyForNumberOfReplicas        int
	SkipLogs                               bool
	SkipLogsForContainers                  []string
	TrackTerminationMode                   multitrack.TrackTerminationMode
	Weight                                 *int
	ManualInternalDependencies             []*InternalDependency
	AutoInternalDependencies               []*InternalDependency
	ExternalDependencies                   []*ExternalDependency
	DeployConditions                       map[common.On][]common.Stage
	Stage                                  common.Stage
}

type DeletableResourceOptions struct{}

func NewDeletableResource(meta *meta.ResourceMeta, releaseNamespace string, stage common.Stage, opts DeletableResourceOptions) *DeletableResource {
	var keep bool
	if err := ValidateResourcePolicy(meta); err != nil {
		keep = true
	} else {
		keep = KeepOnDelete(meta, releaseNamespace)
	}

	var owner common.Ownership
	if err := validateOwnership(meta); err != nil {
		owner = common.OwnershipRelease
	} else {
		owner = ownership(meta, releaseNamespace)
	}

	return &DeletableResource{
		ResourceMeta: meta,
		Ownership:    owner,
		KeepOnDelete: keep,
	}
}

type DeletableResource struct {
	*meta.ResourceMeta

	Ownership    common.Ownership
	KeepOnDelete bool
	Stage        common.Stage
}
