package sqlite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Footprint counts what the local state holds.
type Footprint struct {
	Files        int   `json:"files"`
	Chunks       int   `json:"chunks"`
	CacheEntries int   `json:"query_cache_entries"`
	Memories     int   `json:"memories"`
	Events       int   `json:"analytics_events"`
	DBBytes      int64 `json:"database_bytes"`
}

func (s *Store) Footprint(ctx context.Context) (Footprint, error) {
	var f Footprint
	err := s.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM files),(SELECT count(*) FROM chunks),(SELECT count(*) FROM query_cache),(SELECT count(*) FROM memories),(SELECT count(*) FROM events)`).
		Scan(&f.Files, &f.Chunks, &f.CacheEntries, &f.Memories, &f.Events)
	if err != nil {
		return f, fmt.Errorf("measure state: %w", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if info, err := os.Stat(filepath.Join(s.dir, "index.db"+suffix)); err == nil {
			f.DBBytes += info.Size()
		}
	}
	return f, nil
}

// ResetIndex drops derived data (files, chunks, symbols, skip cache, query
// cache) and forces the next refresh to re-read everything. Curated memories,
// analytics and the generation counter are kept.
func (s *Store) ResetIndex(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin reset: %w", err)
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		`DELETE FROM files`,
		`DELETE FROM skipped_files`,
		`DELETE FROM query_cache`,
		`UPDATE project SET config_hash='',last_scan_at='' WHERE id=1`,
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("reset index: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit reset: %w", err)
	}
	if _, err = s.DB.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("compact database: %w", err)
	}
	return nil
}
