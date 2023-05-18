package plan

import (
	"context"
	"fmt"
	"sort"
	"strings"

	legacyRelease "helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/resource"
)

func NewDeployPlanBuilder(
	deployType DeployType,
	releaseNs struct {
		Local    *resource.UnmanagedResource
		Live     *resource.GenericResource
		Desired  *resource.GenericResource
		UpToDate bool
		Existing bool
	},
	pendingRel *legacyRelease.Release,
	succeededRel *legacyRelease.Release,
) *DeployPlanBuilder {
	return &DeployPlanBuilder{
		deployType:             deployType,
		pendingRelease:         pendingRel,
		succeededRelease:       succeededRel,
		releaseNamespace:       releaseNs,
		createReleaseNamespace: true,
	}
}

type DeployPlanBuilder struct {
	deployType            DeployType
	pendingRelease        *legacyRelease.Release
	succeededRelease      *legacyRelease.Release
	supersededPrevRelease *legacyRelease.Release
	prevReleaseDeployed   bool

	createReleaseNamespace bool
	releaseNamespace       struct {
		Local    *resource.UnmanagedResource
		Live     *resource.GenericResource
		Desired  *resource.GenericResource
		UpToDate bool
		Existing bool
	}

	preloadedCRDs struct {
		UpToDate []struct {
			Local *resource.UnmanagedResource
			Live  *resource.GenericResource
		}
		Outdated []struct {
			Local   *resource.UnmanagedResource
			Live    *resource.GenericResource
			Desired *resource.GenericResource
		}
		OutdatedImmutable []struct {
			Local *resource.UnmanagedResource
			Live  *resource.GenericResource
		}
		NonExisting []struct {
			Local   *resource.UnmanagedResource
			Desired *resource.GenericResource
		}
	}

	matchedHelmHooks struct {
		UpToDate []struct {
			Local *resource.HelmHook
			Live  *resource.GenericResource
		}
		Outdated []struct {
			Local   *resource.HelmHook
			Live    *resource.GenericResource
			Desired *resource.GenericResource
		}
		OutdatedImmutable []struct {
			Local *resource.HelmHook
			Live  *resource.GenericResource
		}
		Unsupported []struct {
			Local *resource.HelmHook
		}
		NonExisting []struct {
			Local   *resource.HelmHook
			Desired *resource.GenericResource
		}
	}

	helmResources struct {
		UpToDate []struct {
			Local *resource.HelmResource
			Live  *resource.GenericResource
		}
		Outdated []struct {
			Local   *resource.HelmResource
			Live    *resource.GenericResource
			Desired *resource.GenericResource
		}
		OutdatedImmutable []struct {
			Local *resource.HelmResource
			Live  *resource.GenericResource
		}
		Unsupported []struct {
			Local *resource.HelmResource
		}
		NonExisting []struct {
			Local   *resource.HelmResource
			Desired *resource.GenericResource
		}
	}

	prevReleaseHelmResources struct {
		Existing []struct {
			Local *resource.HelmResource
			Live  *resource.GenericResource
		}
		NonExisting []*resource.HelmResource
	}
}

func (b *DeployPlanBuilder) WithPreloadedCRDs(crds struct {
	UpToDate []struct {
		Local *resource.UnmanagedResource
		Live  *resource.GenericResource
	}
	Outdated []struct {
		Local   *resource.UnmanagedResource
		Live    *resource.GenericResource
		Desired *resource.GenericResource
	}
	OutdatedImmutable []struct {
		Local *resource.UnmanagedResource
		Live  *resource.GenericResource
	}
	NonExisting []struct {
		Local   *resource.UnmanagedResource
		Desired *resource.GenericResource
	}
}) *DeployPlanBuilder {
	b.preloadedCRDs = crds
	return b
}

func (b *DeployPlanBuilder) WithMatchedHelmHooks(hooks struct {
	UpToDate []struct {
		Local *resource.HelmHook
		Live  *resource.GenericResource
	}
	Outdated []struct {
		Local   *resource.HelmHook
		Live    *resource.GenericResource
		Desired *resource.GenericResource
	}
	OutdatedImmutable []struct {
		Local *resource.HelmHook
		Live  *resource.GenericResource
	}
	Unsupported []struct {
		Local *resource.HelmHook
	}
	NonExisting []struct {
		Local   *resource.HelmHook
		Desired *resource.GenericResource
	}
}) *DeployPlanBuilder {
	b.matchedHelmHooks = hooks
	return b
}

func (b *DeployPlanBuilder) WithHelmResources(resources struct {
	UpToDate []struct {
		Local *resource.HelmResource
		Live  *resource.GenericResource
	}
	Outdated []struct {
		Local   *resource.HelmResource
		Live    *resource.GenericResource
		Desired *resource.GenericResource
	}
	OutdatedImmutable []struct {
		Local *resource.HelmResource
		Live  *resource.GenericResource
	}
	Unsupported []struct {
		Local *resource.HelmResource
	}
	NonExisting []struct {
		Local   *resource.HelmResource
		Desired *resource.GenericResource
	}
}) *DeployPlanBuilder {
	b.helmResources = resources
	return b
}

func (b *DeployPlanBuilder) WithPreviousReleaseHelmResources(resources struct {
	Existing []struct {
		Local *resource.HelmResource
		Live  *resource.GenericResource
	}
	NonExisting []*resource.HelmResource
}) *DeployPlanBuilder {
	b.prevReleaseHelmResources = resources
	return b
}

func (b *DeployPlanBuilder) WithSupersededPreviousRelease(rel *legacyRelease.Release) *DeployPlanBuilder {
	b.supersededPrevRelease = rel
	return b
}

func (b *DeployPlanBuilder) WithPreviousReleaseDeployed(deployed bool) *DeployPlanBuilder {
	b.prevReleaseDeployed = deployed
	return b
}

func (b *DeployPlanBuilder) Build(ctx context.Context) (deployPlan *Plan, referencesToCleanupOnFailure []resource.Referencer, err error) {
	hasPreloadedCRDs := b.preloadedCRDs.UpToDate != nil || b.preloadedCRDs.Outdated != nil || b.preloadedCRDs.OutdatedImmutable != nil || b.preloadedCRDs.NonExisting != nil
	hasMatchHelmHooks := b.matchedHelmHooks.UpToDate != nil || b.matchedHelmHooks.Outdated != nil || b.matchedHelmHooks.OutdatedImmutable != nil || b.matchedHelmHooks.Unsupported != nil || b.matchedHelmHooks.NonExisting != nil
	hasHelmResources := b.helmResources.UpToDate != nil || b.helmResources.Outdated != nil || b.helmResources.OutdatedImmutable != nil || b.helmResources.Unsupported != nil || b.helmResources.NonExisting != nil

	var referencesToCleanup []resource.Referencer
	if orphanedHelmResources, err := b.detectOrphanedResources(ctx); err != nil {
		return nil, nil, fmt.Errorf("error detecting orphaned helm resources: %w", err)
	} else {
		referencesToCleanup = append(referencesToCleanup, resource.CastToReferencers(orphanedHelmResources)...)
		referencesToCleanupOnFailure = append(referencesToCleanupOnFailure, resource.CastToReferencers(orphanedHelmResources)...)
	}

	var phaseReleaseNamespace *Phase
	if b.createReleaseNamespace {
		operations := b.buildReleaseNamespaceOperations()

		if operations != nil {
			phaseReleaseNamespace = NewPhase(PhaseTypeCreateReleaseNamespace).AddOperations(operations...)
		}
	}

	var phasePreloadedCRDs *Phase
	if hasPreloadedCRDs {
		if b.preloadedCRDs.OutdatedImmutable != nil {
			var immutableCRDs []string
			for _, crd := range b.preloadedCRDs.OutdatedImmutable {
				immutableCRDs = append(immutableCRDs, crd.Local.String())
			}

			return nil, nil, fmt.Errorf("you are trying to update following preloaded CRDs with immutable fields that can't be updated: %s", strings.Join(immutableCRDs, ", "))
		}

		operations := b.buildPreloadedCRDsOperations()

		if operations != nil {
			phasePreloadedCRDs = NewPhase(PhaseTypeDeployPreloadedCRDs).AddOperations(operations...)
		}
	}

	var phaseHelmResources *Phase
	if hasHelmResources {
		if b.helmResources.OutdatedImmutable != nil {
			var immutableResources []string
			for _, res := range b.helmResources.OutdatedImmutable {
				immutableResources = append(immutableResources, res.Local.String())
			}

			return nil, nil, fmt.Errorf("you are trying to update following helm resources with immutable fields that can't be updated: %s", strings.Join(immutableResources, ", "))
		}

		operations, err := b.buildHelmResourcesOperations()
		if err != nil {
			return nil, nil, fmt.Errorf("error building operations for helm resources: %w", err)
		}

		if operations != nil {
			phaseHelmResources = NewPhase(PhaseTypeDeployHelmResources).AddOperations(operations...)
		}
	}

	var (
		phasePreHelmHooks       *Phase
		phasePostHelmHooks      *Phase
		nothingToCreateOrUpdate bool
	)
	if hasMatchHelmHooks {
		if b.matchedHelmHooks.OutdatedImmutable != nil {
			var immutableHooks []string
			for _, hook := range b.matchedHelmHooks.OutdatedImmutable {
				if !hook.Local.HasDeletePolicy(common.HelmHookDeletePolicyBeforeCreation) {
					immutableHooks = append(immutableHooks, hook.Local.String())
				}
			}

			if len(immutableHooks) > 0 {
				return nil, nil, fmt.Errorf("you are trying to update following helm hooks with immutable fields that can't be updated: %s", strings.Join(immutableHooks, ", "))
			}
		}

		var preHelmHookType, postHelmHookType common.HelmHookType
		switch b.deployType {
		case DeployTypeInitial, DeployTypeInstall:
			preHelmHookType = common.HelmHookTypePreInstall
			postHelmHookType = common.HelmHookTypePostInstall
		case DeployTypeUpgrade:
			preHelmHookType = common.HelmHookTypePreUpgrade
			postHelmHookType = common.HelmHookTypePostUpgrade
		case DeployTypeRollback:
			preHelmHookType = common.HelmHookTypePreRollback
			postHelmHookType = common.HelmHookTypePostRollback
		}

		preHelmHooksByWeight := b.filterAndGroupHelmHooksByWeight(preHelmHookType)
		postHelmHooksByWeight := b.filterAndGroupHelmHooksByWeight(postHelmHookType)

		var helmHooksChanged bool
		for _, weightGroup := range append(preHelmHooksByWeight, postHelmHooksByWeight...) {
			for _, hook := range weightGroup {
				if !hook.Existing || hook.Outdated || hook.OutdatedImmutable || hook.Unsupported {
					helmHooksChanged = true
					break
				}
			}
		}

		nothingToCreateOrUpdate = phaseReleaseNamespace == nil && phasePreloadedCRDs == nil && phaseHelmResources == nil && !helmHooksChanged

		preOperations, preHooksToCleanupOnFailure, err := b.buildHelmHooksOperations(preHelmHookType, nothingToCreateOrUpdate)
		if err != nil {
			return nil, nil, fmt.Errorf("error building operations for pre helm hooks: %w", err)
		}

		if preOperations != nil {
			phasePreHelmHooks = NewPhase(PhaseTypeRunPreHelmHooks).AddOperations(preOperations...)

			if preHooksToCleanupOnFailure != nil {
				referencesToCleanupOnFailure = append(referencesToCleanupOnFailure, resource.CastToReferencers(preHooksToCleanupOnFailure)...)
			}
		}

		postOperations, postHooksToCleanupOnFailure, err := b.buildHelmHooksOperations(postHelmHookType, nothingToCreateOrUpdate)
		if err != nil {
			return nil, nil, fmt.Errorf("error building operations for post helm hooks: %w", err)
		}

		if postOperations != nil {
			phasePostHelmHooks = NewPhase(PhaseTypeRunPostHelmHooks).AddOperations(postOperations...)

			if postHooksToCleanupOnFailure != nil {
				referencesToCleanupOnFailure = append(referencesToCleanupOnFailure, resource.CastToReferencers(postHooksToCleanupOnFailure)...)
			}
		}
	}

	var phaseCleanup *Phase
	if referencesToCleanup != nil {
		var operations []Operation

		deleteOp := NewOperationDelete().AddTargets(referencesToCleanup...)
		trackOp := NewOperationTrackDeletion().AddTargets(deleteOp.Targets...)
		operations = append(operations, deleteOp, trackOp)

		phaseCleanup = NewPhase(PhaseTypeCleanup).AddOperations(operations...)
	}

	var phasePendingRelease *Phase
	var phaseSucceededRelease *Phase
	var phaseSupersededRelease *Phase
	if phaseReleaseNamespace != nil || phasePreloadedCRDs != nil || phaseHelmResources != nil || phasePreHelmHooks != nil || phasePostHelmHooks != nil || phaseCleanup != nil {
		createPendingReleaseOp := NewOperationCreateReleases().AddReleases(b.pendingRelease)
		phasePendingRelease = NewPhase(PhaseTypeCreateRelease).AddOperations(createPendingReleaseOp)

		updatePendingReleaseToSucceededOp := NewOperationUpdateReleases().AddReleases(b.succeededRelease)
		phaseSucceededRelease = NewPhase(PhaseTypeSucceedRelease).AddOperations(updatePendingReleaseToSucceededOp)

		if b.supersededPrevRelease != nil {
			supersedePreviousReleaseOp := NewOperationUpdateReleases().AddReleases(b.supersededPrevRelease)
			phaseSupersededRelease = NewPhase(PhaseTypeSupersedePreviousRelease).AddOperations(supersedePreviousReleaseOp)
		}
	}

	deployPlan = NewPlan(PlanTypeDeploy)

	if phaseReleaseNamespace != nil {
		deployPlan.AddPhases(phaseReleaseNamespace)
	}
	if phasePendingRelease != nil {
		deployPlan.AddPhases(phasePendingRelease)
	}
	if phasePreloadedCRDs != nil {
		deployPlan.AddPhases(phasePreloadedCRDs)
	}
	if phasePreHelmHooks != nil {
		deployPlan.AddPhases(phasePreHelmHooks)
	}
	if phaseHelmResources != nil {
		deployPlan.AddPhases(phaseHelmResources)
	}
	if phasePostHelmHooks != nil {
		deployPlan.AddPhases(phasePostHelmHooks)
	}
	if phaseCleanup != nil {
		deployPlan.AddPhases(phaseCleanup)
	}
	if phaseSucceededRelease != nil {
		deployPlan.AddPhases(phaseSucceededRelease)
	}
	if phaseSupersededRelease != nil {
		deployPlan.AddPhases(phaseSupersededRelease)
	}

	return deployPlan, referencesToCleanupOnFailure, nil
}

func (b *DeployPlanBuilder) buildReleaseNamespaceOperations() []Operation {
	var operations []Operation

	if !b.releaseNamespace.Existing {
		createOp := NewOperationCreate().AddTargets(b.releaseNamespace.Local)
		trackOp := NewOperationTrackUnmanagedResourcesReadiness().AddTargets(b.releaseNamespace.Local)
		operations = append(operations, createOp, trackOp)
	} else if !b.releaseNamespace.UpToDate {
		updateOp := NewOperationUpdate().AddTargets(b.releaseNamespace.Local)
		trackOp := NewOperationTrackUnmanagedResourcesReadiness().AddTargets(b.releaseNamespace.Local)
		operations = append(operations, updateOp, trackOp)
	}

	return operations
}

func (b *DeployPlanBuilder) buildPreloadedCRDsOperations() []Operation {
	trackOp := NewOperationTrackUnmanagedResourcesReadiness()

	createOp := NewOperationCreate()
	for _, res := range b.preloadedCRDs.NonExisting {
		createOp.AddTargets(res.Local)
		trackOp.AddTargets(res.Local)
	}

	updateOp := NewOperationUpdate()
	for _, res := range b.preloadedCRDs.Outdated {
		updateOp.AddTargets(res.Local)
		trackOp.AddTargets(res.Local)
	}

	if !b.prevReleaseDeployed {
		for _, res := range b.preloadedCRDs.UpToDate {
			trackOp.AddTargets(res.Local)
		}
	}

	var operations []Operation

	if createOp.Targets != nil {
		operations = append(operations, createOp)
	}

	if updateOp.Targets != nil {
		operations = append(operations, updateOp)
	}

	if trackOp.Targets != nil {
		operations = append(operations, trackOp)
	}

	return operations
}

func (b *DeployPlanBuilder) buildHelmHooksOperations(hookType common.HelmHookType, nothingToCreateOrUpdate bool) ([]Operation, []*resource.HelmHook, error) {
	if nothingToCreateOrUpdate && b.prevReleaseDeployed {
		return nil, nil, nil
	}

	// TODO(ilya-lesikov): sort helm hooks with the same weight somehow?
	helmHooksByWeight := b.filterAndGroupHelmHooksByWeight(hookType)

	var operations []Operation
	var hooksToCleanupOnFailure []*resource.HelmHook

	for _, weightGroup := range helmHooksByWeight {
		for _, hook := range weightGroup {
			recreate := hook.Local.HasDeletePolicy(common.HelmHookDeletePolicyBeforeCreation)
			hookUpToDate := hook.Existing && !hook.Outdated && !hook.OutdatedImmutable && !hook.Unsupported

			if hookUpToDate && !recreate && b.prevReleaseDeployed {
				continue
			}

			if extDeps, err := hook.Local.ExternalDependencies(); err != nil {
				return nil, nil, fmt.Errorf("error building external dependencies: %w", err)
			} else if extDeps != nil {
				trackOp := NewOperationTrackExternalDependenciesReadiness().AddTargets(extDeps...)
				operations = append(operations, trackOp)
			}

			if nothingToCreateOrUpdate && !b.prevReleaseDeployed {
				trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
				operations = append(operations, trackReadinessOp)
			} else if !nothingToCreateOrUpdate && b.prevReleaseDeployed {
				if recreate {
					if hook.Existing {
						recreateOp := NewOperationRecreate().AddTargets(hook.Local)
						trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
						operations = append(operations, recreateOp, trackReadinessOp)
					} else {
						createOp := NewOperationCreate().AddTargets(hook.Local)
						trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
						operations = append(operations, createOp, trackReadinessOp)
					}
				} else if hook.Existing {
					if hook.Outdated || hook.OutdatedImmutable {
						updateOp := NewOperationUpdate().AddTargets(hook.Local)
						trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
						operations = append(operations, updateOp, trackReadinessOp)
					}
				} else {
					createOp := NewOperationCreate().AddTargets(hook.Local)
					trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
					operations = append(operations, createOp, trackReadinessOp)
				}
			} else {
				if recreate {
					if hook.Existing {
						recreateOp := NewOperationRecreate().AddTargets(hook.Local)
						trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
						operations = append(operations, recreateOp, trackReadinessOp)
					} else {
						createOp := NewOperationCreate().AddTargets(hook.Local)
						trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
						operations = append(operations, createOp, trackReadinessOp)
					}
				} else if hook.Existing {
					if hook.Outdated || hook.OutdatedImmutable {
						updateOp := NewOperationUpdate().AddTargets(hook.Local)
						trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
						operations = append(operations, updateOp, trackReadinessOp)
					} else {
						trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
						operations = append(operations, trackReadinessOp)
					}
				} else {
					createOp := NewOperationCreate().AddTargets(hook.Local)
					trackReadinessOp := NewOperationTrackHelmHooksReadiness().AddTargets(hook.Local)
					operations = append(operations, createOp, trackReadinessOp)
				}
			}

			if hook.Local.HasDeletePolicy(common.HelmHookDeletePolicyAfterSuceededCompletion) {
				deleteOp := NewOperationDelete().AddTargets(hook.Local)
				trackOp := NewOperationTrackDeletion().AddTargets(deleteOp.Targets...)
				operations = append(operations, deleteOp, trackOp)
			}

			if hook.Local.HasDeletePolicy(common.HelmHookDeletePolicyAfterFailedCompletion) {
				hooksToCleanupOnFailure = append(hooksToCleanupOnFailure, hook.Local)
			}
		}
	}

	return operations, hooksToCleanupOnFailure, nil
}

func (b *DeployPlanBuilder) buildHelmResourcesOperations() ([]Operation, error) {
	var operations []Operation

	// TODO(ilya-lesikov): sort resources with the same weight somehow?
	helmResourcesByWeight := b.groupHelmResourcesByWeight()
	for _, weightGroup := range helmResourcesByWeight {
		extDepTrackOp := NewOperationTrackExternalDependenciesReadiness()
		createOp := NewOperationCreate()
		updateOp := NewOperationUpdate()
		trackOp := NewOperationTrackHelmResourcesReadiness()

		for _, res := range weightGroup {
			if b.prevReleaseDeployed && res.Existing && !res.Outdated {
				continue
			}

			if extDeps, err := res.Local.ExternalDependencies(); err != nil {
				return nil, fmt.Errorf("error building external dependencies: %w", err)
			} else if extDeps != nil {
				extDepTrackOp.AddTargets(extDeps...)
			}

			if b.prevReleaseDeployed {
				if res.Existing && res.Outdated {
					updateOp.AddTargets(res.Local)
					trackOp.AddTargets(res.Local)
				} else {
					createOp.AddTargets(res.Local)
					trackOp.AddTargets(res.Local)
				}
			} else {
				if res.Existing && res.Outdated {
					updateOp.AddTargets(res.Local)
					trackOp.AddTargets(res.Local)
				} else if res.Existing && !res.Outdated {
					trackOp.AddTargets(res.Local)
				} else {
					createOp.AddTargets(res.Local)
					trackOp.AddTargets(res.Local)
				}
			}
		}

		if extDepTrackOp.Targets != nil {
			operations = append(operations, extDepTrackOp)
		}

		if createOp.Targets != nil {
			operations = append(operations, createOp)
		}

		if updateOp.Targets != nil {
			operations = append(operations, updateOp)
		}

		if trackOp.Targets != nil {
			operations = append(operations, trackOp)
		}
	}

	return operations, nil
}

func (b *DeployPlanBuilder) filterAndGroupHelmHooksByWeight(hookType common.HelmHookType) (helmHooksByWeight [][]*genericHook) {
	helmHooksByWeightMap := map[int][]*genericHook{}
	for _, hook := range b.matchedHelmHooks.NonExisting {
		if !hook.Local.HasType(hookType) {
			continue
		}

		hook := &genericHook{
			Local:             hook.Local,
			Desired:           hook.Desired,
			Outdated:          true,
			OutdatedImmutable: false,
			Existing:          false,
			Unsupported:       false,
		}

		weight := hook.Local.Weight()
		if helmHooksByWeightMap[weight] == nil {
			helmHooksByWeightMap[weight] = []*genericHook{}
		}
		helmHooksByWeightMap[weight] = append(helmHooksByWeightMap[weight], hook)
	}

	for _, hook := range b.matchedHelmHooks.Outdated {
		if !hook.Local.HasType(hookType) {
			continue
		}

		hook := &genericHook{
			Local:             hook.Local,
			Live:              hook.Live,
			Desired:           hook.Desired,
			Outdated:          true,
			OutdatedImmutable: false,
			Existing:          true,
			Unsupported:       false,
		}

		weight := hook.Local.Weight()
		if helmHooksByWeightMap[weight] == nil {
			helmHooksByWeightMap[weight] = []*genericHook{}
		}
		helmHooksByWeightMap[weight] = append(helmHooksByWeightMap[weight], hook)
	}

	for _, hook := range b.matchedHelmHooks.OutdatedImmutable {
		if !hook.Local.HasType(hookType) {
			continue
		}

		hook := &genericHook{
			Local:             hook.Local,
			Live:              hook.Live,
			Outdated:          true,
			OutdatedImmutable: true,
			Existing:          true,
			Unsupported:       false,
		}

		weight := hook.Local.Weight()
		if helmHooksByWeightMap[weight] == nil {
			helmHooksByWeightMap[weight] = []*genericHook{}
		}
		helmHooksByWeightMap[weight] = append(helmHooksByWeightMap[weight], hook)
	}

	for _, hook := range b.matchedHelmHooks.UpToDate {
		if !hook.Local.HasType(hookType) {
			continue
		}

		hook := &genericHook{
			Local:             hook.Local,
			Live:              hook.Live,
			Outdated:          false,
			OutdatedImmutable: false,
			Existing:          true,
			Unsupported:       false,
		}

		weight := hook.Local.Weight()
		if helmHooksByWeightMap[weight] == nil {
			helmHooksByWeightMap[weight] = []*genericHook{}
		}
		helmHooksByWeightMap[weight] = append(helmHooksByWeightMap[weight], hook)
	}

	for _, hook := range b.matchedHelmHooks.Unsupported {
		if !hook.Local.HasType(hookType) {
			continue
		}

		hook := &genericHook{
			Local:             hook.Local,
			Outdated:          true,
			OutdatedImmutable: false,
			Existing:          false,
			Unsupported:       true,
		}

		weight := hook.Local.Weight()
		if helmHooksByWeightMap[weight] == nil {
			helmHooksByWeightMap[weight] = []*genericHook{}
		}
		helmHooksByWeightMap[weight] = append(helmHooksByWeightMap[weight], hook)
	}

	var weights []int
	for weight := range helmHooksByWeightMap {
		weights = append(weights, weight)
	}

	sort.Ints(weights)

	for _, weight := range weights {
		helmHooksByWeight = append(helmHooksByWeight, helmHooksByWeightMap[weight])
	}

	return helmHooksByWeight
}

func (b *DeployPlanBuilder) groupHelmResourcesByWeight() (helmResourcesByWeight [][]*genericResource) {
	helmResourcesByWeightMap := map[int][]*genericResource{}

	for _, res := range b.helmResources.NonExisting {
		genRes := &genericResource{
			Local:       res.Local,
			Desired:     res.Desired,
			Outdated:    true,
			Existing:    false,
			Unsupported: false,
		}

		weight := res.Local.Weight()
		if helmResourcesByWeightMap[weight] == nil {
			helmResourcesByWeightMap[weight] = []*genericResource{}
		}
		helmResourcesByWeightMap[weight] = append(helmResourcesByWeightMap[weight], genRes)
	}

	for _, res := range b.helmResources.Outdated {
		genRes := &genericResource{
			Local:       res.Local,
			Live:        res.Live,
			Desired:     res.Desired,
			Outdated:    true,
			Existing:    true,
			Unsupported: false,
		}

		weight := res.Local.Weight()
		if helmResourcesByWeightMap[weight] == nil {
			helmResourcesByWeightMap[weight] = []*genericResource{}
		}
		helmResourcesByWeightMap[weight] = append(helmResourcesByWeightMap[weight], genRes)
	}

	for _, res := range b.helmResources.UpToDate {
		genRes := &genericResource{
			Local:       res.Local,
			Live:        res.Live,
			Outdated:    false,
			Existing:    true,
			Unsupported: false,
		}

		weight := res.Local.Weight()
		if helmResourcesByWeightMap[weight] == nil {
			helmResourcesByWeightMap[weight] = []*genericResource{}
		}
		helmResourcesByWeightMap[weight] = append(helmResourcesByWeightMap[weight], genRes)
	}

	for _, res := range b.helmResources.Unsupported {
		genRes := &genericResource{
			Local:       res.Local,
			Outdated:    true,
			Existing:    false,
			Unsupported: true,
		}

		weight := res.Local.Weight()
		if helmResourcesByWeightMap[weight] == nil {
			helmResourcesByWeightMap[weight] = []*genericResource{}
		}
		helmResourcesByWeightMap[weight] = append(helmResourcesByWeightMap[weight], genRes)
	}

	var weights []int
	for weight := range helmResourcesByWeightMap {
		weights = append(weights, weight)
	}

	sort.Ints(weights)

	for _, weight := range weights {
		helmResourcesByWeight = append(helmResourcesByWeight, helmResourcesByWeightMap[weight])
	}

	return helmResourcesByWeight
}

func (b *DeployPlanBuilder) detectOrphanedResources(ctx context.Context) ([]*resource.GenericResource, error) {
	var liveResources []*resource.GenericResource
	for _, res := range b.helmResources.Outdated {
		liveResources = append(liveResources, res.Live)
	}
	for _, res := range b.helmResources.UpToDate {
		liveResources = append(liveResources, res.Live)
	}

	var prevReleaseLiveResources []*resource.GenericResource
	for _, res := range b.prevReleaseHelmResources.Existing {
		prevReleaseLiveResources = append(prevReleaseLiveResources, res.Live)
	}

	orphanedCandidates, _, _ := resource.DiffReferencerLists(resource.CastToReferencers(prevReleaseLiveResources), resource.CastToReferencers(liveResources))

	var orphaned []*resource.GenericResource
orphanedLoop:
	for _, candidate := range orphanedCandidates {
		for _, res := range liveResources {
			// If some resources that only present in previous release and some resources from
			// current release have the same UID, then they share the same underlying resource,
			// so we shouldn't mark these missing resources as orphaned.
			if candidate.(*resource.GenericResource).Unstructured().GetUID() == res.Unstructured().GetUID() && res.Unstructured().GetUID() != "" {
				continue orphanedLoop
			}
		}

		orphaned = append(orphaned, candidate.(*resource.GenericResource))
	}

	return orphaned, nil
}

type genericHook struct {
	Local             *resource.HelmHook
	Live              *resource.GenericResource
	Desired           *resource.GenericResource
	Outdated          bool
	OutdatedImmutable bool
	Existing          bool
	Unsupported       bool
}

type genericResource struct {
	Local       *resource.HelmResource
	Live        *resource.GenericResource
	Desired     *resource.GenericResource
	Outdated    bool
	Existing    bool
	Unsupported bool
}
