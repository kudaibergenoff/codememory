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

## Addendum 2026-11-09 — CI and benchmark

CI is green on Linux amd64/arm64, macOS arm64/amd64 and Windows amd64 after two
Windows fixes (LF line endings via .gitattributes; `file:///C:/...` SQLite URI).

`codememory benchmark` (estimates from local response sizes, not billed tokens):

- Built-in fixture (14 tiny files): 7/7 tasks pass, mean recall 1.0, but the
  median estimated savings vs. the lexical baseline is -187%: responses carry a
  fixed overhead and fill the budget, which exceeds reading a few small files.
- This repository at cbac858 (79 files), 6 tasks with known answer files: recall
  1.0 on all; context about 1,470-1,500 tokens vs. a lexical baseline of
  83,000-97,500 tokens (about 98% estimated, an upper bound because agents rarely
  read every match) and an oracle of 590-3,360 tokens for reading exactly the
  right files. A negative task failed correctly: its phrase exists in this
  repository's own test data.

No agent-level token savings are claimed; client usage data is unavailable.

Relevance cutoff (candidates below 30% of the best BM25 strength are dropped),
chosen by sweeping 0-0.6 on both suites: on the fixture the median positive
response fell from 575 to 442 estimated tokens with recall 1.0 (0.6 lost an
expected file); on this repository results were unchanged (recall 1.0, budget
filled by relevant snippets). Fixture savings vs. the lexical baseline remain
negative: the remaining cost is per-response provenance overhead.

## Addendum 2026-11-09 — release packaging and licenses

`make dist VERSION=v0.0.0-test` built CGO-free archives for linux amd64/arm64,
darwin amd64/arm64 and windows amd64 (13-14 MB binaries); `shasum -c SHA256SUMS`
passed and the darwin/arm64 binary reported the injected version. Every module
linked into the binary is MIT, BSD-3-Clause or Apache-2.0 (MCP Go SDK; no NOTICE
file); their texts are bundled as THIRD_PARTY_LICENSES. Binaries are not
code-signed. The release workflow has not run yet: it needs a pushed `v*` tag.

## Addendum 2026-11-09 — large repository profile

Corpus: a copy of the Go 1.27.3 standard library source (11,964 files, 156 MB;
10,017 indexed, 800 skipped, 2 secret-like). macOS arm64, CLI process per call
(the MCP server avoids process start-up).

| Operation | Before | After |
|---|---|---|
| first index | 25 s, 376-400 MiB peak | 26 s, 260 MiB peak |
| search / status (no changes) | 0.65 s | 0.10 s / 0.17 s |
| context (cache miss) | 0.68 s | 0.30 s |
| search after editing one file | 0.70 s | 0.18 s |

A CPU profile showed 92% of refresh time in `openat`: `os.Root.Lstat` resolved
every path from the root, one `openat` per segment. The walker now opens an
`os.Root` per directory and looks names up as single components. File bytes are
released right after classification/parsing. The database is about the size of
the indexed text (160 MB here: chunk text plus the FTS index). Observed ranking
issue: test files can outrank their implementation (`server_test.go` first for
an HTTP routing task).
