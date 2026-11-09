package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const keepBenchmarks = 20

// SaveBenchmark stores an encoded benchmark report and keeps the newest runs.
func (s *Store) SaveBenchmark(ctx context.Context, suite string, report []byte) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin benchmark write: %w", err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO benchmark_runs(at_ns,suite,report) VALUES(?,?,?)`, time.Now().UnixNano(), suite, string(report)); err != nil {
		return fmt.Errorf("save benchmark: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM benchmark_runs WHERE id NOT IN (SELECT id FROM benchmark_runs ORDER BY id DESC LIMIT ?)`, keepBenchmarks); err != nil {
		return fmt.Errorf("prune benchmarks: %w", err)
	}
	return tx.Commit()
}

// LatestBenchmark returns the newest encoded report, if any.
func (s *Store) LatestBenchmark(ctx context.Context) ([]byte, bool, error) {
	var report string
	err := s.DB.QueryRowContext(ctx, `SELECT report FROM benchmark_runs ORDER BY id DESC LIMIT 1`).Scan(&report)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read benchmark: %w", err)
	}
	return []byte(report), true, nil
}
