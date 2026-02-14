package plan

import (
	"encoding/json"
	"fmt"
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

// Any config that is needed to execute the operation goes here, as long as it doesn't fit into
// other fields of the Operation struct. The underlying struct can have any number of fields of any
// kind, just make sure they are easily serializable.
type OperationConfig interface {
	ID() string
	IDHuman() string
	MarshalJSON() ([]byte, error)
}

type OperationConfigNoop struct {
	OpID string `json:"opId"`
}

func (c *OperationConfigNoop) ID() string {
	return c.OpID
}

func (c *OperationConfigNoop) IDHuman() string {
	return c.OpID
}

func (c *OperationConfigNoop) MarshalJSON() ([]byte, error) {
	type operationConfigNoopJSON OperationConfigNoop

	data, err := json.Marshal((*operationConfigNoopJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal noop operation config: %w", err)
	}

	return data, nil
}

type OperationConfigCreate struct {
	ResourceSpec  *spec.ResourceSpec `json:"resourceSpec"`
	ForceReplicas *int               `json:"forceReplicas,omitempty"`
}

func (c *OperationConfigCreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigCreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

func (c *OperationConfigCreate) MarshalJSON() ([]byte, error) {
	type operationConfigCreateJSON OperationConfigCreate

	data, err := json.Marshal((*operationConfigCreateJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal create operation config: %w", err)
	}

	return data, nil
}

type OperationConfigRecreate struct {
	ResourceSpec      *spec.ResourceSpec         `json:"resourceSpec"`
	DeletePropagation metav1.DeletionPropagation `json:"deletePropagation"`
	ForceReplicas     *int                       `json:"forceReplicas,omitempty"`
}

func (c *OperationConfigRecreate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigRecreate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

func (c *OperationConfigRecreate) MarshalJSON() ([]byte, error) {
	type operationConfigRecreateJSON OperationConfigRecreate

	data, err := json.Marshal((*operationConfigRecreateJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal recreate operation config: %w", err)
	}

	return data, nil
}

type OperationConfigUpdate struct {
	ResourceSpec *spec.ResourceSpec `json:"resourceSpec"`
}

func (c *OperationConfigUpdate) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigUpdate) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

func (c *OperationConfigUpdate) MarshalJSON() ([]byte, error) {
	type operationConfigUpdateJSON OperationConfigUpdate

	data, err := json.Marshal((*operationConfigUpdateJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal update operation config: %w", err)
	}

	return data, nil
}

type OperationConfigApply struct {
	ResourceSpec *spec.ResourceSpec `json:"resourceSpec"`
}

func (c *OperationConfigApply) ID() string {
	return c.ResourceSpec.ID()
}

func (c *OperationConfigApply) IDHuman() string {
	return c.ResourceSpec.IDHuman()
}

func (c *OperationConfigApply) MarshalJSON() ([]byte, error) {
	type operationConfigApplyJSON OperationConfigApply

	data, err := json.Marshal((*operationConfigApplyJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal apply operation config: %w", err)
	}

	return data, nil
}

type OperationConfigDelete struct {
	ResourceMeta      *spec.ResourceMeta         `json:"resourceMeta"`
	DeletePropagation metav1.DeletionPropagation `json:"deletePropagation"`
}

func (c *OperationConfigDelete) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigDelete) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

func (c *OperationConfigDelete) MarshalJSON() ([]byte, error) {
	type operationConfigDeleteJSON OperationConfigDelete

	data, err := json.Marshal((*operationConfigDeleteJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal delete operation config: %w", err)
	}

	return data, nil
}

type OperationConfigTrackReadiness struct {
	ResourceMeta *spec.ResourceMeta `json:"resourceMeta"`

	FailMode                                 multitrack.FailMode       `json:"failMode"`
	FailuresAllowed                          int                       `json:"failuresAllowed"`
	IgnoreLogs                               bool                      `json:"ignoreLogs"`
	IgnoreLogsForContainers                  []string                  `json:"ignoreLogsForContainers,omitempty"`
	IgnoreLogsByRegex                        *regexp.Regexp            `json:"ignoreLogsByRegex"`
	IgnoreLogsByRegexForContainers           map[string]*regexp.Regexp `json:"ignoreLogsByRegexForContainers"`
	IgnoreReadinessProbeFailsByContainerName map[string]time.Duration  `json:"ignoreReadinessProbeFailsByContainerName,omitempty"`
	NoActivityTimeout                        time.Duration             `json:"noActivityTimeout"`
	SaveEvents                               bool                      `json:"saveEvents"`
	SaveLogsByRegex                          *regexp.Regexp            `json:"saveLogsByRegex"`
	SaveLogsByRegexForContainers             map[string]*regexp.Regexp `json:"saveLogsByRegexForContainers"`
	SaveLogsOnlyForContainers                []string                  `json:"saveLogsOnlyForContainers,omitempty"`
	SaveLogsOnlyForNumberOfReplicas          int                       `json:"saveLogsOnlyForNumberOfReplicas"`
}

func (c *OperationConfigTrackReadiness) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackReadiness) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

func (c *OperationConfigTrackReadiness) MarshalJSON() ([]byte, error) {
	type operationConfigTrackReadinessJSON OperationConfigTrackReadiness

	data, err := json.Marshal((*operationConfigTrackReadinessJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal track readiness operation config: %w", err)
	}

	return data, nil
}

type OperationConfigTrackPresence struct {
	ResourceMeta *spec.ResourceMeta `json:"resourceMeta"`
}

func (c *OperationConfigTrackPresence) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackPresence) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

func (c *OperationConfigTrackPresence) MarshalJSON() ([]byte, error) {
	type operationConfigTrackPresenceJSON OperationConfigTrackPresence

	data, err := json.Marshal((*operationConfigTrackPresenceJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal track presence operation config: %w", err)
	}

	return data, nil
}

type OperationConfigTrackAbsence struct {
	ResourceMeta *spec.ResourceMeta `json:"resourceMeta"`
}

func (c *OperationConfigTrackAbsence) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackAbsence) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

func (c *OperationConfigTrackAbsence) MarshalJSON() ([]byte, error) {
	type operationConfigTrackAbsenceJSON OperationConfigTrackAbsence

	data, err := json.Marshal((*operationConfigTrackAbsenceJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal track absence operation config: %w", err)
	}

	return data, nil
}

type OperationConfigCreateRelease struct {
	Release *helmrelease.Release `json:"release"`
}

func (c *OperationConfigCreateRelease) ID() string {
	return c.Release.ID()
}

func (c *OperationConfigCreateRelease) IDHuman() string {
	return c.Release.IDHuman()
}

func (c *OperationConfigCreateRelease) MarshalJSON() ([]byte, error) {
	type operationConfigCreateReleaseJSON OperationConfigCreateRelease

	data, err := json.Marshal((*operationConfigCreateReleaseJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal create release operation config: %w", err)
	}

	return data, nil
}

type OperationConfigUpdateRelease struct {
	Release *helmrelease.Release `json:"release"`
}

func (c *OperationConfigUpdateRelease) ID() string {
	return c.Release.ID()
}

func (c *OperationConfigUpdateRelease) IDHuman() string {
	return c.Release.IDHuman()
}

func (c *OperationConfigUpdateRelease) MarshalJSON() ([]byte, error) {
	type operationConfigUpdateReleaseJSON OperationConfigUpdateRelease

	data, err := json.Marshal((*operationConfigUpdateReleaseJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal update release operation config: %w", err)
	}

	return data, nil
}

type OperationConfigDeleteRelease struct {
	ReleaseName      string `json:"releaseName"`
	ReleaseNamespace string `json:"releaseNamespace"`
	ReleaseRevision  int    `json:"releaseRevision"`
}

func (c *OperationConfigDeleteRelease) ID() string {
	return helmrelease.ReleaseID(c.ReleaseNamespace, c.ReleaseName, c.ReleaseRevision)
}

func (c *OperationConfigDeleteRelease) IDHuman() string {
	return helmrelease.ReleaseIDHuman(c.ReleaseNamespace, c.ReleaseName, c.ReleaseRevision)
}

func (c *OperationConfigDeleteRelease) MarshalJSON() ([]byte, error) {
	type operationConfigDeleteReleaseJSON OperationConfigDeleteRelease

	data, err := json.Marshal((*operationConfigDeleteReleaseJSON)(c))
	if err != nil {
		return nil, fmt.Errorf("marshal delete release operation config: %w", err)
	}

	return data, nil
}
