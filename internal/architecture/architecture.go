// Package architecture reports verifiable module structure and curated memories.
package architecture

import (
	"context"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/memory"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	maxModules = 300
	maxImports = 50
	basis      = "Modules group indexed files by directory. Go package names and imports are go/ast facts of this index generation; internal imports are resolved against the root go.mod. Memories are curated notes; check their status."
)

var goModule = regexp.MustCompile(`(?m)^module\s+(\S+)`)

type Input struct {
	Scope string `json:"scope,omitempty" jsonschema:"Project-relative directory such as internal/search; empty for the whole project."`
}

type Module struct {
	Path            string   `json:"path"`
	Package         string   `json:"package,omitempty"`
	Files           int      `json:"files"`
	Languages       []string `json:"languages"`
	Imports         []string `json:"imports,omitempty"` // module-internal, as directories
	ExternalImports int      `json:"external_imports,omitempty"`
}

type Response struct {
	search.Envelope
	Scope     string          `json:"scope"`
	GoModule  string          `json:"go_module,omitempty"`
	Basis     string          `json:"basis"`
	Modules   []Module        `json:"modules"`
	Memories  []memory.Record `json:"memories"`
	Truncated bool            `json:"truncated,omitempty"`
}

// Describe refreshes the index and reports modules and memories within scope.
func Describe(ctx context.Context, s *sqlite.Store, root string, cfg config.Config, in Input) (Response, error) {
	scope := strings.TrimRight(strings.TrimSpace(in.Scope), "/")
	if scope != "" {
		if err := search.CheckRelative(scope); err != nil {
			return Response{}, err
		}
	}
	resp := Response{Scope: scope, Basis: basis, Modules: []Module{}, Memories: []memory.Record{}}
	refreshed, ok, err := search.Refresh(ctx, s, root, cfg)
	if err != nil {
		return Response{}, err
	}
	if !ok {
		resp.Envelope, err = search.Withheld(ctx, s, root)
		return resp, err
	}
	facts, err := s.FileFacts(ctx)
	if err != nil {
		return Response{}, err
	}
	gomod, err := s.FirstChunk(ctx, "go.mod")
	if err != nil {
		return Response{}, err
	}
	if m := goModule.FindStringSubmatch(gomod); m != nil {
		resp.GoModule = m[1]
	}
	resp.Modules, resp.Truncated = modules(facts, resp.GoModule, scope)
	if resp.Memories, err = memory.Recall(ctx, s, scope); err != nil {
		return Response{}, err
	}
	resp.Envelope = search.Fresh(root, refreshed.Generation, refreshed)
	return resp, nil
}

func inScope(dir, scope string) bool {
	return scope == "" || dir == scope || strings.HasPrefix(dir, scope+"/")
}

// modules groups file facts by directory, sorted by path.
func modules(facts []sqlite.FileFacts, goModule, scope string) ([]Module, bool) {
	type acc struct {
		module    Module
		languages map[string]bool
		imports   map[string]bool
	}
	byDir := map[string]*acc{}
	for _, f := range facts {
		dir := path.Dir(f.Path)
		if !inScope(dir, scope) {
			continue
		}
		a := byDir[dir]
		if a == nil {
			a = &acc{module: Module{Path: dir}, languages: map[string]bool{}, imports: map[string]bool{}}
			byDir[dir] = a
		}
		a.module.Files++
		a.languages[f.Language] = true
		if strings.HasSuffix(f.Path, "_test.go") {
			continue // test packages and imports do not describe the module
		}
		if f.Package != "" {
			a.module.Package = f.Package
		}
		for _, imp := range f.Imports {
			if rel, internal := relativeImport(imp, goModule); internal {
				a.imports[rel] = true
			} else if !a.imports["ext:"+imp] {
				a.imports["ext:"+imp] = true
				a.module.ExternalImports++
			}
		}
	}
	out := make([]Module, 0, len(byDir))
	for _, a := range byDir {
		a.module.Languages = sortedKeys(a.languages, "")
		a.module.Imports = sortedKeys(a.imports, "ext:")
		if len(a.module.Imports) > maxImports {
			a.module.Imports = a.module.Imports[:maxImports]
		}
		out = append(out, a.module)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if len(out) > maxModules {
		return out[:maxModules], true
	}
	return out, false
}

// relativeImport maps "example.com/shop/internal/store" to "internal/store"
// when goModule is "example.com/shop".
func relativeImport(imp, goModule string) (string, bool) {
	if goModule == "" {
		return "", false
	}
	if imp == goModule {
		return ".", true
	}
	if rest, ok := strings.CutPrefix(imp, goModule+"/"); ok {
		return rest, true
	}
	return "", false
}

// sortedKeys returns keys without the excluded prefix, sorted.
func sortedKeys(set map[string]bool, exclude string) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		if exclude == "" || !strings.HasPrefix(k, exclude) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}
