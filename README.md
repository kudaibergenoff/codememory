# CodeMemory

Local-first code index for AI coding agents. Work in progress: first vertical slice of
[the specification](CODEMEMORY_SPEC_RU.md), not a production release. No model API keys,
cloud service, provider cache manipulation or source uploads.

Build requires Go 1.27 and a C compiler (temporary SQLite driver choice, see ADR-001).

```sh
make build
./bin/codememory init --project /path/to/repository
./bin/codememory index --project /path/to/repository
./bin/codememory search --project /path/to/repository --query "Payment" --limit 5
./bin/codememory status --project /path/to/repository
./bin/codememory stats --project /path/to/repository
./bin/codememory serve --project /path/to/repository
make check
```

Add `.codememory/` to your project's `.gitignore`. Configuration is in
`.codememory/config.json`; private SQLite state stays inside that directory.
Global agent settings are never modified automatically.

Implemented commands: `init`, `index`, `search`, `status`, `stats`, `serve`, `version`.
Search additionally accepts `--language go` and `--path-prefix internal/`.
Place flags before any positional arguments; search uses `--query`, not a bare argument.
All ordinary responses are JSON. `serve` reserves stdout for MCP and stderr for logs.

The MCP tools are `memory_search` and `memory_status`. Follow the
[Claude/Codex setup guide](docs/INTEGRATIONS.md) and use the example files under
`integrations/`. No global agent configuration is changed automatically.

First index parses permitted files; repeated index hashes them and reuses unchanged
chunks. Edits/deletions publish atomically. Search uses SQLite FTS5/BM25 with Go AST
symbol weights, source line ranges and SHA-256. PHP/Python/TypeScript have text
chunks. Search/status re-scan permitted files; stale search returns no snippets and
asks for re-indexing. No watcher, embeddings, model calls or query cache yet.

The default limits are 1 MiB/file, 128 MiB of eligible text, 20000 files and
40 lines/chunk (also capped at 4096 UTF-8 bytes). Hard deny precedes ignore rules;
`.gitignore` and `.codememoryignore` work without Git, including nested rules,
negation, `*`, `?`, character ranges and `**`. POSIX character classes/collating
expressions fail the scan explicitly. Global Git excludes and `.git/info/exclude`
are not imported. Secret detection omits whole suspect files and can have false
positives. See [security scope](docs/SECURITY.md).

`stats` separates measured counts/bytes/latency/file reuse from estimated response
tokens. Actual usage and verified savings are `null` (N/A). Cache reuse is not
token savings; token savings are not subscription cost savings.

`context`, `benchmark`, `report`, `dashboard`, `optimize`, `doctor`, `clean`,
`install` and persistent-memory tools remain pending, not placeholder commands.
The first slice is tested locally on macOS arm64; the native five-platform CI
matrix is provided but has not run. Authenticated Claude/Codex smoke tests and a
current SQLite driver/security review are required before release.

Build/test commands require `-tags sqlite_fts5`; `make` supplies it. Windows source
builds additionally need a compatible C compiler (CGO). A plain CGO-disabled build
does not provide working SQLite. Release binaries have not been published.

See [architecture](docs/ARCHITECTURE.md), [roadmap](docs/ROADMAP.md),
[analytics](docs/ANALYTICS.md), [validation record](docs/VALIDATION.md),
[ADRs](docs/adr/001-storage.md) and [changelog](CHANGELOG.md). MIT licensed.
