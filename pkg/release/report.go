package release

import (
	"encoding/json"
	"fmt"

	"github.com/werf/3p-helm/pkg/time"
)

func NewDeployReport() *DeployReport {
	return &DeployReport{}
}

type DeployReport struct {
	Release           string    `json:"release,omitempty"`
	Namespace         string    `json:"namespace,omitempty"`
	Revision          int       `json:"revision,omitempty"`
	Status            Status    `json:"status,omitempty"`
	LastPhase         *Phase    `json:"last_phase,omitempty"`
	LastStage         *int      `json:"last_stage,omitempty"`
	FirstDeployedTime time.Time `json:"first_deployed,omitempty"`
	LastDeployedTime  time.Time `json:"last_deployed,omitempty"`
}

func (r *DeployReport) FromRelease(release *Release) *DeployReport {
	r.Release = release.Name
	r.Namespace = release.Namespace
	r.Revision = release.Version
	r.Status = release.Info.Status
	r.LastPhase = release.Info.LastPhase
	r.LastStage = release.Info.LastStage
	r.FirstDeployedTime = release.Info.FirstDeployed
	r.LastDeployedTime = release.Info.LastDeployed

	return r
}

func (r *DeployReport) ToJSONData() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "\t")
	if err != nil {
		return nil, fmt.Errorf("error marshalling deploy report: %w", err)
	}

	return data, nil
}
