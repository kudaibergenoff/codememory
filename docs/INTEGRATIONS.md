# Integration setup and verification

Build and index first. Replace paths below with absolute paths. No API key or
CodeMemory cloud account is involved. These commands change client configuration
only when **you run them**; CodeMemory does not install itself automatically.

Claude Code:

```sh
claude mcp add --transport stdio --scope project codememory -- /absolute/path/to/codememory serve --project /absolute/path/to/repository
```

Alternatively merge `integrations/claude/mcp.json` into the project's `.mcp.json`.
Copy `integrations/claude/SKILL.md` to `.claude/skills/codememory/SKILL.md` in the
target project. Use `/mcp` to inspect status and the normal client trust prompts.

Codex CLI:

```sh
codex mcp add codememory -- /absolute/path/to/codememory serve --project /absolute/path/to/repository
codex mcp list
```

Alternatively merge `integrations/codex/config.toml` into your Codex configuration.
Merge the navigation policy from `integrations/codex/AGENTS.md` into the target
project's `AGENTS.md`; preserve existing instructions. Do not overwrite global
files. Codex discovers applicable AGENTS files along the project path.

Manual authenticated smoke test for each client:

1. Use the client's ordinary supported subscription login, with API-key variables
   unset. Verify the selected account/plan in that client's UI.
2. In a fixture repository, initialize and index. Connect the stdio server.
3. Ask the agent to call `memory_status`, then `memory_search` for `RefundPayment`.
4. Confirm relative source path/lines/hash and clean tool JSON.
5. Edit the fixture, repeat search: stale warning and no old snippets.
6. Re-index and verify the new result; disconnect and confirm process exit.

Protocol subprocess tests do not prove authenticated subscription compatibility.
Installed clients were inspected using version/help only; no model request or
account-authenticated session has been run for this delivery.

Official references checked on 2026-11-09:
- [Claude MCP](https://code.claude.com/docs/en/mcp)
- [Claude Skills](https://code.claude.com/docs/en/skills)
- [Codex MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)
- [Codex AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md)

Troubleshooting: build with `-tags sqlite_fts5` if migration reports missing FTS5;
install a C compiler and enable CGO; initialize before serve; use absolute paths
because clients may launch in another directory. A copied database belongs to its
original canonical root. Move it aside and initialize/re-index at the new root.
For `database is locked`, wait for indexing to finish and retry. No partial index
is published on failure.
