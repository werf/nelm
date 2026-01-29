# Codebase Information

## Project Overview

- **Name:** Nelm
- **Module:** `github.com/werf/nelm`
- **Go Version:** 1.23.1
- **Description:** Helm 4 alternative - Kubernetes deployment tool with terraform-like planning, improved CRD management, secrets support, and advanced resource ordering

## Build System

- **Tool:** [Task](https://taskfile.dev/)
- **Prerequisite:** `export TASK_X_REMOTE_TASKFILES=1`
- **Note:** Many tasks are defined in `Taskfile.dist.yaml` (included from `Taskfile.yaml`). Some tasks use Docker and/or download tools (e.g., `deps:install` uses `curl`).

| Command | Purpose |
|---------|---------|
| `task` | Run all checks (format, build, lint, unit tests) |
| `task build` | Build binary to `./bin/` |
| `task format` | Run gci, gofumpt, prettier |
| `task lint` | Run golangci-lint, prettier |
| `task test:unit` | Run unit tests with Ginkgo |
| `task test:ginkgo` | Run raw ginkgo tests |
| `task generate` | Run generators (e.g., Markdown TOCs) |
| `task clean` | Clean build artifacts |
| `task deps:install` | Install all development dependencies |

## Testing Framework

- **Framework:** Ginkgo v2 with Gomega matchers
- **Test Location:** `*_test.go` files alongside source
- **Test Paths:** `./internal`, `./pkg`, `./cmd`

## Repository Structure

See `architecture.md` for detailed package structure.

## Key Dependencies

| Dependency | Purpose |
|------------|---------|
| k8s.io/client-go | Kubernetes client |
| k8s.io/apimachinery | Kubernetes API types and utilities |
| github.com/werf/3p-helm | Customized Helm fork |
| github.com/werf/kubedog | Resource tracking |
| github.com/werf/common-go | Shared Go utilities |
| github.com/dominikbraun/graph | DAG operations |
| github.com/spf13/cobra | CLI framework |
| github.com/dop251/goja | JavaScript runtime (TS charts) |
| github.com/dop251/goja_nodejs | Node.js-like shims (console/require) for Goja |
| github.com/evanw/esbuild | TypeScript/JavaScript bundler |
| github.com/onsi/ginkgo/v2 | Testing framework |
| github.com/onsi/gomega | Test matchers |
| github.com/goccy/go-yaml | YAML parsing |
| github.com/samber/lo | Functional utilities |

## Related Repositories

| Repository | Relationship |
|------------|--------------|
| [werf/werf](https://github.com/werf/werf) | Uses Nelm as deployment engine |
| [werf/3p-helm](https://github.com/werf/3p-helm) | Customized Helm fork |
| [werf/kubedog](https://github.com/werf/kubedog) | Resource tracking library |
| [werf/common-go](https://github.com/werf/common-go) | Shared Go utilities |

## Feature Gates

Nelm uses feature gates to enable/disable specific behaviors (including “preview v2” bundles of changes). Feature gates are defined in `pkg/featgate/feat.go` and are toggled via `NELM_FEAT_*` environment variables.
