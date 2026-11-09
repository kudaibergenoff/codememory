package contextpack

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kudaibergenoff/codememory/internal/analytics"
	"github.com/kudaibergenoff/codememory/internal/search"
	"github.com/kudaibergenoff/codememory/internal/storage/sqlite"
)

const (
	maxPerFile      = 2
	maxFiles        = 10
	minSnippetLines = 3
	filesReservePct = 15
)

// pack greedily adds candidates in rank order while the encoded response fits
// the budget. Counters that are only known at the end are measured at an
// upper bound, so the final response can only be smaller than measured.
func pack(base Response, candidates []sqlite.Match, budget int) Response {
	picked := selectCandidates(candidates)
	bound := len(candidates)
	r := base
	r.Files = []string{}
	r.Snippets = []Snippet{}
	if r.Sources == nil {
		r.Sources = []sqlite.Source{}
	}
	measure := func(x Response) int {
		x.Omitted, x.EstimatedTokens = bound, budget
		x.Summary = summary(bound, bound, bound, budget)
		return cost(x)
	}
	empty := shrinkToFit(r, budget, measure)
	ranked := rankedFiles(picked)
	// A few file names first: the cheapest navigation, kept even when large
	// snippets would otherwise consume the whole budget...
	reserve := min(budget, measure(empty)+budget*filesReservePct/100)
	r = addSnippets(addFiles(empty, ranked, reserve, measure), picked, budget, measure)
	if len(r.Snippets) == 0 && len(picked) > 0 {
		// ...unless they leave no room for even the best snippet.
		r = addSnippets(empty, picked, budget, measure)
	}
	r = addFiles(r, ranked, budget, measure) // fill what the snippets left
	r.Omitted = len(candidates) - len(r.Snippets)
	r.Summary = summary(len(r.Snippets), len(r.Files), r.Omitted, budget)
	r.EstimatedTokens = budget
	r.EstimatedTokens = cost(r) // the real value has no more digits than budget
	return r
}

func cost(r Response) int {
	b, err := search.Encode(r)
	if err != nil {
		return int(^uint(0) >> 1) // unencodable never fits
	}
	return analytics.Estimate(len(b))
}

func summary(snippets, files, omitted, budget int) string {
	return fmt.Sprintf("%d snippets, %d ranked files; %d candidates omitted (caps or %d-token budget).", snippets, files, omitted, budget)
}

// shrinkToFit drops trailing terms if even the empty response exceeds a tiny budget.
func shrinkToFit(r Response, budget int, measure func(Response) int) Response {
	for measure(r) > budget && len(r.Terms) > 0 {
		r.Terms = r.Terms[:len(r.Terms)-1]
	}
	return r
}

// addSnippets appends candidates in rank order, truncating those that do not
// fit whole, while the response measures at most budget.
func addSnippets(r Response, picked []sqlite.Match, budget int, measure func(Response) int) Response {
	for _, m := range picked {
		if next := with(r, Snippet{Match: m}); measure(next) <= budget {
			r = next
		} else if next, ok := truncated(r, m, budget, measure); ok {
			r = next
		}
	}
	return r
}

// addFiles appends ranked file names not yet listed while the response
// measures at most limit.
func addFiles(r Response, ranked []string, limit int, measure func(Response) int) Response {
	for _, f := range ranked[min(len(r.Files), len(ranked)):] {
		next := r
		next.Files = append(r.Files[:len(r.Files):len(r.Files)], f)
		if measure(next) > limit {
			break
		}
		r = next
	}
	return r
}

// with returns a copy of r with s appended; r's slices are never shared.
func with(r Response, s Snippet) Response {
	r.Snippets = append(r.Snippets[:len(r.Snippets):len(r.Snippets)], s)
	r.Sources = append(r.Sources[:len(r.Sources):len(r.Sources)], s.Source)
	return r
}

// truncated keeps the longest prefix of m (at least minSnippetLines lines)
// that fits. Cost grows with the line count, so binary search applies.
func truncated(r Response, m sqlite.Match, budget int, measure func(Response) int) (Response, bool) {
	lines := strings.Split(m.Snippet, "\n")
	cut := func(k int) Snippet {
		s := Snippet{Match: m, Truncated: true}
		s.Snippet = strings.Join(lines[:k], "\n")
		s.End = min(m.End, m.Start+k-1)
		return s
	}
	lo, hi, best := minSnippetLines, len(lines)-1, 0
	for lo <= hi {
		mid := (lo + hi) / 2
		if measure(with(r, cut(mid))) <= budget {
			best, lo = mid, mid+1
		} else {
			hi = mid - 1
		}
	}
	if best == 0 {
		return r, false
	}
	return with(r, cut(best)), true
}

// selectCandidates keeps rank order, at most maxPerFile chunks per file and
// no chunk overlapping an already selected one.
func selectCandidates(candidates []sqlite.Match) []sqlite.Match {
	var picked []sqlite.Match
	perFile := map[string][]sqlite.Match{}
	for _, m := range candidates {
		kept := perFile[m.Path]
		if len(kept) >= maxPerFile || overlaps(kept, m) {
			continue
		}
		perFile[m.Path] = append(kept, m)
		picked = append(picked, m)
	}
	return picked
}

func overlaps(kept []sqlite.Match, m sqlite.Match) bool {
	for _, k := range kept {
		if m.Start <= k.End && k.Start <= m.End {
			return true
		}
	}
	return false
}

// rankedFiles lists distinct paths by their best (lowest BM25) score.
func rankedFiles(picked []sqlite.Match) []string {
	best := map[string]float64{}
	var files []string
	for _, m := range picked {
		if score, seen := best[m.Path]; !seen || m.Score < score {
			if !seen {
				files = append(files, m.Path)
			}
			best[m.Path] = m.Score
		}
	}
	sort.SliceStable(files, func(i, j int) bool { return best[files[i]] < best[files[j]] })
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}
	return files
}
