## Project Context

This repository contains Nelm.

Always read these files to understand the project context:
* README.md: Main Readme.
* ARCHITECTURE.md: Architecture.
* CONTRIBUTING.md: Contribution guidelines, code style, development workflow.

## Code Style

Always enforce code style and design from CONTRIBUTING.md.

## Testing

* build: `task build`
* lint: `task lint`
* format: `task format`
* unit tests: `task test:unit`

## Workflow

1. Edit: Make changes.
2. Verify: Run build, format, lint, and unit tests locally.

## Additional PR review guidelines

Require explicit approval from the reviewer for:
* New external dependencies.
* Breaking user-facing changes (not API changes), unless they are hidden behind a feature flag.
* Changes that may compromise security.
