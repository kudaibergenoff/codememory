package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func codexCheck(t *testing.T, config string) Check {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	if config != "" {
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var r Report
	checkCodex(&r, "/work/shop")
	return r.Checks[0]
}

func TestCodexIntegrationCheck(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, config, status string
	}{
		{"missing config", "", StatusWarn},
		{"no server", "model = \"o3\"\n", StatusWarn},
		{"other project", "[mcp_servers.codememory]\ncommand = \"" + exe + "\"\nargs = [\"serve\", \"--project\", \"/elsewhere\"]\n", StatusWarn},
		{"missing binary", "[mcp_servers.codememory]\ncommand = \"/nope/codememory\"\nargs = [\"serve\", \"--project\", \"/work/shop\"]\n", StatusWarn},
		{"registered", "[mcp_servers.other]\ncommand = \"x\"\n\n[mcp_servers.codememory]\ncommand = \"" + exe + "\"\nargs = [\"serve\", \"--project\", \"/work/shop\"]\n\n[profiles.x]\nmodel = \"y\"\n", StatusOK},
	}
	for _, c := range cases {
		if got := codexCheck(t, c.config); got.Status != c.status {
			t.Errorf("%s: %+v", c.name, got)
		}
	}
}
