# Changelog

## Unreleased

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
