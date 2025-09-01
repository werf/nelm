package operation

import (
	"regexp"
	"time"

	"github.com/werf/nelm/internal/resource/id"
)

const (
	OperationTypeTrackReadiness    = "track-readiness"
	OperationVersionTrackReadiness = 1
)

var _ OperationConfig = (*OperationConfigTrackReadiness)(nil)

type OperationConfigTrackReadiness struct {
	ResourceMeta                             *id.ResourceMeta
	NoActivityTimeout                        time.Duration
	IgnoreReadinessProbeFailsByContainerName map[string]time.Duration
	CaptureLogsFromTime                      time.Time
	SaveLogsOnlyForNumberOfReplicas          int
	SaveLogsOnlyForContainers                []string
	SaveLogsByRegex                          *regexp.Regexp
	SaveLogsByRegexForContainers             map[string]*regexp.Regexp
	IgnoreLogs                               bool
	IgnoreLogsForContainers                  []string
	SaveEvents                               bool
}

func (c *OperationConfigTrackReadiness) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackReadiness) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}
