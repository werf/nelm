# Architecture Decision Records (ADRs)

This directory contains Architecture Decision Records (ADRs) for the Nelm project. ADRs document important architectural decisions, their context, and consequences.

## What are ADRs?

Architecture Decision Records are documents that capture important architectural decisions made along with their context and consequences. They help:

- Document the "why" behind architectural choices
- Provide historical context for future developers
- Enable better decision-making by learning from past decisions
- Support onboarding of new team members

## ADR Format

Each ADR follows a standard format:
- **Status**: Proposed, Accepted, Deprecated, or Superseded
- **Context**: The situation and problem statement
- **Decision**: The architectural decision made
- **Consequences**: Positive and negative impacts

## Index of ADRs

| Number | Title              | Status | Date       |
|--------|--------------------|--------|------------|
| [0001](0001-nelm-plan-freezing.md) | Nelm plan freezing | Proposed | 22-01-2026 |

## ADR Details

### [ADR-0001: Nelm plan freezing](0001-nelm-plan-freezing.md)

**Status**: Proposed

**Summary**: Implement a **plan freezing and verification mechanism** that exports a release install plan to a JSON artifact during `nelm release plan install` and then **rebuilds a fresh plan** during `nelm release install --plan-file=...`, comparing the two plans before deploying.

**Key Features**:
- Plan serialization in JSON format
- Release install considering previously reviewed plan
- CLI support for `--plan-file` flag in `release plan install` and `release install` commands

## Contributing

When creating a new ADR:

1. Use the next sequential number (e.g., `0002-` for the next ADR)
2. Use kebab-case for the filename
3. Follow the standard ADR format
4. Update this README with the new ADR entry
5. Set initial status to "Proposed"

## References

- [Documenting Architecture Decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) by Michael Nygard
- [ADR GitHub Organization](https://adr.github.io/)
- [Nelm Architecture Documentation](../../ARCHITECTURE.md)
