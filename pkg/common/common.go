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

	"github.com/werf/3p-helm/pkg/helmpath"
	"github.com/werf/nelm/pkg/log"
)

var (
	// Use it whenever possible (e.g. in --help output) to refer to the product name, so it can be
	// easily white-labeled in forks.
	Brand   = "Nelm"
	Version = "0.0.0"
)

const (
	// ChartTSSourceDir is the directory containing TypeScript sources in a Helm chart.
	ChartTSSourceDir = "ts/"
	// ChartTSVendorBundleFile is the path to the vendor bundle file in a Helm chart.
	ChartTSVendorBundleFile = ChartTSSourceDir + "vendor/libs.js"
	// ChartTSEntryPointTS is the TypeScript entry point path.
	ChartTSEntryPointTS = "src/index.ts"
	// ChartTSEntryPointJS is the JavaScript entry point path.
	ChartTSEntryPointJS = "src/index.js"
)

// ChartTSEntryPoints defines supported TypeScript/JavaScript entry points (in priority order).
var ChartTSEntryPoints = [...]string{ChartTSEntryPointTS, ChartTSEntryPointJS}

const (
	DefaultBurstLimit = 100
	// TODO(major): switch to if-possible
	DefaultChartProvenanceStrategy = "never"
	// TODO(major): reconsider?
	DefaultDeletePropagation = metav1.DeletePropagationForeground
	DefaultDiffContextLines  = 3
	DefaultFieldManager      = "helm"
	// DefaultResourceValidationKubeVersion Kubernetes version to use during resource validation by kubeconform
	DefaultResourceValidationKubeVersion = "1.35.0"
	// TODO(major): update to a more recent version? Not sure about backwards compatibility.
	DefaultLocalKubeVersion      = "1.20.0"
	DefaultLogColorMode          = log.LogColorModeAuto
	DefaultNetworkParallelism    = 30
	DefaultProgressPrintInterval = 5 * time.Second
	DefaultQPSLimit              = 30
	DefaultReleaseHistoryLimit   = 10
	KubectlEditFieldManager      = "kubectl-edit"
	OldFieldManagerPrefix        = "werf"
	StageEndSuffix               = "end"
	StagePrefix                  = "stage"
	StageStartSuffix             = "start"
	StubReleaseName              = "stub-release"
	StubReleaseNamespace         = "stub-namespace"
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

// Type of the current operation.
type DeployType string

const (
	// Installing revision number 1 of the release always considered "Initial".
	DeployTypeInitial DeployType = "Initial"
	// Revision number > 1 with no successful revisions between revision 1 and the last revision
	// results in install.
	DeployTypeInstall DeployType = "Install"
	// If any successful revision found in history, then the current operation is upgrade.
	DeployTypeUpgrade DeployType = "Upgrade"
	// If current operation is release rollback.
	DeployTypeRollback DeployType = "Rollback"
	// If current operation is release uninstall.
	DeployTypeUninstall DeployType = "Uninstall"
)

// Configures resource deletions during deployment of this resource.
type DeletePolicy string

const (
	// Delete the resource after it is successfully deployed.
	DeletePolicySucceeded DeletePolicy = "succeeded"
	// Delete the resource after it fails to be deployed.
	DeletePolicyFailed DeletePolicy = "failed"
	// Delete the resource before deploying it. Basically means "recreate the resource" instead of
	// updating it.
	DeletePolicyBeforeCreation DeletePolicy = "before-creation"
	// If during resource update we got an immutable error, then recreate the resource instead of
	// updating it.
	DeletePolicyBeforeCreationIfImmutable DeletePolicy = "before-creation-if-immutable"
)

// Resource ownership.
type Ownership string

const (
	// The resource is owned by anyone (e.g. not tied to the release).
	OwnershipAnyone Ownership = "anyone"
	// The resource is owned by a single release.
	OwnershipRelease Ownership = "release"
)

// A sequential stage of the plan.
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

// On which action type the resource should be rendered for the deployment.
type On string

const (
	// Render resource on release installation.
	InstallOnInstall On = "install"
	// Render resource on release upgrade.
	InstallOnUpgrade On = "upgrade"
	// Render resource on release rollback.
	InstallOnRollback On = "rollback"
	// Render resource on release uninstall.
	InstallOnDelete On = "delete"
	// Render resource on release test.
	InstallOnTest On = "test"
)

// The state of the resource in the cluster.
type ResourceState string

const (
	ResourceStateAbsent  ResourceState = "absent"
	ResourceStatePresent ResourceState = "present"
	ResourceStateReady   ResourceState = "ready"
)

// How the resource should be stored in the Helm release.
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

	AnnotationKeyHumanDeleteDependency   = "werf.io/delete-dependency-<name>"
	AnnotationKeyPatternDeleteDependency = regexp.MustCompile(`^werf.io/delete-dependency-(?P<id>.+)$`)

	// TODO(major): get rid
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

var (
	DefaultPlanArtifactLifetime = 2 * time.Hour

	DefaultResourceValidationSchema = []string{
		"https://raw.githubusercontent.com/yannh/kubernetes-json-schema/master/{{ .NormalizedKubernetesVersion }}-standalone{{ .StrictSuffix }}/{{ .ResourceKind }}{{ .KindSuffix }}.json",
		"https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json",
	}
	DefaultResourceValidationCacheLifetime = 48 * time.Hour

	APIResourceValidationJSONSchemasCacheDir = helmpath.CachePath("nelm", "api-resource-json-schemas")
)
