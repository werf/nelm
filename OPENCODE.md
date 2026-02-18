## Exploring the codebase (MANDATORY)

These are NOT suggestions — they are requirements.

- ALWAYS use **LSP** (`goToDefinition`, `findReferences`, `goToImplementation`, `hover`, `incomingCalls`/`outgoingCalls`, `documentSymbol`, `workspaceSymbol`) for navigating code. NEVER use `grep` for finding definitions, references, implementations, or callers — LSP is precise, `grep` is a guess.
  - To find a symbol's definition when you have a call site: use `goToDefinition`.
  - To find a symbol's definition without a call site: use `workspaceSymbol`. If it returns no results, fall back to `grep`.
  - To understand a file's structure: use `documentSymbol`.
  - To find interface implementations: use `goToImplementation`.
  - To check types or godoc: use `hover`.
- ALWAYS use **CodeAlive MCP** (`codealive_codebase_search`, `codealive_codebase_consultant`) for semantic/intent-based code search. Call `codealive_get_data_sources` first. NEVER substitute with `grep` when the query is about intent or behavior.
- Only fall back to `grep`/`glob` for simple literal pattern matching (specific strings, config keys, error messages).
- Only use `task` subagent (`explore`/`general`) for broad codebase exploration or multi-step investigation when the above tools are insufficient.

## External documentation (MANDATORY)

These are NOT suggestions — they are requirements.

- ALWAYS use **LSP** `hover` to look up type signatures and godoc for any symbol (including external dependencies).
- Use **Context7 MCP** (`context7_resolve-library-id` + `context7_query-docs`) for library documentation, guides, or examples.
- Use **codesearch** (Exa) for finding code examples, API references, or documentation for libraries and tools.
- Use **websearch** for current information, recent changes, or topics beyond training data.
- Use **webfetch** to retrieve content from a specific URL (docs page, issue, PR).
- NEVER guess at APIs — look them up.

## Verifying changes (MANDATORY)

ALWAYS verify after making changes:

- ALWAYS run `task build` — verify it compiles.
- ALWAYS run `task format` — fix formatting.
- ALWAYS run `task lint:golangci-lint` — verify linting passes.
- ALWAYS run `task test:unit` — verify tests pass.

When changes affect runtime behavior, ALSO verify against the local dev cluster:

- ALWAYS run `./bin/nelm` against the cluster to deploy/test.
- ALWAYS use **Kubernetes MCP tools** (`kubernetes_*`) to inspect cluster state. NEVER use raw `kubectl` via Bash when an MCP tool can do it.
