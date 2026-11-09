// Package indexer coordinates scanning and parsing; storage publishes one generation.
package indexer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/parser"
	"github.com/kudaibergenoff/codememory/internal/scanner"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

// ConfigHash identifies everything that decides which files are indexed and how.
// A change forces a full re-read, so new exclusion or secret rules also apply to
// files whose stat has not changed.
func ConfigHash(cfg config.Config) string {
	b, _ := json.Marshal(cfg)
	return scanner.Hash(append(b, []byte(parser.Version+":"+scanner.RulesVersion)...))
}

// Index brings the index up to date. Only files whose size or mtime changed are
// read; only files whose SHA-256 changed are re-parsed. It is cheap enough to
// run before every search.
func Index(ctx context.Context, s *sqlite.Store, root string, cfg config.Config) (sqlite.IndexResult, error) {
	return index(ctx, s, root, cfg, false)
}

// IndexFull re-reads and re-parses every file, bypassing the stat prefilter.
// Use it after tools that change content but preserve size and mtime.
func IndexFull(ctx context.Context, s *sqlite.Store, root string, cfg config.Config) (sqlite.IndexResult, error) {
	return index(ctx, s, root, cfg, true)
}

func index(ctx context.Context, s *sqlite.Store, root string, cfg config.Config, force bool) (sqlite.IndexResult, error) {
	cfgHash := ConfigHash(cfg)
	return s.Update(ctx, func(prior scanner.Prior, oldConfig string) (sqlite.Batch, error) {
		d, err := diff(ctx, root, cfg, prior, force || oldConfig != cfgHash)
		if err != nil {
			return sqlite.Batch{}, err
		}
		return d.batch(cfg, cfgHash), nil
	})
}

// Freshness describes whether the index matches the permitted files on disk.
type Freshness struct {
	Stale   bool
	Secrets int
}

// Check compares the index with the project without writing to the index.
func Check(ctx context.Context, s *sqlite.Store, root string, cfg config.Config) (Freshness, error) {
	prior, oldConfig, err := s.Prior(ctx)
	if err != nil {
		return Freshness{}, fmt.Errorf("read index state: %w", err)
	}
	configChanged := oldConfig != ConfigHash(cfg)
	d, err := diff(ctx, root, cfg, prior, configChanged)
	if err != nil {
		return Freshness{}, err
	}
	stale := configChanged || len(d.changed) > 0 || len(d.deleted) > 0
	return Freshness{Stale: stale, Secrets: d.snap.Secrets}, nil
}
