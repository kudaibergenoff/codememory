package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrMemoryLimit is returned when a new memory would exceed the per-project cap.
var ErrMemoryLimit = errors.New("memory limit reached; update or remove existing memories")

// MemorySource is a file a memory cites, with its SHA-256 at write time.
type MemorySource struct {
	Path string `json:"path"`
	Hash string `json:"sha256"`
}

type MemoryRow struct {
	ID                   int64
	Scope, Title, Body   string
	Status               string
	Sources              []MemorySource
	CreatedAt, UpdatedAt string
}

// FileHashes returns the indexed SHA-256 of each path that is in the index.
func (s *Store) FileHashes(ctx context.Context, paths []string) (map[string]string, error) {
	hashes := make(map[string]string, len(paths))
	if len(paths) == 0 {
		return hashes, nil
	}
	list, err := json.Marshal(paths)
	if err != nil {
		return nil, fmt.Errorf("encode paths: %w", err)
	}
	// One query for any number of paths instead of one per path.
	err = eachRow(ctx, s.DB, `SELECT path,sha256 FROM files WHERE path IN (SELECT value FROM json_each(?))`, func(r *sql.Rows) error {
		var p, h string
		if err := r.Scan(&p, &h); err != nil {
			return err
		}
		hashes[p] = h
		return nil
	}, string(list))
	if err != nil {
		return nil, fmt.Errorf("read file hashes: %w", err)
	}
	return hashes, nil
}

// UpsertMemory inserts a memory or replaces the one with the same scope and
// title, and bumps memory_revision. New rows beyond limit are refused.
func (s *Store) UpsertMemory(ctx context.Context, row MemoryRow, limit int) (id int64, updated bool, err error) {
	sources, err := json.Marshal(row.Sources)
	if err != nil {
		return 0, false, fmt.Errorf("encode sources: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("begin memory write: %w", err)
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT id FROM memories WHERE scope=? AND title=?`, row.Scope, row.Title).Scan(&id)
	switch {
	case err == nil:
		updated = true
		_, err = tx.ExecContext(ctx, `UPDATE memories SET body=?,sources_json=?,status=?,updated_at=? WHERE id=?`, row.Body, string(sources), row.Status, now, id)
	case errors.Is(err, sql.ErrNoRows):
		id, err = insertMemory(ctx, tx, row, string(sources), now, limit)
	}
	if err != nil {
		return 0, false, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE project SET memory_revision=memory_revision+1 WHERE id=1`); err != nil {
		return 0, false, fmt.Errorf("bump memory revision: %w", err)
	}
	return id, updated, tx.Commit()
}

func insertMemory(ctx context.Context, tx *sql.Tx, row MemoryRow, sources, now string, limit int) (int64, error) {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM memories`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count memories: %w", err)
	}
	if count >= limit {
		return 0, ErrMemoryLimit
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO memories(scope,title,body,sources_json,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, row.Scope, row.Title, row.Body, sources, row.Status, now, now)
	if err != nil {
		return 0, fmt.Errorf("insert memory: %w", err)
	}
	return res.LastInsertId()
}

// Memories returns all memories ordered by scope, then most recently updated.
func (s *Store) Memories(ctx context.Context) ([]MemoryRow, error) {
	var rows []MemoryRow
	err := eachRow(ctx, s.DB, `SELECT id,scope,title,body,status,sources_json,created_at,updated_at FROM memories ORDER BY scope,updated_at DESC,id`, func(r *sql.Rows) error {
		var m MemoryRow
		var sources string
		if err := r.Scan(&m.ID, &m.Scope, &m.Title, &m.Body, &m.Status, &sources, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(sources), &m.Sources); err != nil {
			return fmt.Errorf("decode sources of memory %d: %w", m.ID, err)
		}
		rows = append(rows, m)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list memories: %w", err)
	}
	return rows, nil
}

// MemoryRevision changes whenever any memory is written.
func (s *Store) MemoryRevision(ctx context.Context) (int64, error) {
	var rev int64
	err := s.DB.QueryRowContext(ctx, `SELECT memory_revision FROM project WHERE id=1`).Scan(&rev)
	return rev, err
}
