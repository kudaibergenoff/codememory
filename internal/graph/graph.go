// Package graph answers who-uses-what questions from the Go reference graph
// stored in the index: callers of a symbol (to a bounded depth), what a symbol
// uses, and the most referenced symbols of a scope.
package graph

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/kudaibergenoff/codememory/internal/architecture"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	defaultDepth   = 1
	maxDepth       = 3
	maxDefinitions = 20
	maxCallers     = 50
	maxCallees     = 60
	maxHubs        = 30
	maxSymbolBytes = 256
	basis          = "Go only. Edges are go/ast references from functions and methods, without type checking: a same-package name can be a shadowing local, and by_name callers reach a method through a call matched by method name only; method values passed without a call (s.handle) are not edges. Callers appear once, at their nearest depth, exact edges first. Callees list functions, methods and types; package variables and constants are not listed. Hubs count distinct non-test functions that reference a symbol."
)

var symbolPattern = regexp.MustCompile(`^[\pL_][\pL\pN_]*(\.[\pL_][\pL\pN_]*){0,2}$`)

type Input struct {
	Symbol string `json:"symbol,omitempty" jsonschema:"Go symbol: Name, pkg.Name, Type.Method or pkg.Type.Method. Empty lists the most referenced symbols."`
	Scope  string `json:"scope,omitempty" jsonschema:"Project-relative directory limiting the most-referenced list, e.g. internal/search."`
	Depth  int    `json:"depth,omitempty" jsonschema:"Caller levels to follow, 1 to 3, default 1."`
	Dir    string `json:"dir,omitempty" jsonschema:"Project-relative package directory that picks one definition when several packages share a name, e.g. cmd/tool."`
}

// Node locates a declaration, or a caller at its use site.
type Node struct {
	Symbol string `json:"symbol"`
	Kind   string `json:"kind,omitempty"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
}

type Caller struct {
	Node
	Depth  int  `json:"depth"`
	ByName bool `json:"by_name,omitempty"`
	Test   bool `json:"test,omitempty"`
}

// Callee is something a symbol uses. Path and DefLine locate project
// definitions; External marks other modules; Candidates counts the project
// methods a by-name method call may reach.
type Callee struct {
	Symbol     string `json:"symbol"`
	Kind       string `json:"kind"`
	Line       int    `json:"line"`
	Uses       int    `json:"uses"`
	Path       string `json:"path,omitempty"`
	DefLine    int    `json:"def_line,omitempty"`
	External   bool   `json:"external,omitempty"`
	Candidates int    `json:"candidates,omitempty"`
}

type Hub struct {
	Node
	Callers int `json:"callers"`
}

type Response struct {
	search.Envelope
	Basis       string   `json:"basis"`
	Symbol      string   `json:"symbol,omitempty"`
	Definitions []Node   `json:"definitions"`
	Callers     []Caller `json:"callers"`
	Callees     []Callee `json:"callees"`
	Hubs        []Hub    `json:"hubs,omitempty"`
	Truncated   bool     `json:"truncated,omitempty"`
}

func normalize(in Input) (Input, error) {
	in.Symbol = strings.TrimSpace(in.Symbol)
	if in.Symbol != "" && (len(in.Symbol) > maxSymbolBytes || !symbolPattern.MatchString(in.Symbol)) {
		return in, errors.New("symbol must look like Name, pkg.Name, Type.Method or pkg.Type.Method")
	}
	if in.Depth == 0 {
		in.Depth = defaultDepth
	}
	if in.Depth < 1 || in.Depth > maxDepth {
		return in, fmt.Errorf("depth must be between 1 and %d", maxDepth)
	}
	for _, dir := range []*string{&in.Scope, &in.Dir} {
		*dir = strings.TrimRight(strings.TrimSpace(*dir), "/")
		if *dir != "" {
			if err := search.CheckRelative(*dir); err != nil {
				return in, err
			}
		}
	}
	return in, nil
}

// Describe refreshes the index, then reports the graph around in.Symbol, or
// the most referenced symbols under in.Scope when no symbol is given.
func Describe(ctx context.Context, s *sqlite.Store, root string, cfg config.Config, in Input) (Response, error) {
	in, err := normalize(in)
	if err != nil {
		return Response{}, err
	}
	resp := Response{Basis: basis, Symbol: in.Symbol, Definitions: []Node{}, Callers: []Caller{}, Callees: []Callee{}}
	refreshed, ok, err := search.Refresh(ctx, s, root, cfg)
	if err != nil {
		return Response{}, err
	}
	if !ok {
		resp.Envelope, err = search.Withheld(ctx, s, root)
		return resp, err
	}
	resp.Envelope = search.Fresh(root, refreshed.Generation, refreshed)
	module, err := architecture.GoModule(ctx, s)
	if err != nil {
		return Response{}, err
	}
	if in.Symbol == "" {
		return withHubs(ctx, s, module, in.Scope, resp)
	}
	return around(ctx, s, module, in, resp)
}

func withHubs(ctx context.Context, s *sqlite.Store, module, scope string, resp Response) (Response, error) {
	hubs, err := s.Hubs(ctx, module, scope, maxHubs)
	if err != nil {
		return Response{}, err
	}
	resp.Hubs = make([]Hub, 0, maxHubs)
	seen := map[string]bool{}
	for _, h := range hubs {
		if seen[declKey(h.Decl)] || len(resp.Hubs) == maxHubs {
			continue // the same declaration in another build-tagged file
		}
		seen[declKey(h.Decl)] = true
		resp.Hubs = append(resp.Hubs, Hub{Node: declNode(h.Decl), Callers: h.Callers})
	}
	return resp, nil
}

func around(ctx context.Context, s *sqlite.Store, module string, in Input, resp Response) (Response, error) {
	defs, err := definitions(ctx, s, in.Symbol, in.Dir)
	if err != nil {
		return Response{}, err
	}
	if len(defs) > maxDefinitions {
		defs, resp.Truncated = defs[:maxDefinitions], true
	}
	for _, d := range defs {
		resp.Definitions = append(resp.Definitions, declNode(d))
	}
	switch len(defs) {
	case 0:
		resp.Warnings = append(resp.Warnings, "Symbol not found among indexed Go functions, methods and types.")
		return resp, nil
	case 1:
	default:
		resp.Warnings = append(resp.Warnings, "Ambiguous symbol: qualify it as pkg.Name or pkg.Type.Method, or pass dir.")
		return resp, nil
	}
	pkgs, err := s.PackageNames(ctx)
	if err != nil {
		return Response{}, err
	}
	if resp.Callers, resp.Truncated, err = callers(ctx, s, module, defs[0], in.Depth, pkgs); err != nil {
		return Response{}, err
	}
	var cut bool
	if resp.Callees, cut, err = callees(ctx, s, module, defs[0], pkgs); err != nil {
		return Response{}, err
	}
	resp.Truncated = resp.Truncated || cut
	return resp, nil
}

// definitions resolves Name, pkg.Name, Type.Method or pkg.Type.Method.
// A non-empty dir keeps only declarations in that directory.
func definitions(ctx context.Context, s *sqlite.Store, symbol, dir string) ([]sqlite.Decl, error) {
	parts := strings.Split(symbol, ".")
	var names []string
	suffix := ""
	switch len(parts) {
	case 1:
		names, suffix = parts, parts[0]
	case 2:
		names = []string{symbol, parts[1]}
	default:
		names = []string{parts[1] + "." + parts[2]}
	}
	decls, err := s.Decls(ctx, names, suffix)
	if err != nil {
		return nil, err
	}
	var out []sqlite.Decl
	seen := map[string]bool{}
	for _, d := range decls {
		if matches(d, parts) && (dir == "" || d.Dir == dir) && !seen[declKey(d)] {
			seen[declKey(d)] = true // build-tagged files may repeat a declaration
			out = append(out, d)
		}
	}
	return preferExact(out, symbol), nil
}

// preferExact keeps the functions and types named exactly like a bare symbol
// when there are any, dropping methods that only share the method name.
func preferExact(decls []sqlite.Decl, symbol string) []sqlite.Decl {
	var exact []sqlite.Decl
	for _, d := range decls {
		if d.Name == symbol {
			exact = append(exact, d)
		}
	}
	if len(exact) == 0 {
		return decls
	}
	return exact
}

// declKey identifies a declaration across the build-tagged files of a package.
func declKey(d sqlite.Decl) string { return d.Dir + "\x00" + d.Name }

func matches(d sqlite.Decl, parts []string) bool {
	switch len(parts) {
	case 1:
		return true
	case 2:
		return d.Name == parts[0]+"."+parts[1] || (d.Package == parts[0] && d.Name == parts[1])
	default:
		return d.Package == parts[0]
	}
}

// callers walks callers level by level from d, visiting each function once
// at its nearest depth and preferring exact edges within a level, so the work
// grows with the callers reported rather than with the paths between them.
func callers(ctx context.Context, s *sqlite.Store, module string, d sqlite.Decl, depth int, pkgs map[string]string) ([]Caller, bool, error) {
	visited := map[string]bool{declKey(d): true}
	frontier := []sqlite.Caller{{Dir: d.Dir, Name: d.Name}}
	var out []Caller
	for level := 1; level <= depth && len(frontier) > 0; level++ {
		found, err := nextLevel(ctx, s, module, frontier, visited)
		if err != nil {
			return nil, false, err
		}
		for _, c := range found {
			out = append(out, Caller{
				Node:  Node{Symbol: qualify(pkgs[c.Dir], c.Dir, c.Name), Path: c.Path, Line: c.Line},
				Depth: level, ByName: c.ByName, Test: strings.HasSuffix(c.Path, "_test.go"),
			})
			if len(out) > maxCallers {
				return out[:maxCallers], true, nil
			}
		}
		frontier = found
	}
	return out, false, nil
}

// nextLevel returns the unvisited direct callers of frontier, one per
// function, exact before by-name, sorted by directory and name. by-name status
// is inherited from the frontier node.
func nextLevel(ctx context.Context, s *sqlite.Store, module string, frontier []sqlite.Caller, visited map[string]bool) ([]sqlite.Caller, error) {
	best := map[string]sqlite.Caller{}
	for _, n := range frontier {
		rows, err := s.DirectCallers(ctx, module, n.Dir, n.Name, maxCallers+len(visited)+1)
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			key := declKey(sqlite.Decl{Dir: r.Dir, Name: r.Name})
			r.ByName = r.ByName || n.ByName
			if prev, seen := best[key]; visited[key] || (seen && (r.ByName || !prev.ByName)) {
				continue
			}
			best[key] = r
		}
		if len(best) > maxCallers {
			break
		}
	}
	found := make([]sqlite.Caller, 0, len(best))
	for key, c := range best {
		visited[key] = true
		found = append(found, c)
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Dir != found[j].Dir {
			return found[i].Dir < found[j].Dir
		}
		return found[i].Name < found[j].Name
	})
	return found, nil
}

func callees(ctx context.Context, s *sqlite.Store, module string, d sqlite.Decl, pkgs map[string]string) ([]Callee, bool, error) {
	rows, err := s.Callees(ctx, module, d.Dir, d.Name, maxCallees+1)
	if err != nil {
		return nil, false, err
	}
	truncated := len(rows) > maxCallees
	if truncated {
		rows = rows[:maxCallees]
	}
	defs, err := internalDecls(ctx, s, rows)
	if err != nil {
		return nil, false, err
	}
	methods, err := s.MethodCounts(ctx, methodNames(rows))
	if err != nil {
		return nil, false, err
	}
	out := make([]Callee, 0, len(rows))
	for _, r := range rows {
		c := Callee{Symbol: r.ToName, Kind: r.Kind, Line: r.Line, Uses: r.Uses}
		switch {
		case r.Kind == "method":
			c.Candidates = methods[r.ToName]
		case r.ToDir != "":
			def, ok := defs[declKey(sqlite.Decl{Dir: r.ToDir, Name: r.ToName})]
			if !ok {
				continue // a local, field, variable or constant: not a graph node
			}
			c.Symbol, c.Path, c.DefLine = qualify(pkgs[def.Dir], def.Dir, def.Name), def.Path, def.Line
		default:
			c.Symbol, c.External = r.ToPkg+"."+r.ToName, true
		}
		out = append(out, c)
	}
	return out, truncated, nil
}

func methodNames(rows []sqlite.Callee) []string {
	var names []string
	seen := map[string]bool{}
	for _, r := range rows {
		if r.Kind == "method" && !seen[r.ToName] {
			seen[r.ToName] = true
			names = append(names, r.ToName)
		}
	}
	return names
}

// internalDecls indexes the project declarations the callees may refer to by
// directory and name.
func internalDecls(ctx context.Context, s *sqlite.Store, rows []sqlite.Callee) (map[string]sqlite.Decl, error) {
	seen := map[string]bool{}
	var names []string
	for _, r := range rows {
		if r.ToDir != "" && !seen[r.ToName] {
			seen[r.ToName] = true
			names = append(names, r.ToName)
		}
	}
	out := map[string]sqlite.Decl{}
	if len(names) == 0 {
		return out, nil
	}
	decls, err := s.Decls(ctx, names, "")
	if err != nil {
		return nil, err
	}
	for _, d := range decls {
		if _, dup := out[declKey(d)]; !dup {
			out[declKey(d)] = d
		}
	}
	return out, nil
}

func declNode(d sqlite.Decl) Node {
	return Node{Symbol: qualify(d.Package, d.Dir, d.Name), Kind: d.Kind, Path: d.Path, Line: d.Line}
}

// qualify prefixes name with its package, or its directory if unknown.
func qualify(pkg, dir, name string) string {
	if pkg == "" {
		pkg = dir
	}
	return pkg + "." + name
}
