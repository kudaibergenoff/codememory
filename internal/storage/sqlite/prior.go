package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kudaibergenoff/codememory/internal/scanner"
)

type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Prior returns the recorded file and skip state plus the config hash of the
// last index, read in one transaction. It never writes.
func (s *Store) Prior(ctx context.Context) (scanner.Prior, string, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, "", fmt.Errorf("begin read: %w", err)
	}
	defer tx.Rollback()
	var configHash string
	if err = tx.QueryRowContext(ctx, `SELECT config_hash FROM project WHERE id=1`).Scan(&configHash); err != nil {
		return nil, "", fmt.Errorf("read config hash: %w", err)
	}
	prior, err := loadPrior(ctx, tx)
	if err != nil {
		return nil, "", err
	}
	return prior, configHash, tx.Commit()
}

func loadPrior(ctx context.Context, q queryer) (scanner.Prior, error) {
	prior := scanner.Prior{}
	err := eachRow(ctx, q, `SELECT path,sha256,size_bytes,mtime_ns FROM files`, func(rows *sql.Rows) error {
		var p string
		var e scanner.Entry
		if err := rows.Scan(&p, &e.Hash, &e.Size, &e.MtimeNS); err != nil {
			return err
		}
		prior[p] = e
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load indexed files: %w", err)
	}
	err = eachRow(ctx, q, `SELECT path,size_bytes,mtime_ns,reason FROM skipped_files`, func(rows *sql.Rows) error {
		var p, reason string
		var e scanner.Entry
		if err := rows.Scan(&p, &e.Size, &e.MtimeNS, &reason); err != nil {
			return err
		}
		e.Skip = scanner.SkipReason(reason)
		prior[p] = e
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load skipped files: %w", err)
	}
	return prior, nil
}

func eachRow(ctx context.Context, q queryer, query string, fn func(*sql.Rows) error, args ...any) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err = fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
