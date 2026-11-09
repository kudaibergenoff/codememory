// Package sqlite owns migrations, transactions, and SQL. One database belongs to one project.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ DB *sql.DB }

func Open(ctx context.Context, dir, root string) (*Store, error) {
	filename := filepath.Join(dir, "index.db")
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(filename)}
	q := u.Query()
	q.Set("_foreign_keys", "on")
	q.Set("_journal_mode", "WAL")
	q.Set("_busy_timeout", "5000")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if err = s.migrate(ctx, root); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { return s.DB.Close() }
func (s *Store) migrate(ctx context.Context, root string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return errors.New("database schema is newer than this binary")
	}
	if version == 0 {
		b, e := migrations.ReadFile("migrations/001_initial.sql")
		if e != nil {
			return e
		}
		if _, err = tx.ExecContext(ctx, string(b)); err != nil {
			return fmt.Errorf("migrate (build with -tags sqlite_fts5): %w", err)
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO project(id,root) VALUES(1,?)", root); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
			return err
		}
	}
	var stored string
	if err = tx.QueryRowContext(ctx, "SELECT root FROM project WHERE id=1").Scan(&stored); err != nil {
		return err
	}
	if root != stored {
		return errors.New("database belongs to another project root; moved projects require a fresh index")
	}
	return tx.Commit()
}

type Status struct {
	Generation int64  `json:"index_generation"`
	LastScan   string `json:"last_scan_at"`
	Files      int    `json:"files"`
	Chunks     int    `json:"chunks"`
	Symbols    int    `json:"symbols"`
}

func (s *Store) Status(ctx context.Context) (Status, error) {
	var v Status
	err := s.DB.QueryRowContext(ctx, `SELECT generation,last_scan_at,(SELECT count(*) FROM files),(SELECT count(*) FROM chunks),(SELECT count(*) FROM symbols) FROM project WHERE id=1`).Scan(&v.Generation, &v.LastScan, &v.Files, &v.Chunks, &v.Symbols)
	return v, err
}
