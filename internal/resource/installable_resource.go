package resource

import (
	"fmt"
	"regexp"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/werf/kubedog/pkg/trackers/rollout/multitrack"
	"github.com/werf/nelm/internal/common"
	"github.com/werf/nelm/internal/plan/dependency"
	"github.com/werf/nelm/internal/resource/id"
)

type InstallableResourceOptions struct {
	Mapper meta.ResettableRESTMapper
}

func NewInstallableResource(res *id.ResourceSpec, deployType common.DeployType, releaseNamespace string, opts InstallableResourceOptions) ([]*InstallableResource, error) {
	if err := validateHook(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate hook configuration: %w", err)
	}

	if err := validateReplicasOnCreation(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate replicas on creation: %w", err)
	}

	if err := validateDeletePolicy(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate delete policy: %w", err)
	}

	if err := ValidateResourcePolicy(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate resource policy: %w", err)
	}

	if err := validateTrack(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate track annotations: %w", err)
	}

	if err := validateWeight(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate weight: %w", err)
	}

	if err := validateDeployDependencies(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate deploy dependencies: %w", err)
	}

	if err := validateInternalDependencies(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate internal dependencies: %w", err)
	}

	if err := validateExternalDependencies(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate external dependencies: %w", err)
	}

	if err := validateSensitive(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate sensitive: %w", err)
	}

	if err := validateDeployOn(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate deploy stage: %w", err)
	}

	if err := validateOwnership(res.ResourceMeta); err != nil {
		return nil, fmt.Errorf("validate ownership: %w", err)
	}

	extDeps, err := externalDependencies(res.ResourceMeta, releaseNamespace, opts.Mapper)
	if err != nil {
		return nil, fmt.Errorf("get external dependencies: %w", err)
	}

	deplConditions := deployConditions(res.ResourceMeta)
	manIntDeps := manualInternalDependencies(res.ResourceMeta)

	var stages []common.Stage
	switch deployType {
	case common.DeployTypeInitial, common.DeployTypeInstall:
		stages = deplConditions[common.InstallOnInstall]
	case common.DeployTypeUpgrade:
		stages = deplConditions[common.InstallOnUpgrade]
	case common.DeployTypeRollback:
		stages = deplConditions[common.InstallOnRollback]
	case common.DeployTypeUninstall:
		stages = deplConditions[common.InstallOnDelete]
	}

	var instResources []*InstallableResource
	for _, stage := range stages {
		instResources = append(instResources, &InstallableResource{
			ResourceSpec:                           res,
			Recreate:                               recreate(res.ResourceMeta),
			DefaultReplicasOnCreation:              defaultReplicasOnCreation(res.ResourceMeta, releaseNamespace),
			Ownership:                              ownership(res.ResourceMeta, releaseNamespace),
			DeleteOnSucceeded:                      deleteOnSucceeded(res.ResourceMeta),
			DeleteOnFailed:                         deleteOnFailed(res.ResourceMeta),
			KeepOnDelete:                           KeepOnDelete(res.ResourceMeta, releaseNamespace),
			FailMode:                               failMode(res.ResourceMeta),
			FailuresAllowed:                        failuresAllowed(res.Unstruct),
			IgnoreReadinessProbeFailsForContainers: ignoreReadinessProbeFailsForContainers(res.ResourceMeta),
			LogRegex:                               logRegex(res.ResourceMeta),
			LogRegexesForContainers:                logRegexesForContainers(res.ResourceMeta),
			NoActivityTimeout:                      noActivityTimeout(res.ResourceMeta),
			ShowLogsOnlyForContainers:              showLogsOnlyForContainers(res.ResourceMeta),
			ShowServiceMessages:                    showServiceMessages(res.ResourceMeta),
			ShowLogsOnlyForNumberOfReplicas:        showLogsOnlyForNumberOfReplicas(res.ResourceMeta),
			SkipLogs:                               skipLogs(res.ResourceMeta),
			SkipLogsForContainers:                  skipLogsForContainers(res.ResourceMeta),
			TrackTerminationMode:                   trackTerminationMode(res.ResourceMeta),
			Weight:                                 weight(res.ResourceMeta, len(manIntDeps) > 0),
			ManualInternalDependencies:             manIntDeps,
			AutoInternalDependencies:               dependency.DetectInternalDependencies(res.Unstruct),
			ExternalDependencies:                   extDeps,
			DeployConditions:                       deplConditions,
			Stage:                                  stage,
		})
	}

	return instResources, nil
}

type InstallableResource struct {
	*id.ResourceSpec

	Ownership                              common.Ownership
	Recreate                               bool
	DefaultReplicasOnCreation              *int
	DeleteOnSucceeded                      bool
	DeleteOnFailed                         bool
	KeepOnDelete                           bool
	FailMode                               multitrack.FailMode
	FailuresAllowed                        int
	IgnoreReadinessProbeFailsForContainers map[string]time.Duration
	LogRegex                               *regexp.Regexp
	LogRegexesForContainers                map[string]*regexp.Regexp
	NoActivityTimeout                      time.Duration
	ShowLogsOnlyForContainers              []string
	ShowServiceMessages                    bool
	ShowLogsOnlyForNumberOfReplicas        int
	SkipLogs                               bool
	SkipLogsForContainers                  []string
	TrackTerminationMode                   multitrack.TrackTerminationMode
	Weight                                 *int
	ManualInternalDependencies             []*dependency.InternalDependency
	AutoInternalDependencies               []*dependency.InternalDependency
	ExternalDependencies                   []*dependency.ExternalDependency
	DeployConditions                       map[common.On][]common.Stage
	Stage                                  common.Stage
}
