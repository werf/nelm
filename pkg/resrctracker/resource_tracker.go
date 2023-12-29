package resrctracker

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/samber/lo"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"nelm.sh/nelm/pkg/log"
	"nelm.sh/nelm/pkg/resrcid"

	"github.com/werf/kubedog/pkg/tracker"
	dogresid "github.com/werf/kubedog/pkg/tracker/resid"
	"github.com/werf/kubedog/pkg/trackers/elimination"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack/generic"
)

var _ ResourceTrackerer = (*ResourceTracker)(nil)

func NewResourceTracker(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper meta.ResettableRESTMapper) *ResourceTracker {
	// FIXME(ilya-lesikov): move it somewhere higher
	// if os.Getenv("WERF_DISABLE_RESOURCES_WAITER") == "1" {
	// 	return nil
	// }

	return &ResourceTracker{
		staticClient:    staticClient,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		mapper:          mapper,
	}
}

type ResourceTracker struct {
	specsForTrackReadiness multitrack.MultitrackSpecs
	specsForTrackDeletion  []*elimination.EliminationTrackerSpec

	staticClient    kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	mapper          meta.ResettableRESTMapper
	kubedogLock     sync.Mutex
}

func (t *ResourceTracker) AddResourceToTrackReadiness(resource *resrcid.ResourceID, opts AddResourceToTrackReadinessOptions) error {
	t.kubedogLock.Lock()
	defer t.kubedogLock.Unlock()

	namespaced, err := resource.Namespaced()
	if err != nil {
		return fmt.Errorf("error checking whether resource is namespaced: %w", err)
	}

	var multitrackSpec *multitrack.MultitrackSpec
	var genericSpec *generic.Spec
	switch resource.GroupVersionKind().GroupKind() {
	case schema.GroupKind{}:
		return fmt.Errorf("resource %q doesn't have Group and Kind", resource.HumanID())
	case schema.GroupKind{Group: "apps", Kind: "Deployment"},
		schema.GroupKind{Group: "apps", Kind: "StatefulSet"},
		schema.GroupKind{Group: "apps", Kind: "DaemonSet"},
		schema.GroupKind{Group: "batch", Kind: "Job"},
		schema.GroupKind{Group: "flagger", Kind: "Canary"}:
		multitrackSpec = &multitrack.MultitrackSpec{
			ResourceName:                             resource.Name(),
			Namespace:                                lo.Ternary(namespaced, resource.Namespace(), ""),
			TrackTerminationMode:                     opts.TrackTerminationMode,
			FailMode:                                 opts.FailMode,
			AllowFailuresCount:                       opts.FailuresAllowed,
			IgnoreReadinessProbeFailsByContainerName: opts.IgnoreReadinessProbeFailsForContainers,
			LogRegex:                                 opts.LogRegex,
			LogRegexByContainerName:                  opts.LogRegexesForContainers,
			SkipLogs:                                 opts.SkipLogs,
			SkipLogsForContainers:                    opts.SkipLogsForContainers,
			ShowLogsOnlyForContainers:                opts.ShowLogsOnlyForContainers,
			ShowServiceMessages:                      opts.ShowServiceMessages,
		}
	default:
		genericSpec = &generic.Spec{
			ResourceID:           dogresid.NewResourceID(resource.Name(), resource.GroupVersionKind(), dogresid.NewResourceIDOptions{lo.Ternary(namespaced, resource.Namespace(), "")}),
			Timeout:              opts.Timeout,
			NoActivityTimeout:    opts.NoActivityTimeout,
			TrackTerminationMode: generic.TrackTerminationMode(opts.TrackTerminationMode),
			FailMode:             generic.FailMode(opts.FailMode),
			AllowFailuresCount:   opts.FailuresAllowed,
			ShowServiceMessages:  opts.ShowServiceMessages,
			StatusProgressPeriod: opts.ShowProgressPeriod,
		}
	}

	switch resource.GroupVersionKind().GroupKind() {
	case schema.GroupKind{Group: "apps", Kind: "Deployment"}:
		t.specsForTrackReadiness.Deployments = append(t.specsForTrackReadiness.Deployments, *multitrackSpec)
	case schema.GroupKind{Group: "apps", Kind: "StatefulSet"}:
		t.specsForTrackReadiness.StatefulSets = append(t.specsForTrackReadiness.StatefulSets, *multitrackSpec)
	case schema.GroupKind{Group: "apps", Kind: "DaemonSet"}:
		t.specsForTrackReadiness.DaemonSets = append(t.specsForTrackReadiness.DaemonSets, *multitrackSpec)
	case schema.GroupKind{Group: "batch", Kind: "Job"}:
		t.specsForTrackReadiness.Jobs = append(t.specsForTrackReadiness.Jobs, *multitrackSpec)
	case schema.GroupKind{Group: "flagger", Kind: "Canary"}:
		t.specsForTrackReadiness.Canaries = append(t.specsForTrackReadiness.Canaries, *multitrackSpec)
	default:
		t.specsForTrackReadiness.Generics = append(t.specsForTrackReadiness.Generics, genericSpec)
	}

	return nil
}

func (t *ResourceTracker) TrackReadiness(ctx context.Context, opts TrackReadinessOptions) error {
	t.kubedogLock.Lock()
	defer t.kubedogLock.Unlock()

	multitrackOpts := multitrack.MultitrackOptions{
		StatusProgressPeriod: opts.ShowProgressPeriod,
		Options: tracker.Options{
			Timeout:      opts.Timeout,
			LogsFromTime: time.Now(),
		},
		DynamicClient:   t.dynamicClient,
		DiscoveryClient: t.discoveryClient,
		Mapper:          t.mapper,
	}

	log.Default.TraceStruct(ctx, t.specsForTrackReadiness, "Multitrack specs:")
	log.Default.TraceStruct(ctx, multitrackOpts, "Multitrack options:")

	if err := log.Default.InfoProcess(ctx, "Tracking resources readiness").DoError(func() error {
		return multitrack.Multitrack(t.staticClient, t.specsForTrackReadiness, multitrackOpts)
	}); err != nil {
		return err
	}
	t.specsForTrackReadiness = multitrack.MultitrackSpecs{}

	return nil
}

func (t *ResourceTracker) AddResourceToTrackDeletion(resource *resrcid.ResourceID) error {
	t.kubedogLock.Lock()
	defer t.kubedogLock.Unlock()

	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return fmt.Errorf("error getting resource GroupVersionResource: %w", err)
	}

	spec := &elimination.EliminationTrackerSpec{
		ResourceName:         resource.Name(),
		Namespace:            resource.Namespace(),
		GroupVersionResource: gvr,
	}

	t.specsForTrackDeletion = append(t.specsForTrackDeletion, spec)

	return nil
}

func (t *ResourceTracker) TrackDeletion(ctx context.Context, opts TrackDeletionOptions) error {
	t.kubedogLock.Lock()
	defer t.kubedogLock.Unlock()

	eliminationOpts := elimination.EliminationTrackerOptions{
		Timeout:              opts.Timeout,
		StatusProgressPeriod: opts.ShowProgressPeriod,
	}

	log.Default.TraceStruct(ctx, t.specsForTrackDeletion, "Elimination specs:")
	log.Default.TraceStruct(ctx, eliminationOpts, "Elimination options:")

	if err := log.Default.InfoProcess(ctx, "Tracking resources deletion").DoError(func() error {
		return elimination.TrackUntilEliminated(ctx, t.dynamicClient, t.specsForTrackDeletion, eliminationOpts)
	}); err != nil {
		return err
	}
	t.specsForTrackDeletion = []*elimination.EliminationTrackerSpec{}

	return nil
}

func (t *ResourceTracker) WaitCreation(ctx context.Context, resource *resrcid.ResourceID, opts WaitCreationOptions) error {
	namespaced, err := resource.Namespaced()
	if err != nil {
		return fmt.Errorf("error checking whether resource is namespaced: %w", err)
	}

	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return fmt.Errorf("error getting resource GroupVersionResource: %w", err)
	}

	var clientResource dynamic.ResourceInterface
	if namespaced {
		clientResource = t.dynamicClient.Resource(gvr).Namespace(resource.Namespace())
	} else {
		clientResource = t.dynamicClient.Resource(gvr)
	}

	log.Default.Debug(ctx, "Polling for resource %q", resource.HumanID())
	if err := wait.PollImmediate(700*time.Millisecond, opts.Timeout, func() (bool, error) {
		if _, err := clientResource.Get(ctx, resource.Name(), metav1.GetOptions{}); err != nil {
			if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) || err == io.EOF || err == io.ErrUnexpectedEOF || apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("error getting resource %q: %w", resource.HumanID(), err)
		}

		return true, nil
	}); err != nil {
		return fmt.Errorf("error polling resource %q: %w", resource.HumanID(), err)
	}

	return nil
}

func (t *ResourceTracker) WaitDeletion(ctx context.Context, resource *resrcid.ResourceID, opts WaitDeletionOptions) error {
	namespaced, err := resource.Namespaced()
	if err != nil {
		return fmt.Errorf("error checking whether resource is namespaced: %w", err)
	}

	gvr, err := resource.GroupVersionResource()
	if err != nil {
		return fmt.Errorf("error getting resource GroupVersionResource: %w", err)
	}

	var clientResource dynamic.ResourceInterface
	if namespaced {
		clientResource = t.dynamicClient.Resource(gvr).Namespace(resource.Namespace())
	} else {
		clientResource = t.dynamicClient.Resource(gvr)
	}

	log.Default.Debug(ctx, "Polling for resource %q", resource.HumanID())
	if err := wait.PollImmediate(700*time.Millisecond, opts.Timeout, func() (bool, error) {
		_, err := clientResource.Get(ctx, resource.Name(), metav1.GetOptions{})
		if err != nil {
			if apierrors.IsResourceExpired(err) || apierrors.IsGone(err) || err == io.EOF || err == io.ErrUnexpectedEOF {
				return false, nil
			}

			if apierrors.IsNotFound(err) {
				return true, nil
			}

			return false, fmt.Errorf("error getting resource %q: %w", resource.HumanID(), err)
		}

		return false, nil
	}); err != nil {
		return fmt.Errorf("error polling resource %q: %w", resource.HumanID(), err)
	}

	return nil
}
