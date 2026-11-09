// Package optimize turns local evidence into suggestions. It is read-only:
// it never changes configuration, ignore files or the index.
package optimize

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/benchmark"
	"github.com/kudaibergenoff/codememory/internal/memory"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	KindSuggest = "suggest"
	KindInfo    = "info"

	largeDirShare     = 0.4
	largeDirMinBytes  = 256 << 10
	saturationShare   = 0.9
	oracleShareOfCtx  = 0.5
	smallerBudgetHint = 800
	Note              = "Suggestions only; nothing was changed. Evidence comes from the local index, analytics (30 days) and the latest benchmark."
)

type Suggestion struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Evidence string `json:"evidence"`
	Action   string `json:"action"`
}

type Result struct {
	Applied     bool         `json:"applied"`
	Suggestions []Suggestion `json:"suggestions"`
	Note        string       `json:"note"`
}

type Evidence struct {
	Sizes         []sqlite.PathSize
	Skips         map[string]int
	Stats         analytics.Report
	Benchmark     *benchmark.Report
	StaleMemories int
	Budget        int // default memory_context budget
}

// Gather reads evidence without refreshing or modifying anything.
func Gather(ctx context.Context, s *sqlite.Store, budget int) (Evidence, error) {
	e := Evidence{Budget: budget}
	var err error
	if e.Sizes, err = s.IndexedSizes(ctx); err != nil {
		return e, err
	}
	if e.Skips, err = s.SkipCounts(ctx); err != nil {
		return e, err
	}
	if e.Stats, err = analytics.Stats(ctx, s.DB, analytics.Filter{}); err != nil {
		return e, err
	}
	raw, ok, err := s.LatestBenchmark(ctx)
	if err != nil {
		return e, err
	}
	if ok {
		var b benchmark.Report
		if err := json.Unmarshal(raw, &b); err != nil {
			return e, fmt.Errorf("decode benchmark: %w", err)
		}
		e.Benchmark = &b
	}
	records, err := memory.Recall(ctx, s, "")
	if err != nil {
		return e, err
	}
	for _, r := range records {
		if r.Status == memory.StatusStale {
			e.StaleMemories++
		}
	}
	return e, nil
}

// Suggest applies every rule to the evidence.
func Suggest(e Evidence) []Suggestion {
	out := []Suggestion{}
	for _, rule := range []func(Evidence) []Suggestion{largeDirectory, secretSkips, toolErrors, staleMemories, benchmarkRules} {
		out = append(out, rule(e)...)
	}
	return out
}

func largeDirectory(e Evidence) []Suggestion {
	var total int64
	byDir := map[string]int64{}
	for _, p := range e.Sizes {
		total += p.Size
		byDir[topDirs(p.Path)] += p.Size
	}
	if total < largeDirMinBytes {
		return nil
	}
	dirs := make([]string, 0, len(byDir))
	for d := range byDir {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool { return byDir[dirs[i]] > byDir[dirs[j]] })
	top := dirs[0]
	share := float64(byDir[top]) / float64(total)
	if top == "." || share < largeDirShare {
		return nil
	}
	return []Suggestion{{ID: "large-directory", Kind: KindSuggest,
		Evidence: fmt.Sprintf("%s/ holds %.0f%% of indexed bytes (%d of %d).", top, share*100, byDir[top], total),
		Action:   fmt.Sprintf("If agents never need it (generated, vendored or fixture code), add %s/ to .codememoryignore.", top)}}
}

// topDirs keeps at most the first two path segments of the file's directory.
func topDirs(p string) string {
	parts := strings.Split(path.Dir(p), "/")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	return strings.Join(parts, "/")
}

func secretSkips(e Evidence) []Suggestion {
	if n := e.Skips["secret"]; n > 0 {
		return []Suggestion{{ID: "secret-skips", Kind: KindInfo,
			Evidence: fmt.Sprintf("%d files were excluded as secret-like.", n),
			Action:   "Nothing to do if they hold secrets. If they are false positives (e.g. test fixtures) that agents need, move fake values out of them; exclusions are never overridden."}}
	}
	return nil
}

func toolErrors(e Evidence) []Suggestion {
	if n := e.Stats.Measured.Errors; n > 0 {
		return []Suggestion{{ID: "tool-errors", Kind: KindSuggest,
			Evidence: fmt.Sprintf("%d of %d recorded requests failed in the last 30 days.", n, e.Stats.Measured.Requests),
			Action:   "Run codememory doctor; check limits in .codememory/config.json if refreshes fail."}}
	}
	return nil
}

func staleMemories(e Evidence) []Suggestion {
	if e.StaleMemories > 0 {
		return []Suggestion{{ID: "stale-memories", Kind: KindSuggest,
			Evidence: fmt.Sprintf("%d memories cite files that changed since they were saved.", e.StaleMemories),
			Action:   "Review them with memory_architecture or codememory architecture, then re-save confirmed ones with codememory remember."}}
	}
	return nil
}
