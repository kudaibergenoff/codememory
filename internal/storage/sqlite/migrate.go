package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// migrationFiles are applied in order; user_version records how many ran.
var migrationFiles = []string{
	"001_initial.sql",
	"002_stat_prefilter.sql",
	"003_memories.sql",
	"004_query_cache.sql",
	"005_event_time_agent.sql",
	"006_benchmark_runs.sql",
	"007_event_provenance.sql",
	"008_code_graph.sql",
}

var schemaVersion = len(migrationFiles)

func (s *Store) migrate(ctx context.Context, root string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > schemaVersion {
		return errors.New("database schema is newer than this binary")
	}
	for v := version + 1; v <= schemaVersion; v++ {
		if err = applyMigration(ctx, tx, v, root); err != nil {
			return err
		}
	}
	if err = checkRoot(ctx, tx, root); err != nil {
		return err
	}
	return tx.Commit()
}

func applyMigration(ctx context.Context, tx *sql.Tx, version int, root string) error {
	name := migrationFiles[version-1]
	b, err := migrations.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}
	if _, err = tx.ExecContext(ctx, string(b)); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if version == 1 {
		if _, err = tx.ExecContext(ctx, "INSERT INTO project(id,root) VALUES(1,?)", root); err != nil {
			return fmt.Errorf("register project: %w", err)
		}
	}
	// PRAGMA does not accept bound parameters; version is a trusted integer.
	if _, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", version)); err != nil {
		return fmt.Errorf("record schema version: %w", err)
	}
	return nil
}

func checkRoot(ctx context.Context, tx *sql.Tx, root string) error {
	var stored string
	if err := tx.QueryRowContext(ctx, "SELECT root FROM project WHERE id=1").Scan(&stored); err != nil {
		return fmt.Errorf("read project root: %w", err)
	}
	if root != stored {
		return errors.New("database belongs to another project root; moved projects require a fresh index")
	}
	return nil
}
