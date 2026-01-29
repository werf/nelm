# Key Interfaces

<!-- toc -->
- [Overview](#overview)
- [Logging](#logging)
- [Kubernetes Client Layer](#kubernetes-client-layer)
- [Release Management](#release-management)
- [Resource Processing](#resource-processing)
- [Deployment Planning](#deployment-planning)
<!-- /toc -->

## Overview

Nelm defines several key interfaces that enable abstraction, testing, and modularity.

## Logging

### Logger

**Purpose:** Unified logging abstraction with multiple log levels, structured output, and grouped/block formatting.

**Key Methods:** `Trace`, `Debug`, `Info`, `Warn`, `Error` (each with `Push`/`Pop` variants), `InfoBlock`, `InfoBlockErr`, `SetLevel`, `Level`

**File:** `pkg/log/logger.go`

**Implementation:** `LogboekLogger` (`pkg/log/logger_logboek.go`)

**Log Levels:** `SilentLevel`, `ErrorLevel`, `WarningLevel`, `InfoLevel`, `DebugLevel`, `TraceLevel`

## Kubernetes Client Layer

### ClientFactorier

**Purpose:** Factory for constructing and providing access to all Kubernetes client types (static, dynamic, discovery).

**Key Methods:** `KubeClient`, `Static`, `Dynamic`, `Discovery`, `Mapper`, `LegacyClientGetter`, `KubeConfig`

**File:** `internal/kube/factory.go`

**Implementation:** `ClientFactory` (same file)

### KubeClienter

**Purpose:** High-level Kubernetes client with CRUD operations, built-in caching, and per-resource locking.

**Key Methods:** `Get`, `Create`, `Apply`, `MergePatch`, `Delete`

**File:** `internal/kube/client_kube.go`

**Implementation:** `KubeClient` (same file)

**Note:** Prefer this over raw static/dynamic clients; uses TTL cache and is thread-safe.

## Release Management

### ReleaseStorager

**Purpose:** Interface for Helm release storage operations (CRUD for releases).

**Key Methods:** `Create`, `Update`, `Delete`, `Query`

**File:** `internal/release/release_storage.go`

**Implementation:** `*helmstorage.Storage` (from `github.com/werf/3p-helm/pkg/storage`)

**Storage Drivers:** `""` (default), `secret`/`secrets`, `configmap`/`configmaps`, `memory`, `sql`

### Historier

**Purpose:** Wraps Helm release history management with access to revisions and state.

**Key Methods:** `Releases`, `FindAllDeployed`, `FindRevision`, `CreateRelease`, `UpdateRelease`, `DeleteRelease`

**File:** `internal/release/history.go`

**Implementation:** `History` (same file)

## Resource Processing

### ResourceTransformer

**Purpose:** Transforms resources during chart processing; can convert one resource into zero or more.

**Key Methods:** `Match`, `Transform`, `Type`

**File:** `internal/resource/spec/transform.go`

**Implementations:**
- `DropInvalidAnnotationsAndLabelsTransformer` - Removes non-string annotation/label values
- `ResourceListsTransformer` - Expands List resources into individual resources

### ResourcePatcher

**Purpose:** Applies patches to resources during processing; always produces exactly one output.

**Key Methods:** `Match`, `Patch`, `Type`

**File:** `internal/resource/spec/patch.go`

**Implementations:**
- `ExtraMetadataPatcher` - Adds user-specified annotations/labels
- `ReleaseMetadataPatcher` - Adds Helm release metadata
- `LegacyOnlyTrackJobsPatcher` - Sets tracking annotations for Jobs/Pods
- `SecretStringDataPatcher` - Converts `.stringData` to base64 `.data`

## Deployment Planning

### OperationConfig

**Purpose:** Marker interface for operation-specific configuration in the deployment plan.

**Key Methods:** `ID`, `IDHuman`

**File:** `internal/plan/operation_config.go`

**Implementations:**
| Config Type | Purpose |
|-------------|---------|
| `OperationConfigNoop` | No-operation placeholder |
| `OperationConfigCreate` | Create new resource |
| `OperationConfigRecreate` | Delete then create resource |
| `OperationConfigUpdate` | Update existing resource |
| `OperationConfigApply` | Server-side apply resource |
| `OperationConfigDelete` | Delete resource |
| `OperationConfigTrackReadiness` | Track resource until ready |
| `OperationConfigTrackPresence` | Track until resource exists |
| `OperationConfigTrackAbsence` | Track until resource is deleted |
| `OperationConfigCreateRelease` | Create Helm release record |
| `OperationConfigUpdateRelease` | Update Helm release record |
| `OperationConfigDeleteRelease` | Delete Helm release record |
