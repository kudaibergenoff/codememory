# Validation record — first vertical slice

Environment: 2026-11-09, Go 1.27.3, macOS arm64, Apple M1, CGO enabled.
The project directory initially contained only `CODEMEMORY_SPEC_RU.md` and was
not a Git repository. No commits, remote pushes or published releases were made.

Executed locally:

```sh
make check
# go test -tags sqlite_fts5 ./...
# go vet -tags sqlite_fts5 ./...
# go test -race -tags sqlite_fts5 ./...
# go build -tags sqlite_fts5 -o bin/codememory ./cmd/codememory

go test -tags sqlite_fts5 -run '^$' -bench . -benchmem ./internal/parser
```

Unit/integration/race suites and build passed. Coverage includes migration
reopening/FTS deletion/project isolation, strict config, nested excludes,
symlinks, secrets, invalid UTF-8, bounded Unicode chunks, Go AST, incremental
reuse/edit/delete/rollback, stale suppression, multilingual retrieval and
concurrent search/index across two independently opened SQLite connections.

MCP tests passed using the current SDK client and a separate stdio process with
the 2025-11-25 initialized lifecycle, list/call tools, JSON-only stdout and EOF
shutdown. Invalid semantic input produces a sanitized tool error; local metrics
record successful and failed handler calls.

The actual built binary was also exercised in a temporary repository:

- First index: generation 1, indexed 1.
- No-op index: generation 1, indexed 0, reused 1.
- CLI and MCP search returned the expected relative path and hash.
- Editing withheld stale results; re-index found the new function.
- Deletion removed one file; SQLite `PRAGMA integrity_check` returned `ok`.
- One MCP request appeared in stats; verified savings remained null.
- SIGTERM after initialization closed the real server with exit code 0.

The CodeMemory repository itself was initialized and indexed: 53 files, no Go
parse errors. A second index reused all 53 without advancing generation; searching
`ConfigHash` returned `internal/indexer/indexer.go` with line ranges and SHA-256.

Parser microbenchmark (small fixture, not a performance SLA): approximately
3875 ns/op, 2232 B/op, 50 allocs/op. No token-saving claim follows from this.

Reproducibility in this restricted environment required writable temporary Go
caches and the pre-existing local module download cache. DNS for Go module
downloads was unavailable; no dependency version was represented as freshly
downloaded or independently security-audited. See ADR-001/002.

Not passed/not executed:

- Full `go mod tidy`: unavailable cached SDK test dependencies (`x/tools`, JWT);
  repeat online. Application builds and tests use committed requirements/sums.
- Skill Creator's `quick_validate.py`: Python lacks PyYAML. The short skill's
  frontmatter and instructions were reviewed directly; no automated validator
  success is claimed.
  A separate minimal frontmatter check and JSON parsing of the MCP configuration
  succeeded; these do not replace the full Skill Creator validator.
- Authenticated Claude Code/Codex sessions. Only local versions/help checked:
  Claude Code 2.1.319 and codex-cli 0.167.0.
- Five-platform CI execution, cross-platform release artifacts, dependency
  vulnerability scan, large-repository performance and full MVP acceptance.

The CI matrix is a checked-in workflow, not evidence that other platforms passed.
Both client login smoke tests remain release gates in INTEGRATIONS.md/ROADMAP.md.

## Addendum 2026-11-09 — pure-Go driver and module path

The commands above reflect the first slice. Since the switch to
`modernc.org/sqlite` (ADR-001), the tags are gone: `make check` now runs
`gofmt`, `go vet`, `go test`, `go test -race` and a `CGO_ENABLED=0` build.
All passed on macOS arm64. Network access was available: `go mod tidy`
completed and `govulncheck ./...` reported no vulnerabilities. The existing
repository database opened unchanged; a re-index and search succeeded.
The module path is now `github.com/kudaibergenoff/codememory`.
