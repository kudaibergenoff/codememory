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

// Query is one full-text search. Expression and Exact are literal FTS5
// expressions built by package search; Exact may be empty.
type Query struct {
	Expression string
	// Exact matches compound identifiers as phrases; chunks that contain one
	// get ExactWeight times their BM25 strength.
	Exact      string
	Language   string
	PathPrefix string
	Limit      int
}

// ExactWeight scales the BM25 strength of chunks that contain a queried
// compound identifier verbatim (e.g. memory_context), so loose matches of its
// parts, or a query word in the heavier symbol column, do not outrank it. It
// is applied in SQL, before LIMIT, so exact matches are never cut off.
const ExactWeight = 2.0

const (
	searchSelect = `SELECT c.path,c.start_line,c.end_line,f.sha256,c.symbol,c.content,%s AS score
 FROM chunks_fts JOIN chunks c ON c.id=chunks_fts.rowid JOIN files f ON f.id=c.file_id
 WHERE chunks_fts MATCH ? AND (?='' OR f.language=?) AND (?='' OR substr(c.path,1,length(?))=?)
 ORDER BY score,c.path,c.start_line,c.id LIMIT ?`
	bm25Score  = `bm25(chunks_fts,5.0,8.0,1.0)`
	exactScore = bm25Score + ` * (CASE WHEN c.id IN (SELECT rowid FROM chunks_fts WHERE chunks_fts MATCH ?) THEN %g ELSE 1.0 END)`
)

var (
	searchSQL      = fmt.Sprintf(searchSelect, bm25Score)
	exactSearchSQL = fmt.Sprintf(searchSelect, fmt.Sprintf(exactScore, ExactWeight))
)

// Search reads the generation and matches from one consistent snapshot.
// Freshness is the caller's responsibility (see indexer.Index / indexer.Check).
func (s *Store) Search(ctx context.Context, q Query) (SearchResult, error) {
	result := SearchResult{Matches: []Match{}}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, fmt.Errorf("begin search: %w", err)
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `SELECT generation FROM project WHERE id=1`).Scan(&result.Generation); err != nil {
		return result, fmt.Errorf("read generation: %w", err)
	}
	query, args := searchSQL, []any{q.Expression, q.Language, q.Language, q.PathPrefix, q.PathPrefix, q.PathPrefix, q.Limit}
	if q.Exact != "" {
		query, args = exactSearchSQL, append([]any{q.Exact}, args...)
	}
	err = eachRow(ctx, tx, query, func(rows *sql.Rows) error {
		var m Match
		if err := rows.Scan(&m.Path, &m.Start, &m.End, &m.Hash, &m.Symbol, &m.Snippet, &m.Score); err != nil {
			return err
		}
		result.Matches = append(result.Matches, m)
		return nil
	}, args...)
	if err != nil {
		return result, fmt.Errorf("full-text search: %w", err)
	}
	return result, tx.Commit()
}
