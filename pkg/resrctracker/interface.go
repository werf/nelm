package resrctracker

import (
	"context"
	"regexp"
	"time"

	"github.com/werf/nelm/pkg/resrcid"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
)

type ResourceTrackerer interface {
	AddResourceToTrackReadiness(resource *resrcid.ResourceID, opts AddResourceToTrackReadinessOptions) error
	TrackReadiness(ctx context.Context, opts TrackReadinessOptions) error
	AddResourceToTrackDeletion(resource *resrcid.ResourceID) error
	TrackDeletion(ctx context.Context, opts TrackDeletionOptions) error
	WaitCreation(ctx context.Context, resource *resrcid.ResourceID, opts WaitCreationOptions) error
	WaitDeletion(ctx context.Context, resource *resrcid.ResourceID, opts WaitDeletionOptions) error
}

type AddResourceToTrackReadinessOptions struct {
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

type TrackReadinessOptions struct {
	Timeout            time.Duration
	ShowProgressPeriod time.Duration
}

type TrackDeletionOptions struct {
	Timeout            time.Duration
	ShowProgressPeriod time.Duration
}

type WaitCreationOptions struct {
	Timeout time.Duration
}

type WaitDeletionOptions struct {
	Timeout time.Duration
}
