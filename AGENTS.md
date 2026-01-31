# Repository Guidelines

Nelm is a Go-based Kubernetes deployment tool (Helm-compatible). For context and expectations, read `README.md`, `CONTRIBUTING.md`, and `docs/summary/index.md`.

## Documentation

- [README.md](README.md) - Project overview, features, CLI reference, annotations.
- [CONTRIBUTING.md](CONTRIBUTING.md) - Development workflow, commit conventions, design and style guidelines. Very important for AI.
- [docs/summary/index.md](docs/summary/index.md) - AI-optimized knowledge base with codebase documentation.

## Commands

Uses [Task](https://taskfile.dev/) as the build system. Requires `export TASK_X_REMOTE_TASKFILES=1`.

```bash
task build              # Build binary for current OS/arch to ./bin/
task build pkg=github.com/werf/nelm/cmd/nelm # Build binary for specific package
task format             # Run all formatters
task format paths="./pkg/action" # Run all formatters for specific package
task lint               # Run golangci-lint and prettier checks
task lint paths="./pkg/action" # Run golangci-lint and prettier checks for specific package
task lint:golangci-lint  # Run only golangci-lint checks
task lint:golangci-lint paths="./pkg/action"  # Run golangci-lint checks for specific package
task lint:prettier      # Run only prettier checks
task test:unit          # Run all unit tests
task test:unit paths="./pkg/action"  # Run unit tests for specific package
task clean              # Clean build artifacts
task generate           # Run generators (e.g., Markdown TOCs)
```

## Work standards

- Don't trade correctness for speed or tokens
- Don't trade thoroughness for speed or tokens
- Be exhaustive, thorough
- Don't assume, verify

## Additional PR review guidelines

Require explicit approval from the reviewer for:
- New external dependencies.
- Breaking user-facing changes (not API changes), unless they are hidden behind a feature flag.
- Changes that may compromise security.
