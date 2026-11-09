package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CacheGet returns the payload stored for key at generation if it has not expired.
func (s *Store) CacheGet(ctx context.Context, key string, generation, nowNS int64) ([]byte, bool, error) {
	var payload string
	err := s.DB.QueryRowContext(ctx, `SELECT response FROM query_cache WHERE cache_key=? AND generation=? AND expires_at_ns>?`, key, generation, nowNS).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read cache: %w", err)
	}
	return []byte(payload), true, nil
}

// CachePut stores payload for key, drops entries from other generations or
// past expiry, and keeps at most maxEntries of the newest entries.
func (s *Store) CachePut(ctx context.Context, key string, generation int64, payload []byte, nowNS, expiresNS int64, maxEntries int) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cache write: %w", err)
	}
	defer tx.Rollback()
	steps := []struct {
		query string
		args  []any
	}{
		// Only older generations: a late writer must never evict newer entries.
		{`DELETE FROM query_cache WHERE generation<? OR expires_at_ns<=?`, []any{generation, nowNS}},
		{`INSERT OR REPLACE INTO query_cache(cache_key,generation,response,created_at_ns,expires_at_ns) VALUES(?,?,?,?,?)`, []any{key, generation, string(payload), nowNS, expiresNS}},
		{`DELETE FROM query_cache WHERE rowid IN (SELECT rowid FROM query_cache ORDER BY created_at_ns DESC,rowid DESC LIMIT -1 OFFSET ?)`, []any{maxEntries}},
	}
	for _, st := range steps {
		if _, err = tx.ExecContext(ctx, st.query, st.args...); err != nil {
			return fmt.Errorf("write cache: %w", err)
		}
	}
	return tx.Commit()
}
