# Nelm Workflows

This document describes the key workflows implemented in Nelm.

<!-- toc -->

- [Release Install Workflow](#release-install-workflow)
- [Release Plan Install Workflow](#release-plan-install-workflow)
- [Release Rollback Workflow](#release-rollback-workflow)
- [Common Patterns Across Workflows](#common-patterns-across-workflows)

<!-- tocstop -->

## Release Install Workflow

The release install workflow (`nelm release install`) deploys a Helm chart to a Kubernetes cluster, tracking resources to readiness, and optionally rolling back on failure.

### Overview

The workflow orchestrates the complete deployment lifecycle from chart rendering through resource application and readiness tracking. It begins by setting up Kubernetes clients and acquiring an exclusive lock on the release to prevent concurrent operations. The release history is queried to determine whether this is an initial install, upgrade, or rollback.

### Key Phases

**Chart Rendering**: The chart is downloaded (from local path, OCI registry, or Helm repo), values are merged from multiple sources, and templates are rendered using the Helm engine. The result is a set of raw resource specifications.

**Resource Building**: Raw specs go through a transformation pipeline that flattens List kinds, removes invalid metadata, adds release annotations/labels, and converts Secret stringData to data. Previous and new release specs are compared to determine which resources need to be created, updated, or deleted.

**Plan Construction**: A directed acyclic graph (DAG) of operations is built based on resource dependencies. This includes weight-based ordering, hook execution phases, and delete-before-recreate patterns. The plan respects `werf.io/deploy-dependency-*` annotations for custom ordering.

**Execution**: Operations execute in dependency order with configurable parallelism. Each resource operation (create/update/delete) is applied to the cluster. For resources requiring tracking, kubedog monitors readiness based on resource type (Deployment rollout, Job completion, etc.).

**Tracking**: The progress printer displays real-time status of tracked resources. Resources are watched until they reach a ready state, fail, or timeout. Failed resources trigger the failure plan and optional auto-rollback.

### Key Files

| File | Description |
|------|-------------|
| `pkg/action/release_install.go` | Main entry point and orchestration |
| `internal/chart/chart_render.go` | Chart loading and template rendering |
| `internal/plan/plan_build.go` | DAG construction from resource infos |
| `internal/plan/plan_execute.go` | Parallel plan execution |
| `internal/plan/resource_info.go` | Resource state comparison |
| `internal/release/history.go` | Release CRUD operations |
| `internal/kube/client_kube.go` | Kubernetes API operations |

### Error Handling

Chart download and template rendering errors are wrapped with context and returned immediately. Validation errors (both local schema validation and remote adoption checks) stop the process before any cluster changes. During execution, errors are collected in a MultiError; on failure, a failure plan runs to mark the release as failed in storage. If `AutoRollback=true` and a previous deployed release exists, an automatic rollback is triggered. The release lock is always released in a defer block. Context timeout cancels all operations.

---

## Release Plan Install Workflow

The release plan install workflow (`nelm release plan install`) performs a dry-run of installation, showing what changes would be made without modifying the cluster.

### Overview

This workflow mirrors the install workflow through the plan construction phase, but instead of executing changes, it calculates and displays diffs. It uses Kubernetes server-side dry-run to compute the exact changes that would result from applying resources, providing accurate diffs even for resources with defaulted fields or admission webhook modifications.

### Key Differences from Install

- No release lock is acquired (read-only operation)
- Server-side dry-run apply is used to compute accurate diffs
- No plan execution occurs
- Unified diffs are generated and displayed with syntax highlighting
- Sensitive data (Secrets) is redacted in diff output
- With `--exit-code`, exit codes indicate whether changes are planned (details depend on enabled feature gates):
  - Default: `2` for any planned changes
  - With `NELM_FEAT_MORE_DETAILED_EXIT_CODE_FOR_PLAN=true` (or `NELM_FEAT_PREVIEW_V2=true`): `2` for resource changes, `3` for “no resource changes planned, but release still should be installed”

The `--error-if-changes-planned` flag enables CI/CD use cases where the command should fail if any changes would be made.

### Key Files

| File | Description |
|------|-------------|
| `pkg/action/release_plan_install.go` | Main entry point |
| `internal/plan/planned_changes.go` | Diff calculation and formatting |
| `internal/plan/resource_info.go` | Dry-run apply logic |

---

## Release Rollback Workflow

The release rollback workflow (`nelm release rollback`) reverts to a previous release revision. It queries the release history to find the target revision (either explicitly specified or the previous deployed release), extracts the resource specifications from that release, and creates a new release with an incremented revision number and `DeployTypeRollback` status. The workflow then builds and executes a plan to restore resources to their previous state, using the same plan execution and tracking infrastructure as the install workflow. If the rollback itself fails, a failure plan marks the release as failed, but no recursive auto-rollback occurs.

---

## Common Patterns Across Workflows

### Resource Transformation Pipeline

All workflows that process resources follow the same transformation pipeline:

```
Chart Templates
    |
    v
RenderChart() -> ResourceSpecs (raw)
    |
    v
BuildTransformedResourceSpecs() -> ResourceSpecs (transformed)
    - ResourceListsTransformer: Flatten List kinds
    - DropInvalidAnnotationsAndLabelsTransformer: Remove invalid metadata
    |
    v
BuildReleasableResourceSpecs() -> ResourceSpecs (patched)
    - ExtraMetadataPatcher: Add extra annotations/labels
    - SecretStringDataPatcher: Convert stringData to data
    - LegacyOnlyTrackJobsPatcher: (optional) Helm-compatible tracking
    |
    v
NewRelease() -> Release object (stored in cluster)
    |
    v
ReleaseToResourceSpecs() -> ResourceSpecs (final)
```

### Plan Execution Model

The plan execution model is shared across install, rollback, and uninstall:

1. **BuildResourceInfos**: Compare desired vs actual state
2. **BuildPlan**: Create DAG of operations
3. **ExecutePlan**: Execute operations in dependency order
4. **BuildFailurePlan**: Handle failures gracefully

### Error Classification

Errors are classified into:
- **Critical errors**: Stop execution, require user intervention
- **Non-critical errors**: Logged but don't stop execution
- **MultiError**: Aggregates multiple errors for reporting
