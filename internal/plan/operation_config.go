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
	Kind() string
	MarshalJSON() ([]byte, error)
	UnmarshalJSON(data []byte) error
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

func (c *OperationConfigNoop) Kind() string {
	return string(OperationTypeNoop)
}

func (c *OperationConfigNoop) MarshalJSON() ([]byte, error) {
	type operationConfigNoopJSON OperationConfigNoop

	return json.Marshal((*operationConfigNoopJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigNoop) UnmarshalJSON(data []byte) error {
	type operationConfigNoopJSON OperationConfigNoop

	if err := json.Unmarshal(data, (*operationConfigNoopJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal noop operation config data: %w", err)
	}

	return nil
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

func (c *OperationConfigCreate) Kind() string {
	return string(OperationTypeCreate)
}

func (c *OperationConfigCreate) MarshalJSON() ([]byte, error) {
	type operationConfigCreateJSON OperationConfigCreate

	return json.Marshal((*operationConfigCreateJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigCreate) UnmarshalJSON(data []byte) error {
	type operationConfigCreateJSON OperationConfigCreate

	if err := json.Unmarshal(data, (*operationConfigCreateJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal create operation config data: %w", err)
	}

	return nil
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

func (c *OperationConfigRecreate) Kind() string {
	return string(OperationTypeRecreate)
}

func (c *OperationConfigRecreate) MarshalJSON() ([]byte, error) {
	type operationConfigRecreateJSON OperationConfigRecreate

	return json.Marshal((*operationConfigRecreateJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigRecreate) UnmarshalJSON(data []byte) error {
	type operationConfigRecreateJSON OperationConfigRecreate

	if err := json.Unmarshal(data, (*operationConfigRecreateJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal recreate operation config data: %w", err)
	}

	return nil
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

func (c *OperationConfigUpdate) Kind() string {
	return string(OperationTypeUpdate)
}

func (c *OperationConfigUpdate) MarshalJSON() ([]byte, error) {
	type operationConfigUpdateJSON OperationConfigUpdate

	return json.Marshal((*operationConfigUpdateJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigUpdate) UnmarshalJSON(data []byte) error {
	type operationConfigUpdateJSON OperationConfigUpdate

	if err := json.Unmarshal(data, (*operationConfigUpdateJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal update operation config data: %w", err)
	}

	return nil
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

func (c *OperationConfigApply) Kind() string {
	return string(OperationTypeApply)
}

func (c *OperationConfigApply) MarshalJSON() ([]byte, error) {
	type operationConfigApplyJSON OperationConfigApply

	return json.Marshal((*operationConfigApplyJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigApply) UnmarshalJSON(data []byte) error {
	type operationConfigApplyJSON OperationConfigApply

	if err := json.Unmarshal(data, (*operationConfigApplyJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal apply operation config data: %w", err)
	}

	return nil
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

func (c *OperationConfigDelete) Kind() string {
	return string(OperationTypeDelete)
}

func (c *OperationConfigDelete) MarshalJSON() ([]byte, error) {
	type operationConfigDeleteJSON OperationConfigDelete

	return json.Marshal((*operationConfigDeleteJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigDelete) UnmarshalJSON(data []byte) error {
	type operationConfigDeleteJSON OperationConfigDelete

	if err := json.Unmarshal(data, (*operationConfigDeleteJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal delete operation config data: %w", err)
	}

	return nil
}

type OperationConfigTrackReadiness struct {
	ResourceMeta *spec.ResourceMeta `json:"resourceMeta"`

	FailMode                                 multitrack.FailMode       `json:"failMode"`
	FailuresAllowed                          int                       `json:"failuresAllowed"`
	IgnoreLogs                               bool                      `json:"ignoreLogs"`
	IgnoreLogsForContainers                  []string                  `json:"ignoreLogsForContainers,omitempty"`
	IgnoreLogsByRegex                        *regexp.Regexp            `json:"-"`
	IgnoreLogsByRegexForContainers           map[string]*regexp.Regexp `json:"-"`
	IgnoreReadinessProbeFailsByContainerName map[string]time.Duration  `json:"ignoreReadinessProbeFailsByContainerName,omitempty"`
	NoActivityTimeout                        time.Duration             `json:"noActivityTimeout"`
	SaveEvents                               bool                      `json:"saveEvents"`
	SaveLogsByRegex                          *regexp.Regexp            `json:"-"`
	SaveLogsByRegexForContainers             map[string]*regexp.Regexp `json:"-"`
	SaveLogsOnlyForContainers                []string                  `json:"saveLogsOnlyForContainers,omitempty"`
	SaveLogsOnlyForNumberOfReplicas          int                       `json:"saveLogsOnlyForNumberOfReplicas"`

	IgnoreLogsByRegexStr              string            `json:"ignoreLogsByRegex,omitempty"`
	IgnoreLogsByRegexForContainersStr map[string]string `json:"ignoreLogsByRegexForContainers,omitempty"`
	SaveLogsByRegexStr                string            `json:"saveLogsByRegex,omitempty"`
	SaveLogsByRegexForContainersStr   map[string]string `json:"saveLogsByRegexForContainers,omitempty"`
}

func (c *OperationConfigTrackReadiness) ID() string {
	return c.ResourceMeta.ID()
}

func (c *OperationConfigTrackReadiness) IDHuman() string {
	return c.ResourceMeta.IDHuman()
}

func (c *OperationConfigTrackReadiness) Kind() string {
	return string(OperationTypeTrackReadiness)
}

func (c *OperationConfigTrackReadiness) MarshalJSON() ([]byte, error) {
	type operationConfigTrackReadinessJSON OperationConfigTrackReadiness

	c.prepareForMarshal()

	return json.Marshal((*operationConfigTrackReadinessJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigTrackReadiness) UnmarshalJSON(data []byte) error {
	type operationConfigTrackReadinessJSON OperationConfigTrackReadiness

	if err := json.Unmarshal(data, (*operationConfigTrackReadinessJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal track readiness operation config data: %w", err)
	}

	if err := c.restoreFromUnmarshal(); err != nil {
		return fmt.Errorf("restore track readiness regex fields: %w", err)
	}

	return nil
}

func (c *OperationConfigTrackReadiness) prepareForMarshal() {
	if c.IgnoreLogsByRegex != nil {
		c.IgnoreLogsByRegexStr = c.IgnoreLogsByRegex.String()
	}

	if c.SaveLogsByRegex != nil {
		c.SaveLogsByRegexStr = c.SaveLogsByRegex.String()
	}

	if c.IgnoreLogsByRegexForContainers != nil {
		c.IgnoreLogsByRegexForContainersStr = make(map[string]string, len(c.IgnoreLogsByRegexForContainers))
		for k, v := range c.IgnoreLogsByRegexForContainers {
			if v != nil {
				c.IgnoreLogsByRegexForContainersStr[k] = v.String()
			}
		}
	}

	if c.SaveLogsByRegexForContainers != nil {
		c.SaveLogsByRegexForContainersStr = make(map[string]string, len(c.SaveLogsByRegexForContainers))
		for k, v := range c.SaveLogsByRegexForContainers {
			if v != nil {
				c.SaveLogsByRegexForContainersStr[k] = v.String()
			}
		}
	}
}

func (c *OperationConfigTrackReadiness) restoreFromUnmarshal() error {
	if c.IgnoreLogsByRegexStr != "" {
		r, err := regexp.Compile(c.IgnoreLogsByRegexStr)
		if err != nil {
			return fmt.Errorf("compile ignoreLogsByRegex: %w", err)
		}

		c.IgnoreLogsByRegex = r
	}

	if c.SaveLogsByRegexStr != "" {
		r, err := regexp.Compile(c.SaveLogsByRegexStr)
		if err != nil {
			return fmt.Errorf("compile saveLogsByRegex: %w", err)
		}

		c.SaveLogsByRegex = r
	}

	if c.IgnoreLogsByRegexForContainersStr != nil {
		c.IgnoreLogsByRegexForContainers = make(map[string]*regexp.Regexp, len(c.IgnoreLogsByRegexForContainersStr))
		for k, v := range c.IgnoreLogsByRegexForContainersStr {
			if v == "" {
				continue
			}

			r, err := regexp.Compile(v)
			if err != nil {
				return fmt.Errorf("compile ignoreLogsByRegexForContainers[%q]: %w", k, err)
			}

			c.IgnoreLogsByRegexForContainers[k] = r
		}
	}

	if c.SaveLogsByRegexForContainersStr != nil {
		c.SaveLogsByRegexForContainers = make(map[string]*regexp.Regexp, len(c.SaveLogsByRegexForContainersStr))
		for k, v := range c.SaveLogsByRegexForContainersStr {
			if v == "" {
				continue
			}

			r, err := regexp.Compile(v)
			if err != nil {
				return fmt.Errorf("compile saveLogsByRegexForContainers[%q]: %w", k, err)
			}

			c.SaveLogsByRegexForContainers[k] = r
		}
	}

	return nil
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

func (c *OperationConfigTrackPresence) Kind() string {
	return string(OperationTypeTrackPresence)
}

func (c *OperationConfigTrackPresence) MarshalJSON() ([]byte, error) {
	type operationConfigTrackPresenceJSON OperationConfigTrackPresence

	return json.Marshal((*operationConfigTrackPresenceJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigTrackPresence) UnmarshalJSON(data []byte) error {
	type operationConfigTrackPresenceJSON OperationConfigTrackPresence

	if err := json.Unmarshal(data, (*operationConfigTrackPresenceJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal track presence operation config data: %w", err)
	}

	return nil
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

func (c *OperationConfigTrackAbsence) Kind() string {
	return string(OperationTypeTrackAbsence)
}

func (c *OperationConfigTrackAbsence) MarshalJSON() ([]byte, error) {
	type operationConfigTrackAbsenceJSON OperationConfigTrackAbsence

	return json.Marshal((*operationConfigTrackAbsenceJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigTrackAbsence) UnmarshalJSON(data []byte) error {
	type operationConfigTrackAbsenceJSON OperationConfigTrackAbsence

	if err := json.Unmarshal(data, (*operationConfigTrackAbsenceJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal track absence operation config data: %w", err)
	}

	return nil
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

func (c *OperationConfigCreateRelease) Kind() string {
	return string(OperationTypeCreateRelease)
}

func (c *OperationConfigCreateRelease) MarshalJSON() ([]byte, error) {
	type operationConfigCreateReleaseJSON OperationConfigCreateRelease

	return json.Marshal((*operationConfigCreateReleaseJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigCreateRelease) UnmarshalJSON(data []byte) error {
	type operationConfigCreateReleaseJSON OperationConfigCreateRelease

	if err := json.Unmarshal(data, (*operationConfigCreateReleaseJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal create release operation config data: %w", err)
	}

	return nil
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

func (c *OperationConfigUpdateRelease) Kind() string {
	return string(OperationTypeUpdateRelease)
}

func (c *OperationConfigUpdateRelease) MarshalJSON() ([]byte, error) {
	type operationConfigUpdateReleaseJSON OperationConfigUpdateRelease

	return json.Marshal((*operationConfigUpdateReleaseJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigUpdateRelease) UnmarshalJSON(data []byte) error {
	type operationConfigUpdateReleaseJSON OperationConfigUpdateRelease

	if err := json.Unmarshal(data, (*operationConfigUpdateReleaseJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal update release operation config data: %w", err)
	}

	return nil
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

func (c *OperationConfigDeleteRelease) Kind() string {
	return string(OperationTypeDeleteRelease)
}

func (c *OperationConfigDeleteRelease) MarshalJSON() ([]byte, error) {
	type operationConfigDeleteReleaseJSON OperationConfigDeleteRelease

	return json.Marshal((*operationConfigDeleteReleaseJSON)(c)) //nolint:wrapcheck
}

func (c *OperationConfigDeleteRelease) UnmarshalJSON(data []byte) error {
	type operationConfigDeleteReleaseJSON OperationConfigDeleteRelease

	if err := json.Unmarshal(data, (*operationConfigDeleteReleaseJSON)(c)); err != nil {
		return fmt.Errorf("unmarshal delete release operation config data: %w", err)
	}

	return nil
}
