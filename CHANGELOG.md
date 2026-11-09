# Changelog

## Unreleased

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
