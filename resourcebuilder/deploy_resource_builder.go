package resourcebuilder

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-multierror"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/werf/client"
	"helm.sh/helm/v3/pkg/werf/common"
	"helm.sh/helm/v3/pkg/werf/plan"
	"helm.sh/helm/v3/pkg/werf/resource"
)

func NewDeployResourceBuilder(releaseNs *resource.UnmanagedResource, deployType plan.DeployType, cli *client.Client) *DeployResourceBuilder {
	return &DeployResourceBuilder{
		deployType:       deployType,
		releaseNamespace: releaseNs,
		client:           cli,
		result:           &DeployResourceBuilderResult{},
	}
}

type DeployResourceBuilder struct {
	deployType           plan.DeployType
	releaseNamespace     *resource.UnmanagedResource
	legacyPreloadedCRDs  []chart.CRD
	legacyHelmHooks      []*release.Hook
	releaseManifests     string
	prevReleaseManifests string

	client *client.Client

	result *DeployResourceBuilderResult
}

func (b *DeployResourceBuilder) WithLegacyPreloadedCRDs(legacyCRDs ...chart.CRD) *DeployResourceBuilder {
	b.legacyPreloadedCRDs = append(b.legacyPreloadedCRDs, legacyCRDs...)
	return b
}

func (b *DeployResourceBuilder) WithLegacyHelmHooks(legacyHooks ...*release.Hook) *DeployResourceBuilder {
	b.legacyHelmHooks = append(b.legacyHelmHooks, legacyHooks...)
	return b
}

func (b *DeployResourceBuilder) WithReleaseManifests(manifests string) *DeployResourceBuilder {
	manifests = strings.TrimSpace(manifests)

	if manifests == "" {
		return b
	}

	if b.releaseManifests == "" {
		b.releaseManifests = manifests
	} else {
		b.releaseManifests += "---\n" + manifests
	}

	return b
}

func (b *DeployResourceBuilder) WithPrevReleaseManifests(prevManifests string) *DeployResourceBuilder {
	prevManifests = strings.TrimSpace(prevManifests)

	if prevManifests == "" {
		return b
	}

	if b.prevReleaseManifests == "" {
		b.prevReleaseManifests = prevManifests
	} else {
		b.prevReleaseManifests += "---\n" + prevManifests
	}

	return b
}

func (b *DeployResourceBuilder) Build(ctx context.Context) (*DeployResourceBuilderResult, error) {
	if err := b.buildReleaseNamespace(ctx); err != nil {
		return nil, fmt.Errorf("error building release namespace: %w", err)
	}

	if b.legacyPreloadedCRDs != nil {
		if err := b.buildPreloadedCRDs(ctx); err != nil {
			return nil, fmt.Errorf("error building preloaded CRDs: %w", err)
		}
	}

	if b.legacyHelmHooks != nil {
		if err := b.buildHelmHooks(ctx); err != nil {
			return nil, fmt.Errorf("error building helm hooks: %w", err)
		}
	}

	if b.releaseManifests != "" {
		if err := b.buildHelmResources(ctx); err != nil {
			return nil, fmt.Errorf("error building helm resources: %w", err)
		}
	}

	if b.prevReleaseManifests != "" {
		if err := b.buildPrevReleaseHelmResources(ctx); err != nil {
			return nil, fmt.Errorf("error building previous release helm resources: %w", err)
		}
	}

	return b.result, nil
}

func (b *DeployResourceBuilder) buildReleaseNamespace(ctx context.Context) error {
	dryApplyResult, errs := b.client.SmartApply(ctx, client.SmartApplyOptions{DryRun: true}, b.releaseNamespace)
	if errs != nil {
		return fmt.Errorf("error dry-run applying local release namespace: %w", multierror.Append(nil, errs...))
	}

	b.result.ReleaseNamespace.Local = b.releaseNamespace

	for _, res := range dryApplyResult.Updated {
		b.result.ReleaseNamespace.Existing = true
		b.result.ReleaseNamespace.Live = res.LiveOriginal
		b.result.ReleaseNamespace.Desired = res.LiveResult
		return nil
	}

	for _, res := range dryApplyResult.Created {
		b.result.ReleaseNamespace.Desired = res.Result
		return nil
	}

	for _, res := range dryApplyResult.Unchanged {
		b.result.ReleaseNamespace.Existing = true
		b.result.ReleaseNamespace.UpToDate = true
		b.result.ReleaseNamespace.Live = res.Live
		b.result.ReleaseNamespace.Desired = res.Live
		return nil
	}

	return nil
}

func (b *DeployResourceBuilder) buildPreloadedCRDs(ctx context.Context) error {
	// FIXME(ilya-lesikov): resource builder should flatten
	localPreloadedCRDs, err := resource.BuildUnmanagedResourcesFromLegacyCRDs(b.client.DiscoveryRESTMapper(), b.client.DiscoveryClient(), b.legacyPreloadedCRDs...)
	if err != nil {
		return fmt.Errorf("error building local preloaded crds: %w", err)
	}
	for _, crd := range localPreloadedCRDs {
		if err := crd.Validate(); err != nil {
			return fmt.Errorf("error validating local preloaded crd: %w", err)
		}
	}

	dryApplyResult, errs := b.client.SmartApply(ctx, client.SmartApplyOptions{FallbackNamespace: b.releaseNamespace.Name(), RecreateOnImmutable: true, DryRun: true}, resource.CastToResourcers(localPreloadedCRDs)...)
	if errs != nil {
		return fmt.Errorf("error dry-run applying local preloaded CRDs: %w", multierror.Append(nil, errs...))
	}

	for _, res := range dryApplyResult.Created {
		nonExisting := struct {
			Local   *resource.UnmanagedResource
			Desired *resource.GenericResource
		}{
			Local:   res.Target.(*resource.UnmanagedResource),
			Desired: res.Result,
		}
		b.result.PreloadedCRDs.NonExisting = append(b.result.PreloadedCRDs.NonExisting, nonExisting)
	}

	for _, res := range dryApplyResult.Updated {
		outdated := struct {
			Local   *resource.UnmanagedResource
			Live    *resource.GenericResource
			Desired *resource.GenericResource
		}{
			Local:   res.Target.(*resource.UnmanagedResource),
			Live:    res.LiveOriginal,
			Desired: res.LiveResult,
		}
		b.result.PreloadedCRDs.Outdated = append(b.result.PreloadedCRDs.Outdated, outdated)
	}

	for _, res := range dryApplyResult.RecreatedImmutable {
		outdated := struct {
			Local *resource.UnmanagedResource
			Live  *resource.GenericResource
		}{
			Local: res.Target.(*resource.UnmanagedResource),
			Live:  res.Live,
		}
		b.result.PreloadedCRDs.OutdatedImmutable = append(b.result.PreloadedCRDs.OutdatedImmutable, outdated)
	}

	for _, res := range dryApplyResult.Unchanged {
		upToDate := struct {
			Local *resource.UnmanagedResource
			Live  *resource.GenericResource
		}{
			Local: res.Target.(*resource.UnmanagedResource),
			Live:  res.Live,
		}
		b.result.PreloadedCRDs.UpToDate = append(b.result.PreloadedCRDs.UpToDate, upToDate)
	}

	return nil
}

func (b *DeployResourceBuilder) buildHelmHooks(ctx context.Context) error {
	// FIXME(ilya-lesikov): resource builder should flatten
	localHelmHooks, err := resource.HelmHooksFromLegacyHooks(b.client.DiscoveryRESTMapper(), b.client.DiscoveryClient(), b.legacyHelmHooks...)
	if err != nil {
		return fmt.Errorf("error building local helm hooks: %w", err)
	}
	for _, hook := range localHelmHooks {
		if err := hook.Validate(); err != nil {
			return fmt.Errorf("error validating local helm hook: %w", err)
		}
	}

	var matchedHelmHookTypes []common.HelmHookType
	switch b.deployType {
	case plan.DeployTypeInitial, plan.DeployTypeInstall:
		matchedHelmHookTypes = []common.HelmHookType{
			common.HelmHookTypePreInstall,
			common.HelmHookTypePostInstall,
		}
	case plan.DeployTypeUpgrade:
		matchedHelmHookTypes = []common.HelmHookType{
			common.HelmHookTypePreUpgrade,
			common.HelmHookTypePostUpgrade,
		}
	case plan.DeployTypeRollback:
		matchedHelmHookTypes = []common.HelmHookType{
			common.HelmHookTypePreRollback,
			common.HelmHookTypePostRollback,
		}
	}

	var matchedLocalHelmHooks []*resource.HelmHook
	for _, hook := range localHelmHooks {
		var match bool
		for _, hookType := range matchedHelmHookTypes {
			if hook.HasType(hookType) {
				match = true
				break
			}
		}

		if match {
			matchedLocalHelmHooks = append(matchedLocalHelmHooks, hook)
		} else {
			b.result.HelmHooks.Unmatched = append(b.result.HelmHooks.Unmatched, hook)
		}
	}

	dryApplyResult, errs := b.client.SmartApply(ctx, client.SmartApplyOptions{FallbackNamespace: b.releaseNamespace.Name(), RecreateOnImmutable: true, ContinueOnUnsupportedResource: true, DryRun: true}, resource.CastToResourcers(matchedLocalHelmHooks)...)
	if errs != nil {
		return fmt.Errorf("error dry-run applying matching local helm hooks: %w", multierror.Append(nil, errs...))
	}

	for _, res := range dryApplyResult.Created {
		nonExisting := struct {
			Local   *resource.HelmHook
			Desired *resource.GenericResource
		}{
			Local:   res.Target.(*resource.HelmHook),
			Desired: res.Result,
		}
		res.Target.Unstructured()
		b.result.HelmHooks.Matched.NonExisting = append(b.result.HelmHooks.Matched.NonExisting, nonExisting)
	}

	for _, res := range dryApplyResult.Updated {
		outdated := struct {
			Local   *resource.HelmHook
			Live    *resource.GenericResource
			Desired *resource.GenericResource
		}{
			Local:   res.Target.(*resource.HelmHook),
			Live:    res.LiveOriginal,
			Desired: res.LiveResult,
		}
		b.result.HelmHooks.Matched.Outdated = append(b.result.HelmHooks.Matched.Outdated, outdated)
	}

	for _, res := range dryApplyResult.RecreatedImmutable {
		outdated := struct {
			Local *resource.HelmHook
			Live  *resource.GenericResource
		}{
			Local: res.Target.(*resource.HelmHook),
			Live:  res.Live,
		}
		b.result.HelmHooks.Matched.OutdatedImmutable = append(b.result.HelmHooks.Matched.OutdatedImmutable, outdated)
	}

	for _, res := range dryApplyResult.SkippedUnsupportedResource {
		unsupported := struct {
			Local *resource.HelmHook
		}{
			Local: res.Target.(*resource.HelmHook),
		}
		b.result.HelmHooks.Matched.Unsupported = append(b.result.HelmHooks.Matched.Unsupported, unsupported)
	}

	for _, res := range dryApplyResult.Unchanged {
		upToDate := struct {
			Local *resource.HelmHook
			Live  *resource.GenericResource
		}{
			Local: res.Target.(*resource.HelmHook),
			Live:  res.Live,
		}
		b.result.HelmHooks.Matched.UpToDate = append(b.result.HelmHooks.Matched.UpToDate, upToDate)
	}

	return nil
}

func (b *DeployResourceBuilder) buildHelmResources(ctx context.Context) error {
	localHelmResources, err := resource.BuildHelmResourcesFromManifests(b.client.DiscoveryRESTMapper(), b.client.DiscoveryClient(), b.releaseManifests)
	if err != nil {
		return fmt.Errorf("error building local helm resources: %w", err)
	}
	for _, res := range localHelmResources {
		if err := res.Validate(); err != nil {
			return fmt.Errorf("error validating local helm resource: %w", err)
		}
	}

	dryApplyResult, errs := b.client.SmartApply(ctx, client.SmartApplyOptions{FallbackNamespace: b.releaseNamespace.Name(), RecreateOnImmutable: true, ContinueOnUnsupportedResource: true, DryRun: true}, resource.CastToResourcers(localHelmResources)...)
	if errs != nil {
		return fmt.Errorf("error dry-run applying local helm resources: %w", multierror.Append(nil, errs...))
	}

	for _, res := range dryApplyResult.Created {
		nonExisting := struct {
			Local   *resource.HelmResource
			Desired *resource.GenericResource
		}{
			Local:   res.Target.(*resource.HelmResource),
			Desired: res.Result,
		}
		b.result.HelmResources.NonExisting = append(b.result.HelmResources.NonExisting, nonExisting)
	}

	for _, res := range dryApplyResult.Updated {
		outdated := struct {
			Local   *resource.HelmResource
			Live    *resource.GenericResource
			Desired *resource.GenericResource
		}{
			Local:   res.Target.(*resource.HelmResource),
			Live:    res.LiveOriginal,
			Desired: res.LiveResult,
		}
		b.result.HelmResources.Outdated = append(b.result.HelmResources.Outdated, outdated)
	}

	for _, res := range dryApplyResult.RecreatedImmutable {
		outdated := struct {
			Local *resource.HelmResource
			Live  *resource.GenericResource
		}{
			Local: res.Target.(*resource.HelmResource),
			Live:  res.Live,
		}
		b.result.HelmResources.OutdatedImmutable = append(b.result.HelmResources.OutdatedImmutable, outdated)
	}

	for _, res := range dryApplyResult.SkippedUnsupportedResource {
		unsupported := struct {
			Local *resource.HelmResource
		}{
			Local: res.Target.(*resource.HelmResource),
		}
		b.result.HelmResources.Unsupported = append(b.result.HelmResources.Unsupported, unsupported)
	}

	for _, res := range dryApplyResult.Unchanged {
		upToDate := struct {
			Local *resource.HelmResource
			Live  *resource.GenericResource
		}{
			Local: res.Target.(*resource.HelmResource),
			Live:  res.Live,
		}
		b.result.HelmResources.UpToDate = append(b.result.HelmResources.UpToDate, upToDate)
	}

	return nil
}

func (b *DeployResourceBuilder) buildPrevReleaseHelmResources(ctx context.Context) error {
	// FIXME(ilya-lesikov): builder should flatten
	prevReleaseHelmResources, err := resource.BuildHelmResourcesFromManifests(b.client.DiscoveryRESTMapper(), b.client.DiscoveryClient(), b.prevReleaseManifests)
	if err != nil {
		return fmt.Errorf("error building previous release local helm resources: %w", err)
	}

	getResult, errs := b.client.Get(ctx, client.GetOptions{FallbackNamespace: b.releaseNamespace.Name()}, resource.CastToReferencers(prevReleaseHelmResources)...)
	if errs != nil {
		return fmt.Errorf("error getting previous release helm resources: %w", multierror.Append(nil, errs...))
	}

	for _, res := range getResult.Found {
		existing := struct {
			Local *resource.HelmResource
			Live  *resource.GenericResource
		}{
			Local: res.Target.(*resource.HelmResource),
			Live:  res.Result,
		}
		b.result.PrevReleaseHelmResources.Existing = append(b.result.PrevReleaseHelmResources.Existing, existing)
	}

	for _, res := range getResult.NotFound {
		nonExisting := res.(*resource.HelmResource)
		b.result.PrevReleaseHelmResources.NonExisting = append(b.result.PrevReleaseHelmResources.NonExisting, nonExisting)
	}

	return nil
}

type DeployResourceBuilderResult struct {
	ReleaseNamespace struct {
		Local    *resource.UnmanagedResource
		Live     *resource.GenericResource
		Desired  *resource.GenericResource
		UpToDate bool
		Existing bool
	}

	PreloadedCRDs struct {
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

	HelmHooks struct {
		Matched struct {
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
		Unmatched []*resource.HelmHook
	}

	HelmResources struct {
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

	PrevReleaseHelmResources struct {
		Existing []struct {
			Local *resource.HelmResource
			Live  *resource.GenericResource
		}
		NonExisting []*resource.HelmResource
	}
}
