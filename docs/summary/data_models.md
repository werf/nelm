# Nelm Data Models Reference

This document describes the key data structures used throughout Nelm for Kubernetes resource management and deployment operations.

For configuration option structs (`KubeConnectionOptions`, `ChartRepoConnectionOptions`, `ValuesOptions`, `SecretValuesOptions`, `TrackingOptions`, `ResourceValidationOptions`), see `pkg/common/options.go` directly.

<!-- toc -->

- [Enums and Constants](#enums-and-constants)
  - [DeployType](#deploytype)
  - [DeletePolicy](#deletepolicy)
  - [Ownership](#ownership)
  - [Stage](#stage)
  - [On (Install Condition)](#on-install-condition)
  - [ResourceState](#resourcestate)
  - [StoreAs](#storeas)
  - [FailMode](#failmode)
  - [TrackTerminationMode](#trackterminationmode)
  - [Output Formats](#output-formats)
  - [Release Storage Drivers](#release-storage-drivers)
- [Resource Types](#resource-types)
  - [ResourceMeta](#resourcemeta)
  - [ResourceSpec](#resourcespec)
  - [InstallableResource](#installableresource)
  - [DeletableResource](#deletableresource)
- [Dependency Types](#dependency-types)
  - [InternalDependency](#internaldependency)
  - [ExternalDependency](#externaldependency)
- [Operation Types](#operation-types)
  - [Operation](#operation)
  - [OperationType](#operationtype)
  - [OperationCategory](#operationcategory)
  - [OperationStatus](#operationstatus)
- [Type Relationships](#type-relationships)

<!-- tocstop -->

## Enums and Constants

### DeployType

**Location:** `pkg/common/common.go`

| Value | Description |
|-------|-------------|
| `DeployTypeInitial` | First revision (revision 1) of a release |
| `DeployTypeInstall` | Revision > 1 with no successful prior revisions |
| `DeployTypeUpgrade` | Upgrade from a successful prior revision |
| `DeployTypeRollback` | Rollback to a previous revision |
| `DeployTypeUninstall` | Release uninstallation |

### DeletePolicy

**Location:** `pkg/common/common.go`

| Value | Description |
|-------|-------------|
| `DeletePolicySucceeded` | Delete after successful deployment |
| `DeletePolicyFailed` | Delete after failed deployment |
| `DeletePolicyBeforeCreation` | Delete before deploying (recreate) |
| `DeletePolicyBeforeCreationIfImmutable` | Recreate only if immutable error occurs |

### Ownership

**Location:** `pkg/common/common.go`

| Value | Description |
|-------|-------------|
| `OwnershipAnyone` | Resource not tied to the release |
| `OwnershipRelease` | Resource owned by a single release |

### Stage

**Location:** `pkg/common/common.go`

Sequential stages of the deployment plan.

| Value | Description |
|-------|-------------|
| `StageInit` | Create pending release |
| `StagePrePreUninstall` | Uninstall previous release resources |
| `StagePrePreInstall` | Install CRDs |
| `StagePreInstall` | Install pre-hooks |
| `StagePreUninstall` | Cleanup pre-hooks |
| `StageInstall` | Install resources |
| `StageUninstall` | Cleanup resources |
| `StagePostInstall` | Install post-hooks |
| `StagePostUninstall` | Cleanup post-hooks |
| `StagePostPostInstall` | Install webhooks |
| `StagePostPostUninstall` | Uninstall CRDs, webhooks |
| `StageFinal` | Succeed pending release, supersede previous |

### On (Install Condition)

**Location:** `pkg/common/common.go`

| Value | Description |
|-------|-------------|
| `InstallOnInstall` | Render on release installation |
| `InstallOnUpgrade` | Render on release upgrade |
| `InstallOnRollback` | Render on release rollback |
| `InstallOnDelete` | Render on release uninstall |
| `InstallOnTest` | Render on release test |

### ResourceState

**Location:** `pkg/common/common.go`

| Value | Description |
|-------|-------------|
| `ResourceStateAbsent` | Resource does not exist |
| `ResourceStatePresent` | Resource exists |
| `ResourceStateReady` | Resource is ready |

### StoreAs

**Location:** `pkg/common/common.go`

| Value | Description |
|-------|-------------|
| `StoreAsNone` | Do not store in release |
| `StoreAsHook` | Store as a hook |
| `StoreAsRegular` | Store as regular resource |

### FailMode

**Location:** `github.com/werf/kubedog/pkg/trackers/rollout/multitrack` (used from `internal/resource/resource.go`)

Controls how Nelm handles failures for tracked resources (e.g., fail the whole deploy immediately vs continue). Values are passed through to kubedog’s multitrack logic; see kubedog for the full list.

### TrackTerminationMode

**Location:** `github.com/werf/kubedog/pkg/trackers/rollout/multitrack` (used from `internal/resource/resource.go`)

Controls how tracking behaves around termination/shutdown. Values are passed through to kubedog’s multitrack logic; see kubedog for the full list.

### Output Formats

**Location:** `pkg/common/common.go`

| Constant | Value |
|----------|-------|
| `OutputFormatJSON` | "json" |
| `OutputFormatTable` | "table" |
| `OutputFormatYAML` | "yaml" |

### Release Storage Drivers

**Location:** `pkg/common/common.go`

| Constant | Value |
|----------|-------|
| `ReleaseStorageDriverDefault` | "" |
| `ReleaseStorageDriverConfigMap` | "configmap" |
| `ReleaseStorageDriverConfigMaps` | "configmaps" |
| `ReleaseStorageDriverSecret` | "secret" |
| `ReleaseStorageDriverSecrets` | "secrets" |
| `ReleaseStorageDriverSQL` | "sql" |
| `ReleaseStorageDriverMemory` | "memory" |

## Resource Types

### ResourceMeta

**Location:** `internal/resource/spec/resource_meta.go`

Basic information about a Kubernetes resource without the full spec. Used for operations that do not require the full resource definition (get, delete, track).

**Key Fields:** `Name`, `Namespace`, `GroupVersionKind`, `FilePath`, `Annotations`, `Labels`

**Key Methods:**
- `ID() string` - Unique identifier: `namespace:group:kind:name`
- `IDHuman() string` - Human-readable format: `Kind/name (namespace=ns)`

### ResourceSpec

**Location:** `internal/resource/spec/resource_spec.go`

Extends ResourceMeta with the full resource specification. Used for create/update operations.

**Key Fields:** `*ResourceMeta` (embedded), `Unstruct` (*unstructured.Unstructured), `StoreAs`

### InstallableResource

**Location:** `internal/resource/resource.go`

Represents a Kubernetes resource that can be installed. Contains deployment configuration derived from annotations.

**Key Fields:** `*spec.ResourceSpec` (embedded) plus:
- Ownership/deletion: `Ownership`, `KeepOnDelete`, `DeletePropagation`, `DeleteOnSucceeded`, `DeleteOnFailed`
- Recreate behavior: `Recreate`, `RecreateOnImmutable`, `DefaultReplicasOnCreation`
- Tracking/failure behavior: `FailMode`, `FailuresAllowed`, `TrackTerminationMode`, `NoActivityTimeout`, log/skip-log regexes and related options
- Ordering/dependencies: `Weight`, `ManualInternalDependencies`, `AutoInternalDependencies`, `ExternalDependencies`
- Conditional deploy: `DeployConditions`

### DeletableResource

**Location:** `internal/resource/resource.go`

Represents a Kubernetes resource that can be deleted.

**Key Fields:** `*spec.ResourceMeta` (embedded), `Ownership`, `KeepOnDelete`, `DeletePropagation`, `ManualInternalDependencies`, `AutoInternalDependencies`

## Dependency Types

### InternalDependency

**Location:** `internal/resource/dependency.go`

Dependency on a Kubernetes resource within the Helm release.

**Key Fields:** `*spec.ResourceMatcher` (embedded), `ResourceState`

Auto-detection is best-effort and currently focuses on a subset of resources (mostly PodSpec-like references and some RBAC refs). It includes `env`/`envFrom` Secret/ConfigMap refs, volumes, service accounts, imagePullSecrets, StatefulSet serviceName, and RoleBinding/ClusterRoleBinding roleRef. See `internal/resource/dependency.go` for the authoritative logic and TODOs.

### ExternalDependency

**Location:** `internal/resource/dependency.go`

Dependency on a resource outside the Helm release.

**Key Fields:** `*spec.ResourceMeta` (embedded)

## Operation Types

### Operation

**Location:** `internal/plan/operation.go`

Represents a single operation in the deployment plan.

**Key Fields:** `Type` (OperationType), `Category` (OperationCategory), `Status` (OperationStatus), `Config` (OperationConfig interface), `Iteration`

**Key Methods:**
- `ID() string` - Unique operation ID: `type/version/iteration/configID`
- `IDHuman() string` - Human-readable operation description

### OperationType

**Location:** `internal/plan/operation.go`

| Value | Description |
|-------|-------------|
| `OperationTypeApply` | Blind apply (when create/update unclear) |
| `OperationTypeCreate` | Create new resource |
| `OperationTypeCreateRelease` | Create Helm release |
| `OperationTypeDelete` | Delete resource |
| `OperationTypeDeleteRelease` | Delete Helm release |
| `OperationTypeNoop` | No operation |
| `OperationTypeRecreate` | Delete and recreate resource |
| `OperationTypeTrackAbsence` | Track until resource is absent |
| `OperationTypeTrackPresence` | Track until resource is present |
| `OperationTypeTrackReadiness` | Track until resource is ready |
| `OperationTypeUpdate` | Update existing resource |
| `OperationTypeUpdateRelease` | Update Helm release |

### OperationCategory

**Location:** `internal/plan/operation.go`

| Value | Description |
|-------|-------------|
| `OperationCategoryMeta` | Meta operations (grouping, noop) |
| `OperationCategoryResource` | Mutate Kubernetes resources |
| `OperationCategoryTrack` | Track resources (read-only) |
| `OperationCategoryRelease` | Mutate Helm releases |

### OperationStatus

**Location:** `internal/plan/operation.go`

| Value | Description |
|-------|-------------|
| `OperationStatusUnknown` | Unknown/initial status |
| `OperationStatusPending` | Waiting to execute |
| `OperationStatusCompleted` | Successfully completed |
| `OperationStatusFailed` | Failed execution |

## Type Relationships

```
ResourceMeta
    |
    +-- ResourceSpec (extends with Unstruct, StoreAs)
            |
            +-- InstallableResource (extends with deploy config)
            |       |
            |       +-- InstallableResourceInfo (adds cluster state, decisions)
            |
            +-- DeletableResource (extends with delete config)
                    |
                    +-- DeletableResourceInfo (adds cluster state, decisions)

Operation
    |
    +-- Type: OperationType
    +-- Category: OperationCategory
    +-- Status: OperationStatus
    +-- Config: OperationConfig (interface)
            |
            +-- OperationConfigCreate
            +-- OperationConfigUpdate
            +-- OperationConfigDelete
            +-- OperationConfigRecreate
            +-- OperationConfigApply
            +-- OperationConfigTrackReadiness
            +-- OperationConfigTrackPresence
            +-- OperationConfigTrackAbsence
            +-- OperationConfigCreateRelease
            +-- OperationConfigUpdateRelease
            +-- OperationConfigDeleteRelease
            +-- OperationConfigNoop

InternalDependency
    |
    +-- ResourceMatcher (pattern matching)
    +-- ResourceState (required state)

ExternalDependency
    |
    +-- ResourceMeta (specific resource)
```

The data flow typically follows:
1. Chart rendering produces `ResourceSpec` instances
2. `ResourceSpec` is enriched to `InstallableResource` or `DeletableResource`
3. Cluster queries produce `ResourceInfo` with decisions
4. `ResourceInfo` is used to build `Operation` instances in the plan
