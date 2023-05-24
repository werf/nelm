package resourcetracker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/werf/resource"
	"helm.sh/helm/v3/pkg/werf/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/kubedog/pkg/tracker"
	"github.com/werf/kubedog/pkg/tracker/resid"
	"github.com/werf/kubedog/pkg/trackers/elimination"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack/generic"
	"github.com/werf/logboek"
)

// FIXME(ilya-lesikov): move some options from constructor
func NewResourceTracker(showResourcesProgressEvery, showHooksProgressEvery time.Duration) *ResourceTracker {
	// FIXME(ilya-lesikov): move it somewhere higher
	// if os.Getenv("WERF_DISABLE_RESOURCES_WAITER") == "1" {
	// 	return nil
	// }

	return &ResourceTracker{
		logsFromTime:               time.Now(),
		showResourcesProgressEvery: showResourcesProgressEvery,
		showHooksProgressEvery:     showHooksProgressEvery,
	}
}

// FIXME(ilya-lesikov): make it singleton?
type ResourceTracker struct {
	logsFromTime               time.Time
	showResourcesProgressEvery time.Duration
	showHooksProgressEvery     time.Duration
}

func (t *ResourceTracker) TrackHelmHooksReadiness(ctx context.Context, options TrackHelmHooksReadinessOptions, helmHooks ...*resource.HelmHook) error {
	specs := multitrack.MultitrackSpecs{}

	var fallbackNamespace string
	if options.FallbackNamespace != "" {
		fallbackNamespace = options.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, helmHook := range helmHooks {
		switch helmHook.GroupVersionKind().GroupKind() {
		case schema.GroupKind{Group: "apps", Kind: "Deployment"}:
			spec := buildHelmHookMultitrackSpec(helmHook, fallbackNamespace)
			specs.Deployments = append(specs.Deployments, spec)
		case schema.GroupKind{Group: "apps", Kind: "StatefulSet"}:
			spec := buildHelmHookMultitrackSpec(helmHook, fallbackNamespace)
			specs.StatefulSets = append(specs.StatefulSets, spec)
		case schema.GroupKind{Group: "apps", Kind: "DaemonSet"}:
			spec := buildHelmHookMultitrackSpec(helmHook, fallbackNamespace)
			specs.DaemonSets = append(specs.DaemonSets, spec)
		case schema.GroupKind{Group: "batch", Kind: "Job"}:
			spec := buildHelmHookMultitrackSpec(helmHook, fallbackNamespace)
			specs.Jobs = append(specs.Jobs, spec)
		case schema.GroupKind{Group: "flagger", Kind: "Canary"}:
			spec := buildHelmHookMultitrackSpec(helmHook, fallbackNamespace)
			specs.Canaries = append(specs.Canaries, spec)
		case schema.GroupKind{}:
			panic("must have group/kind")
		default:
			spec := buildHelmHookGenericSpec(helmHook, options.Timeout, t.showHooksProgressEvery)
			specs.Generics = append(specs.Generics, spec)
		}
	}

	multitrackOptions := multitrack.MultitrackOptions{
		StatusProgressPeriod: t.showHooksProgressEvery,
		Options: tracker.Options{
			Timeout:      options.Timeout,
			LogsFromTime: t.logsFromTime,
		},
		DynamicClient:   kube.DynamicClient,
		DiscoveryClient: kube.CachedDiscoveryClient,
		Mapper:          kube.Mapper,
	}

	logboek.LogOptionalLn()

	return logboek.LogProcess("Waiting for hooks to complete").DoError(func() error {
		return multitrack.Multitrack(kube.Client, specs, multitrackOptions)
	})
}

func (t *ResourceTracker) TrackHelmResourcesReadiness(ctx context.Context, options TrackHelmResourcesReadinessOptions, resources ...*resource.HelmResource) error {
	specs := multitrack.MultitrackSpecs{}

	var fallbackNamespace string
	if options.FallbackNamespace != "" {
		fallbackNamespace = options.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, res := range resources {
		switch res.GroupVersionKind().GroupKind() {
		case schema.GroupKind{Group: "apps", Kind: "Deployment"}:
			spec := buildHelmResourceMultitrackSpec(res, fallbackNamespace)
			specs.Deployments = append(specs.Deployments, spec)
		case schema.GroupKind{Group: "apps", Kind: "StatefulSet"}:
			spec := buildHelmResourceMultitrackSpec(res, fallbackNamespace)
			specs.StatefulSets = append(specs.StatefulSets, spec)
		case schema.GroupKind{Group: "apps", Kind: "DaemonSet"}:
			spec := buildHelmResourceMultitrackSpec(res, fallbackNamespace)
			specs.DaemonSets = append(specs.DaemonSets, spec)
		case schema.GroupKind{Group: "batch", Kind: "Job"}:
			spec := buildHelmResourceMultitrackSpec(res, fallbackNamespace)
			specs.Jobs = append(specs.Jobs, spec)
		case schema.GroupKind{Group: "flagger", Kind: "Canary"}:
			spec := buildHelmResourceMultitrackSpec(res, fallbackNamespace)
			specs.Canaries = append(specs.Canaries, spec)
		case schema.GroupKind{}:
			panic("must have group/kind")
		default:
			spec := buildHelmResourceGenericSpec(res, options.Timeout, t.showResourcesProgressEvery)
			specs.Generics = append(specs.Generics, spec)
		}
	}

	multitrackOptions := multitrack.MultitrackOptions{
		StatusProgressPeriod: t.showResourcesProgressEvery,
		Options: tracker.Options{
			Timeout:      options.Timeout,
			LogsFromTime: t.logsFromTime,
		},
		DynamicClient:   kube.DynamicClient,
		DiscoveryClient: kube.CachedDiscoveryClient,
		Mapper:          kube.Mapper,
	}

	logboek.LogOptionalLn()

	return logboek.LogProcess("Waiting for helm resources to become ready").DoError(func() error {
		return multitrack.Multitrack(kube.Client, specs, multitrackOptions)
	})
}

func (t *ResourceTracker) TrackUnmanagedResourcesReadiness(ctx context.Context, options TrackUnmanagedResourcesReadinessOptions, resources ...*resource.UnmanagedResource) error {
	specs := multitrack.MultitrackSpecs{}

	var fallbackNamespace string
	if options.FallbackNamespace != "" {
		fallbackNamespace = options.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, res := range resources {
		switch res.GroupVersionKind().GroupKind() {
		case schema.GroupKind{Group: "apps", Kind: "Deployment"}:
			spec := buildUnmanagedResourceMultitrackSpec(res, fallbackNamespace)
			specs.Deployments = append(specs.Deployments, spec)
		case schema.GroupKind{Group: "apps", Kind: "StatefulSet"}:
			spec := buildUnmanagedResourceMultitrackSpec(res, fallbackNamespace)
			specs.StatefulSets = append(specs.StatefulSets, spec)
		case schema.GroupKind{Group: "apps", Kind: "DaemonSet"}:
			spec := buildUnmanagedResourceMultitrackSpec(res, fallbackNamespace)
			specs.DaemonSets = append(specs.DaemonSets, spec)
		case schema.GroupKind{Group: "batch", Kind: "Job"}:
			spec := buildUnmanagedResourceMultitrackSpec(res, fallbackNamespace)
			specs.Jobs = append(specs.Jobs, spec)
		case schema.GroupKind{Group: "flagger", Kind: "Canary"}:
			spec := buildUnmanagedResourceMultitrackSpec(res, fallbackNamespace)
			specs.Canaries = append(specs.Canaries, spec)
		case schema.GroupKind{}:
			panic("must have group/kind")
		default:
			spec := buildUnmanagedResourceGenericSpec(res, options.Timeout, t.showResourcesProgressEvery)
			specs.Generics = append(specs.Generics, spec)
		}
	}

	multitrackOptions := multitrack.MultitrackOptions{
		StatusProgressPeriod: t.showResourcesProgressEvery,
		Options: tracker.Options{
			Timeout:      options.Timeout,
			LogsFromTime: t.logsFromTime,
		},
		DynamicClient:   kube.DynamicClient,
		DiscoveryClient: kube.CachedDiscoveryClient,
		Mapper:          kube.Mapper,
	}

	logboek.LogOptionalLn()

	return logboek.LogProcess("Waiting for resources to become ready").DoError(func() error {
		return multitrack.Multitrack(kube.Client, specs, multitrackOptions)
	})
}

func (t *ResourceTracker) TrackExternalDependenciesReadiness(ctx context.Context, options TrackExternalDependenciesReadinessOptions, extDependencies ...*resource.ExternalDependency) error {
	specs := multitrack.MultitrackSpecs{}

	var fallbackNamespace string
	if options.FallbackNamespace != "" {
		fallbackNamespace = options.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, extDep := range extDependencies {
		switch extDep.GroupVersionKind().GroupKind() {
		case schema.GroupKind{Group: "apps", Kind: "Deployment"}:
			spec := buildExternalDependencyMultitrackSpec(extDep, fallbackNamespace)
			specs.Deployments = append(specs.Deployments, spec)
		case schema.GroupKind{Group: "apps", Kind: "StatefulSet"}:
			spec := buildExternalDependencyMultitrackSpec(extDep, fallbackNamespace)
			specs.StatefulSets = append(specs.StatefulSets, spec)
		case schema.GroupKind{Group: "apps", Kind: "DaemonSet"}:
			spec := buildExternalDependencyMultitrackSpec(extDep, fallbackNamespace)
			specs.DaemonSets = append(specs.DaemonSets, spec)
		case schema.GroupKind{Group: "batch", Kind: "Job"}:
			spec := buildExternalDependencyMultitrackSpec(extDep, fallbackNamespace)
			specs.Jobs = append(specs.Jobs, spec)
		case schema.GroupKind{Group: "flagger", Kind: "Canary"}:
			spec := buildExternalDependencyMultitrackSpec(extDep, fallbackNamespace)
			specs.Canaries = append(specs.Canaries, spec)
		case schema.GroupKind{}:
			panic("must have group/kind")
		default:
			spec := buildExternalDependencyGenericSpec(extDep, options.Timeout, t.showResourcesProgressEvery)
			specs.Generics = append(specs.Generics, spec)
		}
	}

	multitrackOptions := multitrack.MultitrackOptions{
		StatusProgressPeriod: t.showResourcesProgressEvery,
		Options: tracker.Options{
			Timeout:      options.Timeout,
			LogsFromTime: t.logsFromTime,
		},
		DynamicClient:   kube.DynamicClient,
		DiscoveryClient: kube.CachedDiscoveryClient,
		Mapper:          kube.Mapper,
	}

	logboek.LogOptionalLn()

	return logboek.LogProcess("Waiting for external dependencies to become ready").DoError(func() error {
		return multitrack.Multitrack(kube.Client, specs, multitrackOptions)
	})
}

func (t *ResourceTracker) TrackDeletion(ctx context.Context, options TrackDeletionOptions, refs ...resource.Referencer) error {
	var specs []*elimination.EliminationTrackerSpec

	var fallbackNamespace string
	if options.FallbackNamespace != "" {
		fallbackNamespace = options.FallbackNamespace
	} else {
		fallbackNamespace = v1.NamespaceDefault
	}

	for _, ref := range refs {
		gvr, _, err := util.ConvertGVKtoGVR(ref.GroupVersionKind(), kube.Mapper)
		if err != nil {
			if strings.Contains(err.Error(), "no matches for kind") {
				resettableRESTMapper := reflect.ValueOf(kube.Mapper).Interface().(meta.ResettableRESTMapper)
				resettableRESTMapper.Reset()
				gvr, _, err = util.ConvertGVKtoGVR(ref.GroupVersionKind(), resettableRESTMapper)
			}

			return fmt.Errorf("unable to convert GVK to GVR: %w", err)
		}

		var namespace string
		if ref.Namespace() != "" {
			namespace = ref.Namespace()
		} else {
			namespace = fallbackNamespace
		}

		spec := &elimination.EliminationTrackerSpec{
			ResourceName:         ref.Name(),
			Namespace:            namespace,
			GroupVersionResource: gvr,
		}

		specs = append(specs, spec)
	}

	return logboek.Default().LogProcess("Waiting for resources deletion").DoError(func() error {
		return elimination.TrackUntilEliminated(ctx, kube.DynamicClient, specs, elimination.EliminationTrackerOptions{Timeout: options.Timeout, StatusProgressPeriod: t.showResourcesProgressEvery})
	})
}

func buildHelmHookMultitrackSpec(helmHook *resource.HelmHook, fallbackNamespace string) multitrack.MultitrackSpec {
	failureAllowed := helmHook.FailuresAllowed()
	ignoreReadinessProbeFailsByContainerName, _ := helmHook.IgnoreReadinessProbeFailsForContainers()
	logRegex, _ := helmHook.LogRegex()
	logRegexByContainerName, _ := helmHook.LogRegexesForContainers()
	skipLogsForContainers, _ := helmHook.SkipLogsForContainers()
	showLogsOnlyForContainers, _ := helmHook.ShowLogsOnlyForContainers()

	var namespace string
	if helmHook.Namespace() != "" {
		namespace = helmHook.Namespace()
	} else {
		namespace = fallbackNamespace
	}

	return multitrack.MultitrackSpec{
		ResourceName:                             helmHook.Name(),
		Namespace:                                namespace,
		TrackTerminationMode:                     helmHook.TrackTerminationMode(),
		FailMode:                                 helmHook.FailMode(),
		AllowFailuresCount:                       &failureAllowed,
		IgnoreReadinessProbeFailsByContainerName: ignoreReadinessProbeFailsByContainerName,
		LogRegex:                                 logRegex,
		LogRegexByContainerName:                  logRegexByContainerName,
		SkipLogs:                                 helmHook.SkipLogs(),
		SkipLogsForContainers:                    skipLogsForContainers,
		ShowLogsOnlyForContainers:                showLogsOnlyForContainers,
		ShowServiceMessages:                      helmHook.ShowServiceMessages(),
	}
}

func buildHelmResourceMultitrackSpec(helmResource *resource.HelmResource, fallbackNamespace string) multitrack.MultitrackSpec {
	failuresAllowed := helmResource.FailuresAllowed()
	ignoreReadinessProbeFailsByContainerName, _ := helmResource.IgnoreReadinessProbeFailsForContainers()
	logRegex, _ := helmResource.LogRegex()
	logRegexByContainerName, _ := helmResource.LogRegexesForContainers()
	skipLogsForContainers, _ := helmResource.SkipLogsForContainers()
	showLogsOnlyForContainers, _ := helmResource.ShowLogsOnlyForContainers()

	var namespace string
	if helmResource.Namespace() != "" {
		namespace = helmResource.Namespace()
	} else {
		namespace = fallbackNamespace
	}

	return multitrack.MultitrackSpec{
		ResourceName:                             helmResource.Name(),
		Namespace:                                namespace,
		TrackTerminationMode:                     helmResource.TrackTerminationMode(),
		FailMode:                                 helmResource.FailMode(),
		AllowFailuresCount:                       &failuresAllowed,
		IgnoreReadinessProbeFailsByContainerName: ignoreReadinessProbeFailsByContainerName,
		LogRegex:                                 logRegex,
		LogRegexByContainerName:                  logRegexByContainerName,
		SkipLogs:                                 helmResource.SkipLogs(),
		SkipLogsForContainers:                    skipLogsForContainers,
		ShowLogsOnlyForContainers:                showLogsOnlyForContainers,
		ShowServiceMessages:                      helmResource.ShowServiceMessages(),
	}
}

func buildUnmanagedResourceMultitrackSpec(unmanagedResource *resource.UnmanagedResource, fallbackNamespace string) multitrack.MultitrackSpec {
	failuresAllowed := unmanagedResource.FailuresAllowed()
	ignoreReadinessProbeFailsByContainerName, _ := unmanagedResource.IgnoreReadinessProbeFailsForContainers()
	logRegex, _ := unmanagedResource.LogRegex()
	logRegexByContainerName, _ := unmanagedResource.LogRegexesForContainers()
	skipLogsForContainers, _ := unmanagedResource.SkipLogsForContainers()
	showLogsOnlyForContainers, _ := unmanagedResource.ShowLogsOnlyForContainers()

	var namespace string
	if unmanagedResource.Namespace() != "" {
		namespace = unmanagedResource.Namespace()
	} else {
		namespace = fallbackNamespace
	}

	return multitrack.MultitrackSpec{
		ResourceName:                             unmanagedResource.Name(),
		Namespace:                                namespace,
		TrackTerminationMode:                     unmanagedResource.TrackTerminationMode(),
		FailMode:                                 unmanagedResource.FailMode(),
		AllowFailuresCount:                       &failuresAllowed,
		IgnoreReadinessProbeFailsByContainerName: ignoreReadinessProbeFailsByContainerName,
		LogRegex:                                 logRegex,
		LogRegexByContainerName:                  logRegexByContainerName,
		SkipLogs:                                 unmanagedResource.SkipLogs(),
		SkipLogsForContainers:                    skipLogsForContainers,
		ShowLogsOnlyForContainers:                showLogsOnlyForContainers,
		ShowServiceMessages:                      unmanagedResource.ShowServiceMessages(),
	}
}

func buildExternalDependencyMultitrackSpec(extDependency *resource.ExternalDependency, fallbackNamespace string) multitrack.MultitrackSpec {
	var namespace string
	if extDependency.Namespace() != "" {
		namespace = extDependency.Namespace()
	} else {
		namespace = fallbackNamespace
	}

	return multitrack.MultitrackSpec{
		ResourceName: extDependency.Name(),
		Namespace:    namespace,
	}
}

func buildHelmHookGenericSpec(helmHook *resource.HelmHook, timeout, showProgressEvery time.Duration) *generic.Spec {
	resourceID := resid.NewResourceID(helmHook.Name(), helmHook.GroupVersionKind(), resid.NewResourceIDOptions{Namespace: helmHook.Namespace()})
	allowFailuresCount := helmHook.FailuresAllowed()
	var noActivityTimeout *time.Duration
	if timeout, set := helmHook.NoActivityTimeout(); set {
		noActivityTimeout = &timeout
	}

	// FIXME(ilya-lesikov): is there no way to provide default namespace for generic spec?
	return &generic.Spec{
		ResourceID:           resourceID,
		Timeout:              timeout,
		NoActivityTimeout:    noActivityTimeout,
		TrackTerminationMode: generic.TrackTerminationMode(helmHook.TrackTerminationMode()),
		FailMode:             generic.FailMode(helmHook.FailMode()),
		AllowFailuresCount:   &allowFailuresCount,
		ShowServiceMessages:  helmHook.ShowServiceMessages(),
		StatusProgressPeriod: showProgressEvery,
	}
}

func buildHelmResourceGenericSpec(helmResource *resource.HelmResource, timeout, showProgressEvery time.Duration) *generic.Spec {
	resourceID := resid.NewResourceID(helmResource.Name(), helmResource.GroupVersionKind(), resid.NewResourceIDOptions{Namespace: helmResource.Namespace()})
	allowFailuresCount := helmResource.FailuresAllowed()
	var noActivityTimeout *time.Duration
	if timeout, set := helmResource.NoActivityTimeout(); set {
		noActivityTimeout = &timeout
	}

	return &generic.Spec{
		ResourceID:           resourceID,
		Timeout:              timeout,
		NoActivityTimeout:    noActivityTimeout,
		TrackTerminationMode: generic.TrackTerminationMode(helmResource.TrackTerminationMode()),
		FailMode:             generic.FailMode(helmResource.FailMode()),
		AllowFailuresCount:   &allowFailuresCount,
		ShowServiceMessages:  helmResource.ShowServiceMessages(),
		StatusProgressPeriod: showProgressEvery,
	}
}

func buildUnmanagedResourceGenericSpec(unmanagedResource *resource.UnmanagedResource, timeout, showProgressEvery time.Duration) *generic.Spec {
	resourceID := resid.NewResourceID(unmanagedResource.Name(), unmanagedResource.GroupVersionKind(), resid.NewResourceIDOptions{Namespace: unmanagedResource.Namespace()})
	allowFailuresCount := unmanagedResource.FailuresAllowed()
	var noActivityTimeout *time.Duration
	if timeout, set := unmanagedResource.NoActivityTimeout(); set {
		noActivityTimeout = &timeout
	}

	return &generic.Spec{
		ResourceID:           resourceID,
		Timeout:              timeout,
		NoActivityTimeout:    noActivityTimeout,
		TrackTerminationMode: generic.TrackTerminationMode(unmanagedResource.TrackTerminationMode()),
		FailMode:             generic.FailMode(unmanagedResource.FailMode()),
		AllowFailuresCount:   &allowFailuresCount,
		ShowServiceMessages:  unmanagedResource.ShowServiceMessages(),
		StatusProgressPeriod: showProgressEvery,
	}
}

func buildExternalDependencyGenericSpec(extDependency *resource.ExternalDependency, timeout, showProgressEvery time.Duration) *generic.Spec {
	resourceID := resid.NewResourceID(extDependency.Name(), extDependency.GroupVersionKind(), resid.NewResourceIDOptions{Namespace: extDependency.Namespace()})

	return &generic.Spec{
		ResourceID:           resourceID,
		Timeout:              timeout,
		StatusProgressPeriod: showProgressEvery,
	}
}

type TrackHelmHooksReadinessOptions struct {
	FallbackNamespace string
	Timeout           time.Duration
}

type TrackHelmResourcesReadinessOptions struct {
	FallbackNamespace string
	Timeout           time.Duration
}

type TrackUnmanagedResourcesReadinessOptions struct {
	FallbackNamespace string
	Timeout           time.Duration
}

type TrackExternalDependenciesReadinessOptions struct {
	FallbackNamespace string
	Timeout           time.Duration
}

type TrackDeletionOptions struct {
	FallbackNamespace string
	Timeout           time.Duration
}
