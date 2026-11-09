// Package contextpack assembles a token-budgeted, source-cited context for a task.
package contextpack

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/config"
	"github.com/kudaibergenoff/codememory/internal/memory"
	"github.com/kudaibergenoff/codememory/internal/qcache"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	defaultTokens  = 1500
	minTokens      = 500 // below this, envelope + one snippet's provenance may not fit
	maxTokens      = 8000
	candidateLimit = 60
)

type Input struct {
	Task                string `json:"task" jsonschema:"The task in natural language; identifiers help ranking. Up to 4000 bytes."`
	MaxTokens           int    `json:"max_tokens,omitempty" jsonschema:"Response budget in estimated tokens, 500 to 8000, default 1500. Counts the whole JSON response."`
	IncludeArchitecture *bool  `json:"include_architecture,omitempty" jsonschema:"Include curated memories that apply to the ranked files or the whole project, with status. Default true."`
}

// Snippet is a ranked match; Truncated means only the first lines fit the budget.
type Snippet struct {
	sqlite.Match
	Truncated bool `json:"truncated,omitempty"`
}

type Response struct {
	search.Envelope
	Summary         string          `json:"summary"`
	Terms           []string        `json:"terms"`
	Files           []string        `json:"files"`
	Snippets        []Snippet       `json:"snippets"`
	Architecture    []memory.Record `json:"architecture,omitempty"`
	MemoriesOmitted int             `json:"memories_omitted,omitempty"`
	Omitted         int             `json:"omitted"`
	EstimatedTokens int             `json:"estimated_tokens"`
	TokenBudget     int             `json:"token_budget"`
	Estimator       string          `json:"estimator"`
}

// Build refreshes the index, retrieves candidates for the task and packs as
// many ranked snippets as fit max_tokens, measured on the encoded response.
func Build(ctx context.Context, s *sqlite.Store, root string, cfg config.Config, in Input) (Response, error) {
	budget, err := budgetOf(in.MaxTokens)
	if err != nil {
		return Response{}, err
	}
	terms, err := taskTerms(in.Task)
	if err != nil {
		return Response{}, err
	}
	base := Response{Terms: terms, TokenBudget: budget, Estimator: analytics.Estimator}
	refreshed, ok, err := search.Refresh(ctx, s, root, cfg)
	if err != nil {
		return Response{}, err
	}
	if !ok {
		if base.Envelope, err = search.Withheld(ctx, s, root); err != nil {
			return Response{}, err
		}
		return pack(base, nil, nil, budget)
	}
	include := in.IncludeArchitecture == nil || *in.IncludeArchitecture
	revision, err := s.MemoryRevision(ctx)
	if err != nil {
		return Response{}, err
	}
	cache := qcache.New(s)
	key := qcache.Key("memory_context", terms, budget, include, revision, refreshed.Secrets > 0)
	var cached Response
	if hit, err := cache.Get(ctx, key, refreshed.Generation, &cached); err != nil {
		slog.Warn("query cache read failed", "error", err)
	} else if hit {
		cached.MarkCache(true)
		return cached, nil
	}
	resp, err := assemble(ctx, s, root, base, refreshed, include, budget)
	if err != nil {
		return Response{}, err
	}
	if err := cache.Put(ctx, key, resp.Generation, resp); err != nil {
		slog.Warn("query cache write failed", "error", err)
	}
	resp.MarkCache(false)
	return resp, nil
}

func assemble(ctx context.Context, s *sqlite.Store, root string, base Response, refreshed sqlite.IndexResult, include bool, budget int) (Response, error) {
	result, err := s.Search(ctx, search.OrExpression(base.Terms, true), "", "", candidateLimit)
	if err != nil {
		return Response{}, err
	}
	base.Envelope = search.Fresh(root, result.Generation, refreshed)
	var memories []memory.Record
	if include {
		if memories, err = memory.Recall(ctx, s, ""); err != nil {
			return Response{}, err
		}
	}
	return pack(base, result.Matches, memories, budget)
}

func budgetOf(requested int) (int, error) {
	if requested == 0 {
		return defaultTokens, nil
	}
	if requested < minTokens || requested > maxTokens {
		return 0, fmt.Errorf("max_tokens must be between %d and %d", minTokens, maxTokens)
	}
	return requested, nil
}
