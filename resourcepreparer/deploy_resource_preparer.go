package resourcepreparer

import (
	"context"
	"fmt"
	"sync"

	"helm.sh/helm/v3/pkg/werf/kubeclientv2"
	"helm.sh/helm/v3/pkg/werf/resourcev2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func PrepareDeployResources(
	ctx context.Context,
	releaseName string,
	releaseNamespace ValidatableGettableDryAppliable,
	prevReleaseGeneralCRDs,
	prevReleaseGeneralResources []ValidatableGettable,
	standaloneCRDs,
	hookCRDs,
	hookResources,
	generalCRDs,
	generalResources []ValidatableGettableDryAppliable,
	kubeClient KubeClient,
	opts PrepareDeployResourcesOptions,
) (*PrepareDeployResourcesResult, error) {
	finalResult := &PrepareDeployResourcesResult{}

	var err error
	finalResult.ReleaseNamespace, err = validateGetDryApplyReleaseNamespace(ctx, releaseNamespace, kubeClient, opts.FallbackNamespace)
	if err != nil {
		return nil, fmt.Errorf("error preparing release namespace resource: %w", err)
	}

	wg := &sync.WaitGroup{}
	wgDoneCh := make(chan struct{}, 1)
	defer func() {
		wgDoneCh <- struct{}{}
	}()

	prevReleaseGeneralCRDsResultsCh := make(chan *ValidatableGettableResult, 1)
	for _, res := range prevReleaseGeneralCRDs {
		validateGet(ctx, res, wg, prevReleaseGeneralCRDsResultsCh, kubeClient, opts.FallbackNamespace)
	}

	prevReleaseGeneralResourcesResultsCh := make(chan *ValidatableGettableResult, 1)
	for _, res := range prevReleaseGeneralResources {
		validateGet(ctx, res, wg, prevReleaseGeneralResourcesResultsCh, kubeClient, opts.FallbackNamespace)
	}

	standaloneCRDsResultsCh := make(chan *ValidatableGettableDryAppliableResult, 1)
	for _, res := range standaloneCRDs {
		validateGetDryApply(ctx, res, wg, standaloneCRDsResultsCh, kubeClient, opts.FallbackNamespace, false, releaseName, releaseNamespace.Name())
	}

	hookCRDsResultsCh := make(chan *ValidatableGettableDryAppliableResult, 1)
	for _, res := range hookCRDs {
		validateGetDryApply(ctx, res, wg, hookCRDsResultsCh, kubeClient, opts.FallbackNamespace, false, releaseName, releaseNamespace.Name())
	}

	hookResourcesResultsCh := make(chan *ValidatableGettableDryAppliableResult, 1)
	for _, res := range hookResources {
		validateGetDryApply(ctx, res, wg, hookResourcesResultsCh, kubeClient, opts.FallbackNamespace, false, releaseName, releaseNamespace.Name())
	}

	generalCRDsResultsCh := make(chan *ValidatableGettableDryAppliableResult, 1)
	for _, res := range generalCRDs {
		validateGetDryApply(ctx, res, wg, generalCRDsResultsCh, kubeClient, opts.FallbackNamespace, true, releaseName, releaseNamespace.Name())
	}

	generalResourcesResultsCh := make(chan *ValidatableGettableDryAppliableResult, 1)
	for _, res := range generalResources {
		validateGetDryApply(ctx, res, wg, generalResourcesResultsCh, kubeClient, opts.FallbackNamespace, true, releaseName, releaseNamespace.Name())
	}

	go func() {
		for {
			select {
			case result := <-prevReleaseGeneralCRDsResultsCh:
				finalResult.PrevReleaseGeneralCRDs = append(finalResult.PrevReleaseGeneralCRDs, result)
			case result := <-prevReleaseGeneralResourcesResultsCh:
				finalResult.PrevReleaseGeneralResources = append(finalResult.PrevReleaseGeneralResources, result)
			case result := <-standaloneCRDsResultsCh:
				finalResult.StandaloneCRDs = append(finalResult.StandaloneCRDs, result)
			case result := <-hookCRDsResultsCh:
				finalResult.HookCRDs = append(finalResult.HookCRDs, result)
			case result := <-hookResourcesResultsCh:
				finalResult.HookResources = append(finalResult.HookResources, result)
			case result := <-generalCRDsResultsCh:
				finalResult.GeneralCRDs = append(finalResult.GeneralCRDs, result)
			case result := <-generalResourcesResultsCh:
				finalResult.GeneralResources = append(finalResult.GeneralResources, result)
			case <-wgDoneCh:
				return
			}
		}
	}()

	wg.Wait()

	return finalResult, nil
}

func validateGetDryApplyReleaseNamespace(ctx context.Context, releaseNamespace ValidatableGettableDryAppliable, kubeClient KubeClient, fallbackNamespace string) (*ValidatableGettableDryAppliableResult, error) {
	result := &ValidatableGettableDryAppliableResult{
		Name:             releaseNamespace.Name(),
		Namespace:        releaseNamespace.Namespace(),
		GroupVersionKind: releaseNamespace.GroupVersionKind(),
	}

	if err := releaseNamespace.Validate(); err != nil {
		return nil, fmt.Errorf("error validating release namespace %q: %w", releaseNamespace.String(), err)
	}

	obj, err := kubeClient.Get(ctx, releaseNamespace, kubeclientv2.GetOptions{
		FallbackNamespace: fallbackNamespace,
	})
	if err != nil {
		result.GetErr = fmt.Errorf("error getting release namespace %q: %w", releaseNamespace.String(), err)
		if !apierrors.IsNotFound(err) {
			return nil, result.GetErr
		}
	}
	if obj != nil {
		result.GetResource = resourcev2.NewRemoteResource(obj)
	}

	obj, err = kubeClient.Apply(ctx, releaseNamespace, kubeclientv2.ApplyOptions{
		FallbackNamespace: fallbackNamespace,
		DryRun:            true,
	})
	if err != nil {
		return nil, fmt.Errorf("error applying release namespace %q: %w", releaseNamespace.Name(), err)
	}
	result.DryApplyResource = resourcev2.NewRemoteResource(obj)

	return result, nil
}

func validateGet(ctx context.Context, res ValidatableGettable, wg *sync.WaitGroup, resultsCh chan<- *ValidatableGettableResult, kubeClient KubeClient, fallbackNamespace string) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		resResult := &ValidatableGettableResult{
			Name:             res.Name(),
			Namespace:        res.Namespace(),
			GroupVersionKind: res.GroupVersionKind(),
		}

		if err := res.Validate(); err != nil {
			resResult.ValidateErr = fmt.Errorf("error validating resource %q: %w", res.String(), err)
			resultsCh <- resResult
			return
		}

		obj, err := kubeClient.Get(ctx, res, kubeclientv2.GetOptions{
			FallbackNamespace: fallbackNamespace,
		})
		if err != nil {
			resResult.GetErr = fmt.Errorf("error getting resource %q: %w", res.String(), err)
			resultsCh <- resResult
			return
		}
		resResult.GetResource = resourcev2.NewRemoteResource(obj)

		resultsCh <- resResult
	}()
}

func validateGetDryApply(ctx context.Context, res ValidatableGettableDryAppliable, wg *sync.WaitGroup, resultsCh chan<- *ValidatableGettableDryAppliableResult, kubeClient KubeClient, fallbackNamespace string, partOfRelease bool, releaseName, releaseNamespace string) {
	wg.Add(1)
	go func() {
		defer wg.Done()

		resResult := &ValidatableGettableDryAppliableResult{
			Name:             res.Name(),
			Namespace:        res.Namespace(),
			GroupVersionKind: res.GroupVersionKind(),
		}

		if err := res.Validate(); err != nil {
			resResult.ValidateErr = fmt.Errorf("error validating resource %q: %w", res.String(), err)
			resultsCh <- resResult
			return
		}

		obj, err := kubeClient.Get(ctx, res, kubeclientv2.GetOptions{
			FallbackNamespace: fallbackNamespace,
		})
		if err != nil {
			resResult.GetErr = fmt.Errorf("error getting resource %q: %w", res.String(), err)
			resultsCh <- resResult
			return
		}
		resResult.GetResource = resourcev2.NewRemoteResource(obj)

		if partOfRelease {
			if ownable, nonOwnableReason := resResult.GetResource.OwnableByRelease(releaseName, releaseNamespace); !ownable {
				resResult.ValidateErr = fmt.Errorf("resource %q can not be adopted by release %q: %s", res.String(), releaseName, nonOwnableReason)
				resultsCh <- resResult
				return
			}
		}

		obj, err = kubeClient.Apply(ctx, res, kubeclientv2.ApplyOptions{
			FallbackNamespace: fallbackNamespace,
			DryRun:            true,
		})
		if err != nil {
			resResult.DryApplyErr = fmt.Errorf("error applying resource %q: %w", res.String(), err)
			resultsCh <- resResult
			return
		}
		resResult.DryApplyResource = resourcev2.NewRemoteResource(obj)

		resultsCh <- resResult
	}()
}

type PrepareDeployResourcesOptions struct {
	FallbackNamespace string
}

type ValidatableGettableResult struct {
	Name             string
	Namespace        string
	GroupVersionKind schema.GroupVersionKind
	ValidateErr      error
	GetResource      *resourcev2.RemoteResource
	GetErr           error
}

type ValidatableGettableDryAppliableResult struct {
	Name             string
	Namespace        string
	GroupVersionKind schema.GroupVersionKind
	ValidateErr      error
	GetResource      *resourcev2.RemoteResource
	GetErr           error
	DryApplyResource *resourcev2.RemoteResource
	DryApplyErr      error
}

type PrepareDeployResourcesResult struct {
	PrevReleaseGeneralCRDs      []*ValidatableGettableResult
	PrevReleaseGeneralResources []*ValidatableGettableResult
	ReleaseNamespace            *ValidatableGettableDryAppliableResult
	StandaloneCRDs              []*ValidatableGettableDryAppliableResult
	HookCRDs                    []*ValidatableGettableDryAppliableResult
	HookResources               []*ValidatableGettableDryAppliableResult
	GeneralCRDs                 []*ValidatableGettableDryAppliableResult
	GeneralResources            []*ValidatableGettableDryAppliableResult
}

func (r *PrepareDeployResourcesResult) ValidateErrs() []error {
	var errs []error

	if r.ReleaseNamespace.ValidateErr != nil {
		errs = append(errs, r.ReleaseNamespace.ValidateErr)
	}

	for _, res := range r.PrevReleaseGeneralCRDs {
		if res.ValidateErr != nil {
			errs = append(errs, res.ValidateErr)
		}
	}

	for _, res := range r.PrevReleaseGeneralResources {
		if res.ValidateErr != nil {
			errs = append(errs, res.ValidateErr)
		}
	}

	for _, res := range r.StandaloneCRDs {
		if res.ValidateErr != nil {
			errs = append(errs, res.ValidateErr)
		}
	}

	for _, res := range r.HookCRDs {
		if res.ValidateErr != nil {
			errs = append(errs, res.ValidateErr)
		}
	}

	for _, res := range r.HookResources {
		if res.ValidateErr != nil {
			errs = append(errs, res.ValidateErr)
		}
	}

	for _, res := range r.GeneralCRDs {
		if res.ValidateErr != nil {
			errs = append(errs, res.ValidateErr)
		}
	}

	for _, res := range r.GeneralResources {
		if res.ValidateErr != nil {
			errs = append(errs, res.ValidateErr)
		}
	}

	return errs
}

type ValidatableGettable interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	String() string
	Validate() error
}

type ValidatableGettableDryAppliable interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	Unstructured() *unstructured.Unstructured
	PartOfRelease() bool
	ShouldHaveServiceMetadata() bool
	String() string
	Validate() error
}

type KubeClient interface {
	Get(ctx context.Context, res kubeclientv2.Gettable, opts kubeclientv2.GetOptions) (*unstructured.Unstructured, error)
	Apply(ctx context.Context, res kubeclientv2.Appliable, opts kubeclientv2.ApplyOptions) (*unstructured.Unstructured, error)
}

func CastToValidatableGettables[T ValidatableGettable](resources []T) []ValidatableGettable {
	result := []ValidatableGettable{}
	for _, res := range resources {
		result = append(result, res)
	}
	return result
}

func CastToValidatableGettableDryAppliables[T ValidatableGettableDryAppliable](resources []T) []ValidatableGettableDryAppliable {
	result := []ValidatableGettableDryAppliable{}
	for _, res := range resources {
		result = append(result, res)
	}
	return result
}
