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
- **Multi-person workflows**: Where one person plans and another person applies
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

We will implement a **plan freezing mechanism** that allows exporting a plan in binary format during `nelm release plan install` and applying that frozen plan during `nelm release install`, similar to Terraform's `terraform plan -out=plan.tfplan` and `terraform apply plan.tfplan` workflow.

### Implementation Approach

1. **Plan Serialization**:
   - Add serialization support to the `Plan` struct and all its dependencies (`Operation`, `OperationConfig` implementations, etc.)
   - Use a binary format (e.g., Protocol Buffers, MessagePack, or Go's `gob` encoding) for efficient storage and versioning
   - Include metadata in the serialized plan:
     - Plan version for compatibility checking
     - Timestamp of when the plan was created
     - Chart information (name, version, repository)
     - Release information (name, namespace)
     - All necessary context to execute the plan without re-rendering

2. **CLI Changes**:
   - Add `--out` or `--plan-file` flag to `nelm release plan install` to specify where to save the plan
   - Add `--plan-file` flag to `nelm release install` to load and execute a frozen plan instead of building a new one
   - Add `nelm release plan show <plan-file>` command to display plan contents in human-readable format
   - When `--plan-file` is provided to `release install`:
     - Skip the plan building phase and proceed directly to execution
     - Do not require `--namespace` and `--release` flags, as this information is already present in the plan file
     - Display the calculated changes summary (similar to what `release plan install` shows) before execution
     - Implement a one-time-use mechanism to prevent reusing the same plan file multiple times
   - For `nelm release plan show`:
     - By default, hide sensitive data (Secrets, resources with `werf.io/sensitive="true"` annotation, or fields matching `werf.io/sensitive-paths`) and show as `<hidden N bytes, hash XXX>`
     - Add `--show-sensitive` flag to display sensitive data in plain text
     - This matches the sensitive data handling behavior of `nelm release plan install --show-sensitive-diffs`

3. **Plan Encryption**:
   - Encrypt plan files using the same encryption mechanism as Nelm's secret values files
   - Use the `NELM_SECRET_KEY` (or `WERF_SECRET_KEY`) environment variable or `--secret-key` flag
   - Plans should be encrypted by default when saved, and decrypted when loaded
   - This ensures that sensitive information in plans (resource specs, values, etc.) is protected at rest

4. **Plan Validation**:
   - Validate that the frozen plan is compatible with the current Nelm version
   - Ensure all required data for execution is present in the frozen plan
   - Ensure that plan is not outdated
   - Note: We do not prevent infrastructure drift (cluster state changes) - the plan only ensures configuration consistency

5. **One-Time-Use Protection**:
   - Include a unique execution token in each plan file
   - After successful execution, mark the plan as "used" (either by modifying the plan file or maintaining a registry)
   - Prevent re-execution of the same plan file to avoid accidental failures
   - Provide a `--force` flag to allow re-execution if explicitly needed (e.g., for retry scenarios)

6. **Backward Compatibility**:
   - The `--plan-file` flag should be optional; if not provided, `release install` continues to work as before (building a fresh plan)
   - Existing workflows remain unchanged unless explicitly opting into the frozen plan workflow

### Data to Include in Frozen Plan

The frozen plan must contain all information necessary to execute without re-rendering or re-querying:

- **Plan structure**: The complete DAG of operations with all dependencies
- **Resource specifications**: All ResourceSpecs that will be created/updated/deleted
- **Resource infos**: Current state information, diffs, and operation decisions
- **Release information**: The Release struct representing what will be deployed
- **Chart metadata**: Chart name, version, repository (for reference/audit)
- **Values**: The resolved values used during rendering (for audit/debugging)
- **Execution context**: Release name, namespace, deploy type, etc.
- **Execution token**: A unique identifier to prevent one-time-use plan files from being reused
- **Plan metadata**: Version, timestamp, encryption status, etc.

## Consequences

### Positive

1. **Consistency**: The plan reviewed is guaranteed to be identical to the plan executed (prevents configuration drift)
   - Eliminates the risk of chart files, values files, or dependencies being modified between plan and apply
   - Ensures the exact changes reviewed are the ones that get deployed
   - Provides confidence that what was approved is what will be executed
2. **Security**: Encrypted plans protect sensitive information at rest, and one-time-use mechanism prevents accidental duplicate deployments
   - Sensitive data in plans (resource specs, values) is encrypted by default
   - One-time-use protection prevents accidental re-execution of plans
   - Reduces risk of unauthorized access to deployment plans
3. **Auditability**: Provides a permanent record of what was planned and executed
   - Frozen plans serve as immutable records of deployment intentions
   - Can be stored alongside code changes for complete audit trail
   - Enables forensic analysis of what was deployed and when
   - Supports compliance with change management policies
4. **CI/CD Integration**: Enables proper approval workflows where plans can be reviewed and approved before execution
   - Plans can be generated automatically in CI/CD pipelines
   - Plans can be reviewed and approved as separate pipeline stages
   - Enables integration with approval systems and gates
   - Supports automated validation checks before deployment
5. **Collaboration**: Allows plans to be shared, reviewed, and executed by different team members
   - Developers can create plans, senior engineers can review them
   - Plans can be attached to pull requests for code review
   - Enables asynchronous review processes
   - Supports separation of concerns (planning vs. execution permissions)
   - Allows discussion of planned changes before execution
6. **Performance**: Skips expensive plan building phase during apply (though this is a minor benefit)
   - Avoids re-rendering charts and dependencies
   - Skips re-querying cluster state for plan building
   - Reduces time to deployment when using frozen plans
   - Can be significant for large or complex deployments
7. **Error Prevention**: Reduces risk of deploying unintended changes
    - Prevents configuration drift between review and deployment
    - Ensures approved changes are what get deployed
    - Reduces human error in re-entering configuration
    - Provides safety checks before automated execution

### Negative

1. **Complexity**: Adds serialization/deserialization logic and versioning concerns
2. **Storage**: Requires storing plan files (though they can be temporary artifacts)
3. **Version Compatibility**: Need to handle plan format versioning and migration
4. **One-Time-Use Tracking**: Need to implement mechanism to track used plans
5. **Plan File Security**: Plan files contain sensitive information that must be protected
6. **CI/CD Pipeline Complexity**: Requires careful orchestration in automation
    - Plan and apply stages must be properly connected
    - Only one plan should be outstanding at a time to avoid applying stale plans
    - Approval workflows must ensure the correct plan is applied
7. **Approval Workflow Challenges**: Implementing interactive approval requires careful design
    - Must ensure only one plan is outstanding at a time, or properly connect approval to specific plan
    - When a plan is applied, other plans against the same state become invalid and must be recomputed
    - Approval mechanisms must pass enough information to ensure the correct plan is applied
8. **Multi-Environment Considerations**: Using frozen plans across environments adds complexity
    - Plans are tied to specific cluster state and may not be portable across environments
    - Each environment may need separate plans generated against its own state

### Risks and Mitigations

1. **Plan Format Breaking Changes**: 
   - **Risk**: Changes to Operation or Plan structure could break compatibility
   - **Mitigation**: Use versioned plan format and provide migration tools or clear error messages

2. **Stale Plans**:
   - **Risk**: Plans executed long after creation may target outdated cluster state (infrastructure drift)
   - **Mitigation**: Include timestamp in plan and optionally warn if plan is too old; allow force flag to proceed anyway. Note: We cannot prevent infrastructure drift, only configuration drift.

3. **Plan Reuse**:
   - **Risk**: Accidental re-execution of the same plan could cause duplicate deployments or conflicts
   - **Mitigation**: Implement one-time-use mechanism with execution tokens; provide `--force` flag for legitimate retry scenarios

4. **Missing Context**:
   - **Risk**: Plan may not contain all necessary information for execution
   - **Mitigation**: Comprehensive testing and careful design of serialized data structure

5. **Plan File Exposure in CI/CD**:
   - **Risk**: Plan files stored as artifacts in CI/CD systems may be accessible to users with artifact access, exposing sensitive data
   - **Mitigation**: Encrypt plan files by default; restrict artifact access; document security best practices

6. **Environment Incompatibility**:
   - **Risk**: Attempting to apply a plan created for one environment (e.g., staging) to another (e.g., production)
   - **Mitigation**: Include environment/release metadata in plan; validate that plan matches target environment before applying; provide clear error messages

7. **Concurrent Plan Execution**:
   - **Risk**: Multiple plans may be generated concurrently, leading to confusion about which plan to apply
   - **Mitigation**: Implement one-plan-at-a-time policy or proper plan identification/tracking; invalidate other plans when one is applied

## Comparing to Terraform Plan Use Cases

HashiCorp Terraform's plan freezing feature (`terraform plan -out=plan.tfplan` and `terraform apply plan.tfplan`) addresses similar concerns and provides several proven use cases that we can learn from:

### 1. **CI/CD Pipeline Approval Workflows**

**Use Case**: In CI/CD pipelines, the plan phase runs automatically and generates a binary plan file. The plan is reviewed and approved (manually or through automated checks) before the apply phase executes the frozen plan.

**Example**:
```bash
# CI stage 1: Plan
terraform plan -out=plan.tfplan
# Upload plan.tfplan as artifact

# CI stage 2: Review and approve
# Human reviews terraform show plan.tfplan

# CI stage 3: Apply (after approval)
terraform apply plan.tfplan
```

**Nelm Equivalent**:
```bash
# CI stage 1: Plan
nelm release plan install -n production -r myapp --out=plan.nelmplan
# Upload plan.nelmplan as artifact

# CI stage 2: Review and approve
# Human reviews plan output

# CI stage 3: Apply (after approval)
nelm release install --plan-file=plan.nelmplan
```

### 2. **Separation of Planning and Execution**

**Use Case**: Different team members or systems can generate plans and execute them. For example, a developer creates a plan, a senior engineer reviews it, and a deployment system applies it.

**Benefits**:
- Clear separation of concerns
- Enables review processes without requiring access to execution environment

**Nelm Equivalent**: Same workflow enables separation between planning and execution.

### 3. **Audit and Compliance**

**Use Case**: Binary plans serve as immutable records of what changes were planned and executed. They can be stored, versioned, and audited later.

**Benefits**:
- Compliance with change management policies
- Forensic analysis of what was deployed
- Historical record of infrastructure changes

**Nelm Equivalent**: Frozen plans can be stored in artifact repositories alongside code changes, providing a complete audit trail.

### 4. **Reproducibility and Rollback**

**Use Case**: If a deployment fails or causes issues, the exact plan can be reviewed to understand what was executed. In some cases, plans can be re-executed (with validation) or used to generate rollback plans.

**Nelm Equivalent**: Frozen plans provide a record of what was deployed, which can inform rollback decisions. While Nelm already has rollback capabilities, frozen plans add transparency to what was originally deployed.

### 5. **Human-Readable Review**

**Use Case**: While plans are stored in binary format, Terraform provides `terraform show plan.tfplan` to generate human-readable output from binary plans. This allows reviewing plans without regenerating them.

**Nelm Equivalent**: Provide a similar command like `nelm release plan show <plan-file>` to display the plan contents in a human-readable format, enabling review of frozen plans. The command should respect sensitive data handling:
- By default, sensitive data (Secrets, resources with `werf.io/sensitive="true"` annotation, or fields matching `werf.io/sensitive-paths`) is redacted and shown as `<hidden N bytes, hash XXX>`
- The `--show-sensitive` flag can be used to display sensitive data in plain text
- This matches the behavior of `nelm release plan install --show-sensitive-diffs`

### 6. **Plan Sharing and Collaboration**

**Use Case**: Binary plans can be shared between team members, attached to pull requests, or discussed in code review processes.

**Benefits**:
- Enables asynchronous review
- Allows discussion of planned changes before execution
- Provides context for code reviews

**Nelm Equivalent**: Plans can be attached to pull requests or shared via chat/email for team review before deployment.

### 7. **Safety in Automated Systems**

**Use Case**: In automated deployment systems, generating a plan first and then applying it ensures that the system doesn't make unexpected changes due to configuration drift or timing issues.

**Benefits**:
- Ensures consistency in automated workflows
- Provides safety checks before automated execution

**Nelm Equivalent**: CI/CD systems can generate plans, validate them (e.g., check for destructive changes), and then apply them with confidence.

## Implementation Notes

### Plan Format Considerations

- **Binary Format**: Use a format that supports versioning and efficient serialization. Consider Protocol Buffers for cross-language compatibility or Go's `gob` for simplicity and type safety.
- **Compression**: Consider compressing plan files, especially for large deployments
- **Checksums**: Include checksums to detect corruption
- **Metadata**: Include version, timestamp, and other metadata for validation and debugging

### CLI Design

```bash
# Generate and save encrypted plan (uses NELM_SECRET_KEY)
nelm release plan install -n namespace -r release --out=plan.nelmplan

# Show plan contents (human-readable, requires decryption)
# Sensitive data is hidden by default
nelm release plan show plan.nelmplan

# Show plan contents including sensitive data
nelm release plan show plan.nelmplan --show-sensitive

# Apply frozen plan (namespace and release read from plan)
# Shows calculated changes summary before execution
nelm release install --plan-file=plan.nelmplan

# Force re-execution of a used plan (if needed for retry)
nelm release install --plan-file=plan.nelmplan --force

# Validate plan without applying
nelm release plan validate plan.nelmplan
```

### Error Handling

- Clear error messages when plan format is incompatible
- Clear error messages when encryption key is missing or incorrect
- Warnings when plan is stale (older than X hours/days)
- Error when attempting to reuse a one-time-use plan (unless `--force` is provided)
- Clear error messages when required data is missing from plan
- Option to force apply even with warnings (with appropriate flags)

## References

- [Terraform Plan Command Documentation](https://developer.hashicorp.com/terraform/cli/commands/plan)
- [Terraform Apply Command Documentation](https://developer.hashicorp.com/terraform/cli/commands/apply)
- Nelm Architecture Documentation: `ARCHITECTURE.md`
- Plan Implementation: `internal/plan/plan.go`
- Operation Implementation: `internal/plan/operation.go`

## Alternatives Considered

1. **JSON/YAML Plan Format**: 
   - **Pros**: Human-readable, easy to debug
   - **Cons**: Larger file size, slower parsing, easier to tamper with
   - **Decision**: Binary format is preferred for efficiency and integrity

2. **Storing Plans in Kubernetes**:
   - **Pros**: Centralized storage, no file management
   - **Cons**: Adds complexity, requires cluster access, harder to share/review
   - **Decision**: File-based approach is simpler and more flexible

3. **One-Time-Use Mechanisms**:
   - **Option A - File Modification**: Mark plan as used by modifying the file
     - **Pros**: Simple, no external dependencies
     - **Cons**: File modification may not be atomic, could be bypassed by file restoration
   - **Option B - External Registry**: Track used plans in Kubernetes or external system
     - **Pros**: More robust, can track across multiple systems
     - **Cons**: Requires cluster access or external service
   - **Decision**: Start with file modification approach (simplest), with option to enhance to external registry if needed
