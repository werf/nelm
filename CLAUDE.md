# Claude Code Instructions

Follow @AGENTS.md for universal project guidelines.

## Exploring the codebase

- To find definitions, references, callers, or implementations → use **LSP** (`goToDefinition`, `findReferences`, `incomingCalls`/`outgoingCalls`, `workspaceSymbol`).
- To find code by intent or understand how something works → use **CodeAlive MCP** (`codebase_search`, `codebase_consultant`). Call `get_data_sources` first.
- To explore broad architecture across many files → use **Task/Explore** subagent as a last resort.

## External documentation

- To look up documentation for any technology, library, or tool → use **Context7 MCP** (`resolve-library-id` + `query-docs`).

## E2E testing

After making changes, verify them end-to-end against the local dev cluster:

1. `task build` — build the binary.
2. Run the built binary from `./bin/nelm` against the cluster to deploy/test.
3. Use **Kubernetes MCP tools** (`mcp__kubernetes__*`) to inspect the cluster state — get resources, describe them, check logs, exec into pods, etc. Prefer these over raw `kubectl` via Bash.

## Code style

- When writing or modifying Go code, strictly follow @CODESTYLE.md.
