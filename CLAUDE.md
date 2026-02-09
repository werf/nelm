# Claude Code Instructions

ALWAYS strictly follow @AGENTS.md for universal project guidelines.

## Exploring the codebase (MANDATORY)

These are NOT suggestions — they are requirements. NEVER skip them in favor of Grep/Glob.

- ALWAYS use **LSP** (`goToDefinition`, `findReferences`, `incomingCalls`/`outgoingCalls`, `workspaceSymbol`) to find definitions, references, callers, or implementations. NEVER use Grep for these — LSP is precise, Grep is a guess.
- ALWAYS use **CodeAlive MCP** (`codebase_search`, `codebase_consultant`) to find code by intent or understand how something works. Call `get_data_sources` first. NEVER substitute this with Grep-based keyword searching when the query is semantic/intent-based.
- Only fall back to **Grep/Glob** for simple literal pattern matching (e.g., finding a specific string, config key, or error message).
- Only use **Task/Explore** subagent as a last resort when the above tools are insufficient.

## External documentation (MANDATORY)

These are NOT suggestions — they are requirements. NEVER skip them in favor of Grep.

- ALWAYS use **LSP** `hover` to look up documentation, type signatures, and godoc for any symbol in the codebase (including symbols from external dependencies).
- ALWAYS use **Context7 MCP** (`resolve-library-id` + `query-docs`) to look up broader documentation, guides, or examples for any technology, library, or tool. NEVER guess at APIs — look them up.

## Verifying changes (MANDATORY)

After making changes, ALWAYS verify them end-to-end against the local dev cluster:

1. `task build` — build the binary.
2. Run the built binary from `./bin/nelm` against the cluster to deploy/test.
3. ALWAYS use **Kubernetes MCP tools** (`mcp__kubernetes__*`) to inspect the cluster state — get resources, describe them, check logs, exec into pods, etc. NEVER use raw `kubectl` via Bash when an MCP tool can do it.

## Code style (MANDATORY)

- ALWAYS strictly follow @CODESTYLE.md when writing or modifying Go code. Every rule in it is a requirement. Read it before writing any code if you haven't already in this session.
