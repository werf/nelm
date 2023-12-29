package opertn

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"helm.sh/helm/v3/pkg/werf/resrcid"
	"helm.sh/helm/v3/pkg/werf/resrctracker"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

var _ Operation = (*TrackResourcesReadinessOperation)(nil)

const TypeTrackResourcesReadinessOperation = "track-resources-readiness"

func NewTrackResourcesReadinessOperation(
	id string,
	tracker resrctracker.ResourceTrackerer,
	opts TrackResourcesReadinessOperationOptions,
) *TrackResourcesReadinessOperation {
	return &TrackResourcesReadinessOperation{
		id:                 id,
		tracker:            tracker,
		timeout:            opts.Timeout,
		showProgressPeriod: opts.ShowProgressPeriod,
	}
}

type TrackResourcesReadinessOperationOptions struct {
	Timeout            time.Duration
	ShowProgressPeriod time.Duration
}

type TrackResourcesReadinessOperation struct {
	id                 string
	resources          []*ResourceToTrackReadiness
	tracker            resrctracker.ResourceTrackerer
	timeout            time.Duration
	showProgressPeriod time.Duration
	status             Status
}

func (o *TrackResourcesReadinessOperation) AddResource(resource *ResourceToTrackReadiness) *TrackResourcesReadinessOperation {
	o.resources = append(o.resources, resource)
	return o
}

func (o *TrackResourcesReadinessOperation) Execute(ctx context.Context) error {
	if o.Empty() {
		o.status = StatusCompleted
		return nil
	}

	for _, res := range o.resources {
		if err := o.tracker.AddResourceToTrackReadiness(res.Resource, resrctracker.AddResourceToTrackReadinessOptions{
			FailuresAllowed:                        res.FailuresAllowed,
			LogRegex:                               res.LogRegex,
			LogRegexesForContainers:                res.LogRegexesForContainers,
			SkipLogsForContainers:                  res.SkipLogsForContainers,
			ShowLogsOnlyForContainers:              res.ShowLogsOnlyForContainers,
			IgnoreReadinessProbeFailsForContainers: res.IgnoreReadinessProbeFailsForContainers,
			TrackTerminationMode:                   res.TrackTerminationMode,
			FailMode:                               res.FailMode,
			SkipLogs:                               res.SkipLogs,
			ShowServiceMessages:                    res.ShowServiceMessages,
			NoActivityTimeout:                      res.NoActivityTimeout,
			Timeout:                                res.Timeout,
			ShowProgressPeriod:                     res.ShowProgressPeriod,
		}); err != nil {
			o.status = StatusFailed
			return fmt.Errorf("error adding resource for readiness tracking: %w", err)
		}
	}

	if err := o.tracker.TrackReadiness(ctx, resrctracker.TrackReadinessOptions{
		Timeout:            o.timeout,
		ShowProgressPeriod: o.showProgressPeriod,
	}); err != nil {
		o.status = StatusFailed
		return fmt.Errorf("error tracking resources readiness: %w", err)
	}

	o.status = StatusCompleted

	return nil
}

func (o *TrackResourcesReadinessOperation) ID() string {
	return o.id
}

func (o *TrackResourcesReadinessOperation) HumanID() string {
	return o.id
}

func (o *TrackResourcesReadinessOperation) Status() Status {
	return o.status
}

func (o *TrackResourcesReadinessOperation) Type() Type {
	return TypeTrackResourcesReadinessOperation
}

func (o *TrackResourcesReadinessOperation) Empty() bool {
	return len(o.resources) == 0
}

type ResourceToTrackReadiness struct {
	Resource                               *resrcid.ResourceID
	FailuresAllowed                        *int
	LogRegex                               *regexp.Regexp
	LogRegexesForContainers                map[string]*regexp.Regexp
	SkipLogsForContainers                  []string
	ShowLogsOnlyForContainers              []string
	IgnoreReadinessProbeFailsForContainers map[string]time.Duration
	TrackTerminationMode                   multitrack.TrackTerminationMode
	FailMode                               multitrack.FailMode
	SkipLogs                               bool
	ShowServiceMessages                    bool
	NoActivityTimeout                      *time.Duration
	Timeout                                time.Duration
	ShowProgressPeriod                     time.Duration
}
