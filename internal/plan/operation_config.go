package plan

import (
	"regexp"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	helmrelease "github.com/werf/3p-helm/pkg/release"
	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/resource/spec"
)

var (
	_ OperationConfig = (*OperationConfigApply)(nil)
	_ OperationConfig = (*OperationConfigCreate)(nil)
	_ OperationConfig = (*OperationConfigCreateRelease)(nil)
	_ OperationConfig = (*OperationConfigDelete)(nil)
	_ OperationConfig = (*OperationConfigDeleteRelease)(nil)
	_ OperationConfig = (*OperationConfigNoop)(nil)
	_ OperationConfig = (*OperationConfigRecreate)(nil)
	_ OperationConfig = (*OperationConfigTrackAbsence)(nil)
	_ OperationConfig = (*OperationConfigTrackPresence)(nil)
	_ OperationConfig = (*OperationConfigTrackReadiness)(nil)
	_ OperationConfig = (*OperationConfigUpdate)(nil)
	_ OperationConfig = (*OperationConfigUpdateRelease)(nil)
)

type OperationConfig interface {
	ID() string
	IDHuman() string
}

type OperationConfigNoop struct {
	OpID string
}

func (c *OperationConfigNoop) ID() string {
	return c.OpID
}

func (c *OperationConfigNoop) IDHuman() string {
	return c.OpID
}

type OperationConfigCreate struct {
	ResourceSpec  *spec.ResourceSpec
	ForceReplicas *int
}

func (c *OperationConfigCreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigCreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

type OperationConfigRecreate struct {
	ResourceSpec      *spec.ResourceSpec
	DeletePropagation metav1.DeletionPropagation
	ForceReplicas     *int
}

func (c *OperationConfigRecreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigRecreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

type OperationConfigUpdate struct {
	ResourceSpec *spec.ResourceSpec
}

func (c *OperationConfigUpdate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigUpdate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

type OperationConfigApply struct {
	ResourceSpec *spec.ResourceSpec
}

func (c *OperationConfigApply) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigApply) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

type OperationConfigDelete struct {
	ResourceMeta      *spec.ResourceMeta
	DeletePropagation metav1.DeletionPropagation
}

func (c *OperationConfigDelete) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigDelete) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

type OperationConfigTrackReadiness struct {
	ResourceMeta *spec.ResourceMeta

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

type OperationConfigTrackPresence struct {
	ResourceMeta *spec.ResourceMeta
}

func (c *OperationConfigTrackPresence) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackPresence) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

type OperationConfigTrackAbsence struct {
	ResourceMeta *spec.ResourceMeta
}

func (c *OperationConfigTrackAbsence) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackAbsence) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

type OperationConfigCreateRelease struct {
	Release *helmrelease.Release
}

func (c *OperationConfigCreateRelease) ID() string {
	return c.Release.ID()
}

func (c *OperationConfigCreateRelease) IDHuman() string {
	return c.Release.IDHuman()
}

type OperationConfigUpdateRelease struct {
	Release *helmrelease.Release
}

func (c *OperationConfigUpdateRelease) ID() string {
	return c.Release.ID()
}

func (c *OperationConfigUpdateRelease) IDHuman() string {
	return c.Release.IDHuman()
}

type OperationConfigDeleteRelease struct {
	ReleaseName      string
	ReleaseNamespace string
	ReleaseRevision  int
}

func (c *OperationConfigDeleteRelease) ID() string {
	return helmrelease.ReleaseID(c.ReleaseNamespace, c.ReleaseName, c.ReleaseRevision)
}

func (c *OperationConfigDeleteRelease) IDHuman() string {
	return helmrelease.ReleaseIDHuman(c.ReleaseNamespace, c.ReleaseName, c.ReleaseRevision)
}
