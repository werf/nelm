# Repository Guidelines

Nelm is a Go-based Kubernetes deployment tool, which deploys Helm charts, is compatible with Helm releases, and is an alternative to Helm. Nelm is built on top of a Helm fork ([werf/3p-helm](https://github.com/werf/3p-helm)) and is also used as the deployment engine of [werf](https://github.com/werf/werf).

## Commands

- `task build` — Build binary for current OS/arch to `./bin/`. Accepts `pkg=...` to build a specific package.
- `task format` — Run all formatters. Accepts `paths="./pkg/..."` to scope to a specific package.
- `task lint` — Run golangci-lint and prettier checks. Accepts `paths="./pkg/..."`.
- `task lint:golangci-lint` — Run only golangci-lint checks. Accepts `paths="./pkg/..."`.
- `task lint:prettier` — Run only prettier checks.
- `task test:unit` — Run all unit tests. Accepts `paths="./pkg/..."`.
- `task clean` — Clean build artifacts.
- `task generate` — Run generators (e.g., Markdown TOCs).

## Project structure

- `cmd/nelm/` — CLI entrypoint and command definitions.
- `pkg/action/` — high-level actions invoked by CLI commands (install, uninstall, lint, render, etc.).
- `pkg/common/` — shared public types and constants.
- `pkg/featgate/` — feature gates.
- `pkg/legacy/` — legacy public APIs (secrets, deploy).
- `pkg/log/` — logging abstraction.
- `internal/chart/` — chart loading and processing.
- `internal/kube/` — Kubernetes client utilities.
- `internal/legacy/` — legacy internal implementations (secrets, deploy).
- `internal/lock/` — distributed locking.
- `internal/plan/` — release plan building and diffing.
- `internal/release/` — release storage and history.
- `internal/resource/` — Kubernetes resource representation, validation, and diffing.
- `internal/test/` — shared test helpers.
- `internal/track/` — resource state tracking during deployment.
- `internal/ts/` — TypeScript support (bundling, rendering).
- `internal/util/` — internal utility functions.

## Testing

- When writing new tests → use `testify` (`assert`, `require`, `suite`).
- When writing tests as an AI agent → name the file `*_ai_test.go`, add `//go:build ai_tests` build tag, prefix test functions with `TestAI_`.
- Place tests alongside source files, not in a separate directory.
- Test helpers go in `helpers_test.go` (or `helpers_ai_test.go` for AI-written helpers).
- Test fixtures go in `testdata/` subdirectory next to the tests.
- Shared test helpers are in `internal/test/`.

## Work standards

- Always use `task` commands for build/test/lint/format — never raw `go build`, `go test`, `go fmt`, or `golangci-lint` directly.
- When logging → use `log.Default` from `pkg/log`. Never use `fmt.Println`, `slog`, or `logrus` directly.
- Read and strictly follow the project code style defined in [CODESTYLE.md](CODESTYLE.md).
- Verify, don't assume — always check the actual state before making changes.
- Don't leave TODOs, stubs, or partial implementations.

## PR review guidelines

- Do not add new external dependencies without flagging to the user first.
- Do not introduce breaking user-facing changes (not API changes) unless they are hidden behind a feature flag. Flag to the user first.
- Do not introduce changes that may compromise security. Flag to the user first.

## Related repositories

- [werf/3p-helm](https://github.com/werf/3p-helm) — Helm fork. Provides chart loading, rendering, and release primitives. Changes to Helm internals go here, not in nelm.
- [werf/kubedog](https://github.com/werf/kubedog) — Kubernetes resource tracking library. Used by `internal/track/`.
- [werf/common-go](https://github.com/werf/common-go) — Shared Go libraries (secrets, CLI utilities, locking).
- [werf/werf](https://github.com/werf/werf) — CI/CD tool that uses nelm as its deployment engine.
