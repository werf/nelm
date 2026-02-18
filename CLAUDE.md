# Claude Code Instructions

ALWAYS strictly follow @AGENTS.md for universal project guidelines.

## Exploring the codebase (MANDATORY)

These are NOT suggestions — they are requirements. NEVER skip them in favor of Grep/Glob.

- ALWAYS use **LSP** (`goToDefinition`, `findReferences`, `incomingCalls`/`outgoingCalls`, `workspaceSymbol`) to find definitions, references, callers, or implementations. NEVER use Grep for these — LSP is precise, Grep is a guess.
  - To find a symbol's definition when you have a call site: use `goToDefinition` on the call site.
  - To find a symbol's definition when you don't have a call site: use `workspaceSymbol` first. If that fails, fall back to Grep.
- ALWAYS use **CodeAlive MCP** (`codebase_search`, `codebase_consultant`) to find code by intent or understand how something works. Call `get_data_sources` first. NEVER substitute this with Grep-based keyword searching when the query is semantic/intent-based.
- Only fall back to **Grep/Glob** for simple literal pattern matching (e.g., finding a specific string, config key, or error message).
- Only use **Task/Explore** subagent as a last resort when the above tools are insufficient.

## External documentation (MANDATORY)

These are NOT suggestions — they are requirements. NEVER skip them in favor of Grep.

- ALWAYS use **LSP** `hover` to look up documentation, type signatures, and godoc for any symbol in the codebase (including symbols from external dependencies)
- ALWAYS use **Context7 MCP** (`resolve-library-id` + `query-docs`) to look up broader documentation, guides, or examples for any technology, library, or tool. NEVER guess at APIs — look them up.

## Verifying changes (MANDATORY)

ALWAYS verify after making changes:

- ALWAYS run `task build` — verify it compiles.
- ALWAYS run `task format` — fix formatting.
- ALWAYS run `task lint` — verify linting passes.
- ALWAYS run `task test:unit` — verify tests pass.

When changes affect runtime behavior, ALSO verify against the local dev cluster:

- ALWAYS run `./bin/nelm` against the cluster to deploy/test.
- ALWAYS use **Kubernetes MCP tools** (`mcp__kubernetes__*`) to inspect cluster state. NEVER use raw `kubectl` via Bash when an MCP tool can do it.
