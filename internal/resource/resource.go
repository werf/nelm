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

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/kube"
	"github.com/werf/nelm/internal/resource/spec"
	"github.com/werf/nelm/pkg/common"
)

type InstallableResource struct {
	*spec.ResourceSpec

	Ownership                              common.Ownership
	Recreate                               bool
	RecreateOnImmutable                    bool
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
	DeletePropagation                      metav1.DeletionPropagation
}

type InstallableResourceOptions struct {
	Remote                   bool
	DefaultDeletePropagation metav1.DeletionPropagation
}

func NewInstallableResource(res *spec.ResourceSpec, releaseNamespace string, clientFactory kube.ClientFactorier, opts InstallableResourceOptions) (*InstallableResource, error) {
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

	if err := validateDeletePropagation(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate delete propagation: %w", err)
	}

	extDeps, err := externalDependencies(res.ResourceMeta, releaseNamespace, clientFactory, opts.Remote)
	if err != nil {
		return nil, fmt.Errorf("get external dependencies: %w", err)
	}

	manIntDeps := manualInternalDependencies(res.ResourceMeta)

	return &InstallableResource{
		ResourceSpec:                           res,
		Recreate:                               recreate(res.ResourceMeta),
		RecreateOnImmutable:                    recreateOnImmutable(res.ResourceMeta),
		DefaultReplicasOnCreation:              defaultReplicasOnCreation(res.ResourceMeta, releaseNamespace),
		Ownership:                              ownership(res.ResourceMeta, releaseNamespace, res.StoreAs),
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
		AutoInternalDependencies:               internalDependencies(res.Unstruct),
		ExternalDependencies:                   extDeps,
		DeployConditions:                       deployConditions(res.ResourceMeta),
		DeletePropagation:                      deletePropagation(res.ResourceMeta, opts.DefaultDeletePropagation),
	}, nil
}

type DeletableResource struct {
	*spec.ResourceMeta

	Ownership         common.Ownership
	KeepOnDelete      bool
	DeletePropagation metav1.DeletionPropagation
}

type DeletableResourceOptions struct {
	DefaultDeletePropagation metav1.DeletionPropagation
}

func NewDeletableResource(spec *spec.ResourceSpec, releaseNamespace string, opts DeletableResourceOptions) *DeletableResource {
	var keep bool
	if err := ValidateResourcePolicy(spec.ResourceMeta); err != nil {
		keep = true
	} else {
		keep = KeepOnDelete(spec.ResourceMeta, releaseNamespace)
	}

	var owner common.Ownership
	if err := validateOwnership(spec.ResourceMeta); err != nil {
		owner = common.OwnershipRelease
	} else {
		owner = ownership(spec.ResourceMeta, releaseNamespace, spec.StoreAs)
	}

	var delPropagation metav1.DeletionPropagation
	if err := validateDeletePropagation(spec.ResourceMeta); err != nil {
		delPropagation = common.DefaultDeletePropagation
	} else {
		delPropagation = deletePropagation(spec.ResourceMeta, opts.DefaultDeletePropagation)
	}

	return &DeletableResource{
		ResourceMeta:      spec.ResourceMeta,
		Ownership:         owner,
		KeepOnDelete:      keep,
		DeletePropagation: delPropagation,
	}
}

type BuildResourcesOptions struct {
	Remote                   bool
	DefaultDeletePropagation metav1.DeletionPropagation
}

func BuildResources(ctx context.Context, deployType common.DeployType, releaseNamespace string, prevRelResSpecs, newRelResSpecs []*spec.ResourceSpec, patchers []spec.ResourcePatcher, clientFactory kube.ClientFactorier, opts BuildResourcesOptions) ([]*InstallableResource, []*DeletableResource, error) {
	var prevRelDelResources []*DeletableResource
	for _, resSpec := range prevRelResSpecs {
		deletableRes := NewDeletableResource(resSpec, releaseNamespace, DeletableResourceOptions{
			DefaultDeletePropagation: opts.DefaultDeletePropagation,
		})
		prevRelDelResources = append(prevRelDelResources, deletableRes)
	}

	var prevRelInstResources []*InstallableResource
	for _, resSpec := range prevRelResSpecs {
		installableResource, err := NewInstallableResource(resSpec, releaseNamespace, clientFactory, InstallableResourceOptions{
			Remote:                   opts.Remote,
			DefaultDeletePropagation: opts.DefaultDeletePropagation,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("construct installable resource: %w", err)
		}

		prevRelInstResources = append(prevRelInstResources, installableResource)
	}

	var newRelInstResources []*InstallableResource
	for _, resSpec := range newRelResSpecs {
		installableResource, err := NewInstallableResource(resSpec, releaseNamespace, clientFactory, InstallableResourceOptions{
			Remote:                   opts.Remote,
			DefaultDeletePropagation: opts.DefaultDeletePropagation,
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

	var instResources []*InstallableResource
	for _, r := range append(filteredPrevRelInstResources, filteredNewRelInstResources...) {
		instRes := r

		var deepCopied bool
		for _, patcher := range patchers {
			if matched, err := patcher.Match(ctx, &spec.ResourcePatcherResourceInfo{
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

			patchedObj, err := patcher.Patch(ctx, &spec.ResourcePatcherResourceInfo{
				Obj:       unstruct,
				Ownership: instRes.Ownership,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("patch deployable resource by %q: %w", patcher.Type(), err)
			}

			resSpec := spec.NewResourceSpec(patchedObj, releaseNamespace, spec.ResourceSpecOptions{
				StoreAs:  instRes.StoreAs,
				FilePath: instRes.FilePath,
			})

			instRes, err = NewInstallableResource(resSpec, releaseNamespace, clientFactory, InstallableResourceOptions{
				Remote:                   opts.Remote,
				DefaultDeletePropagation: opts.DefaultDeletePropagation,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("construct deployable resource from patched object by %q: %w", patcher.Type(), err)
			}
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
