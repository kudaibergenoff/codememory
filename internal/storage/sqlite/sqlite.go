// Package sqlite owns migrations, transactions, and SQL. One database belongs to one project.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ DB *sql.DB }

const (
	driverName    = "sqlite"
	busyTimeoutMS = 5000
)

// dsn builds a modernc.org/sqlite URI. Pragmas run on every new connection;
// immediate transactions reserve the writer lock up front (see ADR-001).
func dsn(filename string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(filename)}
	pragmas := []string{
		"_pragma=foreign_keys(1)",
		"_pragma=journal_mode(WAL)",
		fmt.Sprintf("_pragma=busy_timeout(%d)", busyTimeoutMS),
		"_txlock=immediate",
	}
	return u.String() + "?" + strings.Join(pragmas, "&")
}

func Open(ctx context.Context, dir, root string) (*Store, error) {
	filename := filepath.Join(dir, "index.db")
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, dsn(filename))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
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
