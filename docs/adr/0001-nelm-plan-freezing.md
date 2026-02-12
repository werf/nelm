# ADR-0001: Nelm plan freezing

## Status

**Implemented**

## Context

Currently, Nelm supports a two-step workflow for deploying Helm releases:

1. **Plan Phase**: Administrators run `nelm release plan install` to preview the changes that will be made during deployment. This command builds a plan by:
- Rendering the chart and its dependencies
- Building resource specifications
- Querying the Kubernetes cluster to determine current state (requires cluster access)
- Computing diffs and deciding what actions to take (create, update, delete, etc.)
- Building a Directed Acyclic Graph (DAG) of operations to be performed

2. **Deploy Phase**: After reviewing the planned changes, administrators run `nelm release install` to deploy changes to the infrastructure. This command repeats the entire planning process from scratch.

### Problem Statement

The current workflow has a critical flaw: **there is no artifact passed between the plan and deploy phases**. This creates a window of unpredictability where:

- **Configuration drift**: Chart files, values files, or dependencies may be modified between `plan` and `install` commands, causing the plan reviewed by an administrator to differ from what actually gets deployed
- **Inconsistent deployments**: The plan reviewed may differ from what gets deployed, leading to:
  - Unexpected resource changes
  - Security concerns when the applied changes differ from what was reviewed and approved

This is particularly problematic in:
- **CI/CD pipelines**: Where plan review and approval happens in one stage, and deployment happens later
- **Compliance scenarios**: Where changes must be reviewed and approved before execution
- **Audit requirements**: Where there must be a record of exactly what was planned vs. what was executed

### Current Architecture

According to `ARCHITECTURE.md`, both `ReleasePlanInstall()` and `ReleaseInstall()` follow a similar flow:

1. Initialize Kubernetes clients, registry client, logging
2. Create/lock release namespace
3. Load release history
4. Render chart and build ResourceSpecs
5. Transform and patch ResourceSpecs
6. Build Installable/DeletableResources
7. Validate resources locally
8. Build ResourceInfos (queries cluster, performs dry-runs, computes diffs)
9. Validate resources remotely
10. Build ReleaseInfos
11. **Build Plan** (DAG of operations)
12. Calculate planned changes and display summary

The key difference is:
- `ReleasePlanInstall()` stops after displaying the plan summary with calculated changes
- `ReleaseInstall()` builds the plan but does not display the calculated changes summary; it proceeds directly to execution

The `Plan` struct (from `internal/plan/plan.go`) is a wrapper over a graph containing `Operation` nodes. Each `Operation` contains:
- Type (create, update, delete, track, etc.)
- Category (meta, resource, track, release)
- Configuration (operation-specific data)
- Status (pending, completed, failed)

The plan is currently built fresh on each command execution and is not persisted.

## Decision

During R&D, we considered several approaches, including extensive plan comparison with the actual cluster state intended to protect against infrastructure drift. We found that accurately comparing the planned and current states is not feasible because:

* Helm charts are not idempotent in some cases (they may depend on domain name resolution, random values, or non-deterministic iteration over data structures), which leads to false positives, as every render may produce a different result.
* It is impossible to compare two diff results produced by dry-run SSA to determine which changes were planned/approved and which were not.
* Comparing the planned and current DAGs is not feasible, as the DAG structure changes significantly even with minimal infrastructure drift.
* Comparing the planned and current manifest states is not meaningful, as we assume that planned manifests were previously approved and can be applied regardless of the current manifest state.

Given these constraints, we decided to implement a **plan freezing mechanism** that exports a complete release install plan to a gzip-compressed JSON artifact during `nelm release plan install` and then **directly executes the saved plan** during `nelm release install --use-plan`. The saved plan artifact contains the full DAG of operations with all resource specifications and plan info objects, so that the exact plan that was reviewed is the plan that gets executed.

1. `nelm release plan install --save-plan=plan.gz`:
- Builds a regular install plan (as today).
- Exports the full plan DAG, operation configurations, calculated changes, installable resource infos, and release infos to a gzip-compressed JSON artifact.
- Fails if a file or directory already exists at the target path.
2. `nelm release install --use-plan=plan.gz`:
- Reads the plan artifact.
- Validates the artifact (lifetime, release version match, etc.).
- Executes the stored plan DAG directly.
3. `nelm release plan show plan.gz`:
- Reads the plan artifact.
- Displays the planned changes stored in the artifact for review.

### Implementation Approach

1. **Plan Serialization (gzip-compressed JSON)**:
- Add JSON serialization support for the install `Plan` DAG and relevant metadata.
- Store the artifact as **gzip-compressed JSON** on disk for better size and I/O efficiency while preserving JSON schema semantics.
- The artifact has a two-layer structure:
  - **Top-level metadata** (always visible, even when encrypted): `apiVersion`, `timestamp`, `release`, `deployType`, `options`, `encrypted`, `dataRaw`.
  - **Data payload** (stored in `dataRaw`, potentially encrypted): `dag`, `changes`, `installableResourceInfos`, `releaseInfos`.
- DAG operations store full operation configurations including `ResourceSpec` objects, and info objects are stored explicitly in the payload.

2. **CLI Changes**:
- `nelm release plan install`:
  - Add `--save-plan=PATH` flag to save the plan artifact.
  - Add optional `--secret-key` and `--secret-work-dir` flags for encrypting the artifact.
- `nelm release install --use-plan=plan.gz`:
  - Existing install command can execute a saved plan artifact.
  - Release name and namespace are read from the artifact metadata.
  - Add `--secret-key` and `--secret-work-dir` flags for decrypting the artifact.
  - Add `--plan-lifetime` to configure artifact lifetime validation.
  - Supports the same execution-related flags as `release install` (auto-rollback, annotations, labels, tracking, timeouts, etc.).
- `nelm release plan show plan.gz`:
  - Reads the plan artifact from a positional argument.
  - Add `--secret-key` and `--secret-work-dir` flags for decrypting the artifact.
  - Supports diff display flags (`--show-insignificant-diffs`, `--show-sensitive-diffs`, `--show-verbose-crd-diffs`, `--show-verbose-diffs`).

3. **Plan Validation**:
- Validate that the artifact's `apiVersion` is supported by the running Nelm version.
- Validate that required metadata (release name, release namespace) is present.
- Validate that the release version in the artifact matches the expected next revision in the cluster, ensuring the artifact targets the correct release state.
- Use `timestamp` to enforce a maximum artifact lifetime (2 hours). Artifacts older than this are rejected.
- Validate that the DAG can be successfully reconstructed from the artifact.

4. **Encryption**:
- Plan artifacts can be optionally encrypted using a secret key (`--secret-key` flag) with werf's secrets manager.
- When encrypted, the `encrypted` field is set to `true` and the data payload in `dataRaw` is encrypted. Top-level metadata (release name, namespace, timestamps) remains visible.
- The `--secret-work-dir` flag specifies the working directory for secret operations.

5. **Feature Gate**:
- The plan freezing feature is gated behind the `FeatGatePlanFreezing` feature gate.

6. **Protection from Re-running the Same Plan**:
- Rely on the existing Nelm/Helm release locking mechanism (lock on Helm release ID) to protect from concurrent or duplicated runs.
- The release version validation ensures that the artifact targets the correct next revision.

7. **Backward Compatibility**:
- The `release install` command is completely unchanged and continues to work as before if `--use-plan` is not used.
- Plan freezing is purely additive functionality available only when the feature gate is enabled.

### Data to Include in Plan Artifact

The gzip-compressed JSON artifact contains the full plan needed for both execution and review.

Top-level fields (always visible):

- **apiVersion**: Version of the plan artifact schema (e.g. `"v1"`).
- **timestamp**: UTC time when the plan was created.
- **release**:
  - `name`: Release name.
  - `namespace`: Release namespace.
  - `version`: Expected release revision number.
- **deployType**: The type of deployment (initial, install, upgrade).
- **options**: Options that will be used during plan execution.
- **encrypted**: Boolean indicating whether the data payload is encrypted.
- **dataRaw**: Serialized (and potentially encrypted) data payload.

Data payload fields (inside `dataRaw`):

- **dag**:
  - `operations`: List of operations, each containing:
    - Operation ID, type, version, category, iteration, status.
    - Full operation configuration (`config`) encoded as a generic envelope with `kind` and `data`.
    - Resource operations include complete `ResourceSpec` objects, enabling resource reconstruction without re-rendering the chart.
  - `edges`: List of DAG edges (`from`, `to` operation IDs) defining the execution order.
- **changes**: List of resource changes, each containing:
  - Change type (create, update, delete, recreate, blind apply).
  - Resource metadata.
  - Before/after unstructured Kubernetes objects (for computing diffs on display).
  - Extra operations and reason.
- **installableResourceInfos**: Full list of installable resource info objects required for plan execution.
- **releaseInfos**: Full list of release info objects required for plan execution.

The artifact is self-contained: it has enough information to both execute the plan and display planned changes for review.

## Implementation Notes

### Plan Format Considerations

- **Compressed JSON Format**: Store the plan artifact as gzip-compressed JSON. When encrypted, the top-level metadata is still readable JSON after decompression; only `dataRaw` is encrypted.
- **Schema Versioning**: Include `apiVersion` and evolve the schema in a backward-compatible way when possible.
- **Metadata**: Include timestamp, release information, deploy type, execution options, and other metadata for validation and debugging.

### CLI Design

```bash
# Generate and save plan artifact
nelm release plan install -n namespace -r release --save-plan=plan.gz

# Generate and save encrypted plan artifact
nelm release plan install -n namespace -r release --save-plan=plan.gz --secret-key=mykey

# Show planned changes from artifact
nelm release plan show plan.gz

# Show planned changes from encrypted artifact
nelm release plan show plan.gz --secret-key=mykey

# Execute plan artifact
nelm release install --use-plan plan.gz

# Execute encrypted plan artifact
nelm release install --use-plan=plan.gz --secret-key=mykey
```

### Error Handling

- Clear error messages when plan artifact format is incompatible (unsupported `apiVersion`)
- Error when artifact is older than 2 hours
- Error when release version in artifact does not match the expected next revision
- Error when artifact is encrypted but no secret key is provided
- Error when artifact is not a valid gzip-compressed JSON document
- Error when artifact data is empty or cannot be deserialized
- Error when DAG cannot be reconstructed from artifact operations
- Error when saving artifact to a path that already exists

## References

- Nelm Architecture Documentation: `ARCHITECTURE.md`
- Plan Implementation: `internal/plan/plan.go`
- Plan Artifact Implementation: `internal/plan/plan_artifact.go`
- Operation Implementation: `internal/plan/operation.go`
- Plan Execute Action: `pkg/action/release_plan_execute.go`
- Plan Show Action: `pkg/action/release_plan_show.go`
- Feature Gate: `pkg/featgate/feat.go`
