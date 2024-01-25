package opertn

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/werf/kubedog/pkg/trackers/dyntracker"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/logstore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/statestore"
	"github.com/werf/kubedog/pkg/trackers/dyntracker/util"
	"github.com/werf/nelm/pkg/resrcid"
)

var _ Operation = (*TrackResourceReadinessOperation)(nil)

const TypeTrackResourceReadinessOperation = "track-resource-readiness"

func NewTrackResourceReadinessOperation(
	resource *resrcid.ResourceID,
	taskState *util.Concurrent[*statestore.ReadinessTaskState],
	logStore *util.Concurrent[*logstore.LogStore],
	staticClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	discoveryClient discovery.CachedDiscoveryInterface,
	mapper meta.ResettableRESTMapper,
	opts TrackResourceReadinessOperationOptions,
) *TrackResourceReadinessOperation {
	return &TrackResourceReadinessOperation{
		resource:                                 resource,
		taskState:                                taskState,
		logStore:                                 logStore,
		staticClient:                             staticClient,
		dynamicClient:                            dynamicClient,
		discoveryClient:                          discoveryClient,
		mapper:                                   mapper,
		timeout:                                  opts.Timeout,
		noActivityTimeout:                        opts.NoActivityTimeout,
		ignoreReadinessProbeFailsByContainerName: opts.IgnoreReadinessProbeFailsByContainerName,
		captureLogsFromTime:                      opts.CaptureLogsFromTime,
		saveLogsOnlyForContainers:                opts.SaveLogsOnlyForContainers,
		saveLogsByRegex:                          opts.SaveLogsByRegex,
		saveLogsByRegexForContainers:             opts.SaveLogsByRegexForContainers,
		ignoreLogs:                               opts.IgnoreLogs,
		ignoreLogsForContainers:                  opts.IgnoreLogsForContainers,
		saveEvents:                               opts.SaveEvents,
	}
}

type TrackResourceReadinessOperationOptions struct {
	Timeout                                  time.Duration
	NoActivityTimeout                        time.Duration
	IgnoreReadinessProbeFailsByContainerName map[string]time.Duration
	CaptureLogsFromTime                      time.Time
	SaveLogsOnlyForContainers                []string
	SaveLogsByRegex                          *regexp.Regexp
	SaveLogsByRegexForContainers             map[string]*regexp.Regexp
	IgnoreLogs                               bool
	IgnoreLogsForContainers                  []string
	SaveEvents                               bool
}

type TrackResourceReadinessOperation struct {
	resource                                 *resrcid.ResourceID
	taskState                                *util.Concurrent[*statestore.ReadinessTaskState]
	logStore                                 *util.Concurrent[*logstore.LogStore]
	staticClient                             kubernetes.Interface
	dynamicClient                            dynamic.Interface
	discoveryClient                          discovery.CachedDiscoveryInterface
	mapper                                   meta.ResettableRESTMapper
	timeout                                  time.Duration
	noActivityTimeout                        time.Duration
	ignoreReadinessProbeFailsByContainerName map[string]time.Duration
	captureLogsFromTime                      time.Time
	saveLogsOnlyForContainers                []string
	saveLogsByRegex                          *regexp.Regexp
	saveLogsByRegexForContainers             map[string]*regexp.Regexp
	ignoreLogs                               bool
	ignoreLogsForContainers                  []string
	saveEvents                               bool

	status Status
}

func (o *TrackResourceReadinessOperation) Execute(ctx context.Context) error {
	tracker, err := dyntracker.NewDynamicReadinessTracker(ctx, o.taskState, o.logStore, o.staticClient, o.dynamicClient, o.discoveryClient, o.mapper, dyntracker.DynamicReadinessTrackerOptions{
		Timeout:                                  o.timeout,
		NoActivityTimeout:                        o.noActivityTimeout,
		IgnoreReadinessProbeFailsByContainerName: o.ignoreReadinessProbeFailsByContainerName,
		CaptureLogsFromTime:                      o.captureLogsFromTime,
		SaveLogsOnlyForContainers:                o.saveLogsOnlyForContainers,
		SaveLogsByRegex:                          o.saveLogsByRegex,
		SaveLogsByRegexForContainers:             o.saveLogsByRegexForContainers,
		IgnoreLogs:                               o.ignoreLogs,
		IgnoreLogsForContainers:                  o.ignoreLogsForContainers,
		SaveEvents:                               o.saveEvents,
	})
	if err != nil {
		return fmt.Errorf("create readiness tracker: %w", err)
	}

	if err := tracker.Track(ctx); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("track resource readiness: %w", err)
	}

	o.status = StatusCompleted
	return nil
}

func (o *TrackResourceReadinessOperation) ID() string {
	return TypeTrackResourceReadinessOperation + "/" + o.resource.ID()
}

func (o *TrackResourceReadinessOperation) HumanID() string {
	return "track resource readiness: " + o.resource.HumanID()
}

func (o *TrackResourceReadinessOperation) Status() Status {
	return o.status
}

func (o *TrackResourceReadinessOperation) Type() Type {
	return TypeTrackResourceReadinessOperation
}

func (o *TrackResourceReadinessOperation) Empty() bool {
	return false
}
