// Package cli implements commands without process-global flags or exits.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/kudaibergenoff/codememory/internal/architecture"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/contextpack"
	"github.com/kudaibergenoff/codememory/internal/dashboard"
	"github.com/kudaibergenoff/codememory/internal/graph"
	"github.com/kudaibergenoff/codememory/internal/mcp"
	"github.com/kudaibergenoff/codememory/internal/memory"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

// Version is set at release build time with -ldflags "-X .../internal/cli.Version=vX.Y.Z".
var Version = "0.1.0-dev"

const usage = `codememory <init|index|search|context|status|stats|serve|version> [--project DIR]
index:   [--full]  re-read every file, bypassing the size/mtime prefilter
search:  --query TEXT [--limit 10] [--language go] [--path-prefix internal/]
context: --task TEXT [--max-tokens 1500]
remember: --title TEXT --body TEXT [--scope DIR] [--source PATH]...
architecture: [--scope DIR]
graph:   [--symbol pkg.Name] [--depth 1..3] [--dir DIR] [--scope DIR]   callers/callees, or most referenced symbols
context also accepts --architecture=false to omit curated memories
stats:   [--today | --last 30d] [--agent claude|codex|unknown]
report:  [--format json|csv] [--today | --last 30d] [--agent ...]
install: claude|codex [--yes]   shows a diff; writes only with --yes
clean:   [--all] [--yes]        index only; --all also removes memories and analytics
doctor:  read-only diagnostics
benchmark: [--tasks FILE]     built-in fixture suite, or your tasks against this project
optimize: read-only suggestions from index, analytics and the latest benchmark
dashboard: [--addr 127.0.0.1:0] local read-only web view (loopback only)`

var commands = map[string]bool{"init": true, "index": true, "search": true, "context": true, "remember": true, "architecture": true, "graph": true, "status": true, "stats": true, "report": true, "doctor": true, "install": true, "clean": true, "benchmark": true, "optimize": true, "dashboard": true, "serve": true}

type options struct {
	command string
	project string
	full    bool
	search  search.Input
	context contextpack.Input
	memory  memory.Input
	arch    architecture.Input
	graph   graph.Input
	withArc bool
	window  window
	client  string
	yes     bool
	all     bool
	tasks   string
	addr    string
}

func Run(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := fmt.Fprintln(out, usage)
		return err
	}
	if args[0] == "version" {
		_, err := fmt.Fprintln(out, Version)
		return err
	}
	opts, err := parse(args, errOut)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	root, err := config.Root(opts.project)
	if err != nil {
		return err
	}
	switch opts.command {
	case "doctor":
		return runDoctor(ctx, root, out)
	case "install":
		return runInstall(root, opts, out)
	case "clean":
		return runClean(ctx, root, opts, out)
	}
	if opts.command == "init" {
		if err = config.Init(root); err != nil {
			return err
		}
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	s, err := openStore(ctx, root)
	if err != nil {
		return err
	}
	defer s.Close()
	switch opts.command {
	case "serve":
		return mcp.Serve(ctx, s, root, Version)
	case "dashboard":
		return dashboard.Serve(ctx, s, filepath.Base(root), opts.addr, func(url string) {
			_ = writeJSON(out, map[string]string{"url": url, "note": "Read-only local view; press Ctrl-C to stop."})
		})
	}
	return execute(ctx, opts, s, root, cfg, out)
}

func parse(args []string, errOut io.Writer) (options, error) {
	opts := options{command: args[0]}
	if !commands[opts.command] {
		return opts, fmt.Errorf("unknown command %q", opts.command)
	}
	rest := args[1:]
	if opts.command == "install" && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		opts.client, rest = rest[0], rest[1:]
	}
	flags := flag.NewFlagSet(opts.command, flag.ContinueOnError)
	flags.SetOutput(errOut)
	flags.StringVar(&opts.project, "project", ".", "trusted project directory")
	switch opts.command {
	case "index":
		flags.BoolVar(&opts.full, "full", false, "re-read every file, bypassing the size/mtime prefilter")
	case "search":
		flags.StringVar(&opts.search.Query, "query", "", "plain text query")
		flags.IntVar(&opts.search.Limit, "limit", 10, "maximum matches (1 to 20)")
		flags.StringVar(&opts.search.Language, "language", "", "language filter")
		flags.StringVar(&opts.search.PathPrefix, "path-prefix", "", "relative path prefix")
	case "context":
		flags.StringVar(&opts.context.Task, "task", "", "task description in natural language")
		flags.IntVar(&opts.context.MaxTokens, "max-tokens", 0, "response budget in estimated tokens (500 to 8000, default 1500)")
		flags.BoolVar(&opts.withArc, "architecture", true, "include curated memories for the ranked files")
	case "stats", "report":
		flags.BoolVar(&opts.window.today, "today", false, "only events since local midnight")
		flags.StringVar(&opts.window.last, "last", "", "only events in this window, e.g. 7d or 12h (max 30d)")
		flags.StringVar(&opts.window.agent, "agent", "", "only events from claude, codex or unknown")
		flags.StringVar(&opts.window.format, "format", "json", "report format: json or csv")
	case "dashboard":
		flags.StringVar(&opts.addr, "addr", "127.0.0.1:0", "loopback address to listen on")
	case "benchmark":
		flags.StringVar(&opts.tasks, "tasks", "", "JSON task file to run against this project (default: built-in suite)")
	case "architecture":
		flags.StringVar(&opts.arch.Scope, "scope", "", "project-relative directory (default: whole project)")
	case "graph":
		flags.StringVar(&opts.graph.Symbol, "symbol", "", "Go symbol: Name, pkg.Name, Type.Method or pkg.Type.Method (default: most referenced)")
		flags.IntVar(&opts.graph.Depth, "depth", 1, "caller levels to follow, 1 to 3")
		flags.StringVar(&opts.graph.Scope, "scope", "", "project-relative directory for the most referenced list")
		flags.StringVar(&opts.graph.Dir, "dir", "", "package directory picking one of several same-named symbols")
	case "remember":
		flags.StringVar(&opts.memory.Title, "title", "", "short title, unique within the scope")
		flags.StringVar(&opts.memory.Body, "body", "", "the decision or fact")
		flags.StringVar(&opts.memory.Scope, "scope", "", "area it applies to (default project)")
		flags.Var((*stringList)(&opts.memory.SourcePaths), "source", "supporting project-relative file (repeatable)")
	}
	if opts.command == "install" || opts.command == "clean" {
		flags.BoolVar(&opts.yes, "yes", false, "apply the shown changes")
	}
	if opts.command == "clean" {
		flags.BoolVar(&opts.all, "all", false, "remove the whole .codememory directory, including memories and analytics")
	}
	if err := flags.Parse(rest); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, errors.New("unexpected arguments")
	}
	opts.context.IncludeArchitecture = &opts.withArc
	return opts, nil
}

// stringList is a repeatable string flag.
type stringList []string

func (l *stringList) String() string     { return strings.Join(*l, ",") }
func (l *stringList) Set(v string) error { *l = append(*l, v); return nil }

func openStore(ctx context.Context, root string) (*sqlite.Store, error) {
	dir, err := config.State(root, false)
	if err != nil {
		return nil, err
	}
	return sqlite.Open(ctx, dir, root)
}
