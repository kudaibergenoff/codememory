// Package cli implements commands without process-global flags or exits.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/indexer"
	"github.com/kudaibergenoff/codememory/internal/mcp"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const Version = "0.1.0-dev"

func Run(ctx context.Context, args []string, out, errOut io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := fmt.Fprintln(out, "codememory <init|index|search|status|stats|serve|version> [--project DIR]\nsearch: --query TEXT [--limit 10] [--language go] [--path-prefix internal/]")
		return err
	}
	if args[0] == "version" {
		_, err := fmt.Fprintln(out, Version)
		return err
	}
	switch args[0] {
	case "init", "index", "search", "status", "stats", "serve":
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(errOut)
	project := flags.String("project", ".", "trusted project directory")
	var input search.Input
	if args[0] == "search" {
		flags.StringVar(&input.Query, "query", "", "plain text query")
		flags.IntVar(&input.Limit, "limit", 10, "maximum matches (1 to 20)")
		flags.StringVar(&input.Language, "language", "", "language filter")
		flags.StringVar(&input.PathPrefix, "path-prefix", "", "relative path prefix")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	root, err := config.Root(*project)
	if err != nil {
		return err
	}
	if args[0] == "init" {
		if err = config.Init(root); err != nil {
			return err
		}
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	dir, err := config.State(root, false)
	if err != nil {
		return err
	}
	s, err := sqlite.Open(ctx, dir, root)
	if err != nil {
		return err
	}
	defer s.Close()
	if args[0] == "serve" {
		return mcp.Serve(ctx, s, root, Version)
	}
	started := time.Now()
	event := analytics.Event{Channel: "cli", Tool: args[0]}
	var result any
	switch args[0] {
	case "init":
		result = map[string]any{"project": filepath.Base(root), "initialized": true, "warning": "Exclude .codememory/ from version control"}
	case "status":
		result, err = search.Status(ctx, s, root, cfg)
	case "search":
		result, err = search.Query(ctx, s, root, cfg, input)
	case "index":
		var indexed sqlite.IndexResult
		indexed, err = indexer.Index(ctx, s, root, cfg)
		result = indexed
		event.Indexed = indexed.Indexed
		event.Reused = indexed.Reused
		event.Generation = indexed.Generation
	case "stats":
		result, err = analytics.Stats(ctx, s.DB)
	}
	if response, ok := result.(search.Response); ok {
		event.Generation = response.Generation
	}
	var payload []byte
	if err == nil {
		payload, err = json.Marshal(result)
	}
	event.ResponseBytes = len(payload)
	event.Duration = time.Since(started)
	event.Failed = err != nil
	if args[0] != "stats" && args[0] != "init" {
		if metricErr := analytics.Record(ctx, s.DB, event); metricErr != nil {
			slog.Warn("metric recording failed")
		}
	}
	if err != nil {
		return err
	}
	_, err = out.Write(append(payload, '\n'))
	return err
}
