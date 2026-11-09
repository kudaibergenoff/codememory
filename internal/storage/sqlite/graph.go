package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Decl is a top-level Go declaration: a function, method (Name is Recv.Method),
// type or interface.
type Decl struct {
	Dir, Package, Name, Kind, Path string
	Line                           int
}

// Caller is a function or method that reaches the walked symbol in Depth
// steps. Path and Line are its use site. ByName is set when a step was a method
// call matched by name only, so the edge may belong to another receiver type.
type Caller struct {
	Dir, Name, Path string
	Line, Depth     int
	ByName          bool
}

// Callee is one distinct reference made by a function. ToDir is the project
// directory of the target, or "" for external packages and method calls.
type Callee struct {
	ToPkg, ToName, Kind, ToDir string
	Line, Uses                 int
}

// Hub is a declaration referenced by Callers distinct non-test functions.
type Hub struct {
	Decl
	Callers int
}

const declKinds = `('func','method','type','interface')`

// targetDir resolves the referenced package of refs row r to a project
// directory: the caller's own for same-package names, the path below the Go
// module (:module) for internal imports, NULL otherwise.
const targetDir = `(CASE WHEN r.to_pkg='' THEN r.dir
 WHEN r.to_pkg=:module THEN '.'
 WHEN :module<>'' AND substr(r.to_pkg,1,length(:module)+1)=:module||'/' THEN substr(r.to_pkg,length(:module)+2) END)`

const notTest = `substr(f.path,-8)<>'_test.go'`

// Decls lists declarations whose name is one of names or, when methodSuffix is
// set, methods named Recv.methodSuffix.
func (s *Store) Decls(ctx context.Context, names []string, methodSuffix string) ([]Decl, error) {
	var where []string
	args := []any{}
	if len(names) > 0 {
		where = append(where, "s.name IN (?"+strings.Repeat(",?", len(names)-1)+")")
		for _, n := range names {
			args = append(args, n)
		}
	}
	if methodSuffix != "" {
		where = append(where, "(s.kind='method' AND substr(s.name,-length(?)-1)='.'||?)")
		args = append(args, methodSuffix, methodSuffix)
	}
	if len(where) == 0 {
		return nil, nil
	}
	query := `SELECT f.dir,coalesce((SELECT p.name FROM symbols p WHERE p.file_id=f.id AND p.kind='package'),''),
 s.name,s.kind,f.path,s.start_line FROM symbols s JOIN files f ON f.id=s.file_id
 WHERE s.kind IN ` + declKinds + ` AND (` + strings.Join(where, " OR ") + `) ORDER BY f.path,s.start_line`
	var out []Decl
	err := eachRow(ctx, s.DB, query, func(r *sql.Rows) error {
		var d Decl
		if err := r.Scan(&d.Dir, &d.Package, &d.Name, &d.Kind, &d.Path, &d.Line); err != nil {
			return err
		}
		out = append(out, d)
		return nil
	}, args...)
	if err != nil {
		return nil, fmt.Errorf("list declarations: %w", err)
	}
	return out, nil
}

// directCallersSQL lists the functions that reference (:dir, :name): exact
// edges match the name in its resolved directory; for a Recv.Method target,
// method calls named :method match by name only (by_name=1). Exact rows sort
// first.
const directCallersSQL = `SELECT r.dir,r.from_name,f.path,r.line,0 FROM refs r JOIN files f ON f.id=r.file_id
 WHERE r.to_name=:name AND r.kind<>'method' AND ` + targetDir + `=:dir
UNION ALL
SELECT r.dir,r.from_name,f.path,r.line,1 FROM refs r JOIN files f ON f.id=r.file_id
 WHERE :method<>'' AND r.kind='method' AND r.to_name=:method
ORDER BY 5,1,2,4 LIMIT :limit`

// DirectCallers returns up to limit functions that reference (dir, name) once
// (one step of the caller walk; graph.Describe expands levels with a visited
// set, so the work stays linear in the callers it reports).
func (s *Store) DirectCallers(ctx context.Context, module, dir, name string, limit int) ([]Caller, error) {
	method := ""
	if i := strings.IndexByte(name, '.'); i >= 0 {
		method = name[i+1:]
	}
	var out []Caller
	err := eachRow(ctx, s.DB, directCallersSQL, func(r *sql.Rows) error {
		c := Caller{Depth: 1}
		if err := r.Scan(&c.Dir, &c.Name, &c.Path, &c.Line, &c.ByName); err != nil {
			return err
		}
		out = append(out, c)
		return nil
	}, sql.Named("module", module), sql.Named("dir", dir), sql.Named("name", name),
		sql.Named("method", method), sql.Named("limit", limit))
	if err != nil {
		return nil, fmt.Errorf("list callers: %w", err)
	}
	return out, nil
}

// Callees lists the references made by (dir, name) in source order. Project
// targets that are not declarations (variables, constants, shadowing locals)
// are dropped before the limit, so they cannot crowd out real callees.
func (s *Store) Callees(ctx context.Context, module, dir, name string, limit int) ([]Callee, error) {
	query := `SELECT c.to_pkg,c.to_name,c.kind,coalesce(c.tdir,''),c.line,c.uses FROM (
 SELECT r.id,r.to_pkg,r.to_name,r.kind,r.line,r.uses,` + targetDir + ` AS tdir
 FROM refs r WHERE r.dir=:dir AND r.from_name=:name) c
WHERE c.tdir IS NULL OR EXISTS (SELECT 1 FROM symbols s JOIN files f ON f.id=s.file_id
 WHERE s.name=c.to_name AND f.dir=c.tdir AND s.kind IN ` + declKinds + `)
ORDER BY c.line,c.id LIMIT :limit`
	var out []Callee
	err := eachRow(ctx, s.DB, query, func(r *sql.Rows) error {
		var c Callee
		if err := r.Scan(&c.ToPkg, &c.ToName, &c.Kind, &c.ToDir, &c.Line, &c.Uses); err != nil {
			return err
		}
		out = append(out, c)
		return nil
	}, sql.Named("module", module), sql.Named("dir", dir), sql.Named("name", name), sql.Named("limit", limit))
	if err != nil {
		return nil, fmt.Errorf("list callees: %w", err)
	}
	return out, nil
}

// MethodCounts counts, per method name, the distinct project methods (one per
// package and receiver) a by-name call may reach, in one pass over symbols.
func (s *Store) MethodCounts(ctx context.Context, names []string) (map[string]int, error) {
	out := map[string]int{}
	if len(names) == 0 {
		return out, nil
	}
	args := make([]any, len(names))
	for i, n := range names {
		args[i] = n
	}
	query := `SELECT m,count(*) FROM (
 SELECT DISTINCT substr(s.name,instr(s.name,'.')+1) AS m,f.dir,s.name FROM symbols s JOIN files f ON f.id=s.file_id
 WHERE s.kind='method') WHERE m IN (?` + strings.Repeat(",?", len(names)-1) + `) GROUP BY m`
	err := eachRow(ctx, s.DB, query, func(r *sql.Rows) error {
		var m string
		var n int
		if err := r.Scan(&m, &n); err != nil {
			return err
		}
		out[m] = n
		return nil
	}, args...)
	if err != nil {
		return nil, fmt.Errorf("count methods: %w", err)
	}
	return out, nil
}

// hubCandidates bounds the targets joined with declarations: most top
// targets are declarations, the rest are package variables or constants.
const hubCandidates = 10

// Hubs ranks declarations under scope by how many distinct non-test functions
// reference them exactly (method calls matched by name are not counted). The
// top targets are ranked first and only then joined with declarations by
// name, which keeps this fast on large indexes (10k files: ~0.7 s, not 5 s). A
// declaration repeated in build-tagged files yields one row per file.
func (s *Store) Hubs(ctx context.Context, module, scope string, limit int) ([]Hub, error) {
	query := `WITH t AS (
 SELECT ` + targetDir + ` AS tdir,r.to_name AS name,r.dir AS cdir,r.from_name AS cname
 FROM refs r JOIN files f ON f.id=r.file_id WHERE r.kind<>'method' AND ` + notTest + `
), h AS (
 SELECT tdir,name,count(DISTINCT cdir||char(0)||cname) AS callers FROM t
 WHERE tdir IS NOT NULL AND NOT (cdir=tdir AND cname=name)
  AND (:scope='' OR tdir=:scope OR substr(tdir,1,length(:scope)+1)=:scope||'/')
 GROUP BY tdir,name ORDER BY callers DESC,tdir,name LIMIT :candidates
)
SELECT h.tdir,coalesce((SELECT p.name FROM symbols p WHERE p.file_id=f.id AND p.kind='package'),''),
 h.name,s.kind,f.path,s.start_line,h.callers
FROM h JOIN symbols s ON s.name=h.name AND s.kind IN ` + declKinds + `
 JOIN files f ON f.id=s.file_id AND f.dir=h.tdir AND ` + notTest + `
ORDER BY h.callers DESC,h.tdir,h.name,f.path`
	var out []Hub
	err := eachRow(ctx, s.DB, query, func(r *sql.Rows) error {
		var h Hub
		if err := r.Scan(&h.Dir, &h.Package, &h.Name, &h.Kind, &h.Path, &h.Line, &h.Callers); err != nil {
			return err
		}
		out = append(out, h)
		return nil
	}, sql.Named("module", module), sql.Named("scope", scope), sql.Named("candidates", limit*hubCandidates))
	if err != nil {
		return nil, fmt.Errorf("rank hubs: %w", err)
	}
	return out, nil
}

// PackageNames maps project directories to their Go package clause, preferring
// non-test files.
func (s *Store) PackageNames(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	err := eachRow(ctx, s.DB, `SELECT f.dir,s.name FROM symbols s JOIN files f ON f.id=s.file_id
 WHERE s.kind='package' ORDER BY `+notTest+`,f.path`, func(r *sql.Rows) error {
		var dir, name string
		if err := r.Scan(&dir, &name); err != nil {
			return err
		}
		out[dir] = name // non-test rows sort last and win
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list package names: %w", err)
	}
	return out, nil
}
