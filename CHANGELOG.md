# Changelog

## Unreleased

- Code graph (schema v8, parser v3): `memory_graph` MCP tool and `codememory graph`
  return a Go symbol's definition, callers to depth 3 with call sites, and what it
  uses; without a symbol, the most referenced symbols of a scope. Edges are go/ast
  references stored in SQLite (`refs`), walked level by level with a visited set;
  method calls are matched by name and marked `by_name`; `dir` disambiguates. Method symbols are now named
  `Recv.Method`. Cost on 10k files: first index +26% time, +52% peak RSS, +51% DB.

- Compound identifiers in queries and tasks (`memory_context`, `pkg.Func`,
  `a/b.go`) are kept as FTS5 phrases: chunks containing them verbatim get 2x BM25
  strength, so their split parts or a symbol-column hit no longer outrank them
  (`internal/mcp/server.go` for "memory_architecture handler": rank 16 -> 1).
  Query-cache keys carry a results version, so upgrades never serve old rankings.

- `memory_status`: database size, cache stats, files per language, supported
  languages. Events: config version, invalidations, keyed repeat fingerprints and
  measured metrics overhead (schema v7). Test files are demoted in memory_context.
  Prompt-injection test for indexed files.

- Large repositories: per-directory `os.Root` walk (search 0.65 s -> 0.10 s on the
  10k-file Go stdlib) and lower first-index memory (376 -> 260 MiB peak).

- Release packaging: `make dist`, tag-triggered release workflow, CGO-free
  archives for 5 platforms with SHA256SUMS and THIRD_PARTY_LICENSES; README rewritten.

- memory_context drops candidates below 30% of the best BM25 strength instead of
  padding the budget with weak matches (fixture median response -23%, recall kept).

- `benchmark` (embedded fixture suite with negative cases, or `--tasks`),
  read-only `optimize` suggestions and a loopback-only read-only `dashboard`.
- Context packing measures responses incrementally (7.2 ms -> 0.9 ms per pack).

- `install claude|codex` (dry-run diffs, `--yes` to apply, idempotent marked
  blocks, atomic writes, symlink-safe), read-only `doctor`, and `clean`
  (index only by default; `--all` removes the state directory).

- Windows: valid SQLite URI (`file:///C:/...`); CI is green on all five platforms.
- Analytics: numeric event time and agent attribution (schema v5); `stats`
  filters `--today`, `--last`, `--agent`; `report --format json|csv`.

- Query cache (schema v4) for `memory_search` and `memory_context`, bound to the
  index generation and memory revision; `stats` reports hits, misses and rate.

- `memory_architecture` MCP tool and `codememory architecture`: modules, Go
  packages and internal imports from go/ast (parser v2), plus scoped memories.
- `memory_context` includes applicable curated memories by default.

- `memory_remember` MCP tool and `codememory remember`: curated memories with
  file SHA-256 provenance (schema v3); `sourced`/`unverified`, `stale` derived on
  read; same scope+title updates in place; secret-like text refused.

- `memory_context` MCP tool and `codememory context`: task-driven, deduplicated,
  ranked snippets packed so the encoded JSON response stays within `max_tokens`.
- JSON responses are no longer HTML-escaped (`<`, `>`, `&` cost 6 bytes each).

- Pure-Go SQLite (`modernc.org/sqlite`): no CGO or build tags; module path
  `github.com/kudaibergenoff/codememory`.
- Search refreshes changed files before querying instead of withholding all
  results when any file changed. Size/mtime prefilter (schema v2) avoids reading
  unchanged files; ~30x faster status/search on a 3000-file synthetic repository.

- Foundation: Go CLI, strict project configuration, SQLite/FTS5 migration,
  project isolation and local analytics with explicit unavailable usage values.
- Architecture and integration research recorded in ADRs.
- Safe bounded scanner, nested ignores, SHA-256 incremental indexing, Go AST,
  UTF-8 text chunks, FTS5 ranked search and explicit freshness checks.
- MCP stdio search/status, client integration examples and source provenance.
- Unit, protocol subprocess, multilingual and concurrency tests; CI definition.
