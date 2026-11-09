package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// PathSize is an indexed file and its size in bytes.
type PathSize struct {
	Path string
	Size int64
}

// IndexedSizes lists every indexed file with its size.
func (s *Store) IndexedSizes(ctx context.Context) ([]PathSize, error) {
	var out []PathSize
	err := eachRow(ctx, s.DB, `SELECT path,size_bytes FROM files ORDER BY path`, func(r *sql.Rows) error {
		var p PathSize
		if err := r.Scan(&p.Path, &p.Size); err != nil {
			return err
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list indexed sizes: %w", err)
	}
	return out, nil
}

// SkipCounts counts content-inspected skips by reason (content, secret).
func (s *Store) SkipCounts(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{}
	err := eachRow(ctx, s.DB, `SELECT reason,count(*) FROM skipped_files GROUP BY reason`, func(r *sql.Rows) error {
		var reason string
		var n int
		if err := r.Scan(&reason, &n); err != nil {
			return err
		}
		counts[reason] = n
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("count skipped files: %w", err)
	}
	return counts, nil
}

// FilesByLanguage counts indexed files per detected language.
func (s *Store) FilesByLanguage(ctx context.Context) (map[string]int, error) {
	counts := map[string]int{}
	err := eachRow(ctx, s.DB, `SELECT language,count(*) FROM files GROUP BY language`, func(r *sql.Rows) error {
		var lang string
		var n int
		if err := r.Scan(&lang, &n); err != nil {
			return err
		}
		counts[lang] = n
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("count files by language: %w", err)
	}
	return counts, nil
}
