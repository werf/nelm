## Exploring the codebase (MANDATORY)

These are NOT suggestions — they are requirements.

- ALWAYS use **LSP** for navigating code. NEVER use `grep` for finding definitions, references, or implementations — LSP is precise, `grep` is a guess.
  - To find a symbol's definition when you have a call site: use `lsp_goto_definition`.
  - To find a symbol's definition without a call site: use `lsp_symbols` with `scope="workspace"`. If it returns no results, fall back to `grep`.
  - To find all usages/references of a symbol: use `lsp_find_references`.
  - To understand a file's structure: use `lsp_symbols` with `scope="document"`.
  - To check for errors/warnings before building: use `lsp_diagnostics`.
- ALWAYS use **CodeAlive MCP** (`codealive_codebase_search`, `codealive_codebase_consultant`) for semantic/intent-based code search. Call `codealive_get_data_sources` first. NEVER substitute with `grep` when the query is about intent or behavior.
- Use `ast_grep_search` for structural code pattern matching (e.g. finding all functions matching a signature, all error wrapping patterns, all usages of a specific call shape).
- Only fall back to `grep`/`glob` for simple literal pattern matching (specific strings, config keys, error messages).
- Only use `task` subagent (`explore`) for broad codebase exploration or multi-step investigation when the above tools are insufficient.

## External documentation (MANDATORY)

These are NOT suggestions — they are requirements.

- Use `lsp_goto_definition` to navigate to source and read godoc/type signatures directly.
- Use **Context7 MCP** (`context7_resolve-library-id` + `context7_query-docs`) for library documentation, guides, or examples.
- Use `grep_app_searchGitHub` for finding real-world code examples and usage patterns from public GitHub repositories.
- Use `websearch_web_search_exa` for current information, recent changes, or topics beyond training data.
- Use **webfetch** to retrieve content from a specific URL (docs page, issue, PR).
- NEVER guess at APIs — look them up.

## Verifying changes (MANDATORY)

ALWAYS verify after making changes:

- ALWAYS run `task format` — fix formatting (this mutates files, so run first).
- ALWAYS run `task build` — verify it compiles.
- ALWAYS run `task lint:golangci-lint` — verify linting passes.
- ALWAYS run `task test:unit` — verify tests pass.

For focused changes, scope verification with `paths=` to avoid slow full-project runs (e.g. `task lint:golangci-lint paths="./pkg/foo/..."`). Run full-project verification at the end of a task.

When changes affect CLI commands, deployment logic, or Kubernetes interactions, ALSO verify against the local dev cluster:

- ALWAYS run `./bin/nelm` against the cluster to deploy/test.
- ALWAYS use **Kubernetes MCP tools** (`kubernetes_*`) to inspect cluster state. NEVER use raw `kubectl` via Bash when an MCP tool can do it.
