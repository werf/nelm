package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/dominikbraun/graph"
	"github.com/pkg/errors"
	"github.com/werf/3p-helm/pkg/release"
	"github.com/werf/nelm/pkg/common"

	"github.com/werf/nelm/internal/resource/spec"
)

const InstallPlanArtifactSchemeVersion = "v1"

var ErrPlanArtifactInvalid = errors.New("plan artifact invalid")

type InstallPlanArtifact struct {
	APIVersion string                     `json:"apiVersion"`
	Timestamp  time.Time                  `json:"timestamp"`
	Release    InstallPlanArtifactRelease `json:"release"`
	DAG        InstallPlanArtifactDAG     `json:"dag"`
	Changes    []*ResourceChange          `json:"changes"`
}

func (a *InstallPlanArtifact) GetInstallPlan() (*Plan, error) {
	p := NewPlan()

	for _, opArtifact := range a.DAG.Operations {
		cfg, err := decodeOperationConfig(opArtifact.Config)
		if err != nil {
			return nil, fmt.Errorf("decode operation config for %q: %w", opArtifact.ID, err)
		}

		op := &Operation{
			Type:      opArtifact.Type,
			Version:   opArtifact.Version,
			Category:  opArtifact.Category,
			Iteration: opArtifact.Iteration,
			Status:    opArtifact.Status,
			Config:    cfg,
		}

		if op.ID() != opArtifact.ID {
			return nil, fmt.Errorf("operation id mismatch: expected %q, got %q", opArtifact.ID, op.ID())
		}

		if err := p.Graph.AddVertex(op); err != nil {
			if !errors.Is(err, graph.ErrVertexAlreadyExists) {
				return nil, fmt.Errorf("add vertex %q: %w", opArtifact.ID, err)
			}
		}
	}

	for _, edge := range a.DAG.Edges {
		if err := p.Connect(edge.From, edge.To); err != nil {
			return nil, fmt.Errorf("connect edge from %q to %q: %w", edge.From, edge.To, err)
		}
	}

	return p, nil
}

func (a *InstallPlanArtifact) GetChanges() []*ResourceChange {
	return a.Changes
}

type InstallPlanArtifactDAG struct {
	Operations []InstallPlanArtifactOp   `json:"operations"`
	Edges      []InstallPlanArtifactEdge `json:"edges"`
}

type InstallPlanArtifactEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type InstallPlanArtifactRelease struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
}

type InstallPlanArtifactOp struct {
	ID        string                      `json:"id"`
	Type      OperationType               `json:"type"`
	Version   OperationVersion            `json:"version"`
	Category  OperationCategory           `json:"category"`
	Iteration OperationIteration          `json:"iteration"`
	Status    OperationStatus             `json:"status"`
	Config    InstallPlanArtifactOpConfig `json:"config"`
}

type InstallPlanArtifactOpConfig struct {
	Kind string `json:"kind"`

	Noop           *OperationConfigNoop           `json:"noop,omitempty"`
	Create         *OperationConfigCreate         `json:"create,omitempty"`
	Recreate       *OperationConfigRecreate       `json:"recreate,omitempty"`
	Update         *OperationConfigUpdate         `json:"update,omitempty"`
	Apply          *OperationConfigApply          `json:"apply,omitempty"`
	Delete         *OperationConfigDelete         `json:"delete,omitempty"`
	TrackReadiness *OperationConfigTrackReadiness `json:"trackReadiness,omitempty"`
	TrackPresence  *OperationConfigTrackPresence  `json:"trackPresence,omitempty"`
	TrackAbsence   *OperationConfigTrackAbsence   `json:"trackAbsence,omitempty"`

	CreateRelease *OperationConfigCreateRelease `json:"createRelease,omitempty"`
	UpdateRelease *OperationConfigUpdateRelease `json:"updateRelease,omitempty"`
	DeleteRelease *OperationConfigDeleteRelease `json:"deleteRelease,omitempty"`
}

type InstallPlanArtifactChange struct {
	Type            string             `json:"type"`
	Reason          string             `json:"reason,omitempty"`
	ExtraOperations []string           `json:"extraOperations,omitempty"`
	Resource        *spec.ResourceMeta `json:"resource,omitempty"`
	Udiff           string             `json:"udiff"`
}

func BuildInstallPlanArtifact(p *Plan, changes []*ResourceChange, release InstallPlanArtifactRelease) (*InstallPlanArtifact, error) {
	artifact := &InstallPlanArtifact{
		APIVersion: InstallPlanArtifactSchemeVersion,
		Timestamp:  time.Now().UTC(),
		Release:    release,
	}

	dag, err := buildInstallPlanArtifactDAG(p)
	if err != nil {
		return nil, fmt.Errorf("build dag artifact: %w", err)
	}

	artifact.DAG = dag

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].ResourceMeta.IDHuman() < changes[j].ResourceMeta.IDHuman()
	})

	artifact.Changes = changes

	return artifact, nil
}

func buildInstallPlanArtifactDAG(p *Plan) (InstallPlanArtifactDAG, error) {
	var operations []InstallPlanArtifactOp
	var edges []InstallPlanArtifactEdge

	ops := p.Operations()

	for _, op := range ops {
		item := InstallPlanArtifactOp{
			ID:        op.ID(),
			Type:      op.Type,
			Version:   op.Version,
			Category:  op.Category,
			Iteration: op.Iteration,
			Status:    op.Status,
		}

		cfg, err := encodeOperationConfig(op.Config)
		if err != nil {
			return InstallPlanArtifactDAG{}, fmt.Errorf("encode operation config for %q: %w", op.ID(), err)
		}

		item.Config = cfg

		operations = append(operations, item)
	}

	adjMap, err := p.Graph.AdjacencyMap()
	if err != nil {
		return InstallPlanArtifactDAG{}, fmt.Errorf("get adjacency map: %w", err)
	}

	for fromID, toMap := range adjMap {
		for toID := range toMap {
			edges = append(edges, InstallPlanArtifactEdge{
				From: fromID,
				To:   toID,
			})
		}
	}

	sort.Slice(operations, func(i, j int) bool {
		return operations[i].ID < operations[j].ID
	})

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}

		return edges[i].From < edges[j].From
	})

	return InstallPlanArtifactDAG{
		Operations: operations,
		Edges:      edges,
	}, nil
}

func encodeOperationConfig(cfg OperationConfig) (InstallPlanArtifactOpConfig, error) {
	switch c := cfg.(type) {
	case *OperationConfigNoop:
		return InstallPlanArtifactOpConfig{Kind: "noop", Noop: c}, nil
	case *OperationConfigCreate:
		return InstallPlanArtifactOpConfig{Kind: "create", Create: c}, nil
	case *OperationConfigRecreate:
		return InstallPlanArtifactOpConfig{Kind: "recreate", Recreate: c}, nil
	case *OperationConfigUpdate:
		return InstallPlanArtifactOpConfig{Kind: "update", Update: c}, nil
	case *OperationConfigApply:
		return InstallPlanArtifactOpConfig{Kind: "apply", Apply: c}, nil
	case *OperationConfigDelete:
		return InstallPlanArtifactOpConfig{Kind: "delete", Delete: c}, nil
	case *OperationConfigTrackReadiness:
		c.PrepareForMarshal()
		return InstallPlanArtifactOpConfig{Kind: "track-readiness", TrackReadiness: c}, nil
	case *OperationConfigTrackPresence:
		return InstallPlanArtifactOpConfig{Kind: "track-presence", TrackPresence: c}, nil
	case *OperationConfigTrackAbsence:
		return InstallPlanArtifactOpConfig{Kind: "track-absence", TrackAbsence: c}, nil
	case *OperationConfigCreateRelease:
		return InstallPlanArtifactOpConfig{Kind: "create-release", CreateRelease: c}, nil
	case *OperationConfigUpdateRelease:
		return InstallPlanArtifactOpConfig{Kind: "update-release", UpdateRelease: c}, nil
	case *OperationConfigDeleteRelease:
		return InstallPlanArtifactOpConfig{Kind: "delete-release", DeleteRelease: c}, nil
	default:
		return InstallPlanArtifactOpConfig{}, fmt.Errorf("unsupported operation config type %T", cfg)
	}
}

func MarshalInstallPlanArtifact(artifact *InstallPlanArtifact) ([]byte, error) {
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal install plan artifact to json: %w", err)
	}

	return data, nil
}

func ReadInstallPlanArtifact(path string) (*InstallPlanArtifact, error) {
	installPlanFile, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open plan artifact: %w", err)
	}

	var artifact InstallPlanArtifact

	decoder := json.NewDecoder(installPlanFile)

	if err = decoder.Decode(&artifact); err != nil {
		return nil, fmt.Errorf("decode plan artifact: %w", err)
	}

	return &artifact, nil
}

func decodeOperationConfig(cfg InstallPlanArtifactOpConfig) (OperationConfig, error) {
	switch cfg.Kind {
	case "noop":
		if cfg.Noop == nil {
			return nil, fmt.Errorf("noop config is nil")
		}

		return cfg.Noop, nil
	case "create":
		if cfg.Create == nil {
			return nil, fmt.Errorf("create config is nil")
		}

		return cfg.Create, nil
	case "recreate":
		if cfg.Recreate == nil {
			return nil, fmt.Errorf("recreate config is nil")
		}

		return cfg.Recreate, nil
	case "update":
		if cfg.Update == nil {
			return nil, fmt.Errorf("update config is nil")
		}

		return cfg.Update, nil
	case "apply":
		if cfg.Apply == nil {
			return nil, fmt.Errorf("apply config is nil")
		}

		return cfg.Apply, nil
	case "delete":
		if cfg.Delete == nil {
			return nil, fmt.Errorf("delete config is nil")
		}

		return cfg.Delete, nil
	case "track-readiness":
		if cfg.TrackReadiness == nil {
			return nil, fmt.Errorf("track readiness config is nil")
		}

		if err := cfg.TrackReadiness.RestoreFromUnmarshal(); err != nil {
			return nil, fmt.Errorf("restore track readiness regex fields: %w", err)
		}

		return cfg.TrackReadiness, nil
	case "track-presence":
		if cfg.TrackPresence == nil {
			return nil, fmt.Errorf("track presence config is nil")
		}

		return cfg.TrackPresence, nil
	case "track-absence":
		if cfg.TrackAbsence == nil {
			return nil, fmt.Errorf("track absence config is nil")
		}

		return cfg.TrackAbsence, nil
	case "create-release":
		if cfg.CreateRelease == nil {
			return nil, fmt.Errorf("create release config is nil")
		}

		return cfg.CreateRelease, nil
	case "update-release":
		if cfg.UpdateRelease == nil {
			return nil, fmt.Errorf("update release config is nil")
		}

		return cfg.UpdateRelease, nil
	case "delete-release":
		if cfg.DeleteRelease == nil {
			return nil, fmt.Errorf("delete release config is nil")
		}

		return cfg.DeleteRelease, nil
	default:
		return nil, fmt.Errorf("unsupported config kind %q", cfg.Kind)
	}
}

func ValidateInstallPlanArtifact(artifact *InstallPlanArtifact, releaseName, releaseNamespace string, nextRelease *release.Release, currentPlan *Plan) error {
	if artifact == nil {
		return fmt.Errorf("%w: plan is empty", ErrPlanArtifactInvalid)
	}

	if artifact.Timestamp.Add(common.APIPlanInstallArtifactLifetime).Before(time.Now().UTC()) {
		return fmt.Errorf("%w: plan install artifact is too old", ErrPlanArtifactInvalid)
	}

	if artifact.Release.Namespace != releaseNamespace {
		return fmt.Errorf("%w: plan install artifact release namespace mismatch: expected %s, got %s",
			ErrPlanArtifactInvalid, artifact.Release.Namespace, releaseNamespace)
	}

	if artifact.Release.Name != releaseName {
		return fmt.Errorf("%w: plan install artifact release name mismatch: expected %s, got %s",
			ErrPlanArtifactInvalid, artifact.Release.Name, releaseName)
	}

	// TODO: what to do if release failed?
	if nextRelease != nil && artifact.Release.Version != nextRelease.Version {
		return fmt.Errorf("%w: plan install artifact planned release version mismatch: expected %d, got %d",
			ErrPlanArtifactInvalid, artifact.Release.Version, nextRelease.Version)
	}

	// TODO: need to check if manifests are the same

	// TODO: consider DAG comparison

	_, err := artifact.GetInstallPlan()
	if err != nil {
		return fmt.Errorf("get plan from install artifact: %w", err)
	}

	// TODO: consider changes comparison

	return nil
}
