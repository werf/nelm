<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Overview](#overview)
- [Quick Start for Contributors](#quick-start-for-contributors)
- [Package Structure](#package-structure)
- [Core Concepts](#core-concepts)
  - [Resource Type Hierarchy](#resource-type-hierarchy)
  - [Plan and Operations](#plan-and-operations)
  - [Releases](#releases)
- [Deployment Pipeline](#deployment-pipeline)
  - [Phase 1: Chart Rendering](#phase-1-chart-rendering)
  - [Phase 2: Resource Transformations](#phase-2-resource-transformations)
  - [Phase 3: Release Building](#phase-3-release-building)
  - [Phase 4: Resource Building](#phase-4-resource-building)
  - [Phase 5: Cluster State Analysis](#phase-5-cluster-state-analysis)
  - [Phase 6: Plan Construction](#phase-6-plan-construction)
  - [Phase 7: Plan Execution](#phase-7-plan-execution)
- [Deployment Stages and Conditions](#deployment-stages-and-conditions)
  - [Stage Order](#stage-order)
  - [Deploy Conditions](#deploy-conditions)
  - [Special Resource Handling](#special-resource-handling)
- [Dependency Resolution](#dependency-resolution)
  - [Weight-Based Ordering](#weight-based-ordering)
  - [Manual Dependencies](#manual-dependencies)
  - [Automatic Dependencies](#automatic-dependencies)
  - [External Dependencies](#external-dependencies)
- [Kubernetes Client](#kubernetes-client)
  - [Server-Side Apply](#server-side-apply)
  - [Caching](#caching)
  - [Resource Locking](#resource-locking)
- [Progress Tracking](#progress-tracking)
  - [Kubedog Integration](#kubedog-integration)
  - [Progress Tables](#progress-tables)
  - [Tracking Timeouts](#tracking-timeouts)
- [Failure Handling and Rollback](#failure-handling-and-rollback)
  - [Operation Status Flow](#operation-status-flow)
  - [Failure Plan](#failure-plan)
  - [Auto-Rollback](#auto-rollback)
- [Release Comparison](#release-comparison)
- [Resource Validation](#resource-validation)
- [Secrets Management](#secrets-management)
- [Distributed Locking](#distributed-locking)
- [Feature Gates](#feature-gates)
- [Exit Codes](#exit-codes)
- [Testing](#testing)
- [Key External Dependencies](#key-external-dependencies)
- [Common Patterns](#common-patterns)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Overview

Nelm is a Helm-compatible Kubernetes deployment tool that rewrites the deployment subsystem using a DAG-based approach. Originally the werf deployment engine, it evolved into a standalone tool that reuses Helm's chart loading and templating (via a fork) while providing enhanced deployment capabilities.

**Key architectural decisions:**

| Decision | Rationale |
|----------|-----------|
| DAG-based deployment | Enables parallel execution with dependency ordering, unlike Helm's sequential hooks |
| Server-Side Apply (SSA) | Safer than 3-way merge, better conflict detection, native Kubernetes support |
| Separation of concerns | Clear boundaries: CLI → pkg/action → internal implementation |
| No frameworks, no CGO | Simple, portable Go code with minimal dependencies |
| Helm release compatibility | Stores releases in Helm format, enabling migration and interoperability |

## Quick Start for Contributors

**Entry points:**

| Starting Point | Location | Purpose |
|----------------|----------|---------|
| CLI commands | `cmd/nelm/` | Command-line interface (Cobra) |
| Public API | `pkg/action/` | Functions for external consumers |
| Core deployment | `internal/plan/` | DAG planning and execution |
| Resource handling | `internal/resource/` | Resource parsing and metadata |

**Key files to understand the system:**

```
pkg/action/release_install.go    # Main deployment flow
internal/plan/plan_build.go      # DAG construction
internal/plan/plan_execute.go    # DAG execution
internal/resource/resource.go    # Resource abstraction
internal/resource/dependency.go  # Auto-dependency detection
pkg/common/common.go             # Constants, types, defaults
```

**Following a deployment:**

1. User runs `nelm release install` → `cmd/nelm/release_install.go`
2. Command calls `action.ReleaseInstall()` → `pkg/action/release_install.go`
3. Chart rendered → `internal/chart/chart_render.go`
4. Resources built → `internal/resource/resource.go`
5. Plan constructed → `internal/plan/plan_build.go`
6. Plan executed → `internal/plan/plan_execute.go`

## Package Structure

```
nelm/
├── cmd/nelm/              # CLI Layer (Cobra commands)
│   ├── main.go            # Entry point, exit code handling
│   ├── root.go            # Root command, subcommand registration
│   ├── release_*.go       # Release management commands
│   ├── chart_*.go         # Chart operations
│   ├── chart_secret_*.go  # Secret encryption/decryption
│   └── repo_*.go          # Repository management (wrapped Helm)
│
├── pkg/                   # Public API (importable)
│   ├── action/            # High-level operations
│   │   ├── release_install.go       # Deploy chart
│   │   ├── release_uninstall.go     # Remove release
│   │   ├── release_rollback.go      # Rollback to revision
│   │   ├── release_plan_install.go  # Preview changes
│   │   ├── chart_render.go          # Render templates
│   │   ├── chart_lint.go            # Validate chart
│   │   └── secret_*.go              # Secret key/file operations
│   ├── common/            # Shared types and constants
│   │   ├── common.go      # Stages, deploy types, annotations
│   │   └── options.go     # Option structs for all actions
│   ├── featgate/          # Feature gate system
│   └── log/               # Logging interface (logboek backend)
│
└── internal/              # Private implementation
    ├── chart/             # Chart downloading and rendering
    ├── kube/              # Kubernetes client abstraction
    │   ├── factory.go     # ClientFactory (static, dynamic, discovery)
    │   ├── client_kube.go # KubeClient with caching
    │   └── fake/          # Test doubles
    ├── lock/              # Distributed locking (ConfigMap-based)
    ├── plan/              # DAG planning and execution
    │   ├── plan.go        # Plan structure, optimization
    │   ├── plan_build.go  # DAG construction
    │   ├── plan_execute.go# Parallel execution
    │   ├── operation.go   # Operation types and configs
    │   └── resource_info.go # Cluster state analysis
    ├── release/           # Helm release management
    ├── resource/          # Resource abstraction
    │   ├── resource.go    # InstallableResource, DeletableResource
    │   ├── dependency.go  # Auto-dependency detection
    │   ├── metadata.go    # Annotation parsing, validation
    │   ├── validate.go    # Schema validation (kubeconform)
    │   └── spec/          # ResourceSpec, transformers, patchers
    ├── track/             # Progress table printing
    └── util/              # Utilities (MultiError, diff)
```

## Core Concepts

### Resource Type Hierarchy

Resources flow through several type transformations during deployment:

```
                        Chart Templates
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ ResourceMeta                                                     │
│   Minimal info: Name, Namespace, GVK, Annotations, Labels       │
│   Used for: GET, DELETE, tracking operations                    │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│ ResourceSpec                                                     │
│   Full manifest: Unstructured + ResourceMeta + StoreAs          │
│   Used for: Release storage, CREATE, APPLY operations           │
└─────────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌──────────────────────────┐    ┌──────────────────────────┐
│ InstallableResource      │    │ DeletableResource        │
│   + Weight               │    │   + Ownership            │
│   + Ownership            │    │   + KeepOnDelete         │
│   + Dependencies         │    │   + DeletePropagation    │
│   + DeployConditions     │    │                          │
│   + Tracking config      │    │                          │
└──────────────────────────┘    └──────────────────────────┘
              │                               │
              ▼                               ▼
┌──────────────────────────┐    ┌──────────────────────────┐
│ InstallableResourceInfo  │    │ DeletableResourceInfo    │
│   + GetResult (cluster)  │    │   + GetResult (cluster)  │
│   + DryApplyResult       │    │   + MustDelete           │
│   + MustInstall type     │    │   + MustTrackAbsence     │
│   + Stage assignment     │    │                          │
└──────────────────────────┘    └──────────────────────────┘
```

### Plan and Operations

The deployment plan is a DAG (Directed Acyclic Graph) of operations:

```go
// Plan wraps a DAG of operations.
type Plan struct {
    Graph graph.Graph[string, *Operation]
}

// Operation represents a single action in the DAG.
// Version/Iteration are part of the operation ID to keep it stable and unique.
type Operation struct {
    Type      OperationType
    Version   OperationVersion
    Category  OperationCategory
    Iteration OperationIteration
    Status    OperationStatus
    Config    OperationConfig
}
```

**Operation types:**

| Type | Category | Description |
|------|----------|-------------|
| `create` | `resource` | Create via SSA (resource doesn't exist) |
| `update` | `resource` | Update via SSA (dry-run apply shows changes) |
| `apply` | `resource` | SSA apply when diffing isn't possible (dry-run apply error) |
| `recreate` | `resource` | Delete → track absence → create (requested or immutable changes) |
| `delete` | `resource` | Delete resource |
| `track-readiness` | `track` | Wait for resource to become ready |
| `track-presence` | `track` | Wait for resource to exist |
| `track-absence` | `track` | Wait for resource to be deleted |
| `create-release` | `release` | Persist new release revision |
| `update-release` | `release` | Update release status/info |
| `delete-release` | `release` | Delete release revision from storage |
| `noop` | `meta` | Stage/substage boundary marker |

### Releases

Nelm stores releases in Helm's format for compatibility:

```go
// From 3p-helm fork (simplified).
type Release struct {
    Name      string
    Namespace string
    Version   int               // Revision number
    Manifest  string            // Regular resources as YAML
    Hooks     []*Hook           // Hooks (helm.sh/hook)
    Chart     *Chart
    Config    map[string]any    // User values
    Info      *Info             // Status, notes, timestamps
    Labels    map[string]string // Stored in driver metadata

    UnstoredManifest string // StoreAsNone resources (e.g., standalone CRDs)
}
```

**Deploy types determined by history:**

| Type | Condition |
|------|-----------|
| `Initial` | No previous revisions exist (new revision = 1) |
| `Install` | Previous revisions exist, but no deployed revision exists (retry after failures) |
| `Upgrade` | A deployed revision exists |
| `Rollback` | Explicit rollback command (`nelm release rollback`) |
| `Uninstall` | Explicit uninstall command (`nelm release uninstall`) |

## Deployment Pipeline

### Phase 1: Chart Rendering

```
Chart Path + Values + Secret Values
              │
              ▼
      chart.RenderChart()
              │
              ├── Download chart (local, URL, registry, or repository)
              ├── Download dependencies (Chart.lock)
              ├── Merge values (defaults → values.yaml → --set → secrets)
              └── Render Go templates
              │
              ▼
        []ResourceSpec
```

### Phase 2: Resource Transformations

```
[]ResourceSpec
       │
       ▼
spec.BuildTransformedResourceSpecs()
       │
       ├── ResourceListsTransformer
       │     Expands List-kind resources into individual items
       │
       └── DropInvalidAnnotationsAndLabelsTransformer
             Removes metadata that would fail Kubernetes validation
       │
       ▼
[]ResourceSpec (transformed)
```

### Phase 3: Release Building

```
[]ResourceSpec (transformed)
       │
       ▼
spec.BuildReleasableResourceSpecs()
       │
       ├── ExtraMetadataPatcher
       │     Adds user-provided extra annotations/labels (opt-in)
       │
       └── SecretStringDataPatcher
             Converts Secret.stringData to Secret.data (base64)
       │
       ▼
[]ResourceSpec (releasable)
       │
       ▼
release.NewRelease()
       │
       ├── Separate hooks (helm.sh/hook) from regular resources
       ├── Combine manifests into YAML strings
       └── Build Release struct with chart, config, info
       │
       ▼
*helmrelease.Release
```

### Phase 4: Resource Building

```
Previous Release + New Release
         │
         ▼
resource.BuildResources()
         │
         ├── Patch deployable objects:
         │     ReleaseMetadataPatcher (meta.helm.sh/* + managed-by=Helm for owned resources)
         │     ExtraMetadataPatcher (extra runtime annotations/labels)
         │
         ├── Parse annotations:
         │     werf.io/weight (and helm.sh/hook-weight), werf.io/deploy-on (and helm.sh/hook)
         │     werf.io/ownership, werf.io/delete-policy (and helm.sh/hook-delete-policy)
         │     helm.sh/resource-policy (keep), werf.io/delete-propagation
         │     werf.io/deploy-dependency-*, *.dependency.werf.io, *.external-dependency.werf.io
         │
         ├── Detect automatic dependencies:
         │     Deployment → ConfigMap (via envFrom, volumes)
         │     StatefulSet → Service (via serviceName)
         │     RoleBinding → Role (via roleRef)
         │
         ├── Filter by deploy type:
         │     Install: resources with InstallOnInstall
         │     Upgrade: resources with InstallOnUpgrade
         │     Rollback: resources with InstallOnRollback
         │
         └── Validate configurations
         │
         ▼
[]InstallableResource + []DeletableResource
```

### Phase 5: Cluster State Analysis

```
[]InstallableResource + []DeletableResource
              │
              ▼
      plan.BuildResourceInfos()
              │
              For each InstallableResource:
              ├── GET from cluster → current state
              ├── DRY-RUN SSA → what would change
              ├── Compare → determine action:
              │     None: no changes needed
              │     Create: resource doesn't exist
              │     Update: resource exists, has changes
              │     Recreate: immutable field changes
              │     Apply: fallback for unknown errors
              ├── Determine tracking requirements
              └── Assign to deployment stage
              │
              For each DeletableResource:
              ├── GET from cluster
              ├── Check ownership and resource-policy (keep)
              └── Determine MustDelete, MustTrackAbsence
              │
              ▼
[]InstallableResourceInfo + []DeletableResourceInfo
```

### Phase 6: Plan Construction

```
ResourceInfos + ReleaseInfos
         │
         ▼
  plan.BuildPlan()
         │
         ├── Add stage skeleton (noop operations for boundaries)
         ├── Add weighted substages (parallel groups by weight)
         ├── Add release operations (create/update/supersede)
         ├── Add resource operations (create/update/delete)
         ├── Add tracking operations (readiness/presence/absence)
         ├── Connect dependencies (manual + auto-detected)
         └── Optimize DAG:
               - Transitive reduction (remove redundant edges)
               - Remove useless noops
         │
         ▼
      Plan (DAG)
```

**Example DAG structure:**

```
init.start ─► init.end ─► pre-install.start ─► pre-install.end ─► install.start
                                                                      │
                          ┌───────────────────────────────────────────┤
                          ▼                                           ▼
                   create-release                              apply-configmap
                          │                                           │
                          │         ┌─────────────────────────────────┘
                          │         ▼
                          │   apply-deployment (depends on configmap)
                          │         │
                          └────┬────┘
                               ▼
                        track-readiness(deployment)
                               │
                               ▼
                        supersede-release(prev)
                               │
                               ▼
                        install.end ─► post-install.start ─► ...
```

### Phase 7: Plan Execution

```
Plan (DAG)
     │
     ▼
plan.ExecutePlan()
     │
     ├── Create worker pool (NetworkParallelism, default 30)
     ├── Build predecessor map from DAG
     │
     └── Loop until all operations complete:
           │
           ├── Find operations with no pending predecessors
           ├── Schedule each in worker pool goroutine:
           │     ├── Execute operation (Create/Update/Delete/Track/...)
           │     ├── On success: mark completed, signal channel
           │     └── On failure: mark failed, cancel context
           │
           └── Remove completed operations from predecessor maps
     │
     ▼
Cluster Updated, Release Stored
```

**Parallel execution characteristics:**

- Operations execute as soon as all predecessors complete
- First error cancels context, preventing new operations
- In-flight operations may stop early (operations share the canceled context)
- Results collected: completed, canceled, failed

## Deployment Stages and Conditions

### Stage Order

Stages execute sequentially; within each stage, resources execute in parallel (respecting weights and dependencies):

| Stage | Purpose | Typical Resources |
|-------|---------|-------------------|
| `init` | Create pending release | Release creation |
| `pre-pre-uninstall` | Delete resources from the previous revision that are no longer in the new revision | Removed resources |
| `pre-pre-install` | Install CRDs early | CustomResourceDefinitions |
| `pre-install` | Pre hooks stage (install/upgrade/rollback/delete) | Jobs, hooks with `pre-*` |
| `pre-uninstall` | Cleanup after `pre-install` (delete-policy=succeeded) | Hook deletion |
| `install` | Main deployment (also used by `werf.io/deploy-on: delete`) | Deployments, Services, ConfigMaps |
| `uninstall` | Delete resources (uninstall deploy, and delete-policy=succeeded cleanup after `install`) | Removed resources |
| `post-install` | Post hooks stage | Jobs, hooks with `post-*` |
| `post-uninstall` | Cleanup after `post-install` (delete-policy=succeeded) | Hook deletion |
| `post-post-install` | Install webhooks late | MutatingWebhookConfiguration, ValidatingWebhookConfiguration |
| `post-post-uninstall` | Cleanup after `post-post-install` (CRD/webhook ordering) | CRD, webhook cleanup |
| `final` | Finalize release (succeed pending, supersede previous) | Release finalization |

### Deploy Conditions

The `werf.io/deploy-on` annotation controls when resources deploy:

```yaml
# Deploy only on install
werf.io/deploy-on: install

# Deploy on install and upgrade
werf.io/deploy-on: install,upgrade

# Pre-install hook
werf.io/deploy-on: pre-install

# Multiple conditions
werf.io/deploy-on: pre-install,pre-upgrade
```

**Note:** For uninstall flows, `werf.io/deploy-on` uses Helm naming: `delete`, `pre-delete`, `post-delete`.

**Supported values:**

| Value | Deploy Type | Stage |
|-------|-------------|-------|
| `install` | Initial/Install | `install` |
| `upgrade` | Upgrade | `install` |
| `rollback` | Rollback | `install` |
| `delete` | Uninstall | `install` |
| `pre-install` | Initial/Install | `pre-install` |
| `post-install` | Initial/Install | `post-install` |
| `pre-upgrade` | Upgrade | `pre-install` |
| `post-upgrade` | Upgrade | `post-install` |
| `pre-rollback` | Rollback | `pre-install` |
| `post-rollback` | Rollback | `post-install` |
| `pre-delete` | Uninstall | `pre-install` |
| `post-delete` | Uninstall | `post-install` |
| `test` | Test | `install` |
| `test-success` | Test | `install` |

**Default behavior:**
- Regular resources: `install,upgrade,rollback` → `install`
- CRDs: `install,upgrade,rollback` → `pre-pre-install`
- Webhooks: `install,upgrade,rollback` → `post-post-install`

### Special Resource Handling

**CRDs** (CustomResourceDefinitions):
- Always deploy in `pre-pre-install` (earliest stage)
- Ensures custom types exist before resources using them
- Mapper reset after CRD creation
- Standalone CRDs from chart `crds/` are treated as `StoreAsNone` and stored in `Release.UnstoredManifest`

**Webhooks** (MutatingWebhookConfiguration, ValidatingWebhookConfiguration):
- Deploy in `post-post-install` (after all main resources)
- Prevents webhook from blocking its own deployment
- Can be overridden with manual dependencies

**Hooks** (helm.sh/hook):
- Processed for Helm compatibility
- `werf.io/deploy-on` takes precedence if both specified

## Dependency Resolution

### Weight-Based Ordering

Resources with the same weight deploy in parallel:

```yaml
# Deploy early (lower weight = earlier)
werf.io/weight: "-10"

# Deploy late
werf.io/weight: "100"

# Default weight is 0
```

Within each stage, resources are grouped by weight and executed in weight order.

### Manual Dependencies

Explicit dependencies via annotations:

```yaml
# Wait for StatefulSet "postgres" to be ready
werf.io/deploy-dependency-db: state=ready,kind=StatefulSet,name=postgres

# Wait for any resource matching criteria
werf.io/deploy-dependency-cache: state=present,kind=ConfigMap,name=cache-config
```

These dependencies are resolved only against resources in the current plan/release; for dependencies outside the release, use the external dependency annotations below.

**Dependency states:**
- `present` - Resource exists in cluster
- `ready` - Resource is ready (pods running, etc.)

### Automatic Dependencies

Nelm detects dependencies by analyzing resource references (`internal/resource/dependency.go`):

**Fields below are shown relative to the PodSpec** (Pod: `.spec`; most controllers: `.spec.template.spec`; CronJob: `.spec.jobTemplate.spec.template.spec`).

| Source | Field | Target |
|--------|-------|--------|
| PodSpec (Pod and workload templates) | `containers[].env[].valueFrom.configMapKeyRef` | ConfigMap |
| PodSpec (Pod and workload templates) | `containers[].env[].valueFrom.secretKeyRef` | Secret |
| PodSpec (Pod and workload templates) | `containers[].envFrom[].configMapRef` | ConfigMap |
| PodSpec (Pod and workload templates) | `containers[].envFrom[].secretRef` | Secret |
| PodSpec (Pod and workload templates) | `volumes[].configMap` | ConfigMap |
| PodSpec (Pod and workload templates) | `volumes[].secret` | Secret |
| PodSpec (Pod and workload templates) | `imagePullSecrets[]` | Secret |
| PodSpec (Pod and workload templates) | `serviceAccountName` | ServiceAccount |
| PodSpec (Pod and workload templates) | `priorityClassName` | PriorityClass |
| PodSpec (Pod and workload templates) | `runtimeClassName` | RuntimeClass |
| PodSpec (Pod and workload templates) | `resourceClaims[].source.resourceClaimName` | ResourceClaim |
| PodSpec (Pod and workload templates) | `resourceClaims[].source.resourceClaimTemplateName` | ResourceClaimTemplate |
| StatefulSet | `spec.serviceName` | Service |
| RoleBinding | `roleRef` | Role |
| ClusterRoleBinding | `roleRef` | ClusterRole |

**Note:** References marked `optional: true` are skipped.

### External Dependencies

For resources outside the release:

```yaml
# Preferred format: apiVersion:kind[:namespace]:name
db.external-dependency.werf.io: v1:Secret:default:external-config

# Legacy format (kubectl-style resource string + explicit namespace):
db.external-dependency.werf.io/resource: secrets/external-config
db.external-dependency.werf.io/namespace: default
```

External dependencies are implemented as `track-presence` operations inserted before the dependent resource's deploy operation.

## Kubernetes Client

### Server-Side Apply

All resource mutations use Kubernetes Server-Side Apply (SSA):

```go
// KubeClient.Apply uses SSA with force and field manager
clientResource.Apply(ctx, name, unstruct, metav1.ApplyOptions{
    Force:        true,
    FieldManager: "helm",
})
```

Nelm intentionally uses the field manager `helm` (`common.DefaultFieldManager`) for Helm-compatible ownership semantics.

**Benefits over 3-way merge:**
- Native Kubernetes conflict detection
- Field-level ownership tracking
- Better handling of defaulted fields
- Atomic updates

### Caching

KubeClient maintains a process-local cache for API responses:

```go
type KubeClient struct {
    clusterCache  *ttlcache.Cache[string, *clusterCacheEntry]
    resourceLocks *sync.Map
}
```

**Cache behavior:**
- GET results cached by resource ID + version
- Cache updated after successful mutations
- Cache invalidated on delete
- Dry-run results not cached
- Errors cached to prevent repeated failures

### Resource Locking

Per-resource mutex prevents concurrent operations:

```go
func (c *KubeClient) Get(ctx context.Context, meta *spec.ResourceMeta, opts KubeClientGetOptions) (*unstructured.Unstructured, error) {
    lock := c.resourceLock(meta)  // sync.Mutex per resource
    lock.Lock()
    defer lock.Unlock()
    // ... operation
}
```

## Progress Tracking

### Kubedog Integration

Nelm uses kubedog for resource status tracking:

```go
// Shared stores for all tracking operations
taskStore := statestore.NewTaskStore()      // Task states
logStore := logstore.NewLogStore()          // Container logs
informerFactory := informer.NewFactory()    // Kubernetes informers

// Three tracker types
DynamicReadinessTracker  // Wait for ready state
DynamicPresenceTracker   // Wait for existence
DynamicAbsenceTracker    // Wait for deletion
```

### Progress Tables

Progress is displayed as terminal tables at configurable intervals:

```
┌────────────────────────────────────────┬─────────┬────────────────────────────┐
│ RESOURCE (→READY)                      │ STATE   │ INFO                       │
├────────────────────────────────────────┼─────────┼────────────────────────────┤
│ Deployment/my-app                      │ WAITING │ 1/3 ready                  │
│   └─ Pod/my-app-abc123                 │ READY   │ Running                    │
│   └─ Pod/my-app-def456                 │ WAITING │ ContainerCreating          │
├────────────────────────────────────────┼─────────┼────────────────────────────┤
│ Service/my-app                         │ READY   │ ClusterIP assigned         │
└────────────────────────────────────────┴─────────┴────────────────────────────┘
```

**Color coding:**
- Green: READY, PRESENT, ABSENT (success)
- Yellow: WAITING (in progress)
- Red: FAILED (error)

### Tracking Timeouts

```go
type TrackingOptions struct {
    LegacyHelmCompatibleTracking bool          // Track only hook Jobs (legacy compatibility)
    NoFinalTracking              bool          // Skip tracking ops that don't follow any apply/delete
    NoPodLogs                    bool          // Disable log collection
    NoProgressTablePrint         bool          // Disable progress tables
    ProgressTablePrintInterval   time.Duration // Default: 5s
    TrackCreationTimeout         time.Duration // For presence tracking
    TrackDeletionTimeout         time.Duration // For absence tracking
    TrackReadinessTimeout        time.Duration // For readiness tracking
}
```

**Log filtering options (per-resource annotations):**
- `werf.io/show-logs-only-for-containers`, `werf.io/show-logs-only-for-number-of-replicas`
- `werf.io/skip-logs`, `werf.io/skip-logs-for-containers`
- `werf.io/log-regex`, `werf.io/log-regex-skip`
- `werf.io/log-regex-for-<container>`, `werf.io/log-regex-skip-for-<container>`

## Failure Handling and Rollback

### Operation Status Flow

```
                    ┌─────────────┐
                    │   Pending   │
                    └──────┬──────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
       ┌─────────────┐          ┌─────────────┐
       │  Completed  │          │   Failed    │
       └─────────────┘          └──────┬──────┘
                                       │
                                       ▼
                              Context Canceled
                              (stops scheduling)
```

### Failure Plan

When deployment fails, a failure plan executes:

```go
func BuildFailurePlan(failedPlan *Plan, installableInfos []*InstallableResourceInfo, releaseInfos []*ReleaseInfo, opts BuildFailurePlanOptions) (*Plan, error) {
    // 1. Mark new release as StatusFailed
    // 2. Delete resources whose readiness tracking failed
    //    (only if delete-policy includes "failed")
    // 3. Track absence of deleted resources
}
```

**Failure cleanup requirements:**
- Resource has delete-policy `failed` (`werf.io/delete-policy: failed`, or `helm.sh/hook-delete-policy: hook-failed` for hooks)
- Resource doesn't have `helm.sh/resource-policy: keep`
- Resource's TrackReadiness operation failed
- Resource was actually installed (not just planned)

### Auto-Rollback

If enabled, failed deployments trigger automatic rollback:

```go
// In ReleaseInstall
if opts.AutoRollback && prevDeployedRelease != nil && executePlanErr != nil {
    // 1. Run failure plan (cleanup, mark failed)
    // 2. Build rollback release from prevDeployedRelease
    // 3. Build and execute rollback plan
    // 4. If rollback fails, run failure plan again
}
```

**Complete failure flow:**

```
Deployment Starts
       │
       ▼
  Execute Plan ──────────────────────────────┐
       │                                      │
       │ (operation fails)                    │
       ▼                                      │
  Cancel Context                              │
       │                                      │
       ▼                                      │
  Run Failure Plan                            │
       ├── Mark release as Failed             │
       └── Delete failed resources            │
       │                                      │
       ▼                                      │
  AutoRollback enabled? ──── No ─────────────►├── Return Error
       │                                      │
      Yes                                     │
       │                                      │
       ▼                                      │
  Run Rollback Plan                           │
       │                                      │
       ├── Success ──────────────────────────►├── Return Error
       │                                      │   (deployment failed,
       │ (rollback fails)                     │    rollback succeeded)
       ▼                                      │
  Run Failure Plan (again)                    │
       │                                      │
       ▼                                      │
  Return Error ◄──────────────────────────────┘
```

## Release Comparison

Nelm compares releases using FNV32a hashing to detect changes:

```go
func IsReleaseUpToDate(oldRel, newRel *helmrelease.Release) (bool, error) {
    // 1. Check old release is StatusDeployed
    // 2. Compare release notes
    // 3. Deep-compare Config (values)
    // 4. Hash and compare hook manifests
    // 5. Hash and compare regular manifests
}
```

**Before hashing, resources are "cleaned" to remove non-deterministic fields:**

| Removed Field | Reason |
|---------------|--------|
| `metadata.managedFields` | Changes with each SSA operation |
| `metadata.resourceVersion`, `metadata.uid`, `metadata.generation`, `metadata.creationTimestamp`, `metadata.selfLink`, `metadata.finalizers` | Runtime-assigned |
| `status` | Runtime state, not desired state |
| `meta.helm.sh/release-name`, `meta.helm.sh/release-namespace`, `app.kubernetes.io/managed-by` | Release ownership metadata |
| `ci.werf.io/*`, `project.werf.io/*`, `werf.io/version`, `werf.io/release-channel` | Runtime CI/CD metadata |

**"No changes needed" requires both:**
1. `IsReleaseUpToDate()` returns true
2. Plan has no Resource or Track operations

## Resource Validation

Nelm validates resources against Kubernetes schemas (requires `FeatGateResourceValidation`):

**Validation phases:**

| Phase | Function | Checks |
|-------|----------|--------|
| Local | `ValidateLocal()` | Duplicate detection, schema validation |
| Remote | `ValidateRemote()` | Resource adoption eligibility |

**Schema validation uses kubeconform:**
- Downloads schemas from kubernetes-json-schema
- Supports CRD schemas from datreeio/CRDs-catalog
- Caches schemas in Helm cache dir (by default `~/.cache/helm/nelm/api-resource-json-schemas/`)
- Missing schemas ignored (graceful degradation)

**Skip validation filter:**

```bash
# Skip specific resources
--resource-validation-skip kind=CustomResource
--resource-validation-skip name=legacy-config,namespace=default
```

## Secrets Management

Nelm provides encrypted values files for sensitive data:

**Encryption:**
- Algorithm: AES-128 CBC
- Key: 32 hex characters (128 bits)
- IV: Random per encryption
- Format: Hex-encoded (IV size + IV + ciphertext)

**Key sources (priority order):**
1. `--secret-key` / `NELM_SECRET_KEY` (Nelm CLI) or `WERF_SECRET_KEY` (underlying secrets manager)
2. `.werf_secret_key` file in working directory
3. `~/.werf/global_secret_key` file

**File types:**

| Type | Location | Usage |
|------|----------|-------|
| Secret values | `secret-values.yaml` | YAML with encrypted field values |
| Secret files | `secret/` directory | Any encrypted file |

**Commands:**

```bash
nelm chart secret key create                  # Generate new key
nelm chart secret key rotate                  # Rotate old → new key
nelm chart secret file encrypt <file>         # Encrypt file
nelm chart secret file decrypt <file>         # Decrypt file
nelm chart secret values-file encrypt <file>  # Encrypt values YAML
nelm chart secret values-file decrypt <file>  # Decrypt values YAML
```

## Distributed Locking

Nelm prevents concurrent deployments using ConfigMap-based locks:

**Lock storage:**
- ConfigMap: `werf-synchronization` in release namespace
- Lock name: `release/<release-name>`
- Created automatically if not exists

**Lock lifecycle:**

```
1. Acquire lock (with retry, max 10 attempts)
       │
       ├── If held by another → wait with logging
       │
       ▼
2. Execute deployment
       │
       ├── If lease lost → fail immediately
       │
       ▼
3. Release lock (with retry)
```

**Implementation:** Uses `github.com/werf/lockgate` with Kubernetes backend.

## Feature Gates

Enable features via environment variables (`NELM_FEAT_<NAME>=true`):

| Gate | Env Var | Purpose |
|------|---------|---------|
| `remote-charts` | `NELM_FEAT_REMOTE_CHARTS` | Allow remote chart URLs, adds `--chart-version` |
| `native-release-list` | `NELM_FEAT_NATIVE_RELEASE_LIST` | Use native list implementation |
| `native-release-uninstall` | `NELM_FEAT_NATIVE_RELEASE_UNINSTALL` | Use new uninstall (not fully compatible) |
| `field-sensitive` | `NELM_FEAT_FIELD_SENSITIVE` | JSONPath-based sensitive field redaction |
| `clean-null-fields` | `NELM_FEAT_CLEAN_NULL_FIELDS` | Remove null fields for Helm compatibility |
| `more-detailed-exit-code-for-plan` | `NELM_FEAT_MORE_DETAILED_EXIT_CODE_FOR_PLAN` | Exit 3 when release install needed |
| `resource-validation` | `NELM_FEAT_RESOURCE_VALIDATION` | Validate against Kubernetes schemas |
| `periodic-stack-traces` | `NELM_FEAT_PERIODIC_STACK_TRACES` | Print stack traces (debugging) |
| `preview-v2` | `NELM_FEAT_PREVIEW_V2` | Enable all v2 features |

## Exit Codes

| Code | Meaning | When |
|------|---------|------|
| 0 | Success | Deployment completed, or plan shows no changes |
| 1 | Error | Any error occurred |
| 2 | Changes planned | `release plan install --exit-code` detects changes |
| 3 | Release install needed | Plan shows no resource changes but release must be created (requires `FeatGateMoreDetailedExitCodeForPlan`) |

**CI/CD usage:**

```bash
nelm release plan install --exit-code myrelease ./mychart
case $? in
    0) echo "No changes" ;;
    2) echo "Changes detected, deploying..." && nelm release install ... ;;
    3) echo "Release install needed" && nelm release install ... ;;
    *) echo "Error" && exit 1 ;;
esac
```

## Testing

Nelm uses Ginkgo v2 with Gomega matchers:

```bash
task test:unit                           # All tests
task test:unit paths="./internal/plan"   # Specific package
task test:ginkgo paths="./pkg" -- -v     # With ginkgo flags
```

**Test infrastructure:**
- `internal/kube/fake/` - Fake Kubernetes clients
- `internal/test/` - Test utilities and fixtures

**Test patterns:**
- Table-driven tests with `DescribeTable`
- Fake clients for Kubernetes operations
- In-memory release storage

## Key External Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/werf/3p-helm` | Helm fork (chart loading, templating, release storage) |
| `github.com/werf/kubedog` | Resource status tracking, log/event collection |
| `github.com/dominikbraun/graph` | DAG implementation |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/werf/logboek` | Structured logging |
| `github.com/jellydator/ttlcache/v3` | TTL caching |
| `github.com/werf/lockgate` | Distributed locking |
| `github.com/yannh/kubeconform` | Schema validation |
| `github.com/samber/lo` | Go utilities (map, filter, etc.) |
| `github.com/sourcegraph/conc` | Concurrency utilities |
| `k8s.io/client-go` | Kubernetes client |
| `k8s.io/apimachinery` | Kubernetes API types |

## Common Patterns

**MultiError** - Collect multiple errors:

```go
var errs util.MultiError
errs.Add(validateA())
errs.Add(validateB())
return errs.OrNilIfNoErrs()  // Returns nil if no errors
```

MultiError flattens nested MultiErrors and preserves wrapper context.

**Options structs** - All public functions accept options:

```go
func ReleaseInstall(ctx context.Context, name, ns string, opts ReleaseInstallOptions) error
```

Options embed common groups: `KubeConnectionOptions`, `ValuesOptions`, `TrackingOptions`, etc.

**Interface compile-time checks**:

```go
var _ KubeClienter = (*KubeClient)(nil)
```

**Error wrapping** - Describe action, not failure:

```go
return fmt.Errorf("rendering chart: %w", err)  // Good
return fmt.Errorf("failed to render chart: %w", err)  // Avoid
```

**Concurrent access** - Use transactional helpers:

```go
taskStore.RWTransaction(func(ts *statestore.TaskStore) {
    ts.AddReadinessTaskState(taskState)
})
```

**Resource identification**:

```go
meta.IDHuman()       // "Deployment/my-app" (for logs)
meta.IDWithVersion() // "ns:apps:v1:Deployment:my-app" (for maps)
```
