# ADR-001: transactional SQLite with an isolated driver boundary

Accepted 2026-11-09. Revised 2026-11-09: switched to the pure-Go driver.

One `.codememory/index.db` per canonical root. A versioned SQL migration creates
files, symbols, chunks, FTS5, generation and local analytics. Foreign keys and FTS
triggers delete dependent records atomically. WAL, 5-second busy timeout and one
connection per process bound contention. Write transactions use BEGIN IMMEDIATE.
Readers see complete generations; concurrent indexers serialize before scanning.

## Driver

`modernc.org/sqlite v1.61.1` (BSD-3-Clause), a CGO-free translation of SQLite
with FTS5 compiled in. Consequences:

- `go install`, `go build` and `go test` need no C compiler and no build tags.
- `CGO_ENABLED=0` builds work; verified on macOS arm64, other platforms via CI.
- Pragmas are passed as `_pragma=name(value)` DSN parameters and run on every
  new connection; `_txlock=immediate` keeps the previous locking behaviour.
- The on-disk format is plain SQLite: a database created by the earlier
  development driver opened without migration (verified locally).

v1.61.1 was chosen over the 2-day-old v1.62.0 to let the newest release settle.
`govulncheck ./...` reported no known vulnerabilities on 2026-11-09.

The driver is imported only in `internal/storage/sqlite`. No dynamic SQLite
extensions are loaded by application code. Builds use Go's checksum
verification and the committed `go.sum`.

### Superseded

The first slice used `github.com/mattn/go-sqlite3 v1.14.22` with CGO and the
`sqlite_fts5` tag because the module proxy was unreachable at the time. Builds
failed without the tag, and Windows/cross builds needed a C toolchain.

Sources checked:
- https://sqlite.org/fts5.html (external-content triggers and BM25)
- https://pkg.go.dev/modernc.org/sqlite (driver, DSN `_pragma`/`_txlock`)

The specification's six delivery phases and its final minimal-slice instruction
have different scopes. This delivery follows §15's first slice, with analytics
instrumentation from the start. Complete MVP acceptance stays unchecked.
