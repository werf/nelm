package plan

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/werf/3p-helm/pkg/release"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/resource"
	"github.com/werf/nelm/internal/resource/meta"
)

type (
	OperationType      string
	OperationVersion   int
	OperationCategory  string
	OperationIteration int
	OperationStatus    string
	OperationConfig    interface {
		ID() string
		IDHuman() string
	}
)

const (
	OperationStatusUnknown   OperationStatus = "unknown"
	OperationStatusPending   OperationStatus = "pending"
	OperationStatusCompleted OperationStatus = "completed"
	OperationStatusFailed    OperationStatus = "failed"
)

const (
	OperationCategoryMeta     OperationCategory = "meta"
	OperationCategoryResource OperationCategory = "resource"
	OperationCategoryTrack    OperationCategory = "track"
	OperationCategoryRelease  OperationCategory = "release"
)

type Operation struct {
	Type      OperationType
	Version   OperationVersion
	Category  OperationCategory
	Iteration OperationIteration
	Status    OperationStatus
	Config    OperationConfig
}

func (o *Operation) ID() string {
	return OperationID(o.Type, o.Version, o.Iteration, o.Config.ID())
}

func (o *Operation) IDHuman() string {
	return OperationIDHuman(o.Type, o.Iteration, o.Config.IDHuman())
}

func OperationID(t OperationType, version OperationVersion, iteration OperationIteration, configID string) string {
	return fmt.Sprintf("%s/%s/%s/%s", t, version, iteration, configID)
}

func OperationIDHuman(t OperationType, iteration OperationIteration, configIDHuman string) string {
	id := fmt.Sprintf("%s: ", strings.ReplaceAll(string(t), "-", " "))
	id += configIDHuman
	if iteration > 0 {
		id += fmt.Sprintf(" (%d)", iteration)
	}

	return id
}

const (
	OperationTypeNoop    OperationType    = "noop"
	OperationVersionNoop OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigNoop)(nil)

type OperationConfigNoop struct {
	OpID string
}

func (c *OperationConfigNoop) ID() string {
	return c.OpID
}

func (c *OperationConfigNoop) IDHuman() string {
	return c.OpID
}

const (
	OperationTypeCreate    OperationType    = "create"
	OperationVersionCreate OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigCreate)(nil)

type OperationConfigCreate struct {
	ResourceSpec  *resource.ResourceSpec
	ForceReplicas *int
}

func (c *OperationConfigCreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigCreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

const (
	OperationTypeRecreate    OperationType    = "recreate"
	OperationVersionRecreate OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigRecreate)(nil)

type OperationConfigRecreate struct {
	ResourceSpec  *resource.ResourceSpec
	ForceReplicas *int
}

func (c *OperationConfigRecreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigRecreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

const (
	OperationTypeUpdate    OperationType    = "update"
	OperationVersionUpdate OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigUpdate)(nil)

type OperationConfigUpdate struct {
	ResourceSpec *resource.ResourceSpec
}

func (c *OperationConfigUpdate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigUpdate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

const (
	OperationTypeApply    OperationType    = "apply"
	OperationVersionApply OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigApply)(nil)

type OperationConfigApply struct {
	ResourceSpec *resource.ResourceSpec
}

func (c *OperationConfigApply) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigApply) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

const (
	OperationTypeDelete    OperationType    = "delete"
	OperationVersionDelete OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigDelete)(nil)

type OperationConfigDelete struct {
	ResourceMeta *meta.ResourceMeta
}

func (c *OperationConfigDelete) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigDelete) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

const (
	OperationTypeTrackReadiness    OperationType    = "track-readiness"
	OperationVersionTrackReadiness OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigTrackReadiness)(nil)

type OperationConfigTrackReadiness struct {
	ResourceMeta *meta.ResourceMeta

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

const (
	OperationTypeTrackPresence    OperationType    = "track-presence"
	OperationVersionTrackPresence OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigTrackPresence)(nil)

type OperationConfigTrackPresence struct {
	ResourceMeta *meta.ResourceMeta
}

func (c *OperationConfigTrackPresence) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackPresence) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

const (
	OperationTypeTrackAbsence    OperationType    = "track-absence"
	OperationVersionTrackAbsence OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigTrackAbsence)(nil)

type OperationConfigTrackAbsence struct {
	ResourceMeta *meta.ResourceMeta
}

func (c *OperationConfigTrackAbsence) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackAbsence) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

const (
	OperationTypeCreateRelease    OperationType    = "create-release"
	OperationVersionCreateRelease OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigCreateRelease)(nil)

type OperationConfigCreateRelease struct {
	Release *release.Release
}

func (c *OperationConfigCreateRelease) ID() string {
	return c.Release.ID()
}

func (c *OperationConfigCreateRelease) IDHuman() string {
	return c.Release.IDHuman()
}

const (
	OperationTypeUpdateRelease    OperationType    = "update-release"
	OperationVersionUpdateRelease OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigUpdateRelease)(nil)

type OperationConfigUpdateRelease struct {
	Release *release.Release
}

func (c *OperationConfigUpdateRelease) ID() string {
	return c.Release.ID()
}

func (c *OperationConfigUpdateRelease) IDHuman() string {
	return c.Release.IDHuman()
}

const (
	OperationTypeDeleteRelease    OperationType    = "delete-release"
	OperationVersionDeleteRelease OperationVersion = 1
)

var _ OperationConfig = (*OperationConfigDeleteRelease)(nil)

type OperationConfigDeleteRelease struct {
	ReleaseName      string
	ReleaseNamespace string
	ReleaseRevision  int
}

func (c *OperationConfigDeleteRelease) ID() string {
	return release.ReleaseID(c.ReleaseNamespace, c.ReleaseName, c.ReleaseRevision)
}

func (c *OperationConfigDeleteRelease) IDHuman() string {
	return release.ReleaseIDHuman(c.ReleaseNamespace, c.ReleaseName, c.ReleaseRevision)
}
