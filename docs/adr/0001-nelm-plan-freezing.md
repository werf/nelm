# ADR-0001: Nelm plan freezing

## Status

**Proposed**

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

We will implement a **plan freezing and verification mechanism** that exports a release install plan to a JSON artifact during `nelm release plan install` and then **rebuilds a fresh plan** during `nelm release install --plan-file=...`, comparing the two plans before deploying.

Nelm will not "blindly" apply a precomputed plan. Instead, the previously saved plan is used as a **contracts/guardrail artifact**:

1. `nelm release plan install --out=plan.json`:
  - Builds a regular install plan (as today).
  - Exports it to a JSON artifact
2. `nelm release install --plan-file=plan.json -n <ns> -r <release>`:
  - Rebuilds a new plan from the current configuration and cluster state.
  - Compares the new plan's DAG against the DAG from `plan.json`.
  - If there are **meaningful differences**, the deployment is blocked (or requires explicit override in the future).

This keeps the plan-as-reviewed semantics, while still rebuilding a fresh plan on install.

### Implementation Approach

1. **Plan Serialization (JSON)**:
  - Add JSON serialization support for the install `Plan` DAG and relevant metadata.
  - Use a **JSON** artifact for the exported plan to keep it easy to inspect with standard tools (e.g. `jq`).
  - Include metadata in the serialized plan:
    - `schemeVersion` – version of the plan JSON schema.
    - `timestamp` – time when the plan was created (used for detecting stale plans).
    - Release information (name, namespace, Helm release ID).
    - DAG of operations.
    - Calculated changes (`changes`) sufficient to render diffs for reviewers.
  - Plan artifact encryption is not required at the MVP stage.

2. **CLI Changes**:
  - `nelm release plan install`:
    - Add `--out=PATH` flag to save the plan JSON artifact.
  - `nelm release install`:
    - Add `--plan-file=PATH` flag to load the previously saved plan.
    - **Still require** `--release` and `--namespace` flags.
    - Rebuild a **new** plan using the current chart/values and cluster state.
    - Compare the new plan against the saved one before executing:
      - Compare DAG operations by:
        - Set of operations.
        - Operation types (create, update, recreate, delete, etc.).
        - Operation counts per type.
      - Compare `unstruct` (rendered manifests) attached to operations.
      - The set and types of operations and the manifests they operate on **must not change**.
    - If the DAG or manifests differ in a meaningful way, treat the plan as invalid (MVP behaviour: fail fast with a clear error).
  - We **do not** introduce a separate `nelm release plan show` command in MVP. Users can inspect the JSON plan with tools like `jq` if needed.

3. **Plan Validation**:
  - Validate that the JSON plan's `schemeVersion` is supported by the running Nelm version.
  - Validate that required metadata (release/namespace, DAG, changes) is present.
  - Use `timestamp` to detect obviously stale plans (e.g. older than some recommended threshold) and raise an error.
  - Ensure the rebuilt plan is "compatible" with the saved one by DAG and manifest comparison as described above.

4. **Protection from Re-running the Same Plan**:
  - Rely on the existing Nelm/Helm release locking mechanism (lock on Helm release ID) to protect from concurrent or duplicated runs.

5. **Backward Compatibility**:
  - The `--plan-file` flag is optional; if not provided, `release install` works as today (builds a plan and deploys).
  - Existing workflows that do not use plan artifacts remain unchanged.

### Data to Include in Plan JSON Artifact

The JSON artifact must contain enough information to:
1. Compare the saved plan DAG with a newly built plan.
2. Provide human-reviewable information about planned changes (diff lines).

Minimum fields:

- **schemeVersion**: Version of the plan JSON schema.
- **timestamp**: Time when the plan was created.
- **release**:
  - Name.
  - Namespace.
  - Helm release ID (for correlation with locks/history).
- **dag**:
  - List of operations with:
    - Operation ID.
    - Operation type (create, update, recreate, delete, track, etc.).
    - Operation category (resource, release, track, meta).
    - References to resources/releases they operate on.
    - Associated `unstruct` manifest representation (where applicable).
- **changes**:
  - Structured representation of calculated diffs sufficient to render:
    - Resource-level changes (create/update/delete/no-op).
    - Diffs with appropriate context lines (as in `release plan install` output).

The artifact is **not** intended to be directly executable (we always rebuild a fresh plan), but must be stable enough for comparison and review.

## Consequences

### Positive

1. **Consistency Guardrails**: The saved plan acts as a contract that the rebuilt plan must match
  - Detects configuration drift between plan and deploy phases by comparing DAGs and manifests.
  - Provides confidence that no unexpected operations (extra creates/deletes, etc.) will be introduced silently.
2. **Auditability**: Provides a permanent record of what was planned and executed
3. **Collaboration**: Allows plans to be shared, reviewed, and executed by different team members

### Negative

1. **Complexity**: Adds serialization/deserialization logic, JSON schema, and DAG comparison logic.
2. **Storage**: Requires storing plan JSON artifacts (though they can be temporary CI/CD artifacts).
3. **Version Compatibility**: Need to handle plan JSON schema versioning and migration.
4. **Plan File Security**: Plan files contain sensitive information (e.g., rendered manifests, diffs) and must be protected.
5. **Approval Workflow Challenges**: Implementing interactive approval requires careful design
  - Must ensure only one plan is outstanding at a time, or properly connect approval to specific plan
  - When a plan is applied, other plans against the same state become invalid and must be recomputed

### Risks and Mitigations

1. **Plan Format Breaking Changes (JSON schema)**:
  - **Risk**: Changes to Operation or Plan structure could break compatibility
  - **Mitigation**: Use versioned plan format and provide migration tools or clear error messages

2. **Stale Plans**:
  - **Risk**: Plans executed long after creation may target outdated cluster state (infrastructure drift)
  - **Mitigation**: Include timestamp in plan and optionally warn if plan is too old; allow force flag to proceed anyway. Note: We cannot prevent infrastructure drift, only configuration drift.

3. **Plan File Exposure in CI/CD**:
  - **Risk**: Plan files stored as artifacts in CI/CD systems may be accessible to users with artifact access, exposing sensitive data
  - **Mitigation**: Encrypt plan artifact; restrict artifact access;

4. **Environment Incompatibility**:
  - **Risk**: Attempting to apply a plan created for one environment (e.g., staging) to another (e.g., production)
  - **Mitigation**: Include environment/release metadata in plan; validate that plan matches target environment before applying; provide clear error messages

## Implementation Notes

### Plan Format Considerations

- **JSON Format**: Use plain JSON for the plan artifact so it can be easily inspected with standard tools (e.g. `jq`).
- **Schema Versioning**: Include `schemeVersion` and evolve the schema in a backward-compatible way when possible.
- **Metadata**: Include timestamp, release information and other metadata for validation and debugging.

### CLI Design

```bash
# Generate and save plan JSON
nelm release plan install -n namespace -r release --out=plan.json

# Apply with plan validation (namespace and release still required)
nelm release install -n namespace -r release --plan-file=plan.json

# Validate plan without applying
nelm release plan validate plan.json
```

### Error Handling

- Clear error messages when plan format is incompatible
- Warnings when plan is stale (older than X hours/days)
- Error when attempting to reuse plan
- Clear error messages when required data is missing from plan
- Option to force apply even with warnings (with appropriate flags)

## References

- Nelm Architecture Documentation: `ARCHITECTURE.md`
- Plan Implementation: `internal/plan/plan.go`
- Operation Implementation: `internal/plan/operation.go`
