# Documentation Review Notes

This file tracks documentation gaps and areas needing improvement in the AI summary documentation (`docs/summary/`). It is a lightweight checklist to keep the summary docs accurate and useful.

<!-- toc -->

- [Known Gaps](#known-gaps)
- [Auto-Generation Notice](#auto-generation-notice)

<!-- tocstop -->

## Known Gaps

The following areas have been identified as needing more documentation:

- **Error Handling Patterns** - Panic usage for programmer errors, MultiError aggregation, and error wrapping conventions are not consolidated in summary docs
- **Testing Patterns** - Test file organization, fake implementations in `internal/kube/fake/`, and Ginkgo/Gomega conventions need documentation
- **Configuration Reference** - Environment variables (`NELM_*`, `HELM_*`), default values, and CLI flag mappings are scattered across files
- **Annotation Parsing Details** - Parsing rules, priority/conflict resolution, and validation timing in `internal/resource/metadata.go` are undocumented

## Auto-Generation Notice

This directory does not have a dedicated generator built into the repository; update these notes and the summary files manually when behavior or structure changes.
