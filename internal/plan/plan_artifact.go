package plan

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/samber/lo"

	"github.com/werf/3p-helm/pkg/release"
	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/common"
	"github.com/werf/nelm/pkg/log"
)

const PlanArtifactSchemeVersion = "v1"

type PlanArtifact struct {
	APIVersion string              `json:"apiVersion"`
	Data       *PlanArtifactData   `json:"-"`
	DeployType common.DeployType   `json:"deployType"`
	Encrypted  bool                `json:"encrypted"`
	Release    PlanArtifactRelease `json:"release"`
	Timestamp  time.Time           `json:"timestamp"`

	dataRaw string
}

type PlanArtifactData struct {
	Options                  common.ReleaseInstallRuntimeOptions `json:"options"`
	Changes                  []*ResourceChange                   `json:"changes"`
	Plan                     *Plan                               `json:"plan"`
	Release                  *release.Release                    `json:"release"`
	InstallableResourceInfos []*InstallableResourceInfo          `json:"installableResourceInfos"`
	ReleaseInfos             []*ReleaseInfo                      `json:"releaseInfos"`
}

type PlanArtifactRelease struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
}

func (a *PlanArtifact) MarshalJSON() ([]byte, error) {
	type Alias PlanArtifact

	data, err := json.Marshal(&struct {
		*Alias

		Data string `json:"data,omitempty"`
	}{
		Alias: (*Alias)(a),
		Data:  a.dataRaw,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal plan artifact: %w", err)
	}

	return data, nil
}

func (a *PlanArtifact) UnmarshalJSON(raw []byte) error {
	type Alias PlanArtifact

	aux := &struct {
		*Alias

		Data string `json:"data"`
	}{
		Alias: (*Alias)(a),
	}

	if err := json.Unmarshal(raw, aux); err != nil {
		return fmt.Errorf("unmarshal plan artifact: %w", err)
	}

	a.dataRaw = aux.Data

	return nil
}

func WritePlanArtifact(ctx context.Context, artifact *PlanArtifact, path, secretKey, secretWorkDir string) error {
	dataJSON, err := json.Marshal(artifact.Data)
	if err != nil {
		return fmt.Errorf("marshal artifact data to json: %w", err)
	}

	if secretKey != "" {
		lo.Must0(os.Setenv("WERF_SECRET_KEY", secretKey))

		encoder, err := secrets_manager.Manager.GetYamlEncoder(ctx, secretWorkDir, false)
		if err != nil {
			return fmt.Errorf("get yaml encoder: %w", err)
		}

		encryptedData, err := encoder.Encrypt(dataJSON)
		if err != nil {
			return fmt.Errorf("encrypt artifact data: %w", err)
		}

		artifact.dataRaw = string(encryptedData)
		artifact.Encrypted = true
	} else {
		artifact.dataRaw = string(dataJSON)
		artifact.Encrypted = false
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("create plan artifact file %q: %w", path, err)
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)

	enc := json.NewEncoder(gzipWriter)
	enc.SetIndent("", "  ")

	if err := enc.Encode(artifact); err != nil {
		if err := gzipWriter.Close(); err != nil {
			log.Default.Error(ctx, "Cannot close plan artifact gzip writer: %w", err)
		}

		return fmt.Errorf("marshal plan artifact to json: %w", err)
	}

	if err := gzipWriter.Close(); err != nil {
		return fmt.Errorf("cannot close plan artifact gzip writer: %w", err)
	}

	return nil
}

func ReadPlanArtifact(ctx context.Context, path, secretKey, secretWorkDir string) (*PlanArtifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open plan artifact file: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	var artifact PlanArtifact

	if err := json.NewDecoder(gzipReader).Decode(&artifact); err != nil {
		return nil, fmt.Errorf("decode plan artifact json: %w", err)
	}

	if artifact.dataRaw == "" {
		return nil, fmt.Errorf("artifact data is empty")
	}

	var dataJSON []byte

	if artifact.Encrypted {
		if secretKey == "" {
			return nil, fmt.Errorf("artifact is encrypted but no secret key provided")
		}

		lo.Must0(os.Setenv("WERF_SECRET_KEY", secretKey))

		encoder, err := secrets_manager.Manager.GetYamlEncoder(ctx, secretWorkDir, false)
		if err != nil {
			return nil, fmt.Errorf("get yaml encoder: %w", err)
		}

		dataJSON, err = encoder.Decrypt([]byte(artifact.dataRaw))
		if err != nil {
			return nil, fmt.Errorf("decrypt artifact data: %w", err)
		}
	} else {
		dataJSON = []byte(artifact.dataRaw)
	}

	var data PlanArtifactData

	if err := json.Unmarshal(dataJSON, &data); err != nil {
		return nil, fmt.Errorf("decode artifact data json: %w", err)
	}

	artifact.Data = &data

	return &artifact, nil
}

func ValidatePlanArtifact(artifact *PlanArtifact, expectedRevision int, lifetime time.Duration) error {
	if artifact == nil {
		return errors.New("plan shouldn't be empty")
	}

	if artifact.Timestamp.Add(lifetime).Before(time.Now().UTC()) {
		return fmt.Errorf("plan artifact expired: was valid for %s until %s",
			lifetime, artifact.Timestamp.Add(lifetime).Format(time.RFC3339))
	}

	if artifact.Release.Namespace == "" {
		return errors.New("release namespace is not set")
	}

	if artifact.Release.Name == "" {
		return errors.New("release name is not set")
	}

	if artifact.Release.Version != expectedRevision {
		return fmt.Errorf("plan artifact release version mismatch: expected %d, got %d",
			artifact.Release.Version, expectedRevision)
	}

	if artifact.Data.Plan == nil {
		return errors.New("plan is not set")
	}

	if len(artifact.Data.InstallableResourceInfos) == 0 {
		return errors.New("no installable resource information objects found")
	}

	if len(artifact.Data.ReleaseInfos) == 0 {
		return errors.New("no release information objects found")
	}

	return nil
}
