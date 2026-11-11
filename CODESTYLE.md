## Design

* Prefer stupid and simple over abstract and extendable.
* Prefer a bit of duplication over complex abstractions.
* Prefer clarity over brevity in variable, function and type names.
* Minimize usage of interfaces, generics, embedding.
* Prefer few classes over many.
* Prefer no classes over few.
* Prefer data classes over regular classes.
* Prefer functions over methods.
* Prefer public fields over getters/setters.
* Keep everything private/internal as much as possible.
* Validate early, validate a lot.
* Keep APIs stupid and minimal.
* Avoid global state.
* Prefer simplicity over micro-optimizations.
* For complex things use libraries instead of reinventing the wheel.
* Document only non-obvious public APIs or complex/weird code.

## Conventions

### Functions/methods

* All public functions and methods must accept `context.Context` as the first parameter.
* All arguments of a **public** function are required: passing nil not allowed.
* Optional arguments of a **public** function are provided via `<MyFunctionName>Options` as the last argument.
* Avoid functional options pattern.
* Use guard clauses and early returns/continues to keep the happy path unindented.
* Use `samber/lo` helpers (if nothing similar in the standard lib): `lo.Filter`, `lo.Find`, `lo.Map`, `lo.Contains`, `lo.Ternary`, `lo.ToPtr`, `lo.Must`, etc.

### Constructors

* Constructors are optional.
* Should be named `New<TypeName>[...]`, e.g. `NewResource()` and `NewResourceFromManifest()`.
* No network/filesystem calls or resource-intensive operations. Do it somewhere higher, like in `BuildResources()`.

### Interfaces

* Always add a compile-time check for each implementation, e.g. `var _ Animal = (*Dog)(nil)`.

### Constants

* Avoid `iota`.
* Prefix the enum-like constant name with the type name, e.g. `LogLevelDebug LogLevel = "debug"`.

### Errors

* Always wrap errors with additional context using `fmt.Errorf("...: %w", err)`.
* When needed to aggregate and display/handle many errors, use the `MultiError{}` helper.
* On programmer errors prefer panics, e.g. on an unexpected case in a switch.
* Do one-line `if err := myfunc(); err != nil` wherever possible, generally prefer one-line handling.
* When wrapping errors with fmt.Errorf, describe what is being done, not what failed, e.g. `fmt.Errorf("read config file: %w", err)` instead of `fmt.Errorf("cannot read config file: %w", err)`.

### Concurrency

* Instead of raw mutexes use the transactional `Concurrent{}` helper.

## Nelm-specific

* CLI commands only map flags to `<Action>Options` (`cli.AddFlag`) and call the action; all logic lives in the action. Model new commands on `release install`.
* Remove options that do nothing instead of deprecating them.
* Nelm is also a library: no CLI flag names (`--set`) in errors, logs or comments under `pkg/`. Say "set values".
* No werf concepts in Nelm code, messages or docs (werf, its commands, "env", "service values"). Use Nelm terms: extra values, runtime annotations. Data names like `.werf-data` or `werf.io/*` are fine.
* `pkg/helm` is an upstream fork: keep diffs to upstream files minimal, never touch upstream comments, put Nelm code into `*_nelm.go` / `*_nelm_test.go`.
* Check `go.mod` for a library before hand-writing a parser, regex or algorithm.
* Fix bugs where they live, not with workarounds in consumers. Prefer rejecting odd input to re-plumbing.
* Keep comments and key order when rewriting user YAML (`goccy/go-yaml`).
* Reason about compatibility per major version only.
* Delete what a change leaves unused, in werf and common-go too.
* No README or doc sections nobody asked for.

## Go standard guidelines

Follow these two guides:

* [Effective Go](https://go.dev/doc/effective_go)
* [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
