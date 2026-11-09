// Package cli implements commands without process-global flags or exits.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/contextpack"
	"github.com/kudaibergenoff/codememory/internal/mcp"
	"github.com/kudaibergenoff/codememory/internal/memory"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const Version = "0.1.0-dev"

const usage = `codememory <init|index|search|context|status|stats|serve|version> [--project DIR]
index:   [--full]  re-read every file, bypassing the size/mtime prefilter
search:  --query TEXT [--limit 10] [--language go] [--path-prefix internal/]
context: --task TEXT [--max-tokens 1500]
remember: --title TEXT --body TEXT [--scope DIR] [--source PATH]...`

var commands = map[string]bool{"init": true, "index": true, "search": true, "context": true, "remember": true, "status": true, "stats": true, "serve": true}

type options struct {
	command string
	project string
	full    bool
	search  search.Input
	context contextpack.Input
	memory  memory.Input
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
	if opts.command == "serve" {
		return mcp.Serve(ctx, s, root, Version)
	}
	return execute(ctx, opts, s, root, cfg, out)
}

func parse(args []string, errOut io.Writer) (options, error) {
	opts := options{command: args[0]}
	if !commands[opts.command] {
		return opts, fmt.Errorf("unknown command %q", opts.command)
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
	case "remember":
		flags.StringVar(&opts.memory.Title, "title", "", "short title, unique within the scope")
		flags.StringVar(&opts.memory.Body, "body", "", "the decision or fact")
		flags.StringVar(&opts.memory.Scope, "scope", "", "area it applies to (default project)")
		flags.Var((*stringList)(&opts.memory.SourcePaths), "source", "supporting project-relative file (repeatable)")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return opts, err
	}
	if flags.NArg() != 0 {
		return opts, errors.New("unexpected arguments")
	}
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
