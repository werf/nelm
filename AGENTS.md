# Repository Guidelines

Nelm is a Go-based Kubernetes deployment tool (Helm-compatible). For context and expectations, read `README.md`, `ARCHITECTURE.md`, and `CONTRIBUTING.md`.

## Documentation

- [README.md](README.md) - Project overview, features, CLI reference, annotations
- [ARCHITECTURE.md](ARCHITECTURE.md) - Package structure, data types, transformation pipeline, deployment flow
- [CONTRIBUTING.md](CONTRIBUTING.md) - Development workflow, commit conventions, design and style guidelines

## Build Commands

Uses [Task](https://taskfile.dev/) as the build system. Requires `export TASK_X_REMOTE_TASKFILES=1`.

```bash
task                    # Run all checks: format, build, lint, unit tests
task build              # Build binary for current OS/arch to ./bin/
task format             # Run gci, gofumpt, prettier
task lint               # Run golangci-lint and prettier checks
task test:unit          # Run unit tests with ginkgo
task test:unit paths="./pkg/action"  # Test specific package
task clean              # Clean build artifacts
task generate           # Run generators (e.g., Markdown TOCs)
```

## Testing

Uses Ginkgo v2 with Gomega matchers.

```bash
task test:unit                           # Run all unit tests
task test:unit paths="./internal/plan"   # Test specific package
task test:ginkgo paths="./pkg" -- -v     # Pass flags to ginkgo
```

## Additional PR review guidelines

Require explicit approval from the reviewer for:
- New external dependencies.
- Breaking user-facing changes (not API changes), unless they are hidden behind a feature flag.
- Changes that may compromise security.
