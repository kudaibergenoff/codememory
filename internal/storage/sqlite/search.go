package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

type Source struct {
	Path  string `json:"path"`
	Start int    `json:"start_line"`
	End   int    `json:"end_line"`
	Hash  string `json:"sha256"`
}
type Match struct {
	Source
	Symbol  string  `json:"symbol"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}
type SearchResult struct {
	Generation int64
	Matches    []Match
}

const searchSQL = `SELECT c.path,c.start_line,c.end_line,f.sha256,c.symbol,c.content,bm25(chunks_fts,5.0,8.0,1.0)
 FROM chunks_fts JOIN chunks c ON c.id=chunks_fts.rowid JOIN files f ON f.id=c.file_id
 WHERE chunks_fts MATCH ? AND (?='' OR f.language=?) AND (?='' OR substr(c.path,1,length(?))=?)
 ORDER BY bm25(chunks_fts,5.0,8.0,1.0),c.path,c.start_line,c.id LIMIT ?`

// Search reads the generation and matches from one consistent snapshot.
// Freshness is the caller's responsibility (see indexer.Index / indexer.Check).
func (s *Store) Search(ctx context.Context, expression, language, prefix string, limit int) (SearchResult, error) {
	result := SearchResult{Matches: []Match{}}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, fmt.Errorf("begin search: %w", err)
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT generation FROM project WHERE id=1`).Scan(&result.Generation); err != nil {
		return result, fmt.Errorf("read generation: %w", err)
	}
	err = eachRow(ctx, tx, searchSQL, func(rows *sql.Rows) error {
		var m Match
		if err := rows.Scan(&m.Path, &m.Start, &m.End, &m.Hash, &m.Symbol, &m.Snippet, &m.Score); err != nil {
			return err
		}
		result.Matches = append(result.Matches, m)
		return nil
	}, expression, language, language, prefix, prefix, prefix, limit)
	if err != nil {
		return result, fmt.Errorf("full-text search: %w", err)
	}
	return result, tx.Commit()
}
