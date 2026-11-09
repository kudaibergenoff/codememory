package sqlite

import (
	"context"
	"database/sql"
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
	Stale      bool
	Matches    []Match
	Status     Status
}

// Search checks the snapshot and reads results within the same SQLite transaction.
// Empty expression provides the freshness-aware status without an FTS query.
func (s *Store) Search(ctx context.Context, expression, language, prefix string, limit int, hashes map[string]string, configHash string) (SearchResult, error) {
	result := SearchResult{Matches: []Match{}}
	// The driver's immediate transaction serializes this short read with publication.
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	var storedConfig string
	err = tx.QueryRowContext(ctx, `SELECT generation,last_scan_at,config_hash,(SELECT count(*) FROM files),(SELECT count(*) FROM chunks),(SELECT count(*) FROM symbols) FROM project WHERE id=1`).Scan(&result.Status.Generation, &result.Status.LastScan, &storedConfig, &result.Status.Files, &result.Status.Chunks, &result.Status.Symbols)
	if err != nil {
		return result, err
	}
	result.Generation = result.Status.Generation
	states, err := fileStates(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Stale = result.Status.LastScan == "" || storedConfig != configHash || len(states) != len(hashes)
	for p, v := range states {
		if hashes[p] != v.Hash {
			result.Stale = true
			break
		}
	}
	if result.Stale || expression == "" {
		return result, tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT c.path,c.start_line,c.end_line,f.sha256,c.symbol,c.content,bm25(chunks_fts,5.0,8.0,1.0)
 FROM chunks_fts JOIN chunks c ON c.id=chunks_fts.rowid JOIN files f ON f.id=c.file_id
 WHERE chunks_fts MATCH ? AND (?='' OR f.language=?) AND (?='' OR substr(c.path,1,length(?))=?)
 ORDER BY bm25(chunks_fts,5.0,8.0,1.0),c.path,c.start_line,c.id LIMIT ?`, expression, language, language, prefix, prefix, prefix, limit)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var m Match
		if err = rows.Scan(&m.Path, &m.Start, &m.End, &m.Hash, &m.Symbol, &m.Snippet, &m.Score); err != nil {
			rows.Close()
			return result, err
		}
		result.Matches = append(result.Matches, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}
