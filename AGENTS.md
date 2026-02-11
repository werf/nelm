# Repository Guidelines

All rules in this document are requirements — not suggestions. ALWAYS follow them.

Nelm is a Go-based Kubernetes deployment tool, which deploys Helm charts, is compatible with Helm releases, and is an alternative to Helm. Nelm is built on top of a Helm fork ([werf/3p-helm](https://github.com/werf/3p-helm)) and is also used as the deployment engine of [werf](https://github.com/werf/werf).

## Work standards (MANDATORY)

- ALWAYS use `task` commands for build/test/lint/format — NEVER raw `go build`, `go test`, `go fmt`, or `golangci-lint` directly.
- ALWAYS use `log.Default` from `pkg/log` for logging. NEVER use `fmt.Println`, `slog`, or `logrus` directly.
- ALWAYS verify, don't assume — check the actual state before making changes.
- NEVER leave TODOs, stubs, or partial implementations.
- ALWAYS stay within the scope of what was asked. When asked to update a plan — only update the plan, don't change code. When asked to brainstorm/discuss — only discuss, don't write code. When asked to do X — do X and nothing else. NEVER make unsolicited changes.

## Code style (MANDATORY)

> Don't edit rules below directly. Instead add your rule to the [CODESTYLE.md](CODESTYLE.md) and ask AI to regenerate the section below from the updated [CODESTYLE.md](CODESTYLE.md).

### Design

- ALWAYS prefer stupid and simple over abstract and extendable.
- ALWAYS prefer a bit of duplication over complex abstractions.
- ALWAYS prefer clarity over brevity in names.
- ALWAYS minimize interfaces, generics, embedding.
- ALWAYS prefer fewer types. Prefer no types over few. Prefer data types over types with behavior.
- ALWAYS prefer functions over methods. ALWAYS prefer public fields over getters/setters.
- ALWAYS keep everything private/internal as much as possible.
- ALWAYS validate early, validate a lot. ALWAYS keep APIs stupid and minimal.
- NEVER prefer global state. ALWAYS prefer simplicity over micro-optimizations.
- ALWAYS use libraries for complex things instead of reinventing the wheel.
- NEVER add comments unless they document a non-obvious public API or explain genuinely non-obvious logic. NEVER add obvious/redundant comments, NEVER add comments restating what code does. When in doubt, don't comment.

### Conventions

- All public functions/methods MUST accept `context.Context` as the first parameter.
- All arguments of a public function are required — passing nil not allowed.
- Optional arguments via `<FunctionName>Options` as the last argument. NEVER use functional options.
- Use guard clauses and early returns to keep the happy path unindented.
- Use `samber/lo` helpers: `lo.Filter`, `lo.Find`, `lo.Map`, `lo.Contains`, `lo.Ternary`, `lo.ToPtr`, `lo.Must`, etc.
- Constructors: `New<TypeName>[...]()`. No network/filesystem calls in constructors.
- Interfaces: ALWAYS add `var _ Animal = (*Dog)(nil)` compile-time check.
- Constants: avoid `iota`. Prefix enum constants with type name: `LogLevelDebug LogLevel = "debug"`.
- Errors: ALWAYS wrap with context: `fmt.Errorf("read config: %w", err)`. Describe what is being done, not what failed. Use `MultiError{}` for aggregation. Panic on programmer errors. Prefer one-line `if err := ...; err != nil`.
- Concurrency: use `Concurrent{}` helper instead of raw mutexes.

### Go standard guidelines

Based on [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).

- Naming: MixedCaps, not underscores. Initialisms all-caps: `HTTPClient`, `userID`. Package names: lowercase, single-word. Receiver names: 1-2 letters, consistent, NEVER `this`/`self`.
- Comments: exported names MUST have doc comments starting with the name. Comments are complete sentences. NEVER comment unexported code unless genuinely non-obvious.
- Errors: NEVER discard errors with `_`. Indent error flow, not happy path.
- Functions: `context.Context` first. Accept interfaces, return concrete types. Avoid named returns and naked returns.
- Imports: NEVER use dot imports.
- Getters: `Owner()`, not `GetOwner()`. Setter: `SetOwner()`.
- Slices: prefer `var s []string` over `s := []string{}`.
- Control flow: prefer `switch` over long if-else chains.
- Testing: include actual vs expected: `got X, want Y`. Include the failing input.
- Prefer `var buf bytes.Buffer` over `buf := new(bytes.Buffer)`. Goroutine lifetimes must be clear. Use `crypto/rand` for security.

## Commands (MANDATORY)

ALWAYS use these `task` commands. NEVER use raw `go build`, `go test`, `go fmt`, or `golangci-lint` directly.

- `task build` — Build binary for current OS/arch to `./bin/`. Accepts `pkg=...` to build a specific package.
- `task format` — Run all formatters. Accepts `paths="./pkg/..."` to scope to a specific package.
- `task lint` — Run golangci-lint and prettier checks. Accepts `paths="./pkg/..."`.
- `task lint:golangci-lint` — Run only golangci-lint checks. Accepts `paths="./pkg/..."`.
- `task lint:prettier` — Run only prettier checks.
- `task test:unit` — Run all unit tests. Accepts `paths="./pkg/..."`.
- `task clean` — Clean build artifacts.
- `task generate` — Run generators (e.g., Markdown TOCs).

## Testing (MANDATORY)

- ALWAYS use `testify` (`assert`, `require`) when writing new tests.
- When writing tests as an AI agent → ALWAYS name the file `*_ai_test.go`, add `//go:build ai_tests` build tag, prefix test functions with `TestAI_`.
- ALWAYS place tests alongside source files, not in a separate directory.
- Test helpers go in `helpers_test.go` (or `helpers_ai_test.go` for AI-written helpers).
- Test fixtures go in `testdata/` subdirectory next to the tests.
- Shared test helpers are in `internal/test/`.

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

## PR review guidelines (MANDATORY)

- NEVER add new external dependencies without flagging to the user first.
- NEVER introduce breaking user-facing changes (not API changes) unless they are hidden behind a feature flag. Flag to the user first.
- NEVER introduce changes that may compromise security. Flag to the user first.

## Related repositories

- [werf/3p-helm](https://github.com/werf/3p-helm) — Helm fork. Provides chart loading, rendering, and release primitives. Changes to Helm internals go here, not in nelm.
- [werf/kubedog](https://github.com/werf/kubedog) — Kubernetes resource tracking library. Used by `internal/track/`.
- [werf/common-go](https://github.com/werf/common-go) — Shared Go libraries (secrets, CLI utilities, locking).
- [werf/werf](https://github.com/werf/werf) — CI/CD tool that uses nelm as its deployment engine.
