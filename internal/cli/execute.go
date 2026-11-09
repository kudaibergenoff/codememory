package cli

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/architecture"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/contextpack"
	"github.com/kudaibergenoff/codememory/internal/indexer"
	"github.com/kudaibergenoff/codememory/internal/memory"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

// execute runs one JSON-producing command and records a local metric event.
func execute(ctx context.Context, opts options, s *sqlite.Store, root string, cfg config.Config, out io.Writer) error {
	started := time.Now()
	event := analytics.Event{Channel: "cli", Tool: opts.command}
	result, err := dispatch(ctx, opts, s, root, cfg, &event)
	if r, ok := result.(interface {
		IndexGeneration() int64
		CacheHit() *bool
	}); ok {
		event.Generation, event.CacheHit = r.IndexGeneration(), r.CacheHit()
	}
	var payload []byte
	if err == nil {
		payload, err = search.Encode(result)
	}
	event.ResponseBytes = len(payload)
	event.Duration = time.Since(started)
	event.Failed = err != nil
	if opts.command != "stats" && opts.command != "init" {
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

func dispatch(ctx context.Context, opts options, s *sqlite.Store, root string, cfg config.Config, event *analytics.Event) (any, error) {
	switch opts.command {
	case "init":
		return map[string]any{"project": filepath.Base(root), "initialized": true, "warning": "Exclude .codememory/ from version control"}, nil
	case "status":
		return search.Status(ctx, s, root, cfg)
	case "search":
		return search.Query(ctx, s, root, cfg, opts.search)
	case "context":
		return contextpack.Build(ctx, s, root, cfg, opts.context)
	case "architecture":
		return architecture.Describe(ctx, s, root, cfg, opts.arch)
	case "remember":
		return memory.Remember(ctx, s, root, cfg, opts.memory)
	case "index":
		run := indexer.Index
		if opts.full {
			run = indexer.IndexFull
		}
		indexed, err := run(ctx, s, root, cfg)
		event.Indexed, event.Reused, event.Generation = indexed.Indexed, indexed.Reused, indexed.Generation
		return indexed, err
	default: // "stats"
		return analytics.Stats(ctx, s.DB)
	}
}
