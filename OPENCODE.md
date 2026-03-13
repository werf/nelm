## Hard blocks (MANDATORY)

- When you are about to use the wrong tool — STOP. Use the correct tool listed below. Do NOT proceed with the wrong tool even if it seems faster.
- If you already used the wrong tool — STOP and redo the step with the correct tool. Do NOT continue with the result from the wrong tool.
- NEVER use `explore` for intent-based or behavioral queries (e.g. "how does X work?", "find the orchestration flow for Y"). `explore` can only use grep/glob/ast_grep. For intent/behavior queries, ALWAYS use `codealive_codebase_search` directly or `task(category="quick"/"deep")`.

## Code navigation (MANDATORY)

ALWAYS use **LSP** for navigating code. NEVER use `grep` for finding definitions, references, or implementations — LSP is semantically precise, `grep` matches strings blindly and gives false positives.

Default action when unsure: ALWAYS use LSP.
Tool priority order: `lsp(operation=...)` → `grep` (ONLY as a fallback when LSP returns no results).

IMPORTANT: OMO's built-in LSP tools (`lsp_goto_definition`, `lsp_find_references`, `lsp_symbols`, `lsp_diagnostics`, `lsp_prepare_rename`, `lsp_rename`) are DISABLED via `disabled_tools` in `oh-my-opencode.json` because they use a separate gopls instance that does not support `go.work` workspaces. ALWAYS use the OpenCode `lsp(operation=...)` tool instead. NEVER use `lsp_*` prefixed tools — they will not be available.

- When you want to find where a function/type/variable is defined and you have a call site — NEVER `grep` for it. ALWAYS use `lsp` with `operation="goToDefinition"`. It jumps to the exact definition, even across packages and `go.work` modules.
- When you want to find where a symbol is defined but you don't have a call site — NEVER `grep` for it. ALWAYS use `lsp` with `operation="workspaceSymbol"`. Fall back to `grep` ONLY if LSP returns no results.
- When you want to find all usages of a symbol — NEVER `grep` for the symbol name. ALWAYS use `lsp` with `operation="findReferences"`. Grep will match comments, strings, and unrelated identifiers with the same name.
- When you want to understand what's in a file — NEVER scroll through it or `grep` for `func`. ALWAYS use `lsp` with `operation="documentSymbol"`. It returns the complete structure: functions, types, constants, variables.
- When you want to check a symbol's type or read its documentation — NEVER guess from context. ALWAYS use `lsp` with `operation="hover"`. It returns the exact type signature and godoc.
- When you want to find which types implement an interface — NEVER `grep` for type names. ALWAYS use `lsp` with `operation="goToImplementation"`. Grep cannot reliably find implicit Go interface implementations.
- When you want to trace what calls a function — NEVER `grep` for the function name. ALWAYS use `lsp` → `prepareCallHierarchy` → `incomingCalls`. Grep will miss method calls, aliased imports, and interface dispatch.
- When you want to trace what a function calls — NEVER read through the function body manually. ALWAYS use `lsp` → `prepareCallHierarchy` → `outgoingCalls`.
- When you want to rename a symbol — NEVER find-and-replace. ALWAYS use `ast_grep_replace` for safe AST-aware renaming.
- When you want to check for errors before building — NEVER skip this step. ALWAYS run `task build` and `task lint:golangci-lint`.

## Code search (MANDATORY)

ALWAYS use **CodeAlive MCP** (`codealive_codebase_search`, `codealive_codebase_consultant`) for semantic/intent-based code search. NEVER substitute with `grep` when the query is about intent or behavior — CodeAlive understands code semantics, `grep` only matches character sequences.

Default action when unsure: ALWAYS use `codealive_codebase_search`.
Tool priority order: `codealive_codebase_search` / `codealive_codebase_consultant` → `ast_grep_search` → `grep` / `glob`.

- ALWAYS call `codealive_get_data_sources` before any CodeAlive tool. Without this, CodeAlive calls will fail.
- When you want to find code by intent or behavior (e.g. "how does release planning work?", "where is the DAG built?") — NEVER `grep` for keywords. ALWAYS use `codealive_codebase_search`. Grep will miss relevant code that uses different terminology, and drown you in irrelevant string matches.
- When you want architectural advice or explanations (e.g. "why is the DAG built this way?", "how do these packages relate?") — NEVER guess from reading a few files. ALWAYS use `codealive_codebase_consultant`. It has indexed the entire codebase and understands cross-cutting concerns.
- When you want to find structural code patterns (e.g. all functions with a specific signature, all `fmt.Errorf(... %w ...)` calls, all interface implementations) — NEVER use `grep` with regex hacks. ALWAYS use `ast_grep_search`. It matches on the AST, not on text, so it won't be fooled by comments, strings, or formatting differences.
- ONLY fall back to `grep`/`glob` for simple literal matching (specific strings, config keys, error messages, annotation names). This is the ONLY valid use of `grep` in this codebase.
- When delegating code search to subagents — NEVER use `explore` for CodeAlive or LSP searches. The `explore` agent can only use grep/glob/ast_grep (OMO upstream limitation). ALWAYS use `task(category="quick")` or `task(category="deep")` for semantic search. The `librarian` agent has full CodeAlive/Context7 access and works correctly.
- NEVER use `explore` for intent-based or behavioral queries (e.g. "how does X work?", "find the orchestration flow for Y"). These require CodeAlive, which `explore` cannot access. ALWAYS use `task(category="quick")` or `task(category="deep")` instead, or do the `codealive_codebase_search` yourself. Reserve `explore` ONLY for literal pattern matching (specific identifiers, strings, config keys).

## Subagent tool capabilities (MANDATORY)

- `explore` agent can ONLY use grep/glob/ast_grep. It CANNOT use CodeAlive, LSP, or Context7. NEVER delegate intent-based or behavioral queries to `explore` — it will fail silently or return irrelevant grep matches.
- For semantic/intent-based code search, ALWAYS use `task(category="quick"/"deep")` or do `codealive_codebase_search` directly. NEVER use `explore` as a substitute.
- Tool rules for subagents are injected via `prompt_append` in `oh-my-opencode.json` — NEVER copy-paste tool rules into delegation prompts.
- ALWAYS PREFER direct tools (`codealive_codebase_search` + `LSP` + `read`) over `explore` agents for targeted queries. Direct tools are faster and 100% reliable. Reserve `explore` ONLY for broad multi-angle discovery where 3+ different literal search patterns are needed simultaneously.

## External knowledge (MANDATORY)

NEVER guess at APIs — ALWAYS look them up. Using wrong API signatures wastes time on compilation errors and subtle bugs.

Default action when unsure: ALWAYS use `context7_resolve-library-id` + `context7_query-docs`.
Tool priority order: `lsp` with `operation="goToDefinition"` → Context7 → `grep_app_searchGitHub` → `websearch_web_search_exa`. If you have a URL, ALWAYS use `webfetch`.

- When you want to check a Go type signature or read godoc for a dependency — NEVER guess from memory or training data. ALWAYS use `lsp` with `operation="goToDefinition"` to navigate to the actual source. Training data may be outdated or wrong.
- When you want library documentation, guides, or API examples — NEVER rely on training data. ALWAYS use `context7_resolve-library-id` + `context7_query-docs`. Context7 has up-to-date docs; your training data may be stale.
- When you want real-world usage patterns (how do other projects use this library?) — NEVER invent patterns. ALWAYS use `grep_app_searchGitHub`. It searches real code from real repositories.
- When you need current information, recent changes, or anything that might have changed after your training cutoff — NEVER answer from memory. ALWAYS use `websearch_web_search_exa`.
- When you have a specific URL to read (docs page, GitHub issue, PR) — NEVER summarize from memory. ALWAYS use `webfetch` to retrieve the actual content.

## Verifying changes (MANDATORY)

ALWAYS verify after making changes, in this order. NEVER skip steps. NEVER assume "it probably compiles."

Default action when unsure: ALWAYS run the full verification pipeline.
Tool priority order: `task format` → `task build` → `task lint:golangci-lint` → `task test:unit`.

1. ALWAYS run `task format` first — it mutates files, so other checks must run after it.
2. ALWAYS run `task build` — verify it compiles. NEVER assume your changes compile without checking.
3. ALWAYS run `task lint:golangci-lint` — verify linting passes. NEVER ignore lint errors.
4. ALWAYS run `task test:unit` — verify tests pass. NEVER skip tests.

Scope verification with `paths=` for focused changes (e.g. `task lint:golangci-lint paths="./pkg/foo/..."`). ALWAYS run full-project verification at the end of a task.

When changes affect CLI commands, deployment logic, or Kubernetes interactions, ALSO verify against the local dev cluster:

- ALWAYS run `./bin/nelm` against the cluster to deploy/test.
