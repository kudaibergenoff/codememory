package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func options(t *testing.T) Options {
	t.Helper()
	return Options{Root: t.TempDir(), Executable: "/opt/bin/codememory", CodexHome: t.TempDir()}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func actions(p Plan) map[string]string {
	out := map[string]string{}
	for _, c := range p.Changes {
		out[filepath.Base(c.Path)] = c.Action
	}
	return out
}

func TestClaudeDryRunWritesNothing(t *testing.T) {
	o := options(t)

	plan, err := Claude(o)

	if err != nil || plan.Applied {
		t.Fatalf("%+v %v", plan, err)
	}
	if a := actions(plan); a[".mcp.json"] != ActionCreate || a["SKILL.md"] != ActionCreate {
		t.Fatalf("plan %v", a)
	}
	if entries, _ := os.ReadDir(o.Root); len(entries) != 0 {
		t.Fatalf("dry run wrote %d entries", len(entries))
	}
}

func TestClaudeApplyMergesAndIsIdempotent(t *testing.T) {
	o := options(t)
	existing := `{"mcpServers":{"other":{"command":"x"}},"keep":true}`
	if err := os.WriteFile(filepath.Join(o.Root, ".mcp.json"), []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}
	o.Apply = true

	if _, err := Claude(o); err != nil {
		t.Fatal(err)
	}

	var cfg struct {
		Keep       bool                       `json:"keep"`
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(read(t, filepath.Join(o.Root, ".mcp.json"))), &cfg); err != nil {
		t.Fatal(err)
	}
	server := string(cfg.MCPServers["codememory"])
	if !cfg.Keep || cfg.MCPServers["other"] == nil || !strings.Contains(server, `"/opt/bin/codememory"`) || !strings.Contains(server, o.Root) {
		t.Fatalf("merge lost data: %+v %s", cfg, server)
	}
	if !strings.Contains(read(t, filepath.Join(o.Root, ".claude", "skills", "codememory", "SKILL.md")), "memory_context") {
		t.Fatal("skill not installed")
	}
	again, err := Claude(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range again.Changes {
		if c.Action != ActionUnchanged {
			t.Fatalf("second apply changed %s: %s", c.Path, c.Action)
		}
	}
}

func TestClaudeRefusesInvalidJSONAndSymlinks(t *testing.T) {
	o := options(t)
	o.Apply = true
	bad := filepath.Join(o.Root, ".mcp.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Claude(o); err == nil || read(t, bad) != "{not json" {
		t.Fatalf("invalid JSON overwritten: %v", err)
	}

	o2 := options(t)
	o2.Apply = true
	target := filepath.Join(t.TempDir(), "elsewhere.json")
	if err := os.WriteFile(target, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(o2.Root, ".mcp.json")); err != nil {
		t.Skip(err)
	}
	if _, err := Claude(o2); err == nil || read(t, target) != "{}" {
		t.Fatalf("followed a symlink out of the project: %v", err)
	}
}

func TestCodexApplyPreservesConfigAndIsIdempotent(t *testing.T) {
	o := options(t)
	o.Apply = true
	cfgPath := filepath.Join(o.CodexHome, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("model = \"o3\"\n\n[mcp_servers.other]\ncommand = \"x\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.Root, "AGENTS.md"), []byte("# Team rules\nBe kind.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := Codex(o); err != nil {
		t.Fatal(err)
	}

	cfg := read(t, cfgPath)
	if !strings.Contains(cfg, `model = "o3"`) || !strings.Contains(cfg, "[mcp_servers.other]") || !strings.Contains(cfg, "[mcp_servers.codememory]") || !strings.Contains(cfg, `"/opt/bin/codememory"`) {
		t.Fatalf("config.toml:\n%s", cfg)
	}
	agents := read(t, filepath.Join(o.Root, "AGENTS.md"))
	if !strings.HasPrefix(agents, "# Team rules\nBe kind.\n") || !strings.Contains(agents, "memory_context") {
		t.Fatalf("AGENTS.md:\n%s", agents)
	}
	again, err := Codex(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range again.Changes {
		if c.Action != ActionUnchanged {
			t.Fatalf("second apply changed %s", c.Path)
		}
	}
	if strings.Count(read(t, cfgPath), "[mcp_servers.codememory]") != 1 {
		t.Fatal("duplicate server entry")
	}
}

func TestCodexRefusesHandWrittenEntry(t *testing.T) {
	o := options(t)
	o.Apply = true
	cfgPath := filepath.Join(o.CodexHome, "config.toml")
	hand := "[mcp_servers.codememory]\ncommand = \"mine\"\n"
	if err := os.WriteFile(cfgPath, []byte(hand), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := Codex(o); err == nil || read(t, cfgPath) != hand {
		t.Fatalf("hand-written entry modified: %v", err)
	}
}

func TestTOMLStringEscapesWindowsPaths(t *testing.T) {
	if got := tomlString(`C:\Tools\code"memory`); got != `"C:\\Tools\\code\"memory"` {
		t.Fatalf("got %s", got)
	}
}
