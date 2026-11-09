# Progress and acceptance gates

This is the first vertical slice requested in SPEC §15, not the complete MVP.

1. Foundation: configuration, SQLite migrations, CLI and local analytics schema.
   Gate: idempotent init; migration reopen; FTS trigger deletion; isolation; tests and build.
2. Code intelligence: safe scanner, SHA-256, incremental transactions, Go AST and FTS ranking.
   Gate: no-op preserves chunks/generation; edit/delete update search; ignored/secret/symlink files excluded.
3. MCP slice: search/status tools over stdio, integration examples and protocol tests.
   Gate: real subprocess handshake, list/call, bounded inputs, clean stdout and shutdown.
4. Persistent memory/context: provenance, context JSON budget, dependency facts, query cache.
   Gate: stale source cannot silently appear fresh; invalidation and budget tests.
5. Full analytics: comparable baselines, JSON/CSV, loopback dashboard, optimize recommendations.
   Gate: measured/estimated/verified separation, null without actual usage, quality comparisons.
6. Release: both authenticated clients, cross-platform native builds, security/performance review.
   Gate: manual subscription smoke tests and CI matrix; no unsupported release claims.

Foundational analytics is implemented alongside storage; subsequent phases extend it.
Remaining MVP commands are not registered as fake implementations.

## Delivered in this slice

- [x] Foundation acceptance: CLI, strict configuration, private state, versioned
  migration, foreign keys, FTS5 triggers and project-root isolation.
- [x] Incremental scanner/indexer: limits, nested ignores, secret heuristics,
  symlinks, SHA-256, no-op reuse, edit/delete, rollback and generation versioning.
- [x] Go AST symbols; bounded Unicode text chunks for other languages.
- [x] FTS5/BM25 search with filters, provenance and refresh-before-search freshness
  (size/mtime prefilter, racy-mtime guard, cached binary/secret skips).
- [x] Two MCP tools with SDK and subprocess protocol integration tests.
- [x] Local analytics events/retention and `stats`, measured/estimated/null verified.
- [x] Claude Skill, Codex instructions and manual configuration examples.
- [x] Unit/integration/concurrency/race checks, parser microbenchmark, local build.
- [x] Native CI definition for Linux amd64/arm64, macOS amd64/arm64, Windows amd64.

## Next acceptance gates (not completed)

- [x] Context budget including serialized JSON; dedup; `memory_context` and
  `codememory context` (fixtures: payment files in top 5).
- [x] Architecture context (`include_architecture`, default true) backed by curated memories.
- [x] Persistent decisions with provenance: `memory_remember`, sourced/unverified/stale.
- [x] Architecture/dependency facts (`memory_architecture`), Go imports from go/ast.
- [x] Generation-bound query cache, TTL, invalidation and reuse metrics.
- [ ] Remaining three MCP tools and complete client/session analytics attribution.
- [ ] Reproducible baseline harness with quality criteria, negative cases and
  optional official actual-usage import; no fabricated verified savings.
- [x] JSON/CSV report; stats filters; agent attribution from MCP clientInfo.
- [x] Loopback dashboard and evidence-based optimization advice.
- [x] `doctor`, `clean`, opt-in idempotent installers; analytics filters.
- [x] Fresh network `go mod tidy`, `govulncheck` clean, pure-Go SQLite driver.
- [ ] Dependency license audit.
- [ ] Execute five-platform CI and release packaging (definition alone is not proof).
- [ ] Authenticated smoke test in Claude Code and Codex CLI using subscription login.
- [ ] Performance profile on large repositories and scanner policy/ignore review.

The complete Phase 2 (context) and Phase 3 (all tools/authenticated clients) gates
remain open. This slice proves the smaller §15 scan → SQLite → search → MCP path;
it does not claim all six product phases or production readiness.
