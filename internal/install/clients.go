package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kudaibergenoff/codememory/integrations"
)

const (
	claudeNote = "Project-scoped: .mcp.json and a project skill. Claude Code asks you to approve project MCP servers on first use; check with /mcp."
	codexNote  = "Writes your user-level Codex config (one 'codememory' server for this project) and a marked block in the project's AGENTS.md. Restart Codex, then check with: codex mcp list."

	tomlBegin  = "# BEGIN codememory (managed by `codememory install codex`; edits inside are replaced)"
	tomlEnd    = "# END codememory"
	agentBegin = "<!-- BEGIN codememory (managed by `codememory install codex`) -->"
	agentEnd   = "<!-- END codememory -->"
)

var handWrittenServer = regexp.MustCompile(`(?m)^\s*\[\s*mcp_servers\s*\.\s*"?codememory"?\s*\]`)

// Claude plans the project .mcp.json entry and project skill.
func Claude(o Options) (Plan, error) {
	targets := []target{
		{dir: o.Root, name: ".mcp.json", mode: 0644, render: func(old []byte, exists bool) ([]byte, error) { return mergeMCP(old, exists, o) }},
		{dir: o.Root, name: ".claude/skills/codememory/SKILL.md", mode: 0644, render: func([]byte, bool) ([]byte, error) { return []byte(integrations.ClaudeSkill), nil }},
	}
	return run("claude", claudeNote, o.Apply, targets)
}

// Codex plans the user Codex config entry and the project AGENTS.md block.
func Codex(o Options) (Plan, error) {
	home, err := CodexHome(o.CodexHome)
	if err != nil {
		return Plan{}, err
	}
	targets := []target{
		{dir: home, name: "config.toml", mode: 0600, render: func(old []byte, _ bool) ([]byte, error) { return codexConfig(old, o) }},
		{dir: o.Root, name: "AGENTS.md", mode: 0644, render: func(old []byte, _ bool) ([]byte, error) {
			return replaceBlock(old, agentBegin, agentEnd, strings.TrimSpace(integrations.CodexAgents))
		}},
	}
	return run("codex", codexNote, o.Apply, targets)
}

// CodexHome resolves the Codex config directory: explicit, $CODEX_HOME or ~/.codex.
func CodexHome(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if env := os.Getenv("CODEX_HOME"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".codex"), nil
}

func serverArgs(o Options) []string { return []string{"serve", "--project", o.Root} }

// mergeMCP sets mcpServers.codememory and keeps every other key and server.
func mergeMCP(old []byte, exists bool, o Options) ([]byte, error) {
	doc := map[string]json.RawMessage{}
	if exists && len(bytes.TrimSpace(old)) > 0 {
		if err := json.Unmarshal(old, &doc); err != nil {
			return nil, fmt.Errorf("existing file is not a JSON object; fix it or merge manually: %w", err)
		}
	}
	servers := map[string]json.RawMessage{}
	if raw, ok := doc["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, errors.New("mcpServers is not a JSON object; fix it or merge manually")
		}
	}
	want, err := json.Marshal(map[string]any{"type": "stdio", "command": o.Executable, "args": serverArgs(o)})
	if err != nil {
		return nil, err
	}
	if exists && sameJSON(servers[serverName], want) {
		return old, nil // already installed: keep the file byte-for-byte
	}
	servers[serverName] = want
	if doc["mcpServers"], err = json.Marshal(servers); err != nil {
		return nil, err
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}

func codexConfig(old []byte, o Options) ([]byte, error) {
	outside := removeBlock(string(old), tomlBegin, tomlEnd)
	if handWrittenServer.MatchString(outside) {
		return nil, errors.New("a hand-written [mcp_servers.codememory] exists; remove it or keep it and skip install")
	}
	quoted := make([]string, 0, 3)
	for _, a := range serverArgs(o) {
		quoted = append(quoted, tomlString(a))
	}
	block := fmt.Sprintf("[mcp_servers.%s]\ncommand = %s\nargs = [%s]", serverName, tomlString(o.Executable), strings.Join(quoted, ", "))
	return replaceBlock(old, tomlBegin, tomlEnd, block)
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// replaceBlock swaps the text between the markers (inclusive) for a fresh
// block, or appends one. Unbalanced markers are an error, never guessed at.
func replaceBlock(old []byte, begin, end, body string) ([]byte, error) {
	text := string(old)
	block := begin + "\n" + body + "\n" + end + "\n"
	i, j := strings.Index(text, begin), strings.Index(text, end)
	switch {
	case i >= 0 && j > i:
		tail := strings.TrimPrefix(text[j+len(end):], "\n")
		return []byte(text[:i] + block + tail), nil
	case i >= 0 || j >= 0:
		return nil, errors.New("found only one codememory marker; restore or remove it manually")
	case text == "":
		return []byte(block), nil
	default:
		sep := "\n"
		if !strings.HasSuffix(text, "\n") {
			sep = "\n\n"
		}
		return []byte(text + sep + block), nil
	}
}

func removeBlock(text, begin, end string) string {
	i, j := strings.Index(text, begin), strings.Index(text, end)
	if i >= 0 && j > i {
		return text[:i] + text[j+len(end):]
	}
	return text
}
