// Package doctor diagnoses a CodeMemory installation without changing project
// files. It never executes client binaries; it only looks them up in PATH.
package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/indexer"
	"github.com/kudaibergenoff/codememory/internal/install"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
)

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type Report struct {
	OK       bool    `json:"ok"`
	Version  string  `json:"version"`
	Platform string  `json:"platform"`
	Checks   []Check `json:"checks"`
}

func (r *Report) add(name, status, detail string) {
	r.Checks = append(r.Checks, Check{Name: name, Status: status, Detail: detail})
	if status == StatusFail {
		r.OK = false
	}
}

// Run checks root. Opening the database applies pending schema migrations.
func Run(ctx context.Context, root, version string) Report {
	r := Report{OK: true, Version: version, Platform: runtime.GOOS + "/" + runtime.GOARCH, Checks: []Check{}}
	r.add("project_root", StatusOK, root)
	dir, err := config.State(root, false)
	if err != nil {
		r.add("initialized", StatusFail, "not initialized; run: codememory init --project "+root)
		checkClients(&r)
		return r
	}
	r.add("initialized", StatusOK, dir)
	cfg, err := config.Load(root)
	if err != nil {
		r.add("config", StatusFail, err.Error())
		return r
	}
	r.add("config", StatusOK, "valid")
	checkStore(ctx, &r, root, dir, cfg)
	checkGitignore(&r, root)
	checkClaude(&r, root)
	checkCodex(&r, root)
	checkClients(&r)
	return r
}

func checkStore(ctx context.Context, r *Report, root, dir string, cfg config.Config) {
	s, err := sqlite.Open(ctx, dir, root)
	if err != nil {
		r.add("database", StatusFail, err.Error())
		return
	}
	defer s.Close()
	var integrity string
	if err = s.DB.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&integrity); err != nil || integrity != "ok" {
		r.add("database", StatusFail, fmt.Sprintf("integrity check: %s %v", integrity, err))
		return
	}
	foot, err := s.Footprint(ctx)
	if err != nil {
		r.add("database", StatusFail, err.Error())
		return
	}
	r.add("database", StatusOK, fmt.Sprintf("schema current, integrity ok, %d files, %d memories, %d bytes", foot.Files, foot.Memories, foot.DBBytes))
	fresh, err := indexer.Check(ctx, s, root, cfg)
	switch {
	case err != nil:
		r.add("index", StatusFail, err.Error())
	case foot.Files == 0:
		r.add("index", StatusWarn, "empty; run: codememory index (searches also refresh it)")
	case fresh.Stale:
		r.add("index", StatusWarn, "out of date; the next search refreshes it")
	default:
		r.add("index", StatusOK, "matches the project files")
	}
}

func checkGitignore(r *Report, root string) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return
	}
	b, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	for _, line := range strings.Split(string(b), "\n") {
		if p := strings.Trim(strings.TrimSpace(line), "/"); p == ".codememory" {
			r.add("gitignore", StatusOK, ".codememory/ is ignored")
			return
		}
	}
	r.add("gitignore", StatusWarn, "add .codememory/ to .gitignore; it holds the private index and memories")
}

func checkClaude(r *Report, root string) {
	b, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
	if err != nil {
		r.add("claude_integration", StatusWarn, "no project .mcp.json; run: codememory install claude")
		return
	}
	var cfg struct {
		Servers map[string]struct {
			Command string `json:"command"`
		} `json:"mcpServers"`
	}
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&cfg); err != nil {
		r.add("claude_integration", StatusWarn, ".mcp.json is not valid JSON")
		return
	}
	server, ok := cfg.Servers["codememory"]
	switch {
	case !ok:
		r.add("claude_integration", StatusWarn, ".mcp.json has no codememory server; run: codememory install claude")
	case !isExecutable(server.Command):
		r.add("claude_integration", StatusWarn, "registered command not found: "+server.Command)
	default:
		r.add("claude_integration", StatusOK, "project .mcp.json registers "+server.Command)
	}
}

var (
	codexSection = regexp.MustCompile(`(?ms)^[ \t]*\[[ \t]*mcp_servers[ \t]*\.[ \t]*"?codememory"?[ \t]*\][ \t]*$(.*?)(?:^[ \t]*\[|\z)`)
	codexCommand = regexp.MustCompile(`(?m)^[ \t]*command[ \t]*=[ \t]*"((?:[^"\\]|\\.)*)"`)
)

// checkCodex reads only the codememory server table of the Codex config.
func checkCodex(r *Report, root string) {
	home, err := install.CodexHome("")
	if err != nil {
		r.add("codex_integration", StatusWarn, err.Error())
		return
	}
	b, err := os.ReadFile(filepath.Join(home, "config.toml"))
	section := codexSection.FindSubmatch(b)
	if err != nil || section == nil {
		r.add("codex_integration", StatusWarn, "no [mcp_servers.codememory] in Codex config; run: codememory install codex")
		return
	}
	command := ""
	if m := codexCommand.FindSubmatch(section[1]); m != nil {
		command = strings.ReplaceAll(string(m[1]), `\\`, `\`)
	}
	switch {
	case !isExecutable(command):
		r.add("codex_integration", StatusWarn, "registered command not found: "+command)
	case !strings.Contains(strings.ReplaceAll(string(section[1]), `\\`, `\`), root):
		r.add("codex_integration", StatusWarn, "codememory server is registered for another project; run: codememory install codex")
	default:
		r.add("codex_integration", StatusOK, "Codex config registers "+command+" for this project")
	}
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func checkClients(r *Report) {
	for _, client := range []string{"claude", "codex"} {
		if path, err := exec.LookPath(client); err == nil {
			r.add(client+"_cli", StatusOK, "found at "+path+" (not executed)")
		} else {
			r.add(client+"_cli", StatusWarn, "not found in PATH")
		}
	}
}
