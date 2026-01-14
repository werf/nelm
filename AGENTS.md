# AGENTS.md

## Project Context

This repository contains Nelm.

Nelm is a Helm 4 alternative. It is a Kubernetes deployment tool that manages Helm Charts and deploys them to Kubernetes. Nelm does everything that Helm does, but better, and even quite some on top of it. Nelm is based on an improved and partially rewritten Helm codebase, to introduce:

* `terraform plan`-like capabilities;
* improved CRD management;
* out-of-the-box secrets management;
* advanced resource ordering capabilities;
* advanced resource lifecycle capabilities;
* improved resource state/error tracking;
* continuous printing of logs, events, resource statuses, and errors during deployment;
* fixed hundreds of Helm bugs;
* performance and stability improvements and more.

Always read these files to understand the project context:
* ARCHITECTURE.md: Architecture.
* CONTRIBUTING.md: Contribution guidelines, code style, development workflow.
* .ai/context.md: Contains a dump of symbols, package relationships, repo file tree, etc.

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
