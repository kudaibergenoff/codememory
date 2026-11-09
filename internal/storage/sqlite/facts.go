package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// FileFacts are AST facts recorded for one indexed file.
type FileFacts struct {
	Path, Language string
	Package        string   // Go package clause, empty for other languages
	Imports        []string // Go import paths in source order
}

// FileFacts lists every indexed file with its package and import symbols.
func (s *Store) FileFacts(ctx context.Context) ([]FileFacts, error) {
	var facts []FileFacts
	err := eachRow(ctx, s.DB, `SELECT f.path,f.language,coalesce(s.kind,''),coalesce(s.name,'')
 FROM files f LEFT JOIN symbols s ON s.file_id=f.id AND s.kind IN ('package','import')
 ORDER BY f.path,s.start_line,s.id`, func(r *sql.Rows) error {
		var path, language, kind, name string
		if err := r.Scan(&path, &language, &kind, &name); err != nil {
			return err
		}
		if len(facts) == 0 || facts[len(facts)-1].Path != path {
			facts = append(facts, FileFacts{Path: path, Language: language})
		}
		last := &facts[len(facts)-1]
		switch kind {
		case "package":
			last.Package = name
		case "import":
			last.Imports = append(last.Imports, name)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list file facts: %w", err)
	}
	return facts, nil
}

// FirstChunk returns the first indexed chunk of path, or "" if it is not indexed.
func (s *Store) FirstChunk(ctx context.Context, path string) (string, error) {
	var content string
	err := s.DB.QueryRowContext(ctx, `SELECT content FROM chunks WHERE path=? ORDER BY start_line,id LIMIT 1`, path).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read first chunk: %w", err)
	}
	return content, nil
}
