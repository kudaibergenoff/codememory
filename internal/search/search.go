// Package search owns input validation, literal FTS queries and freshness-aware responses.
package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/indexer"
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
	Status  *sqlite.Status `json:"status,omitempty"`
}

func newResponse(e Envelope) Response {
	return Response{Envelope: e, Matches: []sqlite.Match{}}
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
	result, err := s.Search(ctx, expr, in.Language, in.PathPrefix, in.Limit)
	if err != nil {
		return Response{}, err
	}
	response := newResponse(Fresh(root, result.Generation, refreshed))
	response.Matches = result.Matches
	for _, m := range result.Matches {
		response.Sources = append(response.Sources, m.Source)
	}
	return response, nil
}

// Status reports index counts and whether the index matches the files on
// disk. It never modifies the index.
func Status(ctx context.Context, s *sqlite.Store, root string, cfg config.Config) (Response, error) {
	fresh, err := indexer.Check(ctx, s, root, cfg)
	if err != nil {
		return Response{}, err
	}
	st, err := s.Status(ctx)
	if err != nil {
		return Response{}, fmt.Errorf("read index status: %w", err)
	}
	response := newResponse(NewEnvelope(root))
	response.Generation = st.Generation
	response.Stale = fresh.Stale
	response.Status = &st
	if fresh.Stale {
		response.Warnings = append(response.Warnings, staleNotice)
	}
	if fresh.Secrets > 0 {
		response.Warnings = append(response.Warnings, secretsNotice)
	}
	return response, nil
}
