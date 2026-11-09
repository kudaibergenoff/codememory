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

Search/status perform a fresh bounded scan and compare the complete permitted
file set and configuration with a single database snapshot. Mismatch returns
`is_stale=true` and no snippets. This conservative policy also removes access to
previously indexed files newly excluded by ignore rules. Scanning may be costly;
a watcher/cache needs separate correctness tests before replacing it.

Reserved future boundaries, not empty packages: context assembly (budget including
JSON overhead), persistent memory (provenance/dependencies), generation-bound query
cache, benchmark/reporter and local dashboard. Do not treat these as delivered.

Analytics writes happen after measured work; latency excludes metric storage and
network/client execution. Metrics failures log a warning and do not discard a
valid response. Tool payload measurements exclude the JSON-RPC transport envelope.
