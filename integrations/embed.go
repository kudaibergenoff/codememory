// Package integrations embeds the agent instructions that `codememory install`
// writes, so the installed text always matches this binary's tools.
package integrations

import _ "embed"

//go:embed claude/SKILL.md
var ClaudeSkill string

//go:embed codex/AGENTS.md
var CodexAgents string
