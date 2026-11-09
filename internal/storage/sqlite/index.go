package sqlite

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/kudaibergenoff/codememory/internal/parser"
	"github.com/kudaibergenoff/codememory/internal/scanner"
)

// Document is a new or changed file to (re)index.
type Document struct {
	Path, Hash, Language string
	Size, MtimeNS        int64
	Parsed               parser.Result
}

// Stat refreshes the recorded size/mtime of a file whose content is unchanged.
type Stat struct {
	Path          string
	Size, MtimeNS int64
}

// Batch is the complete set of changes computed from one scan.
type Batch struct {
	ConfigHash                            string
	Changed                               []Document
	Deleted                               []string
	Touched                               []Stat
	Skips                                 []scanner.Skipped
	SkipsChanged                          bool
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
func (s *Store) Update(ctx context.Context, build func(prior scanner.Prior, configHash string) (Batch, error)) (IndexResult, error) {
	var result IndexResult
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf("begin index update: %w", err)
	}
	defer tx.Rollback()
	var oldConfig string
	if err = tx.QueryRowContext(ctx, `SELECT generation,config_hash FROM project WHERE id=1`).Scan(&result.Generation, &oldConfig); err != nil {
		return result, fmt.Errorf("read project state: %w", err)
	}
	prior, err := loadPrior(ctx, tx)
	if err != nil {
		return result, err
	}
	batch, err := build(prior, oldConfig)
	if err != nil {
		return result, err
	}
	if err = apply(ctx, tx, batch); err != nil {
		return result, err
	}
	result = summarize(result.Generation, oldConfig, batch)
	if _, err = tx.ExecContext(ctx, `UPDATE project SET generation=?,last_scan_at=?,config_hash=? WHERE id=1`, result.Generation, time.Now().UTC().Format(time.RFC3339Nano), batch.ConfigHash); err != nil {
		return result, fmt.Errorf("publish generation: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return result, fmt.Errorf("commit index update: %w", err)
	}
	return result, nil
}

func summarize(generation int64, oldConfig string, b Batch) IndexResult {
	if len(b.Changed) > 0 || len(b.Deleted) > 0 || oldConfig != b.ConfigHash {
		generation++
	}
	return IndexResult{
		Generation: generation, Indexed: len(b.Changed), Reused: b.Reused, Deleted: len(b.Deleted),
		Skipped: b.Skipped, Secrets: b.Secrets, ParseErrors: b.ParseErrors,
	}
}

func apply(ctx context.Context, tx execer, b Batch) error {
	for _, p := range b.Deleted {
		if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE path=?`, p); err != nil {
			return fmt.Errorf("delete file: %w", err)
		}
	}
	for _, doc := range b.Changed {
		if err := insertDocument(ctx, tx, doc); err != nil {
			return err
		}
	}
	for _, st := range b.Touched {
		if _, err := tx.ExecContext(ctx, `UPDATE files SET size_bytes=?,mtime_ns=? WHERE path=?`, st.Size, st.MtimeNS, st.Path); err != nil {
			return fmt.Errorf("refresh file stat: %w", err)
		}
	}
	if b.SkipsChanged {
		return replaceSkips(ctx, tx, b.Skips)
	}
	return nil
}

func insertDocument(ctx context.Context, tx execer, doc Document) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE path=?`, doc.Path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	dir := path.Dir(doc.Path)
	res, err := tx.ExecContext(ctx, `INSERT INTO files(path,dir,sha256,size_bytes,mtime_ns,language) VALUES(?,?,?,?,?,?)`, doc.Path, dir, doc.Hash, doc.Size, doc.MtimeNS, doc.Language)
	if err != nil {
		return fmt.Errorf("insert file: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("file id: %w", err)
	}
	for _, sym := range doc.Parsed.Symbols {
		if _, err = tx.ExecContext(ctx, `INSERT INTO symbols(file_id,name,kind,start_line,end_line) VALUES(?,?,?,?,?)`, id, sym.Name, sym.Kind, sym.Start, sym.End); err != nil {
			return fmt.Errorf("insert symbol: %w", err)
		}
	}
	for _, r := range doc.Parsed.Refs {
		if _, err = tx.ExecContext(ctx, `INSERT INTO refs(file_id,dir,from_name,to_pkg,to_name,kind,line,uses) VALUES(?,?,?,?,?,?,?,?)`, id, dir, r.From, r.Pkg, r.Name, r.Kind, r.Line, r.Uses); err != nil {
			return fmt.Errorf("insert ref: %w", err)
		}
	}
	for _, chunk := range doc.Parsed.Chunks {
		if _, err = tx.ExecContext(ctx, `INSERT INTO chunks(file_id,path,symbol,start_line,end_line,content) VALUES(?,?,?,?,?,?)`, id, doc.Path, chunk.Symbol, chunk.Start, chunk.End, chunk.Content); err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}
	}
	return nil
}

func replaceSkips(ctx context.Context, tx execer, skips []scanner.Skipped) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM skipped_files`); err != nil {
		return fmt.Errorf("clear skipped files: %w", err)
	}
	for _, sk := range skips {
		if _, err := tx.ExecContext(ctx, `INSERT INTO skipped_files(path,size_bytes,mtime_ns,reason) VALUES(?,?,?,?)`, sk.Path, sk.Size, sk.MtimeNS, string(sk.Reason)); err != nil {
			return fmt.Errorf("record skipped file: %w", err)
		}
	}
	return nil
}
