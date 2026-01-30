# Components

<!-- toc -->
- [1. ReleaseInstallAction](#1-releaseinstallaction)
- [2. ReleasePlanInstallAction](#2-releaseplaninstallaction)
- [3. Plan](#3-plan)
- [4. Operation](#4-operation)
- [5. Resource Types](#5-resource-types)
- [6. Resource Info Types](#6-resource-info-types)
- [7. KubeClient](#7-kubeclient)
- [8. ClientFactory](#8-clientfactory)
- [9. LockManager](#9-lockmanager)
- [10. TypeScript Charts](#10-typescript-charts)
- [11. Chart Secret Actions](#11-chart-secret-actions)
- [Component Relationships](#component-relationships)
- [File Locations Summary](#file-locations-summary)
<!-- tocstop -->

This document describes the major components in the Nelm codebase, their purposes, key methods, and file locations.

## 1. ReleaseInstallAction

**Purpose:** Orchestrates the complete chart deployment lifecycle to Kubernetes, including chart rendering, resource building, plan construction, execution, and optional auto-rollback on failure.

**Key Methods:**
- `ReleaseInstall` - Main entry point for chart installation
- `releaseInstall` - Core implementation that builds clients, renders charts, constructs and executes deployment plans

**File:** `pkg/action/release_install.go`

**Notes:**
- Embeds multiple option structs for connection, validation, and tracking configuration
- Supports multiple chart sources: local directories, archives, OCI registries, and Helm repositories
- Auto-rollback only triggers when `AutoRollback=true` and a previous successful deployment exists

---

## 2. ReleasePlanInstallAction

**Purpose:** Computes and prints a “plan” (diff) for a release install without mutating the cluster resources, primarily by using server-side dry-run apply and comparing desired vs actual state.

**Key Methods:**
- `ReleasePlanInstall` - Main entry point for planning and diff output
- `releasePlanInstall` - Core implementation that renders charts, builds resources/infos, and formats planned changes

**File:** `pkg/action/release_plan_install.go`

**Notes:**
- Does not acquire a release lock (read-only from the cluster’s perspective, but still queries release storage and can do discovery/dry-run calls)
- With `--exit-code`, exit code behavior depends on feature gates (see `cmd/nelm/release_plan_install.go`)

---

## 3. Plan

**Purpose:** Represents a DAG (Directed Acyclic Graph) of operations for deploying, updating, or deleting Kubernetes resources with proper dependency ordering.

**Key Methods:**
- `NewPlan` - Creates a new empty plan with acyclic, directed graph properties
- `BuildPlan` - Constructs a deployment plan from resource and release information
- `ExecutePlan` - Executes the plan using a worker pool with parallel operations
- `Optimize` - Applies transitive reduction and squashes useless meta operations
- `ToDOT` - Exports the plan as Graphviz DOT format for visualization

**Files:**
- `internal/plan/plan.go`
- `internal/plan/plan_build.go`
- `internal/plan/plan_execute.go`

**How the DAG Works:**
1. **Stages as Scaffolding**: The plan starts with ordered stages (`init`, `pre-pre-uninstall`, `pre-pre-install`, `pre-install`, `pre-uninstall`, `install`, `uninstall`, `post-install`, `post-uninstall`, `post-post-install`, `post-post-uninstall`, `final`). Each stage has start/end noop operations.
2. **Operations within Stages**: Resource operations (Create, Update, Delete, Track) are placed within appropriate stages.
3. **Weighted Sub-stages**: Resources with weights are grouped into sub-stages for explicit ordering.
4. **Dependencies**: Internal dependencies are expressed as edges between operations.
5. **Parallel Execution**: Operations without pending dependencies can execute concurrently.

---

## 4. Operation

**Purpose:** Represents a single action in the deployment plan, such as creating a resource, tracking its readiness, or updating a release.

**Key Methods:**
- `ID` - Returns unique identifier: `{type}/{version}/{iteration}/{config_id}`
- `IDHuman` - Returns human-readable identifier for logging

**Files:**
- `internal/plan/operation.go`
- `internal/plan/operation_config.go`

**Operation Types:** `apply`, `create`, `create-release`, `delete`, `delete-release`, `noop`, `recreate`, `track-absence`, `track-presence`, `track-readiness`, `update`, `update-release`

**Operation Categories:** `meta` (grouping/no-ops), `resource` (K8s mutations), `track` (state tracking), `release` (Helm release mutations)

**Operation Statuses:** `pending`, `completed`, `failed`

---

## 5. Resource Types

**Purpose:** Represent Kubernetes resources at different abstraction levels during the deployment pipeline.

**Key Types:**
- `InstallableResource` - Higher-level representation of a resource to be created/updated, containing resource spec, deployment behavior settings, tracking configuration, and dependency information
- `DeletableResource` - Higher-level representation of a resource to be deleted, containing resource metadata, ownership, retention settings, and delete dependencies

**Key Methods:**
- `NewInstallableResource` - Constructs an InstallableResource by validating annotations and extracting deployment behavior
- `NewDeletableResource` - Constructs a DeletableResource with graceful handling of invalid annotations
- `BuildResources` - Builds both installable and deletable resources from previous and new release specs

**File:** `internal/resource/resource.go`

---

## 6. Resource Info Types

**Purpose:** Store all information needed to determine what action to take for each resource during plan building.

**Key Types:**
- `InstallableResourceInfo` - Contains local resource, cluster state, dry-apply result, install type, and stage information
- `DeletableResourceInfo` - Contains local resource, cluster state, delete flags, and stage information

**Key Methods:**
- `BuildResourceInfos` - Builds resource infos by getting cluster state, performing dry-run applies, and determining install type

**File:** `internal/plan/resource_info.go`

**Resource Install Types:** `none` (no changes), `create` (resource doesn't exist), `recreate` (delete then create), `update` (has changes), `apply` (blindly apply)

---

## 7. KubeClient

**Purpose:** High-level Kubernetes client with caching and resource locking, abstracting dynamic client operations.

**Key Methods:**
- `Get` - Gets a resource, using cache if available
- `Create` - Creates a resource using server-side apply
- `Apply` - Server-side applies a resource, supports dry-run mode
- `MergePatch` - Applies a merge patch to a resource
- `Delete` - Deletes a resource with configurable propagation policy

**File:** `internal/kube/client_kube.go`

**Features:**
- TTL cache for GET results to reduce API calls
- Per-resource mutex to prevent concurrent modifications
- Automatic GVK/GVR mapping

---

## 8. ClientFactory

**Purpose:** Central factory for creating and providing all Kubernetes client types needed throughout the application.

**Key Methods:**
- `NewClientFactory` - Creates all clients from a KubeConfig
- `KubeClient` - Returns high-level operations client with caching
- `Static` - Returns typed client for core resources
- `Dynamic` - Returns untyped client for any resource
- `Discovery` - Returns API resource discovery client
- `Mapper` - Returns GVK to GVR mapper
- `LegacyClientGetter` - Returns Helm-compatible client getter

**File:** `internal/kube/factory.go`

---

## 9. LockManager

**Purpose:** Prevents concurrent operations on the same release (e.g., two installs racing) by acquiring and holding a lock in the target namespace.

**Key Methods:**
- `LockRelease` - Acquire the release lock (name is `release/<releaseName>`)
- `Unlock` - Release the lock

**File:** `internal/lock/lock_manager.go`

**Notes:**
- Implemented via `werf/lockgate` and currently backed by Kubernetes ConfigMaps.

---

## 10. TypeScript Charts

**Purpose:** Enables rendering charts from a `ts/` directory (bundled with esbuild and executed via a JavaScript runtime).

**Key Files:**
- `internal/ts/render.go` - Entrypoint for TS rendering
- `internal/ts/bundle.go` / `internal/ts/esbuild.go` - Bundling pipeline
- `internal/ts/runtime.go` - Executes bundles using Goja

**Notes:**
- Guarded by feature gate `NELM_FEAT_TYPESCRIPT` (see `pkg/featgate/feat.go`).

---

## 11. Chart Secret Actions

**Purpose:** Provides “chart secret” subcommands for encrypting/decrypting/editing values files and arbitrary files, plus key creation/rotation.

**Key Files (actions):**
- `pkg/action/secret_values_file_encrypt.go`, `pkg/action/secret_values_file_decrypt.go`, `pkg/action/secret_values_file_edit.go`
- `pkg/action/secret_file_encrypt.go`, `pkg/action/secret_file_decrypt.go`, `pkg/action/secret_file_edit.go`
- `pkg/action/secret_key_create.go`, `pkg/action/secret_key_rotate.go`

**Key Files (implementation):**
- `pkg/legacy/secret/*.go` - Core encrypt/decrypt/edit/rotate logic

---

## Component Relationships

```
ReleaseInstallAction
    |
    +-- lock.LockManager.LockRelease() --> ensures exclusive release operation
    |
    +-- chart.RenderChart() --> ResourceSpecs
    |
    +-- resource.BuildResources() --> InstallableResource, DeletableResource
    |
    +-- plan.BuildResourceInfos() --> InstallableResourceInfo, DeletableResourceInfo
    |
    +-- plan.BuildPlan() --> Plan (DAG of Operations)
    |
    +-- plan.ExecutePlan()
            |
            +-- execOp() for each Operation
                    |
                    +-- KubeClient.Create/Apply/Delete/Get
                    +-- dyntracker.Track*
                    +-- release.History.CreateRelease/UpdateRelease
```

---

## File Locations Summary

| Component | Primary Files |
|-----------|---------------|
| ReleaseInstallAction | `pkg/action/release_install.go` |
| ReleasePlanInstallAction | `pkg/action/release_plan_install.go` |
| Plan | `internal/plan/plan.go` |
| Plan Building | `internal/plan/plan_build.go` |
| Plan Execution | `internal/plan/plan_execute.go` |
| Operation | `internal/plan/operation.go` |
| Operation Configs | `internal/plan/operation_config.go` |
| Resource Types | `internal/resource/resource.go` |
| Resource Spec | `internal/resource/spec/resource_spec.go` |
| Resource Info | `internal/plan/resource_info.go` |
| KubeClient | `internal/kube/client_kube.go` |
| ClientFactory | `internal/kube/factory.go` |
| LockManager | `internal/lock/lock_manager.go` |
| TypeScript Charts | `internal/ts/render.go` |
| Chart Secret Actions | `pkg/action/secret_values_file_encrypt.go` |
