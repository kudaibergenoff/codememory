// Package indexer coordinates scanning and parsing; storage publishes one generation.
package indexer

import (
	"context"
	"encoding/json"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/parser"
	"github.com/kudaibergenoff/codememory/internal/scanner"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

func ConfigHash(cfg config.Config) string {
	b, _ := json.Marshal(cfg)
	return scanner.Hash(append(b, []byte(parser.Version+":"+scanner.Version)...))
}
func Index(ctx context.Context, s *sqlite.Store, root string, cfg config.Config) (sqlite.IndexResult, error) {
	return s.Update(ctx, func(old map[string]sqlite.FileState, oldConfig string) (sqlite.Batch, error) {
		batch := sqlite.Batch{ConfigHash: ConfigHash(cfg)}
		snap, err := scanner.Scan(ctx, root, cfg)
		if err != nil {
			return batch, err
		}
		batch.Skipped = snap.Skipped
		batch.Secrets = snap.Secrets
		for _, f := range snap.Files {
			if err := ctx.Err(); err != nil {
				return batch, err
			}
			batch.Paths = append(batch.Paths, f.Path)
			if old[f.Path].Hash == f.Hash && oldConfig == batch.ConfigHash {
				batch.Reused++
				continue
			}
			parsed := parser.Parse(f.Path, f.Language, f.Content, cfg.ChunkLines)
			if parsed.ParseError {
				batch.ParseErrors++
			}
			batch.Changed = append(batch.Changed, sqlite.Document{Path: f.Path, Hash: f.Hash, Language: f.Language, Size: len(f.Content), Parsed: parsed})
		}
		return batch, nil
	})
}
