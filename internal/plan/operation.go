package plan

import (
	"encoding/json"
	"fmt"
	"strings"
)

type OperationCategory string

const (
	// Does nothing. Used for things like  grouping.
	OperationCategoryMeta OperationCategory = "meta"
	// Operations that mutate Kubernetes resources in the cluster.
	OperationCategoryResource OperationCategory = "resource"
	// Operations that track resources in the cluster. Never mutate anything.
	OperationCategoryTrack OperationCategory = "track"
	// Operations that mutate Helm releases in the cluster.
	OperationCategoryRelease OperationCategory = "release"
)

type OperationType string

const (
	OperationTypeApply          OperationType = "apply"
	OperationTypeCreate         OperationType = "create"
	OperationTypeCreateRelease  OperationType = "create-release"
	OperationTypeDelete         OperationType = "delete"
	OperationTypeDeleteRelease  OperationType = "delete-release"
	OperationTypeNoop           OperationType = "noop"
	OperationTypeRecreate       OperationType = "recreate"
	OperationTypeTrackAbsence   OperationType = "track-absence"
	OperationTypeTrackPresence  OperationType = "track-presence"
	OperationTypeTrackReadiness OperationType = "track-readiness"
	OperationTypeUpdate         OperationType = "update"
	OperationTypeUpdateRelease  OperationType = "update-release"
)

// Used to handle breaking changes in the Operation struct.
type OperationVersion int

const (
	OperationVersionApply          OperationVersion = 1
	OperationVersionCreate         OperationVersion = 1
	OperationVersionCreateRelease  OperationVersion = 1
	OperationVersionDelete         OperationVersion = 1
	OperationVersionDeleteRelease  OperationVersion = 1
	OperationVersionNoop           OperationVersion = 1
	OperationVersionRecreate       OperationVersion = 1
	OperationVersionTrackAbsence   OperationVersion = 1
	OperationVersionTrackPresence  OperationVersion = 1
	OperationVersionTrackReadiness OperationVersion = 1
	OperationVersionUpdate         OperationVersion = 1
	OperationVersionUpdateRelease  OperationVersion = 1
)

type OperationStatus string

const (
	OperationStatusUnknown   OperationStatus = ""
	OperationStatusPending   OperationStatus = "pending"
	OperationStatusCompleted OperationStatus = "completed"
	OperationStatusFailed    OperationStatus = "failed"
)

// Helps to avoid operation ID collisions. Since you can't have two operations with the same ID in
// the graph, you can increment the iteration to get a new unique ID for the operation. The higher
// the iteration, the later in the plan/graph the operation should appear.
type OperationIteration int

// Represents an operation on a resource, such as create, update, track readiness, etc. The
// operation ID must be unique: you can't have two operations with the same ID in the plan/graph.
// Operation must be easily serializable.
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
	return fmt.Sprintf("%s/%d/%d/%s", t, version, iteration, configID)
}

func OperationIDHuman(t OperationType, iteration OperationIteration, configIDHuman string) string {
	id := fmt.Sprintf("%s: ", strings.ReplaceAll(string(t), "-", " "))

	id += configIDHuman
	if iteration > 0 {
		id += fmt.Sprintf(" (iteration=%d)", iteration)
	}

	return id
}

type operationJSON struct {
	Type      OperationType          `json:"type"`
	Version   OperationVersion       `json:"version"`
	Category  OperationCategory      `json:"category"`
	Iteration OperationIteration     `json:"iteration"`
	Status    OperationStatus        `json:"status"`
	Config    operationConfigPayload `json:"config"`
}

type operationConfigPayload struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

func (o *Operation) MarshalJSON() ([]byte, error) {
	if o.Config == nil {
		return nil, fmt.Errorf("operation config is nil")
	}

	configData, err := o.Config.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal operation config data: %w", err)
	}

	return json.Marshal(operationJSON{ //nolint:wrapcheck
		Type:      o.Type,
		Version:   o.Version,
		Category:  o.Category,
		Iteration: o.Iteration,
		Status:    o.Status,
		Config: operationConfigPayload{
			Kind: o.Config.Kind(),
			Data: configData,
		},
	})
}

func (o *Operation) UnmarshalJSON(data []byte) error {
	var raw operationJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal operation: %w", err)
	}

	cfg, err := operationConfigByKind(raw.Config.Kind)
	if err != nil {
		return fmt.Errorf("resolve operation config by kind %q: %w", raw.Config.Kind, err)
	}

	if err := cfg.UnmarshalJSON(raw.Config.Data); err != nil {
		return fmt.Errorf("unmarshal operation config data: %w", err)
	}

	o.Type = raw.Type
	o.Version = raw.Version
	o.Category = raw.Category
	o.Iteration = raw.Iteration
	o.Status = raw.Status
	o.Config = cfg

	return nil
}

var operationConfigConstructors = map[string]func() OperationConfig{
	string(OperationTypeNoop): func() OperationConfig {
		return &OperationConfigNoop{}
	},
	string(OperationTypeCreate): func() OperationConfig {
		return &OperationConfigCreate{}
	},
	string(OperationTypeRecreate): func() OperationConfig {
		return &OperationConfigRecreate{}
	},
	string(OperationTypeUpdate): func() OperationConfig {
		return &OperationConfigUpdate{}
	},
	string(OperationTypeApply): func() OperationConfig {
		return &OperationConfigApply{}
	},
	string(OperationTypeDelete): func() OperationConfig {
		return &OperationConfigDelete{}
	},
	string(OperationTypeTrackReadiness): func() OperationConfig {
		return &OperationConfigTrackReadiness{}
	},
	string(OperationTypeTrackPresence): func() OperationConfig {
		return &OperationConfigTrackPresence{}
	},
	string(OperationTypeTrackAbsence): func() OperationConfig {
		return &OperationConfigTrackAbsence{}
	},
	string(OperationTypeCreateRelease): func() OperationConfig {
		return &OperationConfigCreateRelease{}
	},
	string(OperationTypeUpdateRelease): func() OperationConfig {
		return &OperationConfigUpdateRelease{}
	},
	string(OperationTypeDeleteRelease): func() OperationConfig {
		return &OperationConfigDeleteRelease{}
	},
}

func operationConfigByKind(kind string) (OperationConfig, error) {
	constructor, found := operationConfigConstructors[kind]
	if !found {
		return nil, fmt.Errorf("unsupported operation config kind %q", kind)
	}

	return constructor(), nil
}
