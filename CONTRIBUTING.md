<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Before you start](#before-you-start)
- [Architecture](#architecture)
- [Development](#development)
- [Commit/branch/PR naming](#commitbranchpr-naming)
- [Design guidelines](#design-guidelines)
- [Style guidelines](#style-guidelines)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Before you start

For any significant change, open the issue first (or comment on an existing one) to discuss with the maintainers on what to do and how. When the solution is agreed upon, you can proceed with implementation and open a pull request.

For small changes, such as few lines bugfixes or documentation improvements, feel free to open a pull request directly.

For easy first issues, check the [good first issue](https://github.com/werf/nelm/issues?q=is%3Aissue%20state%3Aopen%20label%3A%22good%20first%20issue%22) tag.

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md).

## Development

1. Clone the repository.
1. Install Go: https://go.dev/doc/install
1. Install Task: https://taskfile.dev/docs/installation
1. Do `export TASK_X_REMOTE_TASKFILES=1`.
1. Make your changes.
1. Run `task` with no arguments to run all essential checks (build, lint, format, quick tests, and others).
1. Commit your changes. See [Commit/branch/PR naming](#commitbranchpr-naming) for the commit message format. The commit must be signed off (`--signoff`) as an acknowledgment of the [DCO](https://developercertificate.org/).
1. Push and open a pull request.

You can also check out all available tasks with `task -l`.

## Commit/branch/PR naming

The commit message format is `<commit-type>: <description>`.
The branch name format is `<commit-type>/<shortened-description>`.
The PR title format is `<commit-type>: <description>`.

The commit type is one of the following:
* feat: a new feature or an improvement (will bump the minor version)
* fix: a bug fix or a minor improvement (will bump the patch version)
* refactor: any other code change
* chore: anything else

Examples of commit messages and PR titles:
* feat: add `--timeout` flag
* fix: error `cyclic dependency detected`
* refactor: simplify `plan` package
* chore: add unit tests job on PR

Examples of branch names:
* feat/timeout-flag
* fix/cyclic-dependency-error

## Design guidelines

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

## Style guidelines

Follow the basic Go guidelines:
* https://go.dev/doc/effective_go
* https://go.dev/wiki/CodeReviewComments

Functions/methods:
* All arguments of a **public** function are required: passing nil not allowed.
* Optional arguments of a **public** function are provided via `<MyFunctionName>Options` as the last argument.
* Avoid functional options pattern.

Constructors:
* Constructors are optional.
* Should be named `New<TypeName>[...]`, e.g. `NewResource()` and `NewResourceFromManifest()`.
* No network/filesystem calls or resource-intensive operations. Do it somewhere higher, like in `BuildResources()`.

Interfaces:
* Always add a compile-time check for each implementation, e.g. `var _ Animal = (*Dog)(nil)`.

Constants:
* Avoid `iota`.
* Prefix the enum-like constant name with the type name, e.g. `LogLevelDebug LogLevel = "debug"`.

Errors:
* Always wrap errors with additional context using `fmt.Errorf("...: %w", err)`.
* On programmer errors prefer panics, e.g. on an unexpected case in a switch.
* Do one-line `if err := myfunc(); err != nil` wherever possible.
* When wrapping errors with fmt.Errorf, describe what is being done, not what failed, e.g. `fmt.Errorf("reading config file: %w", err)` instead of `fmt.Errorf("cannot read config file: %w", err)`.

Concurrency:
* Instead of raw mutexes use the transactional `Concurrent{}` helper.
