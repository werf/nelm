package common

import (
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"github.com/Masterminds/sprig/v3"
	"github.com/docker/cli/cli/config"
	"github.com/docker/docker/pkg/homedir"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/werf/nelm/pkg/log"
)

var (
	Brand   = "Nelm"
	Version = "0.0.0"
)

const (
	DefaultBurstLimit              = 100
	DefaultChartProvenanceStrategy = "never" // TODO(v2): switch to if-possible
	DefaultDeletePropagation       = metav1.DeletePropagationForeground
	DefaultDiffContextLines        = 3
	DefaultFieldManager            = "helm"
	DefaultLocalKubeVersion        = "1.20.0"
	DefaultLogColorMode            = log.LogColorModeAuto
	DefaultNetworkParallelism      = 30
	DefaultProgressPrintInterval   = 5 * time.Second
	DefaultQPSLimit                = 30
	DefaultReleaseHistoryLimit     = 10
	KubectlEditFieldManager        = "kubectl-edit"
	OldFieldManagerPrefix          = "werf"
	StageEndSuffix                 = "end"
	StagePrefix                    = "stage"
	StageStartSuffix               = "start"
	StubReleaseName                = "stub-release"
	StubReleaseNamespace           = "stub-namespace"
)

const (
	OutputFormatJSON  = "json"
	OutputFormatTable = "table"
	OutputFormatYAML  = "yaml"
)

const (
	ReleaseStorageDriverConfigMap  = "configmap"
	ReleaseStorageDriverConfigMaps = "configmaps"
	ReleaseStorageDriverDefault    = ""
	ReleaseStorageDriverMemory     = "memory"
	ReleaseStorageDriverSQL        = "sql"
	ReleaseStorageDriverSecret     = "secret"
	ReleaseStorageDriverSecrets    = "secrets"
)

type DeployType string

const (
	// Activated for the first revision of the release.
	DeployTypeInitial DeployType = "Initial"
	// Activated when no successful revision found. But for the very first revision
	// DeployTypeInitial is used instead.
	DeployTypeInstall DeployType = "Install"
	// Activated when a successful revision found.
	DeployTypeUpgrade   DeployType = "Upgrade"
	DeployTypeRollback  DeployType = "Rollback"
	DeployTypeUninstall DeployType = "Uninstall"
)

type DeletePolicy string

const (
	DeletePolicySucceeded                 DeletePolicy = "succeeded"
	DeletePolicyFailed                    DeletePolicy = "failed"
	DeletePolicyBeforeCreation            DeletePolicy = "before-creation"
	DeletePolicyBeforeCreationIfImmutable DeletePolicy = "before-creation-if-immutable"
)

type Ownership string

const (
	OwnershipAnyone  Ownership = "anyone"
	OwnershipRelease Ownership = "release"
)

type Stage string

const (
	StageInit              Stage = "init"                // create pending release
	StagePrePreUninstall   Stage = "pre-pre-uninstall"   // uninstall previous release resources
	StagePrePreInstall     Stage = "pre-pre-install"     // install crd
	StagePreInstall        Stage = "pre-install"         // install pre-hooks
	StagePreUninstall      Stage = "pre-uninstall"       // cleanup pre-hooks
	StageInstall           Stage = "install"             // install resources
	StageUninstall         Stage = "uninstall"           // cleanup resources
	StagePostInstall       Stage = "post-install"        // install post-hooks
	StagePostUninstall     Stage = "post-uninstall"      // cleanup post-hooks
	StagePostPostInstall   Stage = "post-post-install"   // install webhook
	StagePostPostUninstall Stage = "post-post-uninstall" // uninstall crd, webhook
	StageFinal             Stage = "final"               // succeed pending release, supersede previous release
)

var StagesOrdered = []Stage{
	StageInit,
	StagePrePreUninstall,
	StagePrePreInstall,
	StagePreInstall,
	StagePreUninstall,
	StageInstall,
	StageUninstall,
	StagePostInstall,
	StagePostUninstall,
	StagePostPostInstall,
	StagePostPostUninstall,
	StageFinal,
}

func StagesSortHandler(stage1, stage2 Stage) bool {
	index1 := lo.IndexOf(StagesOrdered, stage1)
	index2 := lo.IndexOf(StagesOrdered, stage2)

	return index1 < index2
}

func SubStageWeighted(stage Stage, weight int) Stage {
	return Stage(fmt.Sprintf("%s/weight:%d", stage, weight))
}

type On string

const (
	InstallOnInstall  On = "install"
	InstallOnUpgrade  On = "upgrade"
	InstallOnRollback On = "rollback"
	InstallOnDelete   On = "delete"
	InstallOnTest     On = "test"
)

type ResourceState string

const (
	ResourceStateAbsent  ResourceState = "absent"
	ResourceStatePresent ResourceState = "present"
	ResourceStateReady   ResourceState = "ready"
)

type StoreAs string

const (
	StoreAsNone    StoreAs = "none"
	StoreAsHook    StoreAs = "hook"
	StoreAsRegular StoreAs = "regular"
)

var OrderedStoreAs = []StoreAs{StoreAsNone, StoreAsHook, StoreAsRegular}

var DefaultRegistryCredentialsPath = filepath.Join(homedir.Get(), ".docker", config.ConfigFileName)

var (
	LabelKeyHumanManagedBy   = "app.kubernetes.io/managed-by"
	LabelKeyPatternManagedBy = regexp.MustCompile(`^app.kubernetes.io/managed-by$`)

	AnnotationKeyHumanReleaseName   = "meta.helm.sh/release-name"
	AnnotationKeyPatternReleaseName = regexp.MustCompile(`^meta.helm.sh/release-name$`)

	AnnotationKeyHumanReleaseNamespace   = "meta.helm.sh/release-namespace"
	AnnotationKeyPatternReleaseNamespace = regexp.MustCompile(`^meta.helm.sh/release-namespace$`)

	AnnotationKeyHumanHook   = "helm.sh/hook"
	AnnotationKeyPatternHook = regexp.MustCompile(`^helm.sh/hook$`)

	AnnotationKeyHumanResourcePolicy   = "helm.sh/resource-policy"
	AnnotationKeyPatternResourcePolicy = regexp.MustCompile(`^helm.sh/resource-policy$`)

	AnnotationKeyHumanDeletePolicy   = "werf.io/delete-policy"
	AnnotationKeyPatternDeletePolicy = regexp.MustCompile(`^werf.io/delete-policy$`)

	AnnotationKeyHumanHookDeletePolicy   = "helm.sh/hook-delete-policy"
	AnnotationKeyPatternHookDeletePolicy = regexp.MustCompile(`^helm.sh/hook-delete-policy$`)

	AnnotationKeyHumanReplicasOnCreation   = "werf.io/replicas-on-creation"
	AnnotationKeyPatternReplicasOnCreation = regexp.MustCompile(`^werf.io/replicas-on-creation$`)

	AnnotationKeyHumanFailMode   = "werf.io/fail-mode"
	AnnotationKeyPatternFailMode = regexp.MustCompile(`^werf.io/fail-mode$`)

	AnnotationKeyHumanFailuresAllowedPerReplica   = "werf.io/failures-allowed-per-replica"
	AnnotationKeyPatternFailuresAllowedPerReplica = regexp.MustCompile(`^werf.io/failures-allowed-per-replica$`)

	AnnotationKeyHumanIgnoreReadinessProbeFailsFor   = "werf.io/ignore-readiness-probe-fails-for-<container>"
	AnnotationKeyPatternIgnoreReadinessProbeFailsFor = regexp.MustCompile(`^werf.io/ignore-readiness-probe-fails-for-(?P<container>.+)$`)

	AnnotationKeyHumanLogRegex   = "werf.io/log-regex"
	AnnotationKeyPatternLogRegex = regexp.MustCompile(`^werf.io/log-regex$`)

	AnnotationKeyHumanLogRegexSkip   = "werf.io/log-regex-skip"
	AnnotationKeyPatternLogRegexSkip = regexp.MustCompile(`^werf.io/log-regex-skip$`)

	AnnotationKeyHumanLogRegexFor   = "werf.io/log-regex-for-<container>"
	AnnotationKeyPatternLogRegexFor = regexp.MustCompile(`^werf.io/log-regex-for-(?P<container>.+)$`)

	AnnotationKeyHumanSkipLogRegexFor   = "werf.io/log-regex-skip-for-<container>"
	AnnotationKeyPatternSkipLogRegexFor = regexp.MustCompile(`^werf.io/log-regex-skip-for-(?P<container>.+)$`)

	AnnotationKeyHumanNoActivityTimeout   = "werf.io/no-activity-timeout"
	AnnotationKeyPatternNoActivityTimeout = regexp.MustCompile(`^werf.io/no-activity-timeout$`)

	AnnotationKeyHumanShowLogsOnlyForContainers   = "werf.io/show-logs-only-for-containers"
	AnnotationKeyPatternShowLogsOnlyForContainers = regexp.MustCompile(`^werf.io/show-logs-only-for-containers$`)

	AnnotationKeyHumanShowServiceMessages   = "werf.io/show-service-messages"
	AnnotationKeyPatternShowServiceMessages = regexp.MustCompile(`^werf.io/show-service-messages$`)

	AnnotationKeyHumanShowLogsOnlyForNumberOfReplicas   = "werf.io/show-logs-only-for-number-of-replicas"
	AnnotationKeyPatternShowLogsOnlyForNumberOfReplicas = regexp.MustCompile(`^werf.io/show-logs-only-for-number-of-replicas$`)

	AnnotationKeyHumanSkipLogs   = "werf.io/skip-logs"
	AnnotationKeyPatternSkipLogs = regexp.MustCompile(`^werf.io/skip-logs$`)

	AnnotationKeyHumanSkipLogsForContainers   = "werf.io/skip-logs-for-containers"
	AnnotationKeyPatternSkipLogsForContainers = regexp.MustCompile(`^werf.io/skip-logs-for-containers$`)

	AnnotationKeyHumanTrackTerminationMode   = "werf.io/track-termination-mode"
	AnnotationKeyPatternTrackTerminationMode = regexp.MustCompile(`^werf.io/track-termination-mode$`)

	AnnotationKeyHumanWeight   = "werf.io/weight"
	AnnotationKeyPatternWeight = regexp.MustCompile(`^werf.io/weight$`)

	AnnotationKeyHumanHookWeight   = "helm.sh/hook-weight"
	AnnotationKeyPatternHookWeight = regexp.MustCompile(`^helm.sh/hook-weight$`)

	AnnotationKeyHumanDeployDependency   = "werf.io/deploy-dependency-<name>"
	AnnotationKeyPatternDeployDependency = regexp.MustCompile(`^werf.io/deploy-dependency-(?P<id>.+)$`)

	// TODO(v2): get rid
	AnnotationKeyHumanDependency   = "<name>.dependency.werf.io"
	AnnotationKeyPatternDependency = regexp.MustCompile(`^(?P<id>.+).dependency.werf.io$`)

	AnnotationKeyHumanExternalDependency   = "<name>.external-dependency.werf.io"
	AnnotationKeyPatternExternalDependency = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io$`)

	AnnotationKeyHumanLegacyExternalDependencyResource   = "<name>.external-dependency.werf.io/resource"
	AnnotationKeyPatternLegacyExternalDependencyResource = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/resource$`)

	AnnotationKeyHumanLegacyExternalDependencyNamespace   = "<name>.external-dependency.werf.io/namespace"
	AnnotationKeyPatternLegacyExternalDependencyNamespace = regexp.MustCompile(`^(?P<id>.+).external-dependency.werf.io/namespace$`)

	AnnotationKeyHumanSensitive   = "werf.io/sensitive"
	AnnotationKeyPatternSensitive = regexp.MustCompile(`^werf.io/sensitive$`)

	AnnotationKeyHumanSensitivePaths   = "werf.io/sensitive-paths"
	AnnotationKeyPatternSensitivePaths = regexp.MustCompile(`^werf.io/sensitive-paths$`)

	AnnotationKeyHumanDeployOn   = "werf.io/deploy-on"
	AnnotationKeyPatternDeployOn = regexp.MustCompile(`^werf.io/deploy-on$`)

	AnnotationKeyHumanOwnership   = "werf.io/ownership"
	AnnotationKeyPatternOwnership = regexp.MustCompile(`^werf.io/ownership$`)

	AnnotationKeyHumanDeletePropagation   = "werf.io/delete-propagation"
	AnnotationKeyPatternDeletePropagation = regexp.MustCompile(`^werf.io/delete-propagation$`)
)

var SprigFuncs = sprig.TxtFuncMap()
