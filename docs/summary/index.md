# Nelm Codebase Knowledge Base

> **For AI Assistants:** This index is your primary entry point for understanding the Nelm codebase. Use the summaries below to determine which file contains the information you need.

## Quick Reference

| Topic | File | Description |
|-------|------|-------------|
| Project metadata | [codebase_info.md](codebase_info.md) | Build commands, Go version, key dependencies, testing framework |
| Package structure | [architecture.md](architecture.md) | Layered architecture, package purposes, data flow diagrams |
| Major components | [components.md](components.md) | Detailed component documentation (ReleaseInstall, Plan, Operation, etc.) |
| Public interfaces | [interfaces.md](interfaces.md) | Key interface definitions (KubeClienter, Historier, ResourceTransformer) |
| Data structures | [data_models.md](data_models.md) | Struct definitions, enums, type relationships |
| Deployment flows | [workflows.md](workflows.md) | Sequence diagrams for install, rollback, render operations |
| External deps | [dependencies.md](dependencies.md) | Third-party library catalog with versions and purposes |

## Common Questions

### "How do I add a new CLI command?"

See [architecture.md](architecture.md) for CLI layer structure. Each command is defined in `cmd/nelm/` using Cobra, mapping to action functions in `pkg/action/`.

### "How does deployment ordering work?"

See [components.md](components.md) sections on **Plan** and **Operation**. Resources are ordered by:
1. Stages (PreInstall -> Install -> PostInstall, etc.)
2. Weights within stages (`werf.io/weight` annotation)
3. Dependencies (`werf.io/deploy-dependency-*` annotations)

### "What annotations are available?"

See [components.md](components.md) section on **InstallableResource**. Key annotations:
- `werf.io/weight` - Deployment priority
- `werf.io/deploy-dependency-*` - Explicit dependencies
- `werf.io/delete-dependency-*` - Delete dependencies
- `werf.io/delete-policy` - Deletion behavior
- `werf.io/delete-propagation` - Deletion propagation policy
- `werf.io/deploy-on` - Conditional deploy by action/stage
- `werf.io/track-termination-mode` - Tracking behavior when terminating
- `werf.io/fail-mode` - Failure handling behavior
- `werf.io/failures-allowed-per-replica` - Failure threshold
- `werf.io/sensitive`, `werf.io/sensitive-paths` - Sensitive data redaction
- `werf.io/ownership` - Resource ownership model
- `<id>.external-dependency.werf.io/*` - External dependencies

Parsing/validation logic is in `internal/resource/metadata.go`. For the authoritative user-facing list and semantics, see `README.md` “Reference”.

### "How do I run tests?"

See [codebase_info.md](codebase_info.md):
- `task test:unit` - Run all unit tests with Ginkgo
- `task test:unit paths="./pkg/action"` - Test specific package

### "How does the DAG planning work?"

See [components.md](components.md) section on **Plan**:
- Uses `dominikbraun/graph` library
- Operations form vertices, dependencies form edges
- `BuildPlan()` constructs the graph; `ExecutePlan()` runs in topological order with parallelism
- `Optimize()` applies transitive reduction

### "How do I add a new resource transformation?"

See [interfaces.md](interfaces.md) for `ResourceTransformer` and `ResourcePatcher` interfaces:
- **Transformers** convert one resource into zero or more (e.g., expanding List kinds)
- **Patchers** modify a single resource (e.g., adding metadata)

Implementations are in `internal/resource/spec/`.

### "Where is the Kubernetes client abstraction?"

See [interfaces.md](interfaces.md) for `KubeClienter` and `ClientFactorier` interfaces. Implementation in `internal/kube/`.

### "How does release storage work?"

See [interfaces.md](interfaces.md) for `ReleaseStorager` and `Historier` interfaces. Releases are stored in Helm-compatible format via Helm storage drivers (Secrets/ConfigMaps/SQL/Memory). Implementation in `internal/release/`.

### "What is the resource lifecycle during deployment?"

See [data_models.md](data_models.md) and [workflows.md](workflows.md):

```
Chart Templates -> ResourceSpec -> InstallableResource -> InstallableResourceInfo -> Operation -> K8s API
```

## Architecture at a Glance

```
cmd/nelm/           CLI Layer (Cobra commands)
    |
    v
pkg/action/         Action Layer (business logic orchestration)
    |
    v
internal/           Core Layer
    +-- plan/       DAG-based planning and execution
    +-- resource/   Resource abstraction and metadata parsing
    +-- kube/       Kubernetes client abstraction
    +-- chart/      Chart rendering
    +-- lock/       Release locking (lockgate-backed)
    +-- release/    Release storage
    +-- track/      Progress tracking
    +-- ts/         TypeScript chart support
    +-- legacy/     Legacy compatibility helpers
    +-- util/       Shared utilities (errors, parsers, etc.)
```

For detailed diagrams, see [architecture.md](architecture.md).

## Key Design Decisions

1. **DAG-Based Execution** - Operations form a directed acyclic graph enabling parallel execution and proper dependency ordering.

2. **Two-Phase Deployment** - Plan phase builds the graph, Execute phase applies changes. Enables dry-run previews.

3. **Helm Compatibility** - Uses forked Helm SDK (`werf/3p-helm`), stores releases in Helm-compatible format.

4. **Annotation-Driven Behavior** - Resource behavior controlled via `werf.io/*` annotations.

5. **TypeScript Charts** - Native TypeScript/JavaScript support via embedded JavaScript runtime (Goja) with esbuild bundling.

For more details, see the "Key Architectural Decisions" section in [architecture.md](architecture.md).
