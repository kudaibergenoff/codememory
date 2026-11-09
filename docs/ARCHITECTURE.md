# Architecture

The executable is a composition root. `cli` parses explicit command flags and
opens one validated project. `config` owns defaults, strict JSON and state paths.
`scanner` enforces hard denies, nested ignore rules, UTF-8/binary/secret checks,
bounded reads and SHA-256. `parser` extracts Go AST facts and line chunks for text.
`indexer` coordinates work; `storage/sqlite` owns SQL, migration, publication,
FTS triggers, project identity and transaction boundaries. `search` validates
queries, constructs literal terms and checks freshness. `analytics` records local
payload/work measurements. `mcp` adapts the same services to SDK stdio tools.

Each database belongs to a single canonical root. A writer takes SQLite's
reservation before scanning: an older scan cannot overwrite a newer published
generation. The entire update commits or rolls back. Hash + parser/config version
controls parse reuse. Renames are delete + insert. Unchanged chunks keep their IDs.
All eligible files are read to detect content changes even with unchanged mtimes.

FTS5 stores an external-content index maintained by insert/delete triggers.
BM25 weights are path=5, symbol=8, body=1; lower score ranks first. Literal query
terms use OR and deterministic path/line/id tie-breaking. Search filters are SQL
parameters. Text from all supported languages is chunked; only Go gets AST facts.

Freshness uses a size/mtime prefilter with SHA-256 as the identity. Each scan
walks the permitted tree with `lstat` only, through an `os.Root` opened per
directory so every lookup is a single path component (one `fstatat` per file); a file is read and hashed when its
size or mtime differs from the recorded one, and re-parsed only when its hash
differs. Files modified within 2 s of a scan are recorded as unsettled (mtime 0)
and re-read next time, so a same-size edit inside one timestamp tick is not
missed (the git "racy clean" rule). Binary and secret-like files are cached in
`skipped_files` by the same stat so they are not re-read either.

Search refreshes first: it runs the incremental index under the writer lock,
then queries the published generation, so results reflect the files on disk at
refresh time and edited/deleted/newly ignored files never leak. Status is
read-only: it computes the same diff without writing and reports `is_stale`.
A content change that keeps both size and mtime (`cp -p`, `rsync -t`, backup
restores) is not detected until the stat changes; run `codememory index --full`
after such operations. A config change, a scanner rule change or a new secret
pattern (fingerprinted in `scanner.RulesVersion`) forces a full re-read, so
tightened exclusion rules also apply to unchanged files.

If the refresh fails (writer busy beyond the 5 s timeout, size limit, files
replaced mid-scan), search returns `is_stale=true` with no snippets and a
warning; details go to the local stderr log. Read transactions use a deferred
BEGIN, so searches never wait for an indexer's writer lock.

Context assembly (`internal/contextpack`): task words minus English/Russian
stopwords become FTS5 prefix terms (`"refund"*` also matches the identifier token
`refundpayment`; the tokenizer does not split camelCase). Up to 60 BM25-ranked
chunks have test files demoted (BM25 strength x0.6 for `_test.go`, `test_*.py`,
`*.spec.ts`, `tests/` and similar, which otherwise outrank the code they test),
are filtered to those with at least 30% of the best match's BM25
strength (weak matches no longer pad the budget), then deduplicated (no overlapping ranges, at most 2 per file) and packed
greedily: a few ranked file names (15% reserve), then snippets in rank order,
truncated to a line prefix when they do not fit whole, then more file names.
Every step measures the *encoded* response with `utf8-bytes-div-3-ceil-v1`, and
counters known only at the end are measured at upper bounds, so the delivered
JSON never exceeds `max_tokens` (500 to 8000) by this estimate; if even the empty
envelope does not fit, an error is returned instead. The top snippet is usually
but not always included: each snippet carries ~130 tokens of provenance. It is a
size proxy, not the model's tokenizer. Task words follow the unicode61 tokenizer's
boundaries (letters, numbers and combining marks are word characters). Responses are encoded without HTML escaping (`<`, `>`, `&`
would otherwise cost 6 bytes each).

Memories (`internal/memory`, schema v3) cite files by SHA-256. Status is
`sourced` or `unverified` at write time; `stale` is derived on every read when a
cited file's indexed hash differs or it left the index. Scopes are directory-like:
a memory applies to its scope, ancestors' memories apply to descendants, and
`*` (the default, not a valid path, so no directory can collide with it) applies
everywhere. Scopes must be clean relative directories. A memory saved as
`unverified` stays so until it is remembered again with indexed sources.
Applicable memories that do not fit are counted in `memories_omitted`, and a
warning labels memory text as notes, not instructions. `memory_context` (unless `include_architecture` is
false) adds memories that apply to the ranked files within a 30% budget reserve,
sourced before unverified before stale.

`memory_architecture` groups indexed files by directory. Go package names and
imports are recorded by the parser as AST symbols (kind `import`, never used as
chunk labels); imports under the root `go.mod` module path are reported as
directories, the rest only counted. `_test.go` files count as files but do not
define a module's package or dependencies.

`memory_graph` (`internal/graph`, schema v8) reads the `refs` table: one row per
distinct reference from a top-level function or method (`Recv.Method`) to a
package-level name, recorded by the parser from go/ast without type checking.
Targets are a same-package name, an import path plus name, or a method called on
a value (`to_pkg='.'`, matched by method name only and reported as `by_name`).
Import paths under the root `go.mod` resolve to project directories at query
time, so an edited `go.mod` needs no re-parse. Callers are walked level by
level (depth 3, 50 callers) with a visited set: each function appears once at
its nearest depth, exact edges before by-name ones, so method names such as
`Close` cannot multiply paths. Definitions are capped at 20; `dir` picks one of
several same-named packages. Callees drop variables, constants and shadowing
locals in SQL before the limit. Not edges: method values passed without a call
(`s.handle`), and names hidden by a same-named local anywhere in the function
(scopes are not tracked). Import names are guessed from the path, so a package
whose clause differs from its directory is not matched. Declarations repeated in build-tagged files
count once. Cost on the Go standard library (10k files, 481k refs): first index
28.6 s -> 36.0 s, peak RSS 255 -> 388 MB, database 155 -> 235 MB; symbol queries
0.2-0.7 s and the most-referenced list 0.7 s including the freshness check.

Query cache (`internal/qcache`, schema v4): `memory_search` and `memory_context`
responses are stored under SHA-256(tool, normalized input, secrets flag, and for
context the memory revision), bound to the index generation that produced them
and expiring after 1 h; writing drops other generations and keeps 500 entries.
Refresh still runs first, so a hit is only possible while the files are
unchanged. `events.cache_hit` feeds `query_cache_hit_rate` in `stats`; hits are
avoided retrieval work, never reported as token savings.

Reserved future boundaries, not empty packages: benchmark/reporter and local dashboard.

Analytics writes happen after measured work; latency excludes metric storage and
network/client execution. Metrics failures log a warning and do not discard a
valid response. Tool payload measurements exclude the JSON-RPC transport envelope.
