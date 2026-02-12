package plan

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/samber/lo"

	"github.com/werf/3p-helm/pkg/release"
	"github.com/werf/common-go/pkg/secrets_manager"
	"github.com/werf/nelm/pkg/common"
)

const PlanArtifactSchemeVersion = "v1"

var ErrPlanArtifactInvalid = errors.New("plan artifact invalid")

type PlanArtifact struct {
	APIVersion string              `json:"apiVersion"`
	Data       *PlanArtifactData   `json:"-"`
	DataRaw    string              `json:"dataRaw,omitempty"`
	DeployType common.DeployType   `json:"deployType"`
	Encrypted  bool                `json:"encrypted"`
	Options    PlanArtifactOptions `json:"options"`
	Release    PlanArtifactRelease `json:"release"`
	Timestamp  time.Time           `json:"timestamp"`
}

func (a *PlanArtifact) GetChanges() []*ResourceChange {
	return a.Data.Changes
}

func (a *PlanArtifact) GetDeployType() common.DeployType {
	return a.DeployType
}

func (a *PlanArtifact) GetInstallableResourceInfos() []*InstallableResourceInfo {
	return a.Data.InstallableResourceInfos
}

func (a *PlanArtifact) GetPlan() *Plan {
	return a.Data.DAG
}

func (a *PlanArtifact) GetRelease() (*release.Release, error) {
	for _, op := range a.Data.DAG.Operations() {
		if c, ok := op.Config.(*OperationConfigCreateRelease); ok {
			return c.Release, nil
		}
	}

	return nil, fmt.Errorf("no create release operation found in plan artifact")
}

func (a *PlanArtifact) GetReleaseInfos() []*ReleaseInfo {
	return a.Data.ReleaseInfos
}

type PlanArtifactOptions struct {
	common.ResourceValidationOptions

	// DefaultDeletePropagation sets the deletion propagation policy for resource deletions.
	DefaultDeletePropagation string
	// ExtraAnnotations are additional Kubernetes annotations to add to all chart resources during rollback.
	ExtraAnnotations map[string]string
	// ExtraLabels are additional Kubernetes labels to add to all chart resources during rollback.
	ExtraLabels map[string]string
	// ExtraRuntimeAnnotations are additional annotations to add to resources at runtime during rollback.
	ExtraRuntimeAnnotations map[string]string
	// ExtraRuntimeLabels are additional labels to add to resources at runtime during rollback.
	ExtraRuntimeLabels map[string]string
	// ForceAdoption, when true, allows adopting resources that belong to a different Helm release during rollback.
	ForceAdoption bool
	// NoInstallStandaloneCRDs, when true, skips installation of CustomResourceDefinitions from the "crds/" directory.
	// By default, CRDs are installed first before other chart resources.
	NoInstallStandaloneCRDs bool
	// NoRemoveManualChanges, when true, preserves fields manually added to resources in the cluster
	// that are not present in the chart manifests. By default, such fields are removed during updates.
	NoRemoveManualChanges bool
	// ReleaseInfoAnnotations are annotations to add to the release metadata.
	ReleaseInfoAnnotations map[string]string
	// ReleaseLabels are labels to add to the release metadata.
	ReleaseLabels map[string]string
	// ReleaseStorageDriver specifies where to store release metadata ("secrets" or "sql").
	ReleaseStorageDriver string
	// ReleaseStorageSQLConnection is the SQL connection string when using SQL storage driver.
	ReleaseStorageSQLConnection string
}

type PlanArtifactData struct {
	Changes                  []*ResourceChange          `json:"changes"`
	DAG                      *Plan                      `json:"dag"`
	InstallableResourceInfos []*InstallableResourceInfo `json:"installableResourceInfos"`
	ReleaseInfos             []*ReleaseInfo             `json:"releaseInfos"`
}

type PlanArtifactRelease struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
}

func WritePlanArtifact(ctx context.Context, p *Plan, deployType common.DeployType, changes []*ResourceChange, releaseName, releaseNamespace string, releaseVersion int, path, secretKey, secretWorkDir string, instResInfos []*InstallableResourceInfo, relInfos []*ReleaseInfo, opts PlanArtifactOptions) error {
	artifact := BuildPlanArtifact(
		p,
		deployType,
		changes,
		PlanArtifactRelease{
			Name:      releaseName,
			Namespace: releaseNamespace,
			Version:   releaseVersion,
		},
		instResInfos,
		relInfos,
		opts,
	)

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("plan artifact path %q already exists", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat plan artifact path %q: %w", path, err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create plan artifact file %q: %w", path, err)
	}
	defer file.Close()

	if err := createPlanArtifactArchive(ctx, file, artifact, secretKey, secretWorkDir); err != nil {
		return fmt.Errorf("marshal plan artifact: %w", err)
	}

	return nil
}

func BuildPlanArtifact(p *Plan, deployType common.DeployType, changes []*ResourceChange, release PlanArtifactRelease, instResInfos []*InstallableResourceInfo, relInfos []*ReleaseInfo, opts PlanArtifactOptions) *PlanArtifact {
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].ResourceMeta.IDHuman() < changes[j].ResourceMeta.IDHuman()
	})

	return &PlanArtifact{
		APIVersion: PlanArtifactSchemeVersion,
		Data: &PlanArtifactData{
			DAG:                      p,
			Changes:                  changes,
			InstallableResourceInfos: instResInfos,
			ReleaseInfos:             relInfos,
		},
		DeployType: deployType,
		Encrypted:  false,
		Release:    release,
		Options:    opts,
		Timestamp:  time.Now().UTC(),
	}
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

	if err := json.NewDecoder(gzipReader).Decode(&artifact); err != nil { //nolint:musttag
		return nil, fmt.Errorf("decode plan artifact json: %w", err)
	}

	if artifact.DataRaw == "" {
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

		dataJSON, err = encoder.Decrypt([]byte(artifact.DataRaw))
		if err != nil {
			return nil, fmt.Errorf("decrypt artifact data: %w", err)
		}
	} else {
		dataJSON = []byte(artifact.DataRaw)
	}

	var data PlanArtifactData

	if err := json.Unmarshal(dataJSON, &data); err != nil { //nolint: musttag
		return nil, fmt.Errorf("decode artifact data json: %w", err)
	}

	artifact.Data = &data

	return &artifact, nil
}

func ValidatePlanArtifact(artifact *PlanArtifact, expectedRevision int, lifetime time.Duration) error {
	if artifact == nil {
		return fmt.Errorf("%w: plan is empty", ErrPlanArtifactInvalid)
	}

	if artifact.Timestamp.Add(lifetime).Before(time.Now().UTC()) {
		return fmt.Errorf("%w: plan artifact is too old", ErrPlanArtifactInvalid)
	}

	if artifact.Release.Namespace == "" {
		return fmt.Errorf("%w: release namespace is not set",
			ErrPlanArtifactInvalid)
	}

	if artifact.Release.Name == "" {
		return fmt.Errorf("%w: release name is not set",
			ErrPlanArtifactInvalid)
	}

	if artifact.Release.Version != expectedRevision {
		return fmt.Errorf("%w: plan artifact release version mismatch: expected %d, got %d",
			ErrPlanArtifactInvalid, artifact.Release.Version, expectedRevision)
	}

	if artifact.GetPlan() == nil {
		return fmt.Errorf("%w: plan is not set", ErrPlanArtifactInvalid)
	}

	if len(artifact.GetInstallableResourceInfos()) == 0 {
		return fmt.Errorf("%w: no installable resource information objects found", ErrPlanArtifactInvalid)
	}

	if len(artifact.GetReleaseInfos()) == 0 {
		return fmt.Errorf("%w: no release information objects found", ErrPlanArtifactInvalid)
	}

	return nil
}

func createPlanArtifactArchive(ctx context.Context, w io.Writer, artifact *PlanArtifact, secretKey, secretWorkDir string) error {
	dataJSON, err := json.Marshal(artifact.Data) //nolint:musttag
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

		artifact.DataRaw = string(encryptedData)
		artifact.Encrypted = true
	} else {
		artifact.DataRaw = string(dataJSON)
		artifact.Encrypted = false
	}

	gzipWriter := gzip.NewWriter(w)
	defer gzipWriter.Close()

	enc := json.NewEncoder(gzipWriter)
	enc.SetIndent("", "  ")

	if err := enc.Encode(artifact); err != nil { //nolint:musttag
		return fmt.Errorf("marshal plan artifact to json: %w", err)
	}

	return nil
}
