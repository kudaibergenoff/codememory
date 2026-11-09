package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/kudaibergenoff/codememory/internal/parser"
)

type FileState struct {
	Hash string
	ID   int64
}
type Document struct {
	Path, Hash, Language string
	Size                 int
	Parsed               parser.Result
}
type Batch struct {
	Paths                                 []string
	Changed                               []Document
	ConfigHash                            string
	Reused, Skipped, Secrets, ParseErrors int
}
type IndexResult struct {
	Generation  int64 `json:"index_generation"`
	Indexed     int   `json:"indexed"`
	Reused      int   `json:"reused"`
	Deleted     int   `json:"deleted"`
	Skipped     int   `json:"skipped"`
	Secrets     int   `json:"secret_files_omitted"`
	ParseErrors int   `json:"go_parse_errors"`
}

// Update holds the writer reservation before scanning, preventing an older scan
// from overwriting a newer generation from another indexer process.
func (s *Store) Update(ctx context.Context, build func(map[string]FileState, string) (Batch, error)) (IndexResult, error) {
	var result IndexResult
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var cfg string
	if err = tx.QueryRowContext(ctx, `SELECT generation,config_hash FROM project WHERE id=1`).Scan(&result.Generation, &cfg); err != nil {
		return result, err
	}
	states, err := fileStates(ctx, tx)
	if err != nil {
		return result, err
	}
	batch, err := build(states, cfg)
	if err != nil {
		return result, err
	}
	seen := make(map[string]bool, len(batch.Paths))
	for _, p := range batch.Paths {
		seen[p] = true
	}
	for p, state := range states {
		if !seen[p] {
			if _, err = tx.ExecContext(ctx, `DELETE FROM files WHERE id=?`, state.ID); err != nil {
				return result, err
			}
			result.Deleted++
		}
	}
	for _, doc := range batch.Changed {
		if _, err = tx.ExecContext(ctx, `DELETE FROM files WHERE path=?`, doc.Path); err != nil {
			return result, err
		}
		res, e := tx.ExecContext(ctx, `INSERT INTO files(path,sha256,size_bytes,language) VALUES(?,?,?,?)`, doc.Path, doc.Hash, doc.Size, doc.Language)
		if e != nil {
			return result, e
		}
		id, e := res.LastInsertId()
		if e != nil {
			return result, e
		}
		for _, sym := range doc.Parsed.Symbols {
			if _, err = tx.ExecContext(ctx, `INSERT INTO symbols(file_id,name,kind,start_line,end_line) VALUES(?,?,?,?,?)`, id, sym.Name, sym.Kind, sym.Start, sym.End); err != nil {
				return result, err
			}
		}
		for _, chunk := range doc.Parsed.Chunks {
			if _, err = tx.ExecContext(ctx, `INSERT INTO chunks(file_id,path,symbol,start_line,end_line,content) VALUES(?,?,?,?,?,?)`, id, doc.Path, chunk.Symbol, chunk.Start, chunk.End, chunk.Content); err != nil {
				return result, err
			}
		}
	}
	result.Indexed = len(batch.Changed)
	result.Reused = batch.Reused
	result.Skipped = batch.Skipped
	result.Secrets = batch.Secrets
	result.ParseErrors = batch.ParseErrors
	if result.Indexed > 0 || result.Deleted > 0 || cfg != batch.ConfigHash {
		result.Generation++
	}
	if _, err = tx.ExecContext(ctx, `UPDATE project SET generation=?,last_scan_at=?,config_hash=? WHERE id=1`, result.Generation, time.Now().UTC().Format(time.RFC3339Nano), batch.ConfigHash); err != nil {
		return result, err
	}
	return result, tx.Commit()
}
func fileStates(ctx context.Context, tx *sql.Tx) (map[string]FileState, error) {
	rows, err := tx.QueryContext(ctx, `SELECT path,sha256,id FROM files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := map[string]FileState{}
	for rows.Next() {
		var p string
		var state FileState
		if err = rows.Scan(&p, &state.Hash, &state.ID); err != nil {
			return nil, err
		}
		states[p] = state
	}
	return states, rows.Err()
}
