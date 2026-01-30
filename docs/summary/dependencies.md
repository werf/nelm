# External Dependencies

This document lists key external dependencies. Check `go.mod` for current versions.

## Werf Ecosystem

- **werf/3p-helm** - Customized Helm fork for chart rendering, release storage, and hooks
- **werf/kubedog** - Resource tracking and status monitoring during deployments
- **werf/common-go** - Shared Go utilities across werf projects
- **werf/lockgate** - Distributed locking for concurrent deployment coordination
- **werf/logboek** - Structured logging with hierarchical output and progress indicators

## Key Dependencies

### Kubernetes

- **k8s.io/client-go** - Kubernetes client library for cluster interactions
- **k8s.io/api** - Core Kubernetes API types (Pod, Deployment, Service, etc.)
- **k8s.io/apimachinery** - API machinery: meta types, runtime objects, schema

### Graph and State

- **dominikbraun/graph** - DAG operations for resource dependency ordering
- **looplab/fsm** - Finite state machine used in internal parsers (e.g., `internal/util/properties.go`)

### CLI

- **spf13/cobra** - CLI framework
- **gookit/color** - Terminal color output

### TypeScript Support

- **dop251/goja** - ECMAScript runtime in Go for executing TypeScript charts
- **evanw/esbuild** - TypeScript/JavaScript bundler and transpiler

### Data Processing

- **goccy/go-yaml** - High-performance YAML parsing
- **ohler55/ojg** - Optimized JSON/JSONPath operations
- **aymanbagabas/go-udiff** - Unified diff generation for plan output

### Testing

- **onsi/ginkgo/v2** - BDD-style testing framework
- **onsi/gomega** - Assertion/matcher library for Ginkgo

### Utilities

- **samber/lo** - Functional utilities (map, filter, reduce)
- **yannh/kubeconform** - Kubernetes manifest validation

## Key Dependency Relationships

```
Nelm
 +-- werf/3p-helm (Helm core)
 +-- werf/kubedog (Resource tracking)
 +-- k8s.io/client-go (Kubernetes client)
 +-- dop251/goja + evanw/esbuild (TypeScript charts)
 +-- dominikbraun/graph (DAG operations)
 +-- spf13/cobra (CLI)
```
