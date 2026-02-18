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

## Possible solutions

During R&D, we considered several approaches, including extensive plan comparison with the actual cluster state intended to protect against infrastructure drift. We found that accurately comparing the planned and current states is not feasible because:

* Helm charts are not idempotent in some cases (they may depend on domain name resolution, random values, or non-deterministic iteration over data structures), which leads to false positives, as every render may produce a different result.
* It is impossible to compare two diff results produced by dry-run SSA to determine which changes were planned/approved and which were not.
* Comparing the planned and current DAGs is not feasible, as the DAG structure changes significantly even with minimal infrastructure drift.
* Comparing the planned and current manifest states is not meaningful, as we assume that planned manifests were previously approved and can be applied regardless of the current manifest state.

## Chosen solution

Given these constraints, we decided to implement a **plan freezing mechanism** that exports a complete release install plan to a gzip-compressed JSON artifact during `nelm release plan install` and then **directly executes the saved plan** during `nelm release install --use-plan`. The saved plan artifact contains the full DAG of operations with all resource specifications and plan info objects, so that the exact plan that was reviewed is the plan that gets executed.

1. `nelm release plan install --save-plan=plan.gz`:
- Builds a regular install plan (as today).
- Exports the full plan DAG, operation configurations, calculated changes, installable resource infos, and release infos to a gzip-compressed JSON artifact.
2. `nelm release install --use-plan=plan.gz`:
- Reads the plan artifact.
- Validates the artifact (lifetime, release version match, etc.).
- Executes the stored plan DAG directly.
3. `nelm release plan show plan.gz`:
- Reads the plan artifact.
- Displays the planned changes stored in the artifact for review.
