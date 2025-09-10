package operation

import (
	"regexp"
	"time"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/resource/id"
)

const (
	OperationTypeTrackReadiness    OperationType    = "track-readiness"
	OperationVersionTrackReadiness OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigTrackReadiness)(nil)

type OperationConfigTrackReadiness struct {
	ResourceMeta *id.ResourceMeta

	FailMode                                 multitrack.FailMode
	FailuresAllowed                          int
	IgnoreLogs                               bool
	IgnoreLogsForContainers                  []string
	IgnoreReadinessProbeFailsByContainerName map[string]time.Duration
	NoActivityTimeout                        time.Duration
	SaveEvents                               bool
	SaveLogsByRegex                          *regexp.Regexp
	SaveLogsByRegexForContainers             map[string]*regexp.Regexp
	SaveLogsOnlyForContainers                []string
	SaveLogsOnlyForNumberOfReplicas          int
}

func (c *OperationConfigTrackReadiness) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackReadiness) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}
