# ADR-001: transactional SQLite with an isolated driver boundary

Accepted for the development slice, 2026-11-09; release driver review remains open.

One `.codememory/index.db` per canonical root. A versioned SQL migration creates
files, symbols, chunks, FTS5, generation and local analytics. Foreign keys and FTS
triggers delete dependent records atomically. WAL, 5-second busy timeout and one
connection per process bound contention. Write transactions use BEGIN IMMEDIATE.
Readers see complete generations; concurrent indexers serialize before scanning.

Preferred production candidate is pure-Go `modernc.org/sqlite`. Its module could
not be downloaded in this environment (DNS unavailable); only its metadata was cached.
For a testable first slice use locally cached `github.com/mattn/go-sqlite3 v1.14.22`
(MIT), compiled with CGO and `sqlite_fts5`. This is an old development pin, not a
claim of current security suitability. Upgrade/audit or switch drivers before release.
No dynamic SQLite extensions are loaded by application code.

The driver is imported only in storage. Native CI runners are needed for release
builds until the pure-Go candidate is evaluated. Do not advertise CGO-free builds.
The local offline bootstrap used the pre-existing module cache and GOSUMDB=off;
normal builds retain Go's checksum verification and committed go.sum.

Sources checked:
- https://sqlite.org/fts5.html (external-content triggers and BM25)
- https://github.com/mattn/go-sqlite3/blob/v1.14.22/README.md (CGO, FTS5, build tags)
- https://pkg.go.dev/modernc.org/sqlite (candidate driver)

The specification's six delivery phases and its final minimal-slice instruction
have different scopes. This delivery follows §15's first slice, with analytics
instrumentation from the start. Complete MVP acceptance stays unchecked.
