// Package memory stores curated project decisions with file provenance.
// Memory text is data for agents to read, never instructions to execute.
package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	// StatusSourced: every cited file was indexed with the recorded SHA-256 when
	// saved. It proves provenance, not that the claim is true.
	StatusSourced = "sourced"
	// StatusUnverified: no cited files, or some were not in the index.
	StatusUnverified = "unverified"
	// StatusStale: derived on read when a cited file changed or disappeared.
	StatusStale = "stale"

	// DefaultScope applies everywhere. It is not a valid directory path, so a
	// real directory can never collide with it.
	DefaultScope = "*"
	maxMemories  = 1000
)

type Input struct {
	Title       string   `json:"title" jsonschema:"Short title, unique within the scope; an existing title is updated. Up to 200 bytes."`
	Body        string   `json:"body" jsonschema:"The decision or fact, up to 4000 bytes. Stored as data, never executed."`
	Scope       string   `json:"scope,omitempty" jsonschema:"Area it applies to, e.g. internal/search; default * (applies everywhere)."`
	SourcePaths []string `json:"source_paths,omitempty" jsonschema:"Project-relative files supporting it. Without indexed sources the memory is unverified."`
}

// Record is a memory as returned to agents, with status derived at read time.
type Record struct {
	ID        int64                 `json:"id"`
	Scope     string                `json:"scope"`
	Title     string                `json:"title"`
	Body      string                `json:"body"`
	Status    string                `json:"status"`
	Sources   []sqlite.MemorySource `json:"sources"`
	UpdatedAt string                `json:"updated_at"`
}

type RememberResponse struct {
	search.Envelope
	ID      int64  `json:"id"`
	Status  string `json:"status"`
	Updated bool   `json:"updated"`
	// Sources deliberately shadows Envelope.Sources: memories cite whole files
	// by path and SHA-256, without line ranges.
	Sources []sqlite.MemorySource `json:"sources"`
}

// Remember validates and stores a memory. Sources are checked against the
// freshly refreshed index; anything not provable is saved as unverified.
func Remember(ctx context.Context, s *sqlite.Store, root string, cfg config.Config, in Input) (RememberResponse, error) {
	in, err := normalize(in)
	if err != nil {
		return RememberResponse{}, err
	}
	refreshed, fresh, err := search.Refresh(ctx, s, root, cfg)
	if err != nil {
		return RememberResponse{}, err
	}
	hashes, err := s.FileHashes(ctx, in.SourcePaths)
	if err != nil {
		return RememberResponse{}, err
	}
	sources, missing := cite(in.SourcePaths, hashes)
	status := StatusSourced
	if !fresh || len(in.SourcePaths) == 0 || len(missing) > 0 {
		status = StatusUnverified
	}
	row := sqlite.MemoryRow{Scope: in.Scope, Title: in.Title, Body: in.Body, Status: status, Sources: sources}
	id, updated, err := s.UpsertMemory(ctx, row, maxMemories)
	if err != nil {
		return RememberResponse{}, err
	}
	env := search.Fresh(root, refreshed.Generation, refreshed)
	if !fresh {
		if env, err = search.Withheld(ctx, s, root); err != nil {
			return RememberResponse{}, err
		}
	}
	resp := RememberResponse{Envelope: env, ID: id, Status: status, Updated: updated, Sources: sources}
	resp.Warnings = append(resp.Warnings, unverifiedReasons(in.SourcePaths, missing, fresh)...)
	return resp, nil
}

func cite(paths []string, hashes map[string]string) (sources []sqlite.MemorySource, missing []string) {
	sources = make([]sqlite.MemorySource, 0, len(paths))
	for _, p := range paths {
		sources = append(sources, sqlite.MemorySource{Path: p, Hash: hashes[p]})
		if hashes[p] == "" {
			missing = append(missing, p)
		}
	}
	return sources, missing
}

func unverifiedReasons(paths, missing []string, fresh bool) []string {
	var reasons []string
	if !fresh {
		reasons = append(reasons, "Saved as unverified: the index could not be refreshed.")
	}
	if len(paths) == 0 {
		reasons = append(reasons, "Saved as unverified: no source_paths given.")
	}
	if len(missing) > 0 {
		reasons = append(reasons, fmt.Sprintf("Saved as unverified: not in the index (missing, excluded or secret-like): %s.", strings.Join(missing, ", ")))
	}
	return reasons
}

// Recall returns memories relevant to scope with status derived from the
// current index. Callers refresh the index first.
func Recall(ctx context.Context, s *sqlite.Store, scope string) ([]Record, error) {
	rows, err := s.Memories(ctx)
	if err != nil {
		return nil, err
	}
	scope = normalizeScope(scope)
	var relevant []sqlite.MemoryRow
	var paths []string
	for _, r := range rows {
		if inScope(r.Scope, scope) {
			relevant = append(relevant, r)
			for _, src := range r.Sources {
				paths = append(paths, src.Path)
			}
		}
	}
	hashes, err := s.FileHashes(ctx, paths)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(relevant))
	for _, r := range relevant {
		records = append(records, Record{ID: r.ID, Scope: r.Scope, Title: r.Title, Body: r.Body, Status: derive(r, hashes), Sources: r.Sources, UpdatedAt: r.UpdatedAt})
	}
	return records, nil
}

func derive(r sqlite.MemoryRow, current map[string]string) string {
	if r.Status != StatusSourced {
		return r.Status
	}
	for _, src := range r.Sources {
		if current[src.Path] != src.Hash {
			return StatusStale
		}
	}
	return StatusSourced
}

// inScope matches the project-wide scope, the same scope, its ancestors
// (rules for internal apply to internal/search) and its descendants.
func inScope(memoryScope, scope string) bool {
	return scope == "" || memoryScope == DefaultScope || memoryScope == scope ||
		strings.HasPrefix(scope, memoryScope+"/") || strings.HasPrefix(memoryScope, scope+"/")
}
