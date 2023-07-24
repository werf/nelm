package resourcetracker

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"helm.sh/helm/v3/pkg/werf/log"
	"helm.sh/helm/v3/pkg/werf/util"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/kubedog/pkg/tracker"
	"github.com/werf/kubedog/pkg/tracker/resid"
	"github.com/werf/kubedog/pkg/trackers/elimination"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack/generic"
)

func NewResourceTracker(staticClient kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.CachedDiscoveryInterface, mapper meta.ResettableRESTMapper, opts NewResourceTrackerOptions) *ResourceTracker {
	// FIXME(ilya-lesikov): move it somewhere higher
	// if os.Getenv("WERF_DISABLE_RESOURCES_WAITER") == "1" {
	// 	return nil
	// }

	var logger log.Logger
	if opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = log.NewNullLogger()
	}

	return &ResourceTracker{
		staticClient:    staticClient,
		dynamicClient:   dynamicClient,
		discoveryClient: discoveryClient,
		mapper:          mapper,
		logger:          logger,
	}
}

type NewResourceTrackerOptions struct {
	Logger log.Logger
}

type ResourceTracker struct {
	staticClient    kubernetes.Interface
	dynamicClient   dynamic.Interface
	discoveryClient discovery.CachedDiscoveryInterface
	mapper          meta.ResettableRESTMapper
	logger          log.Logger
	trackLock       sync.Mutex
}

func (t *ResourceTracker) TrackReadiness(ctx context.Context, opts TrackReadinessOptions, resources ...ReadinessTrackable) error {
	t.trackLock.Lock()
	defer t.trackLock.Unlock()

	specs := multitrack.MultitrackSpecs{}
	for _, res := range resources {
		switch res.GroupVersionKind().GroupKind() {
		case schema.GroupKind{Group: "apps", Kind: "Deployment"}:
			if spec, err := buildMultitrackSpec(res, opts.FallbackNamespace, t.mapper); err != nil {
				return fmt.Errorf("error building multitrack spec: %w", err)
			} else {
				specs.Deployments = append(specs.Deployments, *spec)
			}
		case schema.GroupKind{Group: "apps", Kind: "StatefulSet"}:
			if spec, err := buildMultitrackSpec(res, opts.FallbackNamespace, t.mapper); err != nil {
				return fmt.Errorf("error building multitrack spec: %w", err)
			} else {
				specs.StatefulSets = append(specs.StatefulSets, *spec)
			}
		case schema.GroupKind{Group: "apps", Kind: "DaemonSet"}:
			if spec, err := buildMultitrackSpec(res, opts.FallbackNamespace, t.mapper); err != nil {
				return fmt.Errorf("error building multitrack spec: %w", err)
			} else {
				specs.DaemonSets = append(specs.DaemonSets, *spec)
			}
		case schema.GroupKind{Group: "batch", Kind: "Job"}:
			if spec, err := buildMultitrackSpec(res, opts.FallbackNamespace, t.mapper); err != nil {
				return fmt.Errorf("error building multitrack spec: %w", err)
			} else {
				specs.Jobs = append(specs.Jobs, *spec)
			}
		case schema.GroupKind{Group: "flagger", Kind: "Canary"}:
			if spec, err := buildMultitrackSpec(res, opts.FallbackNamespace, t.mapper); err != nil {
				return fmt.Errorf("error building multitrack spec: %w", err)
			} else {
				specs.Canaries = append(specs.Canaries, *spec)
			}
		case schema.GroupKind{}:
			return fmt.Errorf("resource %q doesn't have Group and Kind", res.String())
		default:
			if spec, err := buildGenericTrackSpec(res, opts.Timeout, opts.ShowProgressPeriod, opts.FallbackNamespace, t.mapper); err != nil {
				return fmt.Errorf("error building generic track spec: %w", err)
			} else {
				specs.Generics = append(specs.Generics, spec)
			}
		}
	}
	t.logger.TraceStruct(specs, "Multitrack specs:")

	var logsFromTime time.Time
	if !opts.LogsFromTime.IsZero() {
		logsFromTime = opts.LogsFromTime
	} else {
		logsFromTime = time.Now()
	}

	multitrackOpts := multitrack.MultitrackOptions{
		StatusProgressPeriod: opts.ShowProgressPeriod,
		Options: tracker.Options{
			Timeout:      opts.Timeout,
			LogsFromTime: logsFromTime,
		},
		DynamicClient:   t.dynamicClient,
		DiscoveryClient: t.discoveryClient,
		Mapper:          t.mapper,
	}
	t.logger.TraceStruct(multitrackOpts, "Multitrack options:")

	return t.logger.LogBlock(func() error {
		return multitrack.Multitrack(t.staticClient, specs, multitrackOpts)
	}, "Tracking resources readiness")
}

func (t *ResourceTracker) TrackDeletion(ctx context.Context, opts TrackDeletionOptions, resources ...DeletionTrackable) error {
	t.trackLock.Lock()
	defer t.trackLock.Unlock()

	var specs []*elimination.EliminationTrackerSpec
	for _, res := range resources {
		if spec, err := buildEliminationTrackSpec(res, opts.FallbackNamespace, t.mapper); err != nil {
			return fmt.Errorf("error building elimination tracker spec: %w", err)
		} else {
			specs = append(specs, spec)
		}
	}
	t.logger.TraceStruct(specs, "Elimination specs:")

	eliminationOpts := elimination.EliminationTrackerOptions{Timeout: opts.Timeout, StatusProgressPeriod: opts.ShowProgressPeriod}
	t.logger.TraceStruct(eliminationOpts, "Elimination options:")

	return t.logger.LogBlock(func() error {
		return elimination.TrackUntilEliminated(ctx, t.dynamicClient, specs, eliminationOpts)
	}, "Tracking resources deletion")
}

func buildMultitrackSpec(res ReadinessTrackable, fallbackNamespace string, mapper meta.ResettableRESTMapper) (*multitrack.MultitrackSpec, error) {
	namespaced, err := util.IsResourceNamespaced(res.GroupVersionKind(), mapper)
	if err != nil {
		return nil, fmt.Errorf("error checking if resource %q is namespaced: %w", res.String(), err)
	}

	var namespace string
	if namespaced {
		if res.Namespace() != "" {
			namespace = res.Namespace()
		} else if fallbackNamespace != "" {
			namespace = fallbackNamespace
		} else {
			namespace = v1.NamespaceDefault
		}
	}

	failuresAllowed := res.FailuresAllowed()

	return &multitrack.MultitrackSpec{
		ResourceName:                             res.Name(),
		Namespace:                                namespace,
		TrackTerminationMode:                     res.TrackTerminationMode(),
		FailMode:                                 res.FailMode(),
		AllowFailuresCount:                       &failuresAllowed,
		IgnoreReadinessProbeFailsByContainerName: res.IgnoreReadinessProbeFailsForContainers(),
		LogRegex:                                 res.LogRegex(),
		LogRegexByContainerName:                  res.LogRegexesForContainers(),
		SkipLogs:                                 res.SkipLogs(),
		SkipLogsForContainers:                    res.SkipLogsForContainers(),
		ShowLogsOnlyForContainers:                res.ShowLogsOnlyForContainers(),
		ShowServiceMessages:                      res.ShowServiceMessages(),
	}, nil
}

func buildGenericTrackSpec(res ReadinessTrackable, timeout, showProgressPeriod time.Duration, fallbackNamespace string, mapper meta.ResettableRESTMapper) (*generic.Spec, error) {
	namespaced, err := util.IsResourceNamespaced(res.GroupVersionKind(), mapper)
	if err != nil {
		return nil, fmt.Errorf("error checking if resource %q is namespaced: %w", res.String(), err)
	}

	var namespace string
	if namespaced {
		if res.Namespace() != "" {
			namespace = res.Namespace()
		} else if fallbackNamespace != "" {
			namespace = fallbackNamespace
		} else {
			namespace = v1.NamespaceDefault
		}
	}

	resourceID := resid.NewResourceID(res.Name(), res.GroupVersionKind(), resid.NewResourceIDOptions{Namespace: namespace})
	failuresAllowed := res.FailuresAllowed()

	return &generic.Spec{
		ResourceID:           resourceID,
		Timeout:              timeout,
		NoActivityTimeout:    res.NoActivityTimeout(),
		TrackTerminationMode: generic.TrackTerminationMode(res.TrackTerminationMode()),
		FailMode:             generic.FailMode(res.FailMode()),
		AllowFailuresCount:   &failuresAllowed,
		ShowServiceMessages:  res.ShowServiceMessages(),
		StatusProgressPeriod: showProgressPeriod,
	}, nil
}

func buildEliminationTrackSpec(res DeletionTrackable, fallbackNamespace string, mapper meta.ResettableRESTMapper) (*elimination.EliminationTrackerSpec, error) {
	gvr, namespaced, err := util.ConvertGVKtoGVR(res.GroupVersionKind(), mapper)
	if err != nil {
		// FIXME(ilya-lesikov): remove this Reset() when all resets managed together with CRDs
		if strings.Contains(err.Error(), "no matches for kind") {
			mapper.Reset()
			gvr, _, err = util.ConvertGVKtoGVR(res.GroupVersionKind(), mapper)
		}

		if err != nil {
			return nil, fmt.Errorf("error converting GroupVersionKind to GroupVersionResource for resource %q: %w", res.String(), err)
		}
	}

	var namespace string
	if namespaced {
		if res.Namespace() != "" {
			namespace = res.Namespace()
		} else if fallbackNamespace != "" {
			namespace = fallbackNamespace
		} else {
			namespace = v1.NamespaceDefault
		}
	}

	return &elimination.EliminationTrackerSpec{
		GroupVersionResource: gvr,
		ResourceName:         res.Name(),
		Namespace:            namespace,
	}, nil
}

type TrackReadinessOptions struct {
	FallbackNamespace  string
	Timeout            time.Duration
	ShowProgressPeriod time.Duration
	LogsFromTime       time.Time
}

type TrackDeletionOptions struct {
	FallbackNamespace  string
	Timeout            time.Duration
	ShowProgressPeriod time.Duration
}

type ReadinessTrackable interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	// FIXME(ilya-lesikov):
	String() string

	FailuresAllowed() int
	LogRegex() *regexp.Regexp
	LogRegexesForContainers() map[string]*regexp.Regexp
	SkipLogsForContainers() []string
	ShowLogsOnlyForContainers() []string
	IgnoreReadinessProbeFailsForContainers() map[string]time.Duration
	TrackTerminationMode() multitrack.TrackTerminationMode
	FailMode() multitrack.FailMode
	SkipLogs() bool
	ShowServiceMessages() bool
	NoActivityTimeout() *time.Duration
}

type DeletionTrackable interface {
	Name() string
	Namespace() string
	GroupVersionKind() schema.GroupVersionKind
	// FIXME(ilya-lesikov):
	String() string
}
