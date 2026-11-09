# CodeMemory

Local-first code index for AI coding agents. Work in progress: first vertical slice of
[the specification](CODEMEMORY_SPEC_RU.md), not a production release. No model API keys,
cloud service, provider cache manipulation or source uploads.

Build requires Go 1.27 only: the SQLite driver is pure Go (no CGO, no build tags).

```sh
go install github.com/kudaibergenoff/codememory/cmd/codememory@latest
```

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

Implemented commands: `init`, `index [--full]`, `search`, `context`, `remember`, `status`, `stats`, `serve`, `version`.
`context --task "add a refund endpoint" --max-tokens 1500` returns ranked snippets and
file names whose whole JSON response fits the estimated token budget.
Search additionally accepts `--language go` and `--path-prefix internal/`.
Place flags before any positional arguments; search uses `--query`, not a bare argument.
All ordinary responses are JSON. `serve` reserves stdout for MCP and stderr for logs.

The MCP tools are `memory_context`, `memory_search`, `memory_status` and `memory_remember`.
Memories cite files by SHA-256: `sourced` (cited files indexed when saved, which
proves provenance, not truth), `unverified` (no indexed sources) or `stale` (a cited
file changed since). Secret-like text is refused. Follow the
[Claude/Codex setup guide](docs/INTEGRATIONS.md) and use the example files under
`integrations/`. No global agent configuration is changed automatically.

First index parses permitted files; repeated index hashes them and reuses unchanged
chunks. Edits/deletions publish atomically. Search uses SQLite FTS5/BM25 with Go AST
symbol weights, source line ranges and SHA-256. PHP/Python/TypeScript have text
chunks. Search refreshes the index first (size/mtime prefilter, SHA-256 identity),
so edits are visible without a manual `index`; status reports staleness read-only.
After tools that preserve size and mtime (`cp -p`, `rsync -t`) run `index --full`.
No watcher, embeddings, model calls or query cache yet.

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

`benchmark`, `report`, `dashboard`, `optimize`, `doctor`, `clean`,
`install` and `memory_architecture` remain pending, not placeholder commands.
The first slice is tested locally on macOS arm64; the native five-platform CI
matrix is provided but has not run. Authenticated Claude/Codex smoke tests and a
dependency/license review are required before release.

Plain `go build`/`go test` need no CGO and no build tags. A `CGO_ENABLED=0` build
is verified on macOS arm64; other platforms await the CI run. Only `go test -race`
needs CGO (a Go race-detector requirement, not an application one).
Release binaries have not been published.

See [architecture](docs/ARCHITECTURE.md), [roadmap](docs/ROADMAP.md),
[analytics](docs/ANALYTICS.md), [validation record](docs/VALIDATION.md),
[ADRs](docs/adr/001-storage.md) and [changelog](CHANGELOG.md). MIT licensed.
