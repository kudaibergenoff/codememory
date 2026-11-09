# CodeMemory

Local code index and curated project memory for AI coding agents (Claude Code,
Codex CLI) over MCP stdio. It runs entirely on your machine: no model API keys,
no cloud service, no source uploads, no telemetry. Agents keep using their own
subscription login; CodeMemory is only a local tool server.

Implemented per [the specification](CODEMEMORY_SPEC_RU.md). Not yet verified in
authenticated Claude Code and Codex sessions — see [Status](#status).

## Install

Download an archive for your platform from
[Releases](https://github.com/kudaibergenoff/codememory/releases) and verify it:

```sh
shasum -a 256 -c SHA256SUMS --ignore-missing   # Linux: sha256sum -c ...
tar -xzf codememory_<version>_<os>_<arch>.tar.gz # Windows: unzip the .zip
```

Binaries are not code-signed; on macOS you may need
`xattr -d com.apple.quarantine ./codememory`. Or build with Go 1.27+ (pure Go,
no C compiler needed):

```sh
go install github.com/kudaibergenoff/codememory/cmd/codememory@latest
# or from a checkout:
make build          # bin/codememory
```

## Quick start

```sh
cd /path/to/repository
codememory init                   # creates .codememory/ (add it to .gitignore)
codememory index                  # first full index; later searches refresh it
codememory install claude         # shows the plan and diffs; nothing written yet
codememory install claude --yes   # project .mcp.json + .claude/skills/codememory
codememory install codex --yes    # ~/.codex/config.toml block + AGENTS.md block
codememory doctor                 # read-only checks of state and integrations
```

Then ask the agent about the codebase; it can call the tools below. Manual
configuration and the smoke-test checklist are in [INTEGRATIONS](docs/INTEGRATIONS.md).

## MCP tools

| Tool | Purpose |
|---|---|
| `memory_context` | Ranked snippets for a task, packed so the whole JSON response fits `max_tokens` (default 1500, estimated); includes applicable memories |
| `memory_search` | Literal full-text search with path, line range and SHA-256 per match |
| `memory_status` | Index counts and whether the index matches the files (read-only) |
| `memory_remember` | Save a decision with supporting files; `sourced`, `unverified` or later `stale` |
| `memory_architecture` | Directories, Go packages and internal imports (go/ast) plus scoped memories |
| `memory_graph` | Go reference graph: a symbol's definition, callers (depth 1-3, call sites) and what it uses; without a symbol, the most referenced symbols |

Search and context re-index changed files first, so edits are visible without a
manual `index`. Source text and memories are returned as untrusted data.

## Commands

| Command | What it does |
|---|---|
| `init`, `index [--full]` | Create state; index (stat prefilter, `--full` re-reads everything) |
| `search --query Q`, `context --task T` | CLI equivalents of the MCP tools |
| `remember`, `architecture [--scope DIR]` | Curate and inspect memories |
| `graph [--symbol pkg.Name] [--depth 1..3] [--scope DIR]` | Callers and callees of a Go symbol, or the most referenced symbols |
| `status`, `doctor` | Freshness; read-only diagnostics |
| `install claude\|codex [--yes]` | Idempotent integration setup; diff first, write only with `--yes` |
| `stats`, `report --format json\|csv` | Local measurements (`--today`, `--last 7d`, `--agent`) |
| `benchmark [--tasks FILE]` | Recall and response size vs. a lexical baseline and an oracle |
| `optimize` | Evidence-based suggestions; never changes anything |
| `dashboard [--addr 127.0.0.1:0]` | Read-only local web view, loopback only |
| `clean [--all] [--yes]` | Drop index and caches (keeps memories); `--all` removes `.codememory/` |
| `serve` | MCP stdio server (stdout is protocol only, logs on stderr) |

Flags go before positional arguments; every command accepts `--project DIR`.

## What the numbers mean

`stats`, `report` and `dashboard` separate **measured** values (requests, bytes,
latency, cache hits), **estimated** tokens (`bytes/3`, a size proxy, not a model
tokenizer) and **verified** savings, which stay `null`: clients do not expose
per-request usage to local tools. A cache hit saves local work, not tokens.

`codememory benchmark` on this repository (79 files, 6 tasks with known answer
files) found every expected file; responses were ~1,500 estimated tokens against
590–3,360 for reading exactly the right files and 83k–97k for reading every file
matching a task term. On a 14-file fixture responses are *larger* than reading
the few relevant files, because each response carries provenance overhead. Run
it on your own tasks before relying on it; details in [ANALYTICS](docs/ANALYTICS.md)
and [VALIDATION](docs/VALIDATION.md).

## Safety and limits

- Indexes only the trusted root: `.gitignore`/`.codememoryignore` (nested, negation,
  `**`), a hard deny list (`.git`, `.env*`, keys, `node_modules`, …) and skipped
  symlinks; files matching secret patterns are excluded whole (false positives possible).
- Defaults: 1 MiB per file, 128 MiB total, 20,000 files, 40-line chunks. Tune in
  `.codememory/config.json`.
- Go gets AST symbols and imports; PHP, Python and TypeScript are indexed as text chunks.
- Content changed while size and mtime are preserved (`cp -p`, `rsync -t`) is found
  by `index --full`.
- No shell execution from MCP tools; global agent instructions are never edited.

See [SECURITY](docs/SECURITY.md) and [ARCHITECTURE](docs/ARCHITECTURE.md).

## Troubleshooting

- **`project not initialized`** — run `codememory init` in the repository root.
- **Agent does not see the tools** — `codememory doctor`; Claude Code: `/mcp` and
  approve the project server; Codex: restart, then `codex mcp list`.
- **`database belongs to another project root`** — the repository moved; run
  `codememory clean --all --yes`, then `init` and `index`.
- **Results withheld / `is_stale`** — another indexer was busy or a limit was hit;
  retry, check stderr, or raise limits in `.codememory/config.json`.
- **Wrong results after `rsync -t`/`cp -p`** — `codememory index --full`.

## Status

CI passes on Linux amd64/arm64, macOS arm64/amd64 and Windows amd64 (tests,
race detector, `govulncheck`). Not yet done: authenticated smoke tests in Claude
Code and Codex with subscription logins, code signing, and Tree-sitter parsers.
See [ROADMAP](docs/ROADMAP.md) and [CHANGELOG](CHANGELOG.md).

## Development

```sh
make check                # gofmt, vet, tests, race tests, CGO-free build
make vuln                 # govulncheck
make dist VERSION=v0.1.0  # release archives and SHA256SUMS in dist/
```

Releases are built by `.github/workflows/release.yml` when a `v*` tag is pushed.
MIT licensed.
