<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->

- [Before you start](#before-you-start)
- [Development](#development)
- [Commit/branch/PR naming](#commitbranchpr-naming)
- [Code style](#code-style)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Before you start

For any significant change, open the issue first (or comment on an existing one) to discuss with the maintainers on what to do and how. When the solution is agreed upon, you can proceed with implementation and open a pull request.

For small changes, such as few lines bugfixes or documentation improvements, feel free to open a pull request directly.

For easy first issues, check the [good first issue](https://github.com/werf/nelm/issues?q=is%3Aissue%20state%3Aopen%20label%3A%22good%20first%20issue%22) tag.

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

## Code style

See [CODESTYLE.md](CODESTYLE.md).
