// Package search owns input validation, literal FTS queries and freshness-aware responses.
package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/indexer"
	"github.com/kudaibergenoff/codememory/internal/parser"
	"github.com/kudaibergenoff/codememory/internal/qcache"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	defaultLimit   = 10
	maxLimit       = 20
	maxLanguageLen = 32
	maxPrefixBytes = 512
)

type Input struct {
	Query      string `json:"query" jsonschema:"Plain text query, up to 512 bytes"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum matches, 1 to 20, default 10"`
	Language   string `json:"language,omitempty"`
	PathPrefix string `json:"path_prefix,omitempty"`
}

type Response struct {
	Envelope
	Matches []sqlite.Match `json:"matches"`
	Status  *StatusInfo    `json:"status,omitempty"`
}

// StatusInfo is memory_status's report (SPEC §7): counts, freshness, database
// size, cache statistics and supported languages.
type StatusInfo struct {
	sqlite.Status
	DBBytes            int64                    `json:"database_bytes"`
	CacheEntries       int                      `json:"query_cache_entries"`
	CacheHitRate       *float64                 `json:"query_cache_hit_rate"`
	FilesByLanguage    map[string]int           `json:"files_by_language"`
	SupportedLanguages []parser.LanguageSupport `json:"supported_languages"`
}

func newResponse(e Envelope) Response {
	return Response{Envelope: e, Matches: []sqlite.Match{}}
}

// CheckRelative rejects anything but a clean, project-relative slash path.
func CheckRelative(p string) error {
	if p == "" || len(p) > maxPrefixBytes || !utf8.ValidString(p) || strings.Contains(p, "\\") ||
		path.IsAbs(p) || path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") {
		return fmt.Errorf("path %q must be a clean project-relative path", p)
	}
	return nil
}

// normalize validates filters and returns the input with defaults applied.
func normalize(in Input) (Input, error) {
	if in.Limit == 0 {
		in.Limit = defaultLimit
	}
	if in.Limit < 1 || in.Limit > maxLimit {
		return in, fmt.Errorf("limit must be between 1 and %d", maxLimit)
	}
	if len(in.Language) > maxLanguageLen || len(in.PathPrefix) > maxPrefixBytes || strings.Contains(in.PathPrefix, "\\") || strings.HasPrefix(in.PathPrefix, "/") || !utf8.ValidString(in.PathPrefix) {
		return in, errors.New("invalid filter")
	}
	for _, part := range strings.Split(in.PathPrefix, "/") {
		if part == ".." || part == "." {
			return in, errors.New("path prefix must be project-relative")
		}
	}
	return in, nil
}

// Query refreshes changed files into the index, then searches it, so results
// reflect the files on disk at refresh time.
func Query(ctx context.Context, s *sqlite.Store, root string, cfg config.Config, in Input) (Response, error) {
	expr, err := Expression(in.Query)
	if err != nil {
		return Response{}, err
	}
	if in, err = normalize(in); err != nil {
		return Response{}, err
	}
	refreshed, ok, err := Refresh(ctx, s, root, cfg)
	if err != nil {
		return Response{}, err
	}
	if !ok {
		e, err := Withheld(ctx, s, root)
		return newResponse(e), err
	}
	cache := qcache.New(s)
	key := qcache.Key("memory_search", in, refreshed.Secrets > 0)
	var cached Response
	if hit, err := cache.Get(ctx, key, refreshed.Generation, &cached); err != nil {
		slog.Warn("query cache read failed", "error", err)
	} else if hit {
		cached.MarkCache(true)
		cached.MarkRequest(key)
		return cached, nil
	}
	result, err := s.Search(ctx, sqlite.Query{Expression: expr, Exact: PhraseExpression(Phrases(in.Query)),
		Language: in.Language, PathPrefix: in.PathPrefix, Limit: in.Limit})
	if err != nil {
		return Response{}, err
	}
	response := newResponse(Fresh(root, result.Generation, refreshed))
	response.Matches = result.Matches
	for _, m := range result.Matches {
		response.Sources = append(response.Sources, m.Source)
	}
	if err := cache.Put(ctx, key, result.Generation, response); err != nil {
		slog.Warn("query cache write failed", "error", err)
	}
	response.MarkCache(false)
	response.MarkRequest(key)
	return response, nil
}

// Status reports index counts and whether the index matches the files on
// disk. It never modifies the index.
func Status(ctx context.Context, s *sqlite.Store, root string, cfg config.Config) (Response, error) {
	fresh, err := indexer.Check(ctx, s, root, cfg)
	if err != nil {
		return Response{}, err
	}
	info, err := statusInfo(ctx, s)
	if err != nil {
		return Response{}, err
	}
	response := newResponse(NewEnvelope(root))
	response.Generation = info.Generation
	response.Stale = fresh.Stale
	response.Status = &info
	if fresh.Stale {
		response.Warnings = append(response.Warnings, staleNotice)
	}
	if fresh.Secrets > 0 {
		response.Warnings = append(response.Warnings, secretsNotice)
	}
	return response, nil
}

func statusInfo(ctx context.Context, s *sqlite.Store) (StatusInfo, error) {
	st, err := s.Status(ctx)
	if err != nil {
		return StatusInfo{}, fmt.Errorf("read index status: %w", err)
	}
	foot, err := s.Footprint(ctx)
	if err != nil {
		return StatusInfo{}, err
	}
	langs, err := s.FilesByLanguage(ctx)
	if err != nil {
		return StatusInfo{}, err
	}
	usage, err := analytics.Stats(ctx, s.DB, analytics.Filter{})
	if err != nil {
		return StatusInfo{}, err
	}
	return StatusInfo{Status: st, DBBytes: foot.DBBytes, CacheEntries: foot.CacheEntries, CacheHitRate: usage.QueryCacheHitRate,
		FilesByLanguage: langs, SupportedLanguages: parser.Supported}, nil
}
