package resource

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/werf/kubedog/pkg/dyntracker/statestore"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/resource/spec"
)

// Represent a Kubernetes resource that can be installed. Higher level than ResourceSpec, but lower
// level than InstallableResourceInfo. If something can be computed on this level instead of doing
// this on higher levels, it's better to do it here.
type InstallableResource struct {
	*spec.ResourceSpec `json:"resourceSpec"`

	Ownership                              common.Ownership                `json:"ownership"`
	Recreate                               bool                            `json:"recreate"`
	RecreateOnImmutable                    bool                            `json:"recreateOnImmutable"`
	DefaultReplicasOnCreation              *int                            `json:"defaultReplicasOnCreation,omitempty"`
	DeleteOnSucceeded                      bool                            `json:"deleteOnSucceeded"`
	DeleteOnFailed                         bool                            `json:"deleteOnFailed"`
	KeepOnDelete                           bool                            `json:"keepOnDelete"`
	FailMode                               statestore.FailMode             `json:"failMode"`
	FailuresAllowed                        int                             `json:"failuresAllowed"`
	IgnoreReadinessProbeFailsForContainers map[string]time.Duration        `json:"ignoreReadinessProbeFailsForContainers,omitempty"`
	LogRegex                               *regexp.Regexp                  `json:"logRegex"`
	LogRegexesForContainers                map[string]*regexp.Regexp       `json:"logRegexesForContainers"`
	NoActivityTimeout                      time.Duration                   `json:"noActivityTimeout"`
	ShowLogsOnlyForContainers              []string                        `json:"showLogsOnlyForContainers,omitempty"`
	ShowServiceMessages                    bool                            `json:"showServiceMessages"`
	ShowLogsOnlyForNumberOfReplicas        int                             `json:"showLogsOnlyForNumberOfReplicas"`
	SkipLogs                               bool                            `json:"skipLogs"`
	SkipLogsForContainers                  []string                        `json:"skipLogsForContainers,omitempty"`
	SkipLogsRegex                          *regexp.Regexp                  `json:"skipLogsRegex"`
	SkipLogsRegexForContainers             map[string]*regexp.Regexp       `json:"skipLogsRegexForContainers"`
	TrackTerminationMode                   statestore.TrackTerminationMode `json:"trackTerminationMode"`
	Weight                                 *int                            `json:"weight,omitempty"`
	ManualDependencies                     []*Dependency                   `json:"manualDependencies,omitempty"`
	AutoInternalDependencies               []*Dependency                   `json:"autoInternalDependencies,omitempty"`
	DeployConditions                       map[common.On][]common.Stage    `json:"deployConditions"`
	DeletePropagation                      metav1.DeletionPropagation      `json:"deletePropagation"`
}

// Construct an InstallableResource from a ResourceSpec. Must never contact the cluster, because
// this is called even when no cluster access allowed.
func NewInstallableResource(ctx context.Context, res *spec.ResourceSpec, otherResourceSpecs []*spec.ResourceSpec, releaseNamespace string, opts InstallableResourceOptions) (*InstallableResource, error) {
	otherResourceMetaList := lo.Map(otherResourceSpecs, func(resSpec *spec.ResourceSpec, _ int) *spec.ResourceMeta {
		return resSpec.ResourceMeta
	})

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

	if err := validateDeployDependencies(res.ResourceMeta, otherResourceMetaList); err != nil {
		return nil, fmt.Errorf("validate deploy dependencies: %w", err)
	}

	if err := validateDeleteDependencies(res.ResourceMeta, otherResourceMetaList); err != nil {
		return nil, fmt.Errorf("validate delete dependencies: %w", err)
	}

	warnDeprecatedExternalDependencies(ctx, res.ResourceMeta)

	if err := validateSensitive(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate sensitive: %w", err)
	}

	if err := validateDeployOn(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate deploy stage: %w", err)
	}

	if err := validateOwnership(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate ownership: %w", err)
	}

	if err := validateDeletePropagation(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate delete propagation: %w", err)
	}

	manDeps := manualDeployDependencies(res.ResourceMeta, otherResourceMetaList)
	internalDeps := lo.Filter(manDeps, func(item *Dependency, _ int) bool {
		return !item.External
	})

	return &InstallableResource{
		ResourceSpec:                           res,
		AutoInternalDependencies:               internalDeployDependencies(res.Unstruct),
		DefaultReplicasOnCreation:              defaultReplicasOnCreation(res.ResourceMeta, releaseNamespace),
		DeleteOnFailed:                         deleteOnFailed(res.ResourceMeta),
		DeleteOnSucceeded:                      deleteOnSucceeded(res.ResourceMeta),
		DeletePropagation:                      deletePropagation(res.ResourceMeta, opts.DefaultDeletePropagation),
		DeployConditions:                       deployConditions(res.ResourceMeta, len(internalDeps) > 0),
		FailMode:                               failMode(res.ResourceMeta),
		FailuresAllowed:                        failuresAllowed(res.Unstruct),
		IgnoreReadinessProbeFailsForContainers: ignoreReadinessProbeFailsForContainers(res.ResourceMeta),
		KeepOnDelete:                           KeepOnDelete(res.ResourceMeta, releaseNamespace),
		LogRegex:                               logRegex(res.ResourceMeta),
		LogRegexesForContainers:                logRegexesForContainers(res.ResourceMeta),
		ManualDependencies:                     manDeps,
		NoActivityTimeout:                      noActivityTimeout(res.ResourceMeta),
		Ownership:                              ownership(res.ResourceMeta, releaseNamespace, res.StoreAs),
		Recreate:                               recreate(res.ResourceMeta),
		RecreateOnImmutable:                    recreateOnImmutable(res.ResourceMeta),
		ShowLogsOnlyForContainers:              showLogsOnlyForContainers(res.ResourceMeta),
		ShowLogsOnlyForNumberOfReplicas:        showLogsOnlyForNumberOfReplicas(res.ResourceMeta),
		ShowServiceMessages:                    showServiceMessages(res.ResourceMeta),
		SkipLogs:                               skipLogs(res.ResourceMeta, opts.NoPodLogs),
		SkipLogsForContainers:                  skipLogsForContainers(res.ResourceMeta),
		SkipLogsRegex:                          skipLogRegex(res.ResourceMeta),
		SkipLogsRegexForContainers:             skipLogRegexesForContainers(res.ResourceMeta),
		TrackTerminationMode:                   trackTerminationMode(res.ResourceMeta),
		Weight:                                 weight(res.ResourceMeta, len(internalDeps) > 0),
	}, nil
}

type InstallableResourceOptions struct {
	DefaultDeletePropagation metav1.DeletionPropagation
	NoPodLogs                bool
}

// Represent a Kubernetes resource that can be deleted. Higher level than ResourceMeta, but lower
// level than DeletableResourceInfo. If something can be computed on this level instead of doing
// this on higher levels, it's better to do it here.
type DeletableResource struct {
	*spec.ResourceMeta

	AutoInternalDependencies []*Dependency
	DeletePropagation        metav1.DeletionPropagation
	KeepOnDelete             bool
	ManualDependencies       []*Dependency
	Ownership                common.Ownership
}

// Construct a DeletableResource from a ResourceSpec. Must never contact the cluster, because
// this is called even when no cluster access allowed.
func NewDeletableResource(resourceSpec *spec.ResourceSpec, otherResourceSpecs []*spec.ResourceSpec, releaseNamespace string, opts DeletableResourceOptions) (*DeletableResource, error) {
	var keep bool
	if err := ValidateResourcePolicy(resourceSpec.ResourceMeta); err != nil {
		keep = true
	} else {
		keep = KeepOnDelete(resourceSpec.ResourceMeta, releaseNamespace)
	}

	var owner common.Ownership
	if err := validateOwnership(resourceSpec.ResourceMeta); err != nil {
		owner = common.OwnershipRelease
	} else {
		owner = ownership(resourceSpec.ResourceMeta, releaseNamespace, resourceSpec.StoreAs)
	}

	var delPropagation metav1.DeletionPropagation
	if err := validateDeletePropagation(resourceSpec.ResourceMeta); err != nil {
		delPropagation = common.DefaultDeletePropagation
	} else {
		delPropagation = deletePropagation(resourceSpec.ResourceMeta, opts.DefaultDeletePropagation)
	}

	otherResourceMetaList := lo.Map(otherResourceSpecs, func(resSpec *spec.ResourceSpec, _ int) *spec.ResourceMeta {
		return resSpec.ResourceMeta
	})

	if err := validateDeleteDependencies(resourceSpec.ResourceMeta, otherResourceMetaList); err != nil {
		return nil, fmt.Errorf("validate delete dependencies: %w", err)
	}

	unstructList := lo.Map(otherResourceSpecs, func(resSpec *spec.ResourceSpec, _ int) *unstructured.Unstructured {
		return resSpec.Unstruct
	})

	return &DeletableResource{
		ResourceMeta:             resourceSpec.ResourceMeta,
		AutoInternalDependencies: internalDeleteDependencies(resourceSpec.Unstruct, unstructList),
		DeletePropagation:        delPropagation,
		KeepOnDelete:             keep,
		ManualDependencies:       manualDeleteDependencies(resourceSpec.ResourceMeta, otherResourceMetaList),
		Ownership:                owner,
	}, nil
}

type DeletableResourceOptions struct {
	DefaultDeletePropagation metav1.DeletionPropagation
}

type BuildResourcesOptions struct {
	DefaultDeletePropagation metav1.DeletionPropagation
	NoPodLogs                bool
}

// Build Installable/DeletableResources from ResourceSpecs. Resulting Resources can be used to
// construct Installable/DeletableResourceInfos later. Must never contact the cluster, because this
// is called even when no cluster access allowed.
func BuildResources(ctx context.Context, deployType common.DeployType, releaseNamespace string, prevRelResSpecs, newRelResSpecs []*spec.ResourceSpec, patchers []spec.ResourcePatcher, opts BuildResourcesOptions) ([]*InstallableResource, []*DeletableResource, error) {
	var prevRelDelResources []*DeletableResource
	for _, resSpec := range prevRelResSpecs {
		deletableRes, err := NewDeletableResource(resSpec, lo.Without(prevRelResSpecs, resSpec), releaseNamespace, DeletableResourceOptions{
			DefaultDeletePropagation: opts.DefaultDeletePropagation,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("construct deletable resource: %w", err)
		}

		prevRelDelResources = append(prevRelDelResources, deletableRes)
	}

	var prevRelInstResources []*InstallableResource
	for _, resSpec := range prevRelResSpecs {
		installableResource, err := NewInstallableResource(ctx, resSpec, lo.Without(prevRelResSpecs, resSpec), releaseNamespace, InstallableResourceOptions{
			DefaultDeletePropagation: opts.DefaultDeletePropagation,
			NoPodLogs:                opts.NoPodLogs,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("construct installable resource: %w", err)
		}

		prevRelInstResources = append(prevRelInstResources, installableResource)
	}

	var newRelInstResources []*InstallableResource
	for _, resSpec := range newRelResSpecs {
		installableResource, err := NewInstallableResource(ctx, resSpec, lo.Without(newRelResSpecs, resSpec), releaseNamespace, InstallableResourceOptions{
			DefaultDeletePropagation: opts.DefaultDeletePropagation,
			NoPodLogs:                opts.NoPodLogs,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("construct installable resource: %w", err)
		}

		newRelInstResources = append(newRelInstResources, installableResource)
	}

	var filteredPrevRelInstResources []*InstallableResource
	if deployType == common.DeployTypeUninstall {
		filteredPrevRelInstResources = lo.Filter(prevRelInstResources, func(instRes *InstallableResource, _ int) bool {
			if len(instRes.DeployConditions) == 0 {
				return false
			}

			return len(instRes.DeployConditions[common.InstallOnDelete]) > 0
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
		case common.OwnershipAnyone:
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
			return len(instRes.DeployConditions[common.InstallOnInstall]) > 0
		case common.DeployTypeUpgrade:
			return len(instRes.DeployConditions[common.InstallOnUpgrade]) > 0
		case common.DeployTypeRollback:
			return len(instRes.DeployConditions[common.InstallOnRollback]) > 0
		default:
			panic("unexpected deploy type")
		}
	})

	var patchedResSpecs []*spec.ResourceSpec
	for _, r := range append(filteredPrevRelInstResources, filteredNewRelInstResources...) {
		unstruct := r.Unstruct

		var deepCopied bool
		for _, patcher := range patchers {
			if matched, err := patcher.Match(ctx, &spec.ResourcePatcherResourceInfo{
				Obj:       unstruct,
				Ownership: r.Ownership,
			}); err != nil {
				return nil, nil, fmt.Errorf("match deployable resource for patching by %q: %w", patcher.Type(), err)
			} else if !matched {
				continue
			}

			if !deepCopied {
				unstruct = unstruct.DeepCopy()
				deepCopied = true
			}

			patchedObj, err := patcher.Patch(ctx, &spec.ResourcePatcherResourceInfo{
				Obj:       unstruct,
				Ownership: r.Ownership,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("patch deployable resource by %q: %w", patcher.Type(), err)
			}

			unstruct = patchedObj
		}

		resSpec := spec.NewResourceSpec(unstruct, releaseNamespace, spec.ResourceSpecOptions{
			StoreAs:  r.StoreAs,
			FilePath: r.FilePath,
		})
		patchedResSpecs = append(patchedResSpecs, resSpec)
	}

	var instResources []*InstallableResource
	for _, resSpec := range patchedResSpecs {
		instRes, err := NewInstallableResource(ctx, resSpec, lo.Without(patchedResSpecs, resSpec), releaseNamespace, InstallableResourceOptions{
			DefaultDeletePropagation: opts.DefaultDeletePropagation,
			NoPodLogs:                opts.NoPodLogs,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("construct deployable resource: %w", err)
		}

		instResources = append(instResources, instRes)
	}

	sort.SliceStable(instResources, func(i, j int) bool {
		return spec.ResourceSpecSortHandler(instResources[i].ResourceSpec, instResources[j].ResourceSpec)
	})

	sort.SliceStable(delResources, func(i, j int) bool {
		return spec.ResourceMetaSortHandler(delResources[i].ResourceMeta, delResources[j].ResourceMeta)
	})

	return instResources, delResources, nil
}
