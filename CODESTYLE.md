> **AI agents**: all rules below are mandatory. NEVER deviate from them.

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
* Document only public APIs or complex/weird code.

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

## Go standard guidelines

Based on [Effective Go](https://go.dev/doc/effective_go) and [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).

### Naming

* MixedCaps, not underscores: `ServerTimeout`, not `server_timeout`.
* Initialisms are all-caps: `HTTPClient`, `userID`, not `HttpClient`, `userId`.
* Package names: lowercase, single-word, no underscores or mixedCaps. Package name is part of the API: `http.Client`, not `http.HTTPClient`.
* Interface names: single-method interfaces use method name + `-er` suffix: `Reader`, `Formatter`.
* Receiver names: short (1-2 letters), consistent across methods, never `this` or `self`.

### Comments

* Exported names must have doc comments, starting with the name: `// Client represents ...`.
* Comments are complete sentences, ending with a period.

### Error handling

* Never discard errors with `_`. Handle or explicitly document why it's safe to ignore.
* Indent the error flow, not the happy path. The non-error path should stay at minimal indentation.

### Function signatures

* `context.Context` is always the first parameter.
* Accept interfaces, return concrete types.
* Avoid named return values except for godoc clarity on same-type returns.
* Avoid naked returns.

### Imports

* Never use dot imports.

### Getters

* Getter: `Owner()`, not `GetOwner()`. Setter: `SetOwner()`.

### Slices

* Prefer `var s []string` (nil slice) over `s := []string{}` unless you specifically need non-nil.

### Control flow

* Prefer `switch` over long if-else chains.

### Testing

* Include actual vs expected in test failures: `got X, want Y`.
* Include the input that caused the failure.

### Miscellaneous

* Prefer `var buf bytes.Buffer` over `buf := new(bytes.Buffer)` when zero value is useful.
* Goroutine lifetimes: always make it clear when/how a goroutine exits.
* Use `crypto/rand`, not `math/rand`, for anything security-related.
